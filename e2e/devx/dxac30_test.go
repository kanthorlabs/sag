//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

const registryImage = "docker.io/library/registry:3.0.0@sha256:832ef18332f0dde1edff9b45d796231c68f9b9108cf91acb5e6fac450b8fbdcb"

func init() {
	imageTests["DXAC30_ImageSelection"] = DXAC30_ImageSelection
}

func DXAC30_ImageSelection(t *testing.T, ctx context.Context) {
	t.Helper()

	r := dxh.NewRunner()
	home := os.Getenv("HOME")
	tcfg := dxh.TeardownConfig{
		Home:   home,
		UID:    fmt.Sprintf("%d", os.Getuid()),
		CfgDir: dxh.CfgDir(home),
	}

	var registry string
	registryContainerName := "sag-devx-e2e-registry-dxac30"

	confBefore := dxac30RegistriesConf(t, ctx, r)
	var digestSeen string
	t.Cleanup(func() {
		cctx := context.Background()
		if out, _ := r.Output(cctx, "podman", "ps", "-a", "--filter", "name=^"+registryContainerName+"$", "--format", "{{.Names}}"); strings.TrimSpace(string(out)) != "" {
			t.Errorf("DXAC30: registry container %s remains", registryContainerName)
		}
		for _, ref := range []string{registry + "/kanthorlabs/sag:main", registry + "/kanthorlabs/sag@" + digestSeen} {
			if _, err := r.Output(cctx, "podman", "image", "exists", ref); err == nil {
				t.Errorf("DXAC30: pulled reference %s remains", ref)
			}
		}
		if confAfter := dxac30RegistriesConf(t, cctx, r); confAfter != confBefore {
			t.Error("DXAC30: the registry configuration of podman-machine-default changed")
		}
	})

	t.Cleanup(func() {
		r.Output(context.Background(), "podman", "rm", "-f", "--ignore", registryContainerName)
	})
	registry = dxac30StartRegistry(t, ctx, r, registryContainerName)
	waitForRegistry(t, ctx, registry)

	destRef := registry + "/kanthorlabs/sag:main"
	if out, err := r.Output(ctx, "podman", "push", "--tls-verify=false", "localhost/sag:devx-e2e", destRef); err != nil {
		t.Fatalf("push to registry: %v\n%s", err, out)
	}

	digest := registryDigest(t, ctx, registry)
	digestSeen = digest

	repoRoot, err := dxh.RepoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}

	binDir := t.TempDir()
	digestBin := filepath.Join(binDir, "sag-dxac30-digest")
	noDigestBin := filepath.Join(binDir, "sag-dxac30-nodigest")

	buildDevxtestBinary(t, ctx, repoRoot, digestBin,
		"-X github.com/kanthorlabs/sag/internal/deploy.ImageDigest="+digest)
	buildDevxtestBinary(t, ctx, repoRoot, noDigestBin, "")

	digestRef := registry + "/kanthorlabs/sag@" + digest
	mainRef := registry + "/kanthorlabs/sag:main"

	removeRegistryRefs := func() {
		r.Output(context.Background(), "podman", "rmi", digestRef)
		r.Output(context.Background(), "podman", "rmi", mainRef)
	}
	removeRegistryRefs()
	t.Cleanup(func() {
		dxh.Teardown(context.Background(), r, tcfg)
		removeRegistryRefs()
	})

	t.Run("digest_binary_pulls_by_digest", func(t *testing.T) {
		dxac30AssertAbsent(t, ctx, r, digestRef, mainRef)
		stdout, runErr := runBinaryBootstrap(t, ctx, digestBin, registry)
		if runErr != nil {
			t.Fatalf("digest binary bootstrap: %v\nstdout: %s", runErr, stdout)
		}

		if _, err := r.Output(ctx, "podman", "image", "exists", digestRef); err != nil {
			t.Fatalf("image %q not present after digest binary run", digestRef)
		}

		if dxac30HasLine(stdout, dxac30WarnLine) {
			t.Fatalf("unexpected warn line in digest binary stdout:\n%s", stdout)
		}
		if !dxac30HasLine(stdout, "install image "+digestRef) {
			t.Fatalf("line %q missing in digest binary stdout:\n%s", "install image "+digestRef, stdout)
		}
		if _, err := r.Output(ctx, "podman", "image", "exists", mainRef); err == nil {
			t.Fatalf("digest binary also pulled %s", mainRef)
		}

		r.Output(ctx, "podman", "rm", "-f", "sag")
		removeRegistryRefs()
	})

	t.Run("nodigest_binary_pulls_main_and_warns", func(t *testing.T) {
		dxac30AssertAbsent(t, ctx, r, digestRef, mainRef)
		stdout, runErr := runBinaryBootstrap(t, ctx, noDigestBin, registry)
		if runErr != nil {
			t.Fatalf("no-digest binary bootstrap: %v\nstdout: %s", runErr, stdout)
		}

		if _, err := r.Output(ctx, "podman", "image", "exists", mainRef); err != nil {
			t.Fatalf("image %q not present after no-digest binary run", mainRef)
		}

		if !dxac30HasLine(stdout, dxac30WarnLine) {
			t.Fatalf("warn line %q missing in no-digest binary stdout:\n%s", dxac30WarnLine, stdout)
		}
		if !dxac30HasLine(stdout, "install image "+mainRef) {
			t.Fatalf("line %q missing in no-digest binary stdout:\n%s", "install image "+mainRef, stdout)
		}
	})
}

func waitForRegistry(t *testing.T, ctx context.Context, registry string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", registry, time.Second)
		if err == nil {
			conn.Close()
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("registry start: context cancelled")
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatalf("registry at %s did not start in 15s", registry)
}

func registryDigest(t *testing.T, ctx context.Context, registry string) string {
	t.Helper()
	url := "http://" + registry + "/v2/kanthorlabs/sag/manifests/main"
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		t.Fatalf("registry digest request: %v", err)
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Add("Accept", "application/vnd.oci.image.index.v1+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("registry digest fetch: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		t.Fatal("registry returned no Docker-Content-Digest header")
	}
	return digest
}

func buildDevxtestBinary(t *testing.T, ctx context.Context, repoRoot, out, extraLDFlags string) {
	t.Helper()
	ldflags := "-X github.com/kanthorlabs/sag/internal/cli.Version=dxac30-test"
	if extraLDFlags != "" {
		ldflags += " " + extraLDFlags
	}
	cmd := exec.CommandContext(ctx,
		"go", "build",
		"-tags", "devxtest",
		"-trimpath",
		"-ldflags", ldflags,
		"-o", out,
		"./cmd/sag",
	)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", out, err, output)
	}
}

func runBinaryBootstrap(t *testing.T, ctx context.Context, bin, registry string) (string, error) {
	t.Helper()
	groupDir := t.TempDir()
	childEnv, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		return "", fmt.Errorf("child env: %w", err)
	}
	filtered := make([]string, 0, len(childEnv))
	for _, e := range childEnv {
		key, _, _ := strings.Cut(e, "=")
		if key == "SAG_TEST_IMAGE" {
			continue
		}
		filtered = append(filtered, e)
	}
	filtered = append(filtered,
		"SAG_TEST_REGISTRY="+registry,
		"PATH="+dxh.TestPATH(t.TempDir()),
	)

	cmd := exec.CommandContext(ctx, bin,
		"bootstrap",
		"--autostart", "no",
		"--agent-setup", "claude-code=no",
		"--agent-setup", "pi=no",
	)
	cmd.Env = filtered
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("%w; stderr: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

const dxac30WarnLine = "warn image dev build; pulls the main tag"

func dxac30HasLine(out, line string) bool {
	return slices.Contains(strings.Split(out, "\n"), line)
}

func dxac30AssertAbsent(t *testing.T, ctx context.Context, r dxh.Runner, refs ...string) {
	t.Helper()
	for _, ref := range refs {
		if _, err := r.Output(ctx, "podman", "image", "exists", ref); err == nil {
			t.Fatalf("DXAC30: %s exists before the run; the pull is not proven", ref)
		}
	}
}

func dxac30RegistriesConf(t *testing.T, ctx context.Context, r dxh.Runner) string {
	t.Helper()
	script := "cat /etc/containers/registries.conf /etc/containers/registries.conf.d/* 2>/dev/null; true"
	args := []string{"podman", "machine", "ssh", "--", "sh", "-c", script}
	if runtime.GOOS == "linux" {
		args = []string{"sh", "-c", "cat /etc/containers/registries.conf /etc/containers/registries.conf.d/* \"$HOME/.config/containers/registries.conf\" \"$HOME\"/.config/containers/registries.conf.d/* 2>/dev/null; true"}
	}
	out, err := r.Output(ctx, args[0], args[1:]...)
	if err != nil {
		t.Fatalf("DXAC30: read registry configuration: %v", err)
	}
	return string(out)
}

func dxac30StartRegistry(t *testing.T, ctx context.Context, r dxh.Runner, name string) string {
	t.Helper()
	var failures []string
	for attempt := 0; attempt < 3; attempt++ {
		registry := fmt.Sprintf("127.0.0.1:%d", freePort(t))
		out, err := exec.CommandContext(ctx, "podman", "run", "-d",
			"--rm",
			"--label", "sag.devx-e2e=1",
			"--name", name,
			"-p", registry+":5000",
			registryImage,
		).CombinedOutput()
		if err == nil {
			return registry
		}
		failures = append(failures, fmt.Sprintf("%s: %v: %s", registry, err, strings.TrimSpace(string(out))))
		r.Output(ctx, "podman", "rm", "-f", "--ignore", name)
	}
	t.Fatalf("start registry:\n%s", strings.Join(failures, "\n"))
	return ""
}

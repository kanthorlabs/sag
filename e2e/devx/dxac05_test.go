//go:build devxe2e

package devx_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	installTests["DXAC05_PodmanAptInstall"] = DXAC05_PodmanAptInstall
}

const aptPodman = "/usr/bin/podman"
const sagTestImage = "localhost/sag:devx-e2e"

func DXAC05_PodmanAptInstall(t *testing.T, ctx context.Context) {
	t.Helper()

	r := dxh.NewRunner()

	dpkgVersionBefore, err := r.Output(ctx, "dpkg-query", "-W", "-f=${Version}", "podman")
	if err != nil {
		t.Fatalf("DXAC05: dpkg-query before remove: %v", err)
	}
	pkgVersionBefore := strings.TrimSpace(string(dpkgVersionBefore))
	if pkgVersionBefore == "" {
		t.Fatal("DXAC05: empty dpkg-query output before remove")
	}
	policyOut, err := r.Output(ctx, "apt-cache", "policy", "podman")
	if err != nil {
		t.Fatalf("DXAC05: apt-cache policy podman: %v", err)
	}
	candidate := dxac05Candidate(string(policyOut))
	if candidate == "" || candidate == "(none)" {
		t.Fatalf("DXAC05: no apt candidate for podman:\n%s", policyOut)
	}

	imageIDOut, err := r.Output(ctx, aptPodman, "image", "inspect", "--format", "{{.Id}}", sagTestImage)
	if err != nil {
		t.Fatalf("DXAC05: podman image inspect %s: %v", sagTestImage, err)
	}
	imageIDBefore := strings.TrimSpace(string(imageIDOut))
	if imageIDBefore == "" {
		t.Fatalf("DXAC05: empty image ID for %s", sagTestImage)
	}

	if _, err := r.Output(ctx, "sudo", "-n", "apt-get", "remove", "-y", "podman"); err != nil {
		t.Fatalf("DXAC05: apt-get remove podman: %v", err)
	}
	t.Cleanup(func() {
		if _, statErr := os.Stat(aptPodman); statErr == nil {
			return
		}
		if _, rerr := r.Output(context.Background(), "sudo", "-n", "apt-get", "install", "-y", "podman"); rerr != nil {
			t.Errorf("DXAC05 cleanup: apt-get install podman failed, /usr/bin/podman is absent: %v", rerr)
		}
	})

	if _, statErr := os.Stat(aptPodman); statErr == nil {
		t.Fatalf("DXAC05: %s still present after apt-get remove", aptPodman)
	}

	sagBin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("DXAC05: test binary: %v", err)
	}

	groupDir := t.TempDir()
	binDir := t.TempDir()
	childEnv, err := dxh.ChildEnv(os.Environ(), groupDir)
	if err != nil {
		t.Fatalf("DXAC05: child env: %v", err)
	}
	childEnv = append(childEnv, "PATH="+dxh.TestPATH(binDir))

	stdout, stderr, code := runBootstrapNoTerminal(t, ctx, sagBin, childEnv, "--auto-approve", "podman@5.4.2")
	if code != 0 {
		t.Fatalf("DXAC05: bootstrap exit code = %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "install podman") {
		t.Fatalf("DXAC05: stdout = %q; want 'install podman' line", stdout)
	}

	if _, statErr := os.Stat(aptPodman); statErr != nil {
		t.Fatalf("DXAC05: %s absent after bootstrap: %v", aptPodman, statErr)
	}

	if _, err := os.Stat("/usr/bin/newuidmap"); err != nil {
		t.Fatalf("DXAC05: /usr/bin/newuidmap absent after bootstrap: %v", err)
	}

	dpkgVersionAfter, err := r.Output(ctx, "dpkg-query", "-W", "-f=${Version}", "podman")
	if err != nil {
		t.Fatalf("DXAC05: dpkg-query after bootstrap: %v", err)
	}
	pkgVersionAfter := strings.TrimSpace(string(dpkgVersionAfter))
	if pkgVersionAfter != candidate {
		t.Fatalf("DXAC05: installed podman %q after bootstrap; want the apt candidate %q (before the remove: %q)", pkgVersionAfter, candidate, pkgVersionBefore)
	}

	imageIDOut2, err := r.Output(ctx, aptPodman, "image", "inspect", "--format", "{{.Id}}", sagTestImage)
	if err != nil {
		t.Fatalf("DXAC05: podman image inspect %s after bootstrap: %v", sagTestImage, err)
	}
	imageIDAfter := strings.TrimSpace(string(imageIDOut2))
	if imageIDAfter != imageIDBefore {
		t.Fatalf("DXAC05: image %s ID changed: before=%q after=%q", sagTestImage, imageIDBefore, imageIDAfter)
	}
}

func dxac05Candidate(policy string) string {
	for _, line := range strings.Split(policy, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "Candidate:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

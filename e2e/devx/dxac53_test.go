//go:build devxe2e

package devx_test

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func DXAC53_PortHolder(t *testing.T, ctx context.Context) {
	t.Helper()

	home := os.Getenv("HOME")
	cfgDir := dxh.CfgDir(home)

	t.Run("BootstrapProcess", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:8443")
		if err != nil {
			t.Skipf("cannot listen on 127.0.0.1:8443: %v", err)
		}
		t.Cleanup(func() { _ = ln.Close() })

		out, runErr := runSAG(t, ctx, "bootstrap", "--autostart", "no")
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 1 {
			t.Fatalf("bootstrap with process holder: exit err = %v; output:\n%s", runErr, out)
		}
		if !strings.Contains(string(out), "fail port 8443 held by ") {
			t.Fatalf("bootstrap with process holder: fail line missing; output:\n%s", out)
		}
		if _, statErr := os.Stat(cfgDir); statErr == nil {
			t.Fatal("bootstrap with process holder: cfg dir exists after failed bootstrap")
		}
	})

	t.Run("BootstrapContainer", func(t *testing.T) {
		r := dxh.NewRunner()
		img := buildHolderImage(t, ctx)

		if _, err := r.Output(ctx, "podman", "run", "-d",
			"--name", "dx53-holder",
			"--label", "sag.devx-e2e=1",
			"-p", "127.0.0.1:8443:8443",
			img); err != nil {
			t.Fatalf("start holder container: %v", err)
		}
		t.Cleanup(func() {
			_, _ = r.Output(context.Background(), "podman", "rm", "-f", "dx53-holder")
		})

		waitContainerRunning(t, ctx, r, "dx53-holder")

		out, runErr := runSAG(t, ctx, "bootstrap", "--autostart", "no")
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 1 {
			t.Fatalf("bootstrap with container holder: exit err = %v; output:\n%s", runErr, out)
		}
		if !strings.Contains(string(out), "fail port 8443 held by container dx53-holder") {
			t.Fatalf("bootstrap with container holder: fail line missing; output:\n%s", out)
		}
		if _, statErr := os.Stat(cfgDir); statErr == nil {
			t.Fatal("bootstrap with container holder: cfg dir exists after failed bootstrap")
		}
	})

	dxh.EnsureBootstrapped(t)
	r := dxh.NewRunner()

	if out, err := runSAG(t, ctx, "service", "stop"); err != nil {
		t.Fatalf("service stop before service start tests: %v\n%s", err, out)
	}

	ctrIDOut, err := r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{.Id}}")
	if err != nil {
		t.Fatalf("container inspect sag: %v", err)
	}
	ctrID := strings.TrimSpace(string(ctrIDOut))

	t.Run("ServiceStartProcess", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:8443")
		if err != nil {
			t.Skipf("cannot listen on 127.0.0.1:8443: %v", err)
		}
		t.Cleanup(func() { _ = ln.Close() })

		out, runErr := runSAG(t, ctx, "service", "start")
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 1 {
			t.Fatalf("service start with process holder: exit err = %v; output:\n%s", runErr, out)
		}
		if !strings.Contains(string(out), "fail port 8443 held by ") {
			t.Fatalf("service start with process holder: fail line missing; output:\n%s", out)
		}

		ctrIDOut2, err := r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{.Id}}")
		if err != nil {
			t.Fatalf("container inspect sag after service start: %v", err)
		}
		if got := strings.TrimSpace(string(ctrIDOut2)); got != ctrID {
			t.Fatalf("container ID changed after service start with process holder: before=%q after=%q", ctrID, got)
		}
	})

	t.Run("ServiceStartContainer", func(t *testing.T) {
		img := buildHolderImage(t, ctx)

		if _, err := r.Output(ctx, "podman", "run", "-d",
			"--name", "dx53-holder2",
			"--label", "sag.devx-e2e=1",
			"-p", "127.0.0.1:8443:8443",
			img); err != nil {
			t.Fatalf("start holder container: %v", err)
		}
		t.Cleanup(func() {
			_, _ = r.Output(context.Background(), "podman", "rm", "-f", "dx53-holder2")
		})

		waitContainerRunning(t, ctx, r, "dx53-holder2")

		out, runErr := runSAG(t, ctx, "service", "start")
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 1 {
			t.Fatalf("service start with container holder: exit err = %v; output:\n%s", runErr, out)
		}
		if !strings.Contains(string(out), "fail port 8443 held by container dx53-holder2") {
			t.Fatalf("service start with container holder: fail line missing; output:\n%s", out)
		}

		ctrIDOut2, err := r.Output(ctx, "podman", "container", "inspect", "sag", "--format", "{{.Id}}")
		if err != nil {
			t.Fatalf("container inspect sag after service start: %v", err)
		}
		if got := strings.TrimSpace(string(ctrIDOut2)); got != ctrID {
			t.Fatalf("container ID changed after service start with container holder: before=%q after=%q", ctrID, got)
		}
	})
}

func waitContainerRunning(t *testing.T, ctx context.Context, r dxh.Runner, name string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		out, _ := r.Output(ctx, "podman", "ps", "--filter", "name=^"+name+"$", "--format", "{{.Names}}")
		if strings.Contains(string(out), name) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("container %q did not appear in podman ps within 10s", name)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func buildHolderImage(t *testing.T, ctx context.Context) string {
	t.Helper()

	srcDir := t.TempDir()

	holderSrc := "package main\n\nimport (\n\t\"os\"\n\t\"os/signal\"\n\t\"syscall\"\n)\n\nfunc main() {\n\tch := make(chan os.Signal, 1)\n\tsignal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)\n\t<-ch\n}\n"
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(holderSrc), 0o644); err != nil {
		t.Fatalf("write holder main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "go.mod"), []byte("module holder\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write holder go.mod: %v", err)
	}

	holderBin := filepath.Join(srcDir, "holder")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", holderBin, ".")
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(), "GOOS=linux", "CGO_ENABLED=0", "GOARCH="+runtime.GOARCH)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build holder binary: %v\n%s", err, data)
	}

	cfile := filepath.Join(srcDir, "Containerfile")
	cfContent := "FROM scratch\nCOPY holder /holder\nENTRYPOINT [\"/holder\"]\n"
	if err := os.WriteFile(cfile, []byte(cfContent), 0o644); err != nil {
		t.Fatalf("write Containerfile: %v", err)
	}

	img := "localhost/sag-dx53-holder:1"
	r := dxh.NewRunner()
	if _, err := r.Output(ctx, "podman", "build", "-t", img, "-f", cfile, srcDir); err != nil {
		t.Fatalf("podman build holder image: %v", err)
	}
	t.Cleanup(func() {
		_, _ = r.Output(context.Background(), "podman", "rmi", img)
	})

	return img
}

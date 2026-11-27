//go:build devxtest

package image_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/image"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
	"io"
)

func TestRegistryHookReplacesDockerIO(t *testing.T) {
	t.Setenv("SAG_TEST_IMAGE", "")
	old := deploy.ImageDigest
	defer func() { deploy.ImageDigest = old }()

	t.Run("main_tag", func(t *testing.T) {
		deploy.ImageDigest = ""
		t.Setenv("SAG_TEST_REGISTRY", "localhost:5555")
		ref, devBuild := image.Reference()
		if !devBuild {
			t.Fatal("devBuild = false; want true for no-digest build")
		}
		want := "localhost:5555/kanthorlabs/sag:main"
		if ref != want {
			t.Fatalf("ref = %q; want %q", ref, want)
		}
	})

	t.Run("digest", func(t *testing.T) {
		deploy.ImageDigest = "sha256:deadbeef"
		t.Setenv("SAG_TEST_REGISTRY", "localhost:5555")
		ref, devBuild := image.Reference()
		if devBuild {
			t.Fatal("devBuild = true; want false for digest build")
		}
		want := "localhost:5555/kanthorlabs/sag@sha256:deadbeef"
		if ref != want {
			t.Fatalf("ref = %q; want %q", ref, want)
		}
	})

	t.Run("no_registry_keeps_docker_io", func(t *testing.T) {
		deploy.ImageDigest = ""
		t.Setenv("SAG_TEST_REGISTRY", "")
		ref, _ := image.Reference()
		want := "docker.io/kanthorlabs/sag:main"
		if ref != want {
			t.Fatalf("ref = %q; want %q", ref, want)
		}
	})
}

func TestRegistryHookTLSOffForHookOnly(t *testing.T) {
	t.Setenv("SAG_TEST_IMAGE", "")
	t.Setenv("SAG_TEST_REGISTRY", "localhost:5555")
	old := deploy.ImageDigest
	deploy.ImageDigest = ""
	defer func() { deploy.ImageDigest = old }()

	t.Run("hook_ref_gets_tls_off", func(t *testing.T) {
		fr := &fake{fn: func(args []string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "image" && args[1] == "exists" {
				return nil, &podman.RunError{ExitCode: 1, Stderr: "image not known"}
			}
			return nil, nil
		}}
		out := stepline.New(io.Discard)
		if _, err := image.Ensure(context.Background(), fr, "localhost:5555/kanthorlabs/sag:main", out); err != nil {
			t.Fatal(err)
		}
		var pullArgs []string
		for _, c := range fr.calls {
			if len(c.args) >= 1 && c.args[0] == "pull" {
				pullArgs = c.args
				break
			}
		}
		if pullArgs == nil {
			t.Fatal("no pull call found")
		}
		found := false
		for _, a := range pullArgs {
			if strings.Contains(a, "tls-verify=false") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("--tls-verify=false missing in pull args %v", pullArgs)
		}
	})

	t.Run("non_hook_ref_keeps_tls", func(t *testing.T) {
		fr := &fake{fn: func(args []string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "image" && args[1] == "exists" {
				return nil, &podman.RunError{ExitCode: 1, Stderr: "image not known"}
			}
			return nil, nil
		}}
		out := stepline.New(io.Discard)
		if _, err := image.Ensure(context.Background(), fr, "docker.io/kanthorlabs/sag:main", out); err != nil {
			t.Fatal(err)
		}
		for _, c := range fr.calls {
			if len(c.args) >= 1 && c.args[0] == "pull" {
				for _, a := range c.args {
					if strings.Contains(a, "tls-verify") {
						t.Fatalf("unexpected tls-verify flag for non-hook ref: %v", c.args)
					}
				}
			}
		}
	})
}

package image_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/image"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func TestDigestReference(t *testing.T) {
	t.Setenv("SAG_TEST_IMAGE", "")
	t.Setenv("SAG_TEST_REGISTRY", "")
	old := deploy.ImageDigest
	deploy.ImageDigest = "sha256:deadbeefcafe"
	defer func() { deploy.ImageDigest = old }()

	ref, devBuild := image.Reference()
	if devBuild {
		t.Fatal("devBuild = true; want false when digest is set")
	}
	want := "docker.io/kanthorlabs/sag@sha256:deadbeefcafe"
	if ref != want {
		t.Fatalf("ref = %q; want %q", ref, want)
	}
}

func TestDevBuildMainTag(t *testing.T) {
	t.Setenv("SAG_TEST_IMAGE", "")
	t.Setenv("SAG_TEST_REGISTRY", "")
	old := deploy.ImageDigest
	deploy.ImageDigest = ""
	defer func() { deploy.ImageDigest = old }()

	ref, devBuild := image.Reference()
	if !devBuild {
		t.Fatal("devBuild = false; want true when no digest is set")
	}
	want := "docker.io/kanthorlabs/sag:main"
	if ref != want {
		t.Fatalf("ref = %q; want %q", ref, want)
	}
}

func TestWarnOnlyOnPull(t *testing.T) {
	t.Setenv("SAG_TEST_IMAGE", "")
	t.Setenv("SAG_TEST_REGISTRY", "")
	old := deploy.ImageDigest
	defer func() { deploy.ImageDigest = old }()

	t.Run("dev_build_warns_on_pull", func(t *testing.T) {
		deploy.ImageDigest = ""
		fr := &fake{fn: func(args []string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "image" && args[1] == "exists" {
				return nil, &podman.RunError{ExitCode: 1, Stderr: "image not known"}
			}
			return nil, nil
		}}
		var buf bytes.Buffer
		out := stepline.New(&buf)
		if _, err := image.Ensure(context.Background(), fr, "docker.io/kanthorlabs/sag:main", out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "dev build; pulls the main tag") {
			t.Fatalf("warn line missing; got %q", buf.String())
		}
	})

	t.Run("dev_build_no_warn_when_present", func(t *testing.T) {
		deploy.ImageDigest = ""
		fr := &fake{fn: func(args []string) ([]byte, error) {
			return nil, nil
		}}
		var buf bytes.Buffer
		out := stepline.New(&buf)
		if _, err := image.Ensure(context.Background(), fr, "docker.io/kanthorlabs/sag:main", out); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), "dev build; pulls the main tag") {
			t.Fatalf("unexpected warn line when image is present; got %q", buf.String())
		}
	})

	t.Run("digest_build_no_warn_on_pull", func(t *testing.T) {
		deploy.ImageDigest = "sha256:abc"
		fr := &fake{fn: func(args []string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "image" && args[1] == "exists" {
				return nil, &podman.RunError{ExitCode: 1, Stderr: "image not known"}
			}
			return nil, nil
		}}
		var buf bytes.Buffer
		out := stepline.New(&buf)
		if _, err := image.Ensure(context.Background(), fr, "docker.io/kanthorlabs/sag@sha256:abc", out); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), "dev build; pulls the main tag") {
			t.Fatalf("unexpected warn line for digest build; got %q", buf.String())
		}
	})
}

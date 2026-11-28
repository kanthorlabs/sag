//go:build !devxtest

package image_test

import (
	"context"
	"io"
	"slices"
	"testing"

	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/image"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

type recordingPodman struct {
	calls [][]string
}

func (p *recordingPodman) Run(_ context.Context, _ io.Reader, args ...string) ([]byte, error) {
	p.calls = append(p.calls, args)
	if len(args) >= 2 && args[0] == "image" && args[1] == "exists" {
		return nil, &podman.RunError{ExitCode: 1, Stderr: "image not known"}
	}
	return nil, nil
}

func TestProductBuildIgnoresTestHooks(t *testing.T) {
	t.Setenv("SAG_TEST_IMAGE", "localhost/sag:hook")
	t.Setenv("SAG_TEST_REGISTRY", "127.0.0.1:5000")
	old := deploy.ImageDigest
	defer func() { deploy.ImageDigest = old }()

	deploy.ImageDigest = ""
	ref, devBuild := image.Reference()
	if ref != "docker.io/kanthorlabs/sag:main" || !devBuild {
		t.Fatalf("Reference() = %q, %v; want docker.io/kanthorlabs/sag:main, true", ref, devBuild)
	}
	pm := &recordingPodman{}
	if _, err := image.Ensure(context.Background(), pm, ref, stepline.New(io.Discard)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"pull", ref}; !slices.ContainsFunc(pm.calls, func(c []string) bool { return slices.Equal(c, want) }) {
		t.Fatalf("calls = %v; want %v without --tls-verify", pm.calls, want)
	}

	deploy.ImageDigest = "sha256:0123abcd"
	if ref, _ := image.Reference(); ref != "docker.io/kanthorlabs/sag@sha256:0123abcd" {
		t.Fatalf("Reference() = %q; want the docker.io digest reference", ref)
	}
}

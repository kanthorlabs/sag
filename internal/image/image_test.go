package image_test

import (
	"context"
	"io"
	"testing"

	"github.com/kanthorlabs/sag/internal/image"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
)

type fake struct {
	calls []fakeCall
	fn    func(args []string) ([]byte, error)
}

type fakeCall struct {
	args []string
}

func (f *fake) Run(_ context.Context, _ io.Reader, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{args: args})
	return f.fn(args)
}

func TestPullOnlyWhenAbsent(t *testing.T) {
	const ref = "docker.io/example/test:v1"

	t.Run("absent", func(t *testing.T) {
		fr := &fake{fn: func(args []string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "image" && args[1] == "exists" {
				return nil, &podman.RunError{ExitCode: 1, Stderr: "image not known"}
			}
			return nil, nil
		}}
		out := stepline.New(io.Discard)
		changed, err := image.Ensure(context.Background(), fr, ref, out)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("expected changed=true when image is absent")
		}
		var pullRef string
		for _, c := range fr.calls {
			if len(c.args) >= 1 && c.args[0] == "pull" {
				if len(c.args) >= 2 {
					pullRef = c.args[1]
				}
			}
		}
		if pullRef == "" {
			t.Fatal("expected pull call when image is absent")
		}
		if pullRef != ref {
			t.Fatalf("pull ref = %q; want %q", pullRef, ref)
		}
	})

	t.Run("present", func(t *testing.T) {
		fr := &fake{fn: func(args []string) ([]byte, error) {
			return nil, nil
		}}
		out := stepline.New(io.Discard)
		changed, err := image.Ensure(context.Background(), fr, ref, out)
		if err != nil {
			t.Fatal(err)
		}
		if changed {
			t.Fatal("expected changed=false when image is present")
		}
		for _, c := range fr.calls {
			if len(c.args) >= 1 && c.args[0] == "pull" {
				t.Fatalf("unexpected pull call: %v", c.args)
			}
		}
	})

	t.Run("podman_error_exit125_no_pull", func(t *testing.T) {
		fr := &fake{fn: func(args []string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "image" && args[1] == "exists" {
				return nil, &podman.RunError{ExitCode: 125, Stderr: "Error: socket not found"}
			}
			return nil, nil
		}}
		out := stepline.New(io.Discard)
		_, err := image.Ensure(context.Background(), fr, ref, out)
		if err == nil {
			t.Fatal("expected error when image exists exits 125")
		}
		for _, c := range fr.calls {
			if len(c.args) >= 1 && c.args[0] == "pull" {
				t.Fatalf("pull called on exit 125: %v", c.args)
			}
		}
	})
}

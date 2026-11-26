//go:build darwin

package portcheck

import (
	"context"
	"testing"
)

func TestContainerSagIsOk(t *testing.T) {
	psJSON := `[{"Names":["sag"],"Ports":[{"host_port":8443}]}]`
	h, err := check(context.Background(),
		func(_ context.Context) ([]byte, error) { return []byte(psJSON), nil },
		func(_ context.Context) ([]byte, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderSAG {
		t.Fatalf("Kind = %v; want HolderSAG", h.Kind)
	}
	if h.Name != "sag" {
		t.Fatalf("Name = %q; want %q", h.Name, "sag")
	}
}

func TestOtherContainerFails(t *testing.T) {
	psJSON := `[{"Names":["foreign"],"Ports":[{"host_port":8443}]}]`
	h, err := check(context.Background(),
		func(_ context.Context) ([]byte, error) { return []byte(psJSON), nil },
		func(_ context.Context) ([]byte, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderContainer {
		t.Fatalf("Kind = %v; want HolderContainer", h.Kind)
	}
	if h.Name != "foreign" {
		t.Fatalf("Name = %q; want %q", h.Name, "foreign")
	}
}

func TestProcessFails(t *testing.T) {
	lsofOut := "COMMAND  PID\nsomeproc 123\n"
	h, err := check(context.Background(),
		func(_ context.Context) ([]byte, error) { return []byte(`[]`), nil },
		func(_ context.Context) ([]byte, error) { return []byte(lsofOut), nil })
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderProcess {
		t.Fatalf("Kind = %v; want HolderProcess", h.Kind)
	}
	if h.Name != "someproc" {
		t.Fatalf("Name = %q; want %q", h.Name, "someproc")
	}
}

func TestFreePortContinues(t *testing.T) {
	h, err := check(context.Background(),
		func(_ context.Context) ([]byte, error) { return []byte(`[]`), nil },
		func(_ context.Context) ([]byte, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderNone {
		t.Fatalf("Kind = %v; want HolderNone", h.Kind)
	}
}

func TestLsofParse(t *testing.T) {
	out := "COMMAND  PID   USER   FD   TYPE DEVICE SIZE/OFF NODE NAME\nmyproc  999  user   5u   IPv4 0t0  TCP 127.0.0.1:8443 (LISTEN)\n"
	h, err := parseLsof([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderProcess {
		t.Fatalf("Kind = %v; want HolderProcess", h.Kind)
	}
	if h.Name != "myproc" {
		t.Fatalf("Name = %q; want %q", h.Name, "myproc")
	}
}

func TestGvproxyNotAHolderWhenPodmanPublishes(t *testing.T) {
	psJSON := `[{"Names":["sag"],"Ports":[{"host_port":8443}]}]`
	lsofOut := "COMMAND  PID\ngvproxy 999\n"
	lsofCalled := false
	h, err := check(context.Background(),
		func(_ context.Context) ([]byte, error) { return []byte(psJSON), nil },
		func(_ context.Context) ([]byte, error) {
			lsofCalled = true
			return []byte(lsofOut), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderSAG {
		t.Fatalf("Kind = %v; want HolderSAG", h.Kind)
	}
	if lsofCalled {
		t.Fatal("lsof was called when podman ps found the holder")
	}
}

//go:build linux

package portcheck

import (
	"context"
	"testing"
)

func TestSSParse(t *testing.T) {
	ssOut := "Netid  State   Recv-Q  Send-Q  Local Address:Port  Peer Address:Port  Process\n" +
		`tcp    LISTEN  0       128     127.0.0.1:8443      0.0.0.0:*          users:(("myproc",pid=1234,fd=10))`

	h, err := parseSS([]byte(ssOut))
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

func TestSSRootlessport(t *testing.T) {
	ssOut := "Netid  State   Recv-Q  Send-Q  Local Address:Port  Peer Address:Port  Process\n" +
		"tcp    LISTEN  0       128     127.0.0.1:8443      0.0.0.0:*"

	h, err := parseSS([]byte(ssOut))
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderProcess {
		t.Fatalf("Kind = %v; want HolderProcess", h.Kind)
	}
}

func TestSSEmpty(t *testing.T) {
	h, err := parseSS([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderNone {
		t.Fatalf("Kind = %v; want HolderNone", h.Kind)
	}
}

func TestSSHeaderOnly(t *testing.T) {
	ssOut := "Netid  State   Recv-Q  Send-Q  Local Address:Port  Peer Address:Port  Process"

	h, err := parseSS([]byte(ssOut))
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderNone {
		t.Fatalf("Kind = %v; want HolderNone", h.Kind)
	}
}

func TestCheckLinuxSSUsedWhenNoContainer(t *testing.T) {
	ssOut := "Netid  State   Recv-Q  Send-Q  Local Address:Port  Peer Address:Port  Process\n" +
		`tcp    LISTEN  0       128     127.0.0.1:8443      0.0.0.0:*          users:(("otherd",pid=99,fd=5))`

	h, err := check(context.Background(),
		func(_ context.Context) ([]byte, error) { return []byte(`[]`), nil },
		func(_ context.Context) ([]byte, error) { return []byte(ssOut), nil })
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderProcess {
		t.Fatalf("Kind = %v; want HolderProcess", h.Kind)
	}
	if h.Name != "otherd" {
		t.Fatalf("Name = %q; want otherd", h.Name)
	}
}

func TestCheckLinuxContainerSagBeforeSS(t *testing.T) {
	psJSON := `[{"Names":["sag"],"Ports":[{"host_port":8443}]}]`
	ssCalled := false
	h, err := check(context.Background(),
		func(_ context.Context) ([]byte, error) { return []byte(psJSON), nil },
		func(_ context.Context) ([]byte, error) {
			ssCalled = true
			return nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderSAG {
		t.Fatalf("Kind = %v; want HolderSAG", h.Kind)
	}
	if ssCalled {
		t.Fatal("ss was called when podman ps found the holder")
	}
}

func TestCheckLinuxNoHolderFreePort(t *testing.T) {
	h, err := check(context.Background(),
		func(_ context.Context) ([]byte, error) { return []byte(`[]`), nil },
		func(_ context.Context) ([]byte, error) { return []byte(""), nil })
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != HolderNone {
		t.Fatalf("Kind = %v; want HolderNone", h.Kind)
	}
}

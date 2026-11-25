package core

import (
	"context"
	"testing"

	"github.com/kanthorlabs/sag/internal/secret"
)

type fakeAdapter struct{ kind string }

func (a fakeAdapter) Kind() string { return a.kind }

func (a fakeAdapter) Capabilities() []Capability { return nil }

func (a fakeAdapter) Open(context.Context, ResourceConfig, secret.Credential) (Resource, error) {
	return nil, nil
}

func TestRegisterDuplicateKindPanics(t *testing.T) {
	Register(fakeAdapter{kind: "test-duplicate"})
	defer func() {
		if recover() == nil {
			t.Fatal("second Register of the same kind did not panic")
		}
	}()
	Register(fakeAdapter{kind: "test-duplicate"})
}

func TestLookupUnknownKind(t *testing.T) {
	a, ok := Lookup("test-unknown")
	if ok || a != nil {
		t.Fatalf("Lookup(unknown) = %v, %v; want nil, false", a, ok)
	}
}

func TestLookupRegisteredKind(t *testing.T) {
	Register(fakeAdapter{kind: "test-registered"})
	a, ok := Lookup("test-registered")
	if !ok || a.Kind() != "test-registered" {
		t.Fatalf("Lookup(registered) = %v, %v", a, ok)
	}
	found := false
	for _, k := range Kinds() {
		if k == "test-registered" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Kinds() = %v; want test-registered", Kinds())
	}
}

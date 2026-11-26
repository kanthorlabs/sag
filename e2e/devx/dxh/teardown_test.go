package dxh_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestTeardownOrder(t *testing.T) {
	r := newFake()
	r.set("podman secret ls --filter label=sag.resource= --format {{.Name}}", nil, nil)
	r.set("podman ps -a --filter label=sag.devx-e2e=1 --format {{.Names}}", nil, nil)
	r.set("podman volume ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("podman network ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("launchctl bootout gui/ net.kanthorlabs.sag", nil, nil)
	r.set("podman rm -f sag", nil, nil)
	r.set("podman volume rm sag-state", nil, nil)
	r.set("podman secret rm sag-master-key sag-tls-cert sag-tls-key", nil, nil)
	r.set("podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}", nil, nil)

	cfg := dxh.TeardownConfig{Home: "/home/u", UID: "1000"}
	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	calls := r.calls
	labeledIdx := firstIndexContaining(calls, "label=sag.devx-e2e=1")
	credIdx := firstIndexContaining(calls, "credential")
	bootoutIdx := firstIndexContaining(calls, "bootout")
	rmSagIdx := firstIndexContaining(calls, "podman rm")
	if credIdx >= 0 {
		t.Errorf("teardown called credential delete: %v", calls[credIdx])
	}
	if labeledIdx < 0 {
		t.Error("teardown did not check for labeled objects")
	}
	if bootoutIdx < 0 {
		t.Error("teardown did not call launchctl bootout")
	}
	if rmSagIdx < 0 {
		t.Error("teardown did not remove sag container")
	}
	if labeledIdx >= bootoutIdx {
		t.Error("labeled object removal should happen before launchctl bootout")
	}
}

func TestTeardownRemovesOnlyLabeled(t *testing.T) {
	r := newFake()
	r.set("podman secret ls --filter label=sag.resource= --format {{.Name}}", nil, nil)
	r.set("podman ps -a --filter label=sag.devx-e2e=1 --format {{.Names}}", []byte("test-container\n"), nil)
	r.set("podman volume ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("podman network ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("podman rm -f test-container", nil, nil)
	r.set("launchctl bootout gui/ net.kanthorlabs.sag", nil, nil)
	r.set("podman rm -f sag", nil, nil)
	r.set("podman volume rm sag-state", nil, nil)
	r.set("podman secret rm sag-master-key sag-tls-cert sag-tls-key", nil, nil)
	r.set("podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}", nil, nil)

	cfg := dxh.TeardownConfig{Home: "/home/u", UID: "1000"}
	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	removed := callsContaining(r.calls, "podman rm")
	for _, call := range removed {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "foreign-app") || strings.Contains(joined, "foreign-db") || strings.Contains(joined, "foreign-cache") {
			t.Errorf("teardown removed non-SAG object: %v", call)
		}
	}
}

func TestTeardownNeverCallsCredentialDelete(t *testing.T) {
	r := newFake()
	r.set("podman secret ls --filter label=sag.resource= --format {{.Name}}", []byte("sag-res-canary.conn\n"), nil)
	r.set("podman secret rm sag-res-canary.conn", nil, nil)
	r.set("podman ps -a --filter label=sag.devx-e2e=1 --format {{.Names}}", nil, nil)
	r.set("podman volume ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("podman network ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("launchctl bootout gui/ net.kanthorlabs.sag", nil, nil)
	r.set("podman rm -f sag", nil, nil)
	r.set("podman volume rm sag-state", nil, nil)
	r.set("podman secret rm sag-master-key sag-tls-cert sag-tls-key", nil, nil)
	r.set("podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}", nil, nil)

	cfg := dxh.TeardownConfig{Home: "/home/u", UID: "1000"}
	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	for _, call := range r.calls {
		if slices.Contains(call, "credential") && slices.Contains(call, "delete") {
			t.Errorf("teardown called credential delete: %v", call)
		}
	}
}

func TestTeardownKeepsDevxImage(t *testing.T) {
	r := newFake()
	r.set("podman secret ls --filter label=sag.resource= --format {{.Name}}", nil, nil)
	r.set("podman ps -a --filter label=sag.devx-e2e=1 --format {{.Names}}", nil, nil)
	r.set("podman volume ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("podman network ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("launchctl bootout gui/ net.kanthorlabs.sag", nil, nil)
	r.set("podman rm -f sag", nil, nil)
	r.set("podman volume rm sag-state", nil, nil)
	r.set("podman secret rm sag-master-key sag-tls-cert sag-tls-key", nil, nil)
	r.set("podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}", []byte("docker.io/kanthorlabs/sag:main\n"), nil)
	r.set("podman rmi docker.io/kanthorlabs/sag:main", nil, nil)

	cfg := dxh.TeardownConfig{Home: "/home/u", UID: "1000"}
	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	for _, call := range r.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "localhost/sag:devx-e2e") && (strings.Contains(joined, "rmi") || strings.Contains(joined, "rm ")) {
			t.Errorf("teardown removed devx-e2e image: %v", call)
		}
	}
}

func firstIndexContaining(calls [][]string, text string) int {
	for i, call := range calls {
		if strings.Contains(strings.Join(call, " "), text) {
			return i
		}
	}
	return -1
}

func callsContaining(calls [][]string, text string) [][]string {
	var out [][]string
	for _, call := range calls {
		if strings.Contains(strings.Join(call, " "), text) {
			out = append(out, call)
		}
	}
	return out
}

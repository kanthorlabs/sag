package dxh_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func newCleanTeardownFake(home, uid, cfgDir string) (*fakeRunner, dxh.TeardownConfig) {
	r := newFake()
	r.set("podman secret ls --format {{.Name}}", nil, nil)
	r.set("podman ps -a --filter label=sag.devx-e2e=1 --format {{.Names}}", nil, nil)
	r.set("podman volume ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("podman network ls --filter label=sag.devx-e2e=1 --format {{.Name}}", nil, nil)
	r.set("launchctl bootout gui/"+uid+"/net.kanthorlabs.sag", nil, nil)
	r.set("rm -f "+home+"/Library/LaunchAgents/net.kanthorlabs.sag.plist", nil, nil)
	r.set("rm -rf "+home+"/Library/Logs/sag", nil, nil)
	r.set("podman rm -f sag", nil, nil)
	r.set("podman volume ls --filter name=sag-state --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter name=sag-master-key --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter name=sag-tls-cert --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter name=sag-tls-key --format {{.Name}}", nil, nil)
	r.set("podman secret ls --filter name=sag-aws-credentials --format {{.Name}}", nil, nil)
	r.set("podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}", nil, nil)
	if cfgDir != "" {
		r.set("rm -rf "+cfgDir, nil, nil)
	}
	cfg := dxh.TeardownConfig{Home: home, UID: uid, CfgDir: cfgDir}
	return r, cfg
}

func TestTeardownOrder(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")

	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"podman", "secret", "ls", "--format", "{{.Name}}"},
		{"podman", "ps", "-a", "--filter", "label=sag.devx-e2e=1", "--format", "{{.Names}}"},
		{"podman", "volume", "ls", "--filter", "label=sag.devx-e2e=1", "--format", "{{.Name}}"},
		{"podman", "secret", "ls", "--format", "{{.Name}}"},
		{"podman", "network", "ls", "--filter", "label=sag.devx-e2e=1", "--format", "{{.Name}}"},
		{"launchctl", "bootout", "gui/1000/net.kanthorlabs.sag"},
		{"rm", "-f", "/home/u/Library/LaunchAgents/net.kanthorlabs.sag.plist"},
		{"rm", "-rf", "/home/u/Library/Logs/sag"},
		{"podman", "rm", "-f", "sag"},
		{"podman", "volume", "ls", "--filter", "name=sag-state", "--format", "{{.Name}}"},
		{"podman", "secret", "ls", "--filter", "name=sag-master-key", "--format", "{{.Name}}"},
		{"podman", "secret", "ls", "--filter", "name=sag-tls-cert", "--format", "{{.Name}}"},
		{"podman", "secret", "ls", "--filter", "name=sag-tls-key", "--format", "{{.Name}}"},
		{"podman", "secret", "ls", "--filter", "name=sag-aws-credentials", "--format", "{{.Name}}"},
		{"podman", "image", "ls", "--filter", "reference=docker.io/kanthorlabs/sag", "--format", "{{.Repository}}:{{.Tag}}"},
		{"rm", "-rf", "/home/u/.config/sag"},
	}
	if len(r.calls) != len(want) {
		t.Fatalf("got %d calls, want %d:\n%v", len(r.calls), len(want), r.calls)
	}
	for i, w := range want {
		if !slices.Equal(r.calls[i], w) {
			t.Errorf("call[%d] = %v, want %v", i, r.calls[i], w)
		}
	}
}

func TestTeardownRemovesOnlyLabeled(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")
	r.set("podman ps -a --filter label=sag.devx-e2e=1 --format {{.Names}}", []byte("test-container\n"), nil)
	r.set("podman rm -f test-container", nil, nil)

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
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")
	r.set("podman secret ls --format {{.Name}}", []byte("sag-res-canary.conn\n"), nil)
	r.set(`podman secret inspect sag-res-canary.conn --format {{index .Spec.Labels "sag.resource"}}`, []byte("true"), nil)
	r.set("podman secret rm sag-res-canary.conn", nil, nil)
	r.set(`podman secret inspect sag-res-canary.conn --format {{index .Spec.Labels "sag.devx-e2e"}}`, nil, nil)

	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	for _, call := range r.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "credential") && strings.Contains(joined, "delete") {
			t.Errorf("teardown called credential delete: %v", call)
		}
	}
}

func TestTeardownKeepsDevxImage(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")
	r.set("podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}", []byte("docker.io/kanthorlabs/sag:main\nlocalhost/sag:e2e\nlocalhost/sag:devx-e2e\n"), nil)
	r.set("podman rmi docker.io/kanthorlabs/sag:main", nil, nil)
	r.set("podman rmi localhost/sag:e2e", nil, nil)

	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	for _, call := range r.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "localhost/sag:devx-e2e") && (strings.Contains(joined, "rmi") || strings.Contains(joined, "rm ")) {
			t.Errorf("teardown removed devx-e2e image: %v", call)
		}
	}
	e2eRemoved := false
	for _, call := range r.calls {
		if slices.Contains(call, "rmi") && slices.Contains(call, "localhost/sag:e2e") {
			e2eRemoved = true
		}
	}
	if !e2eRemoved {
		t.Error("teardown did not remove localhost/sag:e2e")
	}
}

func TestTeardownCleanMachineIsNil(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")

	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatalf("expected nil on clean machine: %v", err)
	}
}

func TestTeardownBootoutTarget(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")

	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	for _, call := range r.calls {
		if len(call) > 0 && call[0] == "launchctl" {
			if len(call) != 3 {
				t.Errorf("bootout call has %d args, want 3: %v", len(call), call)
				continue
			}
			want := "gui/1000/net.kanthorlabs.sag"
			if call[2] != want {
				t.Errorf("bootout target = %q, want %q", call[2], want)
			}
		}
	}
}

func TestTeardownRemovesFiles(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")
	r.set("podman volume ls --filter name=sag-state --format {{.Name}}", []byte("sag-state\n"), nil)
	r.set("podman volume rm sag-state", nil, nil)
	r.set("podman secret ls --filter name=sag-aws-credentials --format {{.Name}}", []byte("sag-aws-credentials\n"), nil)
	r.set("podman secret rm sag-aws-credentials", nil, nil)

	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	wantSubstrings := []string{
		"/home/u/Library/LaunchAgents/net.kanthorlabs.sag.plist",
		"/home/u/Library/Logs/sag",
		"/home/u/.config/sag",
		"sag-aws-credentials",
	}
	joined := func() string {
		var parts []string
		for _, call := range r.calls {
			parts = append(parts, strings.Join(call, " "))
		}
		return strings.Join(parts, "\n")
	}()
	for _, want := range wantSubstrings {
		if !strings.Contains(joined, want) {
			t.Errorf("teardown did not act on %q", want)
		}
	}
}

func TestTeardownRunsEachStepAfterAFailure(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")
	r.set("podman rm -f sag", nil, fmt.Errorf("podman rm failed"))

	err := dxh.Teardown(context.Background(), r, cfg)
	if err == nil {
		t.Fatal("expected error when a step fails")
	}

	subsequentCalls := []string{
		"podman volume ls --filter name=sag-state --format {{.Name}}",
		"podman secret ls --filter name=sag-master-key --format {{.Name}}",
		"podman image ls --filter reference=docker.io/kanthorlabs/sag --format {{.Repository}}:{{.Tag}}",
		"rm -rf /home/u/.config/sag",
	}
	for _, want := range subsequentCalls {
		if firstIndexContaining(r.calls, want) < 0 {
			t.Errorf("teardown did not run step %q after prior failure", want)
		}
	}
}

func TestTeardownSecretInspectBeforeRm(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")
	r.set("podman secret ls --format {{.Name}}", []byte("sag-res-canary.conn\n"), nil)
	r.set(`podman secret inspect sag-res-canary.conn --format {{index .Spec.Labels "sag.resource"}}`, []byte("true"), nil)
	r.set("podman secret rm sag-res-canary.conn", nil, nil)
	r.set(`podman secret inspect sag-res-canary.conn --format {{index .Spec.Labels "sag.devx-e2e"}}`, nil, nil)

	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	inspectIdx := firstIndexContaining(r.calls, `inspect sag-res-canary.conn --format`)
	rmIdx := firstIndexContaining(r.calls, "secret rm sag-res-canary.conn")
	if inspectIdx < 0 {
		t.Fatal("no secret inspect call found")
	}
	if rmIdx < 0 {
		t.Fatal("no secret rm call found")
	}
	if inspectIdx >= rmIdx {
		t.Errorf("inspect (call[%d]) must come before rm (call[%d])", inspectIdx, rmIdx)
	}
}

func TestTeardownDevxE2ELabeledSecret(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")
	r.set("podman secret ls --format {{.Name}}", []byte("test-secret\n"), nil)
	r.set(`podman secret inspect test-secret --format {{index .Spec.Labels "sag.resource"}}`, nil, nil)
	r.set(`podman secret inspect test-secret --format {{index .Spec.Labels "sag.devx-e2e"}}`, []byte("1"), nil)
	r.set("podman secret rm test-secret", nil, nil)

	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	rmFound := false
	for _, call := range r.calls {
		if slices.Contains(call, "secret") && slices.Contains(call, "rm") && slices.Contains(call, "test-secret") {
			rmFound = true
		}
	}
	if !rmFound {
		t.Error("teardown did not remove sag.devx-e2e labeled secret")
	}
}

func TestTeardownSkipsUnlabeledSecret(t *testing.T) {
	r, cfg := newCleanTeardownFake("/home/u", "1000", "/home/u/.config/sag")
	r.set("podman secret ls --format {{.Name}}", []byte("unrelated-secret\n"), nil)
	r.set(`podman secret inspect unrelated-secret --format {{index .Spec.Labels "sag.resource"}}`, nil, nil)
	r.set(`podman secret inspect unrelated-secret --format {{index .Spec.Labels "sag.devx-e2e"}}`, nil, nil)

	if err := dxh.Teardown(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}

	for _, call := range r.calls {
		if slices.Contains(call, "secret") && slices.Contains(call, "rm") && slices.Contains(call, "unrelated-secret") {
			t.Errorf("teardown removed unlabeled secret: %v", call)
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

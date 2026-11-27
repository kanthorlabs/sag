package harness

import (
	"slices"
	"testing"
)

func TestNetworkIP(t *testing.T) {
	inspect := []byte(`[{"NetworkSettings":{"Networks":{"other":{"IPAddress":"10.0.0.9"},"sag-e2e-x":{"IPAddress":"10.89.0.3"}}}}]`)
	got, err := networkIP(inspect, "sag-e2e-x")
	if err != nil || got != "10.89.0.3" {
		t.Fatalf("networkIP = %q, %v; want 10.89.0.3", got, err)
	}
	if _, err := networkIP(inspect, "missing"); err == nil {
		t.Fatal("networkIP on a missing network: want an error")
	}
	if _, err := networkIP([]byte(`[]`), "sag-e2e-x"); err == nil {
		t.Fatal("networkIP on an empty inspect: want an error")
	}
}

func TestRunArgsAddHost(t *testing.T) {
	args := runArgs(runSpec{
		Name:    "sag-e2e-x-sag",
		Network: "sag-e2e-x",
		Hosts:   []string{"sag-e2e-x-pg13:10.89.0.2", "sag-e2e-x-pg17:10.89.0.3"},
		Image:   "localhost/sag:e2e",
	})
	want := []string{"--network", "sag-e2e-x", "--add-host", "sag-e2e-x-pg13:10.89.0.2", "--add-host", "sag-e2e-x-pg17:10.89.0.3", "--log-driver"}
	if !slices.Equal(args[4:11], want) {
		t.Fatalf("args[4:11] = %q; want %q", args[4:11], want)
	}
}

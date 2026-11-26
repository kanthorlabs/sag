package tailscale

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/stepline"
)

type testCmd struct {
	responses map[string]func() ([]byte, error)
	recorded  []string
}

func (c *testCmd) Output(_ context.Context, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	c.recorded = append(c.recorded, key)
	if fn, ok := c.responses[key]; ok {
		return fn()
	}
	return nil, errors.New("unexpected call: " + key)
}

func TestAddressRange(t *testing.T) {
	cases := []struct {
		addr  string
		valid bool
	}{
		{"100.64.0.1", true},
		{"100.127.255.255", true},
		{"100.64.0.0", true},
		{"0.0.0.0", false},
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"100.128.0.0", false},
		{"", false},
		{"notanip", false},
	}
	for _, tc := range cases {
		got := ValidateCGNATAddress(tc.addr)
		if got != tc.valid {
			t.Errorf("ValidateCGNATAddress(%q) = %v; want %v", tc.addr, got, tc.valid)
		}
	}
}

func TestSkipReasons(t *testing.T) {
	t.Run("down when ip fails", func(t *testing.T) {
		cmd := &testCmd{responses: map[string]func() ([]byte, error){
			"ip -4": func() ([]byte, error) { return nil, errors.New("not connected") },
		}}
		state, err := Detect(context.Background(), cmd)
		if err != nil {
			t.Fatalf("Detect: %v", err)
		}
		if state.SkipReason != SkipDown {
			t.Fatalf("SkipReason = %q; want %q", state.SkipReason, SkipDown)
		}
	})

	t.Run("address invalid for non-CGNAT", func(t *testing.T) {
		cmd := &testCmd{responses: map[string]func() ([]byte, error){
			"ip -4": func() ([]byte, error) { return []byte("192.168.1.10\n"), nil },
		}}
		state, err := Detect(context.Background(), cmd)
		if err != nil {
			t.Fatalf("Detect: %v", err)
		}
		if state.SkipReason != SkipAddressInvalid {
			t.Fatalf("SkipReason = %q; want %q", state.SkipReason, SkipAddressInvalid)
		}
	})

	t.Run("address invalid for 0.0.0.0", func(t *testing.T) {
		cmd := &testCmd{responses: map[string]func() ([]byte, error){
			"ip -4": func() ([]byte, error) { return []byte("0.0.0.0\n"), nil },
		}}
		state, err := Detect(context.Background(), cmd)
		if err != nil {
			t.Fatalf("Detect: %v", err)
		}
		if state.SkipReason != SkipAddressInvalid {
			t.Fatalf("SkipReason = %q; want %q", state.SkipReason, SkipAddressInvalid)
		}
	})
}

func TestServeRuleMatch(t *testing.T) {
	cmd := &testCmd{responses: map[string]func() ([]byte, error){
		"ip -4": func() ([]byte, error) { return []byte("100.64.1.1\n"), nil },
		"serve status --json": func() ([]byte, error) {
			return []byte(`{"TCP":{"8443":{"TCPForward":"127.0.0.1:8443"}}}`), nil
		},
	}}
	state, err := Detect(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if state.SkipReason != "" {
		t.Fatalf("unexpected SkipReason %q", state.SkipReason)
	}
	if !state.HasSAGRule {
		t.Fatal("HasSAGRule = false; want true")
	}
	if state.OtherTarget {
		t.Fatal("OtherTarget = true; want false")
	}
}

func TestOtherTargetConflict(t *testing.T) {
	cases := []struct {
		name   string
		status string
	}{
		{"different target", `{"TCP":{"8443":{"TCPForward":"other:9090"}}}`},
		{"extra key", `{"TCP":{"8443":{"TCPForward":"127.0.0.1:8443","ExtraKey":"v"}}}`},
		{"unknown key", `{"TCP":{"8443":{"Unknown":"127.0.0.1:8443"}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statusJSON := tc.status
			cmd := &testCmd{responses: map[string]func() ([]byte, error){
				"ip -4": func() ([]byte, error) { return []byte("100.64.1.1\n"), nil },
				"serve status --json": func() ([]byte, error) {
					return []byte(statusJSON), nil
				},
			}}
			state, err := Detect(context.Background(), cmd)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if !state.OtherTarget {
				t.Fatalf("OtherTarget = false for %s; want true", tc.name)
			}
			if state.HasSAGRule {
				t.Fatal("HasSAGRule = true; want false")
			}
		})
	}
}

func TestFunnelConflict(t *testing.T) {
	cmd := &testCmd{responses: map[string]func() ([]byte, error){
		"ip -4": func() ([]byte, error) { return []byte("100.64.1.1\n"), nil },
		"serve status --json": func() ([]byte, error) {
			return []byte(`{"AllowFunnel":{"ts-node:8443":true}}`), nil
		},
	}}
	state, err := Detect(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !state.FunnelActive {
		t.Fatal("FunnelActive = false; want true")
	}
	if state.HasSAGRule {
		t.Fatal("HasSAGRule = true when funnel active; want false")
	}
}

func TestNoServeWhenRulePresent(t *testing.T) {
	var buf bytes.Buffer
	out := stepline.New(&buf)
	cmd := &testCmd{responses: map[string]func() ([]byte, error){}}

	state := State{HasSAGRule: true}
	if err := Apply(context.Background(), state, cmd, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	for _, call := range cmd.recorded {
		if strings.HasPrefix(call, "serve --bg") {
			t.Fatalf("Apply called serve when rule already present: %q", call)
		}
	}

	if !strings.Contains(buf.String(), "ok tailscale-serve") {
		t.Fatalf("Apply did not write ok line; got %q", buf.String())
	}
}

func TestNeverFunnel(t *testing.T) {
	var buf bytes.Buffer
	out := stepline.New(&buf)
	cmd := &testCmd{responses: map[string]func() ([]byte, error){
		"serve --bg --tcp 8443 tcp://127.0.0.1:8443": func() ([]byte, error) { return nil, nil },
	}}

	state := State{HasSAGRule: false}
	if err := Apply(context.Background(), state, cmd, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	for _, call := range cmd.recorded {
		if strings.Contains(call, "funnel") {
			t.Fatalf("Apply called funnel subcommand: %q", call)
		}
	}

	if !strings.Contains(buf.String(), "install tailscale-serve") {
		t.Fatalf("Apply did not write install line when rule absent; got %q", buf.String())
	}
}

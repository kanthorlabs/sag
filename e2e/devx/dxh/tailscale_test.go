package dxh_test

import (
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestNoAddressInOutput(t *testing.T) {
	ip := "100.64.1.42"

	t.Run("ip replaced with placeholder", func(t *testing.T) {
		out := dxh.MaskTailscaleIP("connect to "+ip+" failed", ip)
		if strings.Contains(out, ip) {
			t.Errorf("MaskTailscaleIP: IP still present: %q", out)
		}
		if !strings.Contains(out, "<tailscale IP>") {
			t.Errorf("MaskTailscaleIP: placeholder absent: %q", out)
		}
	})

	t.Run("non-ip content preserved", func(t *testing.T) {
		input := "bootstrap: exit 0\nok tailscale-serve\n"
		out := dxh.MaskTailscaleIP(input, ip)
		if out != input {
			t.Errorf("MaskTailscaleIP: content changed without IP: got %q; want %q", out, input)
		}
	})

	t.Run("multiple occurrences replaced", func(t *testing.T) {
		two := "curl https://" + ip + ":8443/healthz -> " + ip
		out := dxh.MaskTailscaleIP(two, ip)
		if strings.Contains(out, ip) {
			t.Errorf("MaskTailscaleIP: IP still present after multiple replacement: %q", out)
		}
		count := strings.Count(out, "<tailscale IP>")
		if count != 2 {
			t.Errorf("MaskTailscaleIP: placeholder count = %d; want 2: %q", count, out)
		}
	})

	t.Run("empty string unchanged", func(t *testing.T) {
		out := dxh.MaskTailscaleIP("", ip)
		if out != "" {
			t.Errorf("MaskTailscaleIP: empty string became %q", out)
		}
	})

	t.Run("other ip not masked", func(t *testing.T) {
		otherIP := "100.64.9.9"
		out := dxh.MaskTailscaleIP("addr "+otherIP, ip)
		if !strings.Contains(out, otherIP) {
			t.Errorf("MaskTailscaleIP: other IP %q was masked", otherIP)
		}
	})
}

func TestStatusCompare(t *testing.T) {
	empty := []byte(`{}`)
	withRule := []byte(`{"TCP":{"8443":{"TCPForward":"127.0.0.1:8443"}}}`)
	withOther := []byte(`{"TCP":{"9000":{"TCPForward":"127.0.0.1:9000"}}}`)
	withFunnel := []byte(`{"AllowFunnel":{"ts-node:8443":true}}`)

	t.Run("equal empty statuses", func(t *testing.T) {
		if err := dxh.EqualServeStatus(empty, empty); err != nil {
			t.Errorf("EqualServeStatus: empty vs empty: got error %v; want nil", err)
		}
	})

	t.Run("equal statuses with SAG rule", func(t *testing.T) {
		if err := dxh.EqualServeStatus(withRule, withRule); err != nil {
			t.Errorf("EqualServeStatus: rule vs rule: got error %v; want nil", err)
		}
	})

	t.Run("rule vs empty differs", func(t *testing.T) {
		if err := dxh.EqualServeStatus(withRule, empty); err == nil {
			t.Fatal("EqualServeStatus: rule vs empty: got nil; want error")
		}
	})

	t.Run("empty vs rule differs", func(t *testing.T) {
		if err := dxh.EqualServeStatus(empty, withRule); err == nil {
			t.Fatal("EqualServeStatus: empty vs rule: got nil; want error")
		}
	})

	t.Run("different ports differ", func(t *testing.T) {
		if err := dxh.EqualServeStatus(withRule, withOther); err == nil {
			t.Fatal("EqualServeStatus: 8443 vs 9000: got nil; want error")
		}
	})

	t.Run("equal other-port statuses", func(t *testing.T) {
		if err := dxh.EqualServeStatus(withOther, withOther); err != nil {
			t.Errorf("EqualServeStatus: other vs other: got error %v; want nil", err)
		}
	})

	t.Run("funnel vs empty differs", func(t *testing.T) {
		if err := dxh.EqualServeStatus(withFunnel, empty); err == nil {
			t.Fatal("EqualServeStatus: funnel vs empty: got nil; want error")
		}
	})

	t.Run("parse error on invalid JSON", func(t *testing.T) {
		if err := dxh.EqualServeStatus([]byte(`not-json`), empty); err == nil {
			t.Fatal("EqualServeStatus: invalid JSON first: got nil; want error")
		}
	})

	t.Run("parse error on invalid second JSON", func(t *testing.T) {
		if err := dxh.EqualServeStatus(empty, []byte(`not-json`)); err == nil {
			t.Fatal("EqualServeStatus: invalid JSON second: got nil; want error")
		}
	})
}

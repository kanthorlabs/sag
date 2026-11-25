package postgres

import (
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
)

func TestLoopbackIPLiteralExempt(t *testing.T) {
	for _, mode := range []core.TLSMode{core.TLSVerifyFull, ""} {
		for _, host := range []string{"127.0.0.1", "127.1.2.3", "::1"} {
			c, warning := checkTLS(mode, host, false)
			if !c.Passed || warning != "" || c.Name != checkTLSName {
				t.Errorf("checkTLS(%q, %q, false) = %+v, %q; want passed without warning", mode, host, c, warning)
			}
		}
	}
}

func TestLoopbackExemptOnlyUnderVerifyFull(t *testing.T) {
	for _, mode := range []core.TLSMode{core.TLSInsecureSkipVerify, core.TLSDisable} {
		c, warning := checkTLS(mode, "127.0.0.1", false)
		if !c.Passed || warning == "" {
			t.Errorf("checkTLS(%q, 127.0.0.1, false) = %+v, %q; want the opt-out warning", mode, c, warning)
		}
	}
}

func TestLocalhostNameNotExempt(t *testing.T) {
	for _, host := range []string{"localhost", "db.example.net", "128.0.0.1", "10.0.0.1", "::2", "127.0.0.1.nip.io"} {
		c, warning := checkTLS(core.TLSVerifyFull, host, false)
		if c.Passed || warning != "" {
			t.Errorf("checkTLS(verify-full, %q, false) = %+v, %q; want refused", host, c, warning)
		}
	}
}

func TestVerifyFullNeedsSessionSSL(t *testing.T) {
	c, warning := checkTLS(core.TLSVerifyFull, "db.example.net", true)
	if !c.Passed || warning != "" {
		t.Fatalf("checkTLS(verify-full, host, true) = %+v, %q; want passed without warning", c, warning)
	}
}

func TestOptOutWarnsWithAnySession(t *testing.T) {
	for _, mode := range []core.TLSMode{core.TLSInsecureSkipVerify, core.TLSDisable} {
		for _, ssl := range []bool{true, false} {
			c, warning := checkTLS(mode, "db.example.net", ssl)
			if !c.Passed || warning == "" {
				t.Errorf("checkTLS(%q, host, %v) = %+v, %q; want passed with a warning", mode, ssl, c, warning)
			}
		}
	}
}

func TestUnknownTLSModeRefused(t *testing.T) {
	c, warning := checkTLS(core.TLSMode("prefer"), "127.0.0.1", true)
	if c.Passed || warning != "" {
		t.Fatalf("checkTLS(prefer) = %+v, %q; want refused", c, warning)
	}
}

func TestOneWarningPerPreflight(t *testing.T) {
	for _, mode := range []core.TLSMode{core.TLSInsecureSkipVerify, core.TLSDisable} {
		f := healthyCatalog()
		f.answers[queryTLS] = [][]any{{mode == core.TLSInsecureSkipVerify}}
		report := runPreflight(t, testDialect(mode, nil), f)
		if !report.Passed || len(report.Warnings) != 1 {
			t.Errorf("mode %q: report = %+v; want passed with exactly one warning", mode, report)
		}
	}
	report := runPreflight(t, testDialect(core.TLSVerifyFull, nil), healthyCatalog())
	if len(report.Warnings) != 0 {
		t.Errorf("verify-full: warnings = %v; want none", report.Warnings)
	}
}

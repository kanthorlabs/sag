package dxh_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestVersionGate(t *testing.T) {
	ctx := context.Background()

	t.Run("correct version passes", func(t *testing.T) {
		r := newFake()
		r.set("claude --version", []byte("Claude Code 2.1.332\n"), nil)
		if err := dxh.VersionGate(ctx, r, "claude", "2.1.332"); err != nil {
			t.Errorf("VersionGate: correct version: got error %v; want nil", err)
		}
	})

	t.Run("wrong version prerequisite missing", func(t *testing.T) {
		r := newFake()
		r.set("claude --version", []byte("Claude Code 2.0.0\n"), nil)
		err := dxh.VersionGate(ctx, r, "claude", "2.1.332")
		if err == nil {
			t.Fatal("VersionGate: wrong version: got nil; want error")
		}
		if !strings.Contains(err.Error(), "prerequisite missing") {
			t.Errorf("VersionGate: error = %q; want 'prerequisite missing'", err.Error())
		}
	})

	t.Run("command error prerequisite missing", func(t *testing.T) {
		r := newFake()
		r.set("pi --version", nil, errors.New("not found"))
		err := dxh.VersionGate(ctx, r, "pi", "1.6.1")
		if err == nil {
			t.Fatal("VersionGate: command error: got nil; want error")
		}
		if !strings.Contains(err.Error(), "prerequisite missing") {
			t.Errorf("VersionGate: error = %q; want 'prerequisite missing'", err.Error())
		}
	})

	t.Run("pi correct version passes", func(t *testing.T) {
		r := newFake()
		r.set("pi --version", []byte("pi 1.6.1\n"), nil)
		if err := dxh.VersionGate(ctx, r, "pi", "1.6.1"); err != nil {
			t.Errorf("VersionGate: correct pi version: got error %v; want nil", err)
		}
	})

	t.Run("longer version suffix must not pass", func(t *testing.T) {
		r := newFake()
		r.set("claude --version", []byte("Claude Code 2.1.3320\n"), nil)
		err := dxh.VersionGate(ctx, r, "claude", "2.1.332")
		if err == nil {
			t.Fatal("VersionGate: version suffix 2.1.3320: got nil; want error")
		}
		if !strings.Contains(err.Error(), "prerequisite missing") {
			t.Errorf("VersionGate: error = %q; want 'prerequisite missing'", err.Error())
		}
	})

	t.Run("longer version prefix must not pass", func(t *testing.T) {
		r := newFake()
		r.set("claude --version", []byte("Claude Code 12.1.332\n"), nil)
		err := dxh.VersionGate(ctx, r, "claude", "2.1.332")
		if err == nil {
			t.Fatal("VersionGate: version prefix 12.1.332: got nil; want error")
		}
		if !strings.Contains(err.Error(), "prerequisite missing") {
			t.Errorf("VersionGate: error = %q; want 'prerequisite missing'", err.Error())
		}
	})
}

func TestOutputHasNoJWT(t *testing.T) {
	jwt := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJjbGF1ZGUtY29kZSIsIm5hbWUiOiJjbGF1ZGUtY29kZSJ9.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

	t.Run("jwt removed", func(t *testing.T) {
		out := dxh.FilterNoJWT("some output " + jwt + " more output")
		if strings.Contains(out, jwt) {
			t.Errorf("FilterNoJWT: JWT still present in output: %q", out)
		}
		if !strings.Contains(out, "some output") || !strings.Contains(out, "more output") {
			t.Errorf("FilterNoJWT: non-JWT content removed: %q", out)
		}
	})

	t.Run("non-jwt content preserved", func(t *testing.T) {
		input := "sag: connected\nresource: x\nok agent claude-code\n"
		out := dxh.FilterNoJWT(input)
		if out != input {
			t.Errorf("FilterNoJWT: non-JWT content changed: got %q; want %q", out, input)
		}
	})

	t.Run("jwt replaced with placeholder", func(t *testing.T) {
		out := dxh.FilterNoJWT(jwt)
		if out != "<JWT>" {
			t.Errorf("FilterNoJWT: JWT not replaced with <JWT>: got %q", out)
		}
	})

	t.Run("multiple jwts removed", func(t *testing.T) {
		two := jwt + " and " + jwt
		out := dxh.FilterNoJWT(two)
		if strings.Contains(out, jwt) {
			t.Errorf("FilterNoJWT: JWT still present after double replacement: %q", out)
		}
	})
}

func TestParseClaudeMCPListSAG(t *testing.T) {
	t.Run("connected line passes", func(t *testing.T) {
		output := "other: https://other.example.com (HTTP) - ✓ Connected\nsag: https://sag.example.com (HTTP) - ✓ Connected\n"
		if err := dxh.ParseClaudeMCPListSAG(output); err != nil {
			t.Errorf("ParseClaudeMCPListSAG: connected: got error %v; want nil", err)
		}
	})

	t.Run("failed to connect returns error", func(t *testing.T) {
		output := "sag: https://sag.example.com (HTTP) - ✗ Failed to connect\n"
		if err := dxh.ParseClaudeMCPListSAG(output); err == nil {
			t.Fatal("ParseClaudeMCPListSAG: failed: got nil; want error")
		}
	})

	t.Run("needs authentication returns error", func(t *testing.T) {
		output := "sag: https://sag.example.com (HTTP) - ✗ Needs authentication\n"
		if err := dxh.ParseClaudeMCPListSAG(output); err == nil {
			t.Fatal("ParseClaudeMCPListSAG: auth: got nil; want error")
		}
	})

	t.Run("absent sag returns error", func(t *testing.T) {
		output := "other: https://other.example.com (HTTP) - ✓ Connected\n"
		if err := dxh.ParseClaudeMCPListSAG(output); err == nil {
			t.Fatal("ParseClaudeMCPListSAG: absent: got nil; want error")
		}
	})

	t.Run("sag-other prefix does not match", func(t *testing.T) {
		output := "sag-other: https://sag.example.com (HTTP) - ✓ Connected\n"
		if err := dxh.ParseClaudeMCPListSAG(output); err == nil {
			t.Fatal("ParseClaudeMCPListSAG: sag-other: got nil; want error")
		}
	})
}

func TestParseClaudePermissionDenials(t *testing.T) {
	t.Run("correct entry passes", func(t *testing.T) {
		output := `{"is_error":false,"result":"denied","permission_denials":[{"tool_name":"Bash","tool_input":{"command":"sag credential delete x"}}]}`
		if err := dxh.ParseClaudePermissionDenials(output, "sag credential delete x"); err != nil {
			t.Errorf("ParseClaudePermissionDenials: correct: got error %v; want nil", err)
		}
	})

	t.Run("empty list returns error", func(t *testing.T) {
		output := `{"is_error":false,"result":"ok","permission_denials":[]}`
		if err := dxh.ParseClaudePermissionDenials(output, "sag credential delete x"); err == nil {
			t.Fatal("ParseClaudePermissionDenials: empty: got nil; want error")
		}
	})

	t.Run("wrong command returns error", func(t *testing.T) {
		output := `{"is_error":false,"result":"denied","permission_denials":[{"tool_name":"Bash","tool_input":{"command":"echo hello"}}]}`
		if err := dxh.ParseClaudePermissionDenials(output, "sag credential delete x"); err == nil {
			t.Fatal("ParseClaudePermissionDenials: wrong command: got nil; want error")
		}
	})

	t.Run("wrong tool name returns error", func(t *testing.T) {
		output := `{"is_error":false,"result":"denied","permission_denials":[{"tool_name":"Read","tool_input":{"command":"sag credential delete x"}}]}`
		if err := dxh.ParseClaudePermissionDenials(output, "sag credential delete x"); err == nil {
			t.Fatal("ParseClaudePermissionDenials: wrong tool: got nil; want error")
		}
	})
}

func TestParsePiMCPListSAG(t *testing.T) {
	t.Run("connected server passes", func(t *testing.T) {
		output := `{"servers":[{"name":"sag","state":"connected","url":"https://sag.example.com"}],"errors":[]}`
		if err := dxh.ParsePiMCPListSAG(output); err != nil {
			t.Errorf("ParsePiMCPListSAG: connected: got error %v; want nil", err)
		}
	})

	t.Run("disabled server returns error", func(t *testing.T) {
		output := `{"servers":[{"name":"sag","state":"disabled"}],"errors":[]}`
		if err := dxh.ParsePiMCPListSAG(output); err == nil {
			t.Fatal("ParsePiMCPListSAG: disabled: got nil; want error")
		}
	})

	t.Run("missing sag server returns error", func(t *testing.T) {
		output := `{"servers":[{"name":"other","state":"connected"}],"errors":[]}`
		if err := dxh.ParsePiMCPListSAG(output); err == nil {
			t.Fatal("ParsePiMCPListSAG: absent: got nil; want error")
		}
	})

	t.Run("empty servers returns error", func(t *testing.T) {
		output := `{"servers":[],"errors":[]}`
		if err := dxh.ParsePiMCPListSAG(output); err == nil {
			t.Fatal("ParsePiMCPListSAG: empty: got nil; want error")
		}
	})
}

func TestParsePiGuardResult(t *testing.T) {
	t.Run("correct event passes", func(t *testing.T) {
		output := `{"type":"session_start"}
{"type":"tool_execution_end","toolName":"bash","isError":true,"result":{"content":[{"type":"text","text":"sag-guard: podman secret rm blocked"}],"details":{}}}
{"type":"session_end"}`
		if err := dxh.ParsePiGuardResult(output); err != nil {
			t.Errorf("ParsePiGuardResult: correct: got error %v; want nil", err)
		}
	})

	t.Run("prompt echo text only returns error", func(t *testing.T) {
		output := "Run this bash command: podman secret rm x\nsag-guard: podman secret rm blocked"
		if err := dxh.ParsePiGuardResult(output); err == nil {
			t.Fatal("ParsePiGuardResult: plain text: got nil; want error")
		}
	})

	t.Run("no tool_execution_end returns error", func(t *testing.T) {
		output := `{"type":"session_start"}
{"type":"message_update","usage":{"input":10,"output":5}}
{"type":"session_end"}`
		if err := dxh.ParsePiGuardResult(output); err == nil {
			t.Fatal("ParsePiGuardResult: no event: got nil; want error")
		}
	})

	t.Run("isError false returns error", func(t *testing.T) {
		output := `{"type":"tool_execution_end","toolName":"bash","isError":false,"result":{"content":[{"type":"text","text":"sag-guard: podman secret rm blocked"}],"details":{}}}`
		if err := dxh.ParsePiGuardResult(output); err == nil {
			t.Fatal("ParsePiGuardResult: isError false: got nil; want error")
		}
	})

	t.Run("text without sag-guard prefix returns error", func(t *testing.T) {
		output := `{"type":"tool_execution_end","toolName":"bash","isError":true,"result":{"content":[{"type":"text","text":"command failed"}],"details":{}}}`
		if err := dxh.ParsePiGuardResult(output); err == nil {
			t.Fatal("ParsePiGuardResult: no sag-guard prefix: got nil; want error")
		}
	})
}

func TestParsePiGuardResultNamesModelError(t *testing.T) {
	out := `{"type":"agent_start"}
{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"model is not supported"}}
{"type":"agent_end"}
`
	err := dxh.ParsePiGuardResult(out)
	if err == nil {
		t.Fatal("ParsePiGuardResult: want error for a model error, got nil")
	}
	if !strings.Contains(err.Error(), "pi model error: model is not supported") {
		t.Errorf("ParsePiGuardResult: error %q does not name the model error", err)
	}
}

func TestParsePiGuardResultLongLine(t *testing.T) {
	long := `{"type":"message_start","message":{"role":"system","content":"` + strings.Repeat("x", 200*1024) + `"}}`
	out := long + "\n" + `{"type":"tool_execution_end","toolName":"bash","isError":true,"result":{"content":[{"type":"text","text":"sag-guard: blocked"}]}}` + "\n"
	if err := dxh.ParsePiGuardResult(out); err != nil {
		t.Errorf("ParsePiGuardResult after a 200 KiB line: %v", err)
	}
}

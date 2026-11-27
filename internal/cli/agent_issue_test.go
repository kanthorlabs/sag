package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/token"
)

func TestTerminalStdoutExit3(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        errOut,
		IsTerminal: func(any) bool { return true },
		Mode:       ModeContainer,
	}
	code := agentIssue([]string{"test-sub"}, std)
	if code != ExitRefusal {
		t.Errorf("exit = %d; want %d (ExitRefusal)", code, ExitRefusal)
	}
	if strings.Contains(out.String(), "eyJ") {
		t.Error("stdout contains a JWT; want none")
	}
	if strings.Contains(errOut.String(), "eyJ") {
		t.Error("stderr contains a JWT; want none")
	}
}

func TestUnlistedSubIssued(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	const sub = "unlisted-sub"
	const lifetime = 24 * time.Hour

	orig := loadAgentIssuer
	loadAgentIssuer = func() (jwtIssuer, error) {
		return jwtIssuer{
			Lifetime: lifetime,
			Known:    func(string) bool { return false },
			Key:      func() ([]byte, error) { return key, nil },
		}, nil
	}
	defer func() { loadAgentIssuer = orig }()

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	std := IO{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        errOut,
		IsTerminal: func(any) bool { return false },
		Mode:       ModeContainer,
	}
	code := agentIssue([]string{sub}, std)
	if code != ExitOK {
		t.Fatalf("exit = %d; want 0; stderr: %s", code, errOut.String())
	}
	raw := strings.TrimSpace(out.String())
	if raw == "" {
		t.Fatal("no output")
	}
	claims, err := token.Verify(key, raw, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Sub != sub {
		t.Errorf("sub = %q; want %q", claims.Sub, sub)
	}
	if strings.Contains(errOut.String(), "eyJ") {
		t.Error("stderr contains a JWT; want none")
	}
}

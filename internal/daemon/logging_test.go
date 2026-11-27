package daemon

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
)

const (
	logTestPassword = "hunter2-log-pw"
	logTestDSN      = "postgres://sag:" + logTestPassword + "@db:5432/app"
)

type logValuer string

func (v logValuer) LogValue() slog.Value {
	return slog.StringValue(string(v))
}

type hiddenDSN struct {
	DSN string
}

func (hiddenDSN) String() string {
	return "hidden"
}

type stringer string

func (v stringer) String() string {
	return string(v)
}

func TestLogHandlerScrubs(t *testing.T) {
	var buf bytes.Buffer
	s := scrub.New([]secret.Credential{secret.NewCredential("conn-main", []byte(logTestDSN))})
	log := slog.New(scrubHandler(slog.NewJSONHandler(&buf, nil), s))

	log = log.With("dsn", logTestDSN).WithGroup("g")
	log.Error("open "+logTestDSN,
		"error", errors.New("connect: "+logTestDSN),
		"cause", fmt.Errorf("auth: %w", errors.New("password "+logTestPassword)),
		"plain", "pw="+logTestPassword,
		slog.Group("inner", "value", logTestPassword, "deep", slog.Group("x", "y", logTestDSN)),
		"valuer", logValuer(logTestPassword),
		"stringer", stringer(logTestPassword),
		"list", []string{"ok", logTestPassword},
		"hidden", hiddenDSN{DSN: logTestDSN},
		"count", 3,
	)

	out := buf.String()
	if strings.Contains(out, logTestPassword) {
		t.Fatalf("log line holds the password: %s", out)
	}
	for _, want := range []string{`"count":3`, `"msg":"open [REDACTED]"`, `"dsn":"[REDACTED]"`, `"g":{`, `"error":"connect: [REDACTED]"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("log line %s does not hold %s", out, want)
		}
	}
}

func TestLogHandlerKeepsCleanAttrs(t *testing.T) {
	var buf bytes.Buffer
	s := scrub.New([]secret.Credential{secret.NewCredential("conn-main", []byte(logTestDSN))})
	slog.New(scrubHandler(slog.NewJSONHandler(&buf, nil), s)).Info("served", "checks", []string{"a", "b"}, "n", 1)
	if out := buf.String(); !strings.Contains(out, `"checks":["a","b"]`) || !strings.Contains(out, `"n":1`) {
		t.Fatalf("log line = %s; want clean attrs kept", out)
	}
}

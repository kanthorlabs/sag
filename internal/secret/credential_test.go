package secret

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const testValue = "postgres://app:hunter2-secret@db:5432/app"

type holder struct {
	cred  Credential
	ptr   *Credential
	Cred  Credential
	Ptr   *Credential
	creds map[string]Credential
	Creds map[string]Credential
}

func assertRedacted(t *testing.T, label, out string) {
	t.Helper()
	for _, part := range []string{testValue, "hunter2", fmt.Sprintf("%x", testValue), strings.Trim(fmt.Sprint([]byte(testValue)[:8]), "[]")} {
		if strings.Contains(out, part) {
			t.Fatalf("%s leaks the value: %q", label, out)
		}
	}
}

func TestCredentialRedacted(t *testing.T) {
	c := NewCredential("sag-main", []byte(testValue))

	if got := c.String(); got != "[REDACTED]" {
		t.Fatalf("String = %q", got)
	}
	if got := c.GoString(); got != "[REDACTED]" {
		t.Fatalf("GoString = %q", got)
	}
	js, err := c.MarshalJSON()
	if err != nil || string(js) != `"[REDACTED]"` {
		t.Fatalf("MarshalJSON = %s, %v", js, err)
	}
	if got := c.LogValue(); got.Kind() != slog.KindString || got.String() != "[REDACTED]" {
		t.Fatalf("LogValue = %v", got)
	}

	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%T %v"} {
		assertRedacted(t, verb, fmt.Sprintf(verb, c))
		assertRedacted(t, "pointer "+verb, fmt.Sprintf(verb, &c))
	}

	h := holder{
		cred:  c,
		ptr:   &c,
		Cred:  c,
		Ptr:   &c,
		creds: map[string]Credential{"a": c},
		Creds: map[string]Credential{"a": c},
	}
	for _, verb := range []string{"%v", "%+v", "%#v", "%d", "%x"} {
		assertRedacted(t, "nested "+verb, fmt.Sprintf(verb, h))
	}

	out, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	assertRedacted(t, "json", string(out))

	var buf bytes.Buffer
	for _, handler := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(handler).Info("load", "cred", c, "ptr", &c, "holder", h, "creds", h.Creds)
	}
	assertRedacted(t, "slog", buf.String())
}

func TestCredentialReveal(t *testing.T) {
	c := NewCredential("sag-main", []byte(testValue))
	if c.Name() != "sag-main" {
		t.Fatalf("Name = %q", c.Name())
	}
	if string(c.Reveal()) != testValue {
		t.Fatal("Reveal does not return the value")
	}
	if (Credential{}).Reveal() != nil {
		t.Fatal("zero Credential reveals a value")
	}
}

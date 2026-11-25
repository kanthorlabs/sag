package scrub

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

const (
	testPassword = `p@ss w'rd/%"x\y`
	testKVPass   = `kv s3cr'et`
	testToken    = "tok-0123456789abcdef"
)

func testDSN() string {
	u := url.URL{Scheme: "postgres", User: url.UserPassword("sag", testPassword), Host: "db:5432", Path: "/app"}
	return u.String()
}

func testKVDSN() string {
	return `host=db user=sag password='kv s3cr\'et' dbname=app`
}

func testScrubber() Scrubber {
	return New([]secret.Credential{
		secret.NewCredential("conn-main", []byte(testDSN()+"\n")),
		secret.NewCredential("conn-kv", []byte(testKVDSN())),
		secret.NewCredential("token", []byte(testToken)),
	})
}

func assertClean(t *testing.T, text string) {
	t.Helper()
	for _, needle := range []string{
		testPassword,
		testDSN(),
		url.QueryEscape(testPassword),
		url.PathEscape(testPassword),
		strings.Trim(strconv.Quote(testPassword), `"`),
		testKVPass,
		testKVDSN(),
		`kv s3cr\'et`,
		testToken,
	} {
		if strings.Contains(text, needle) {
			t.Fatalf("text %q holds secret form %q", text, needle)
		}
	}
}

func TestRemovesSecretAndPassword(t *testing.T) {
	s := testScrubber()
	u, err := url.Parse(testDSN())
	if err != nil {
		t.Fatal(err)
	}
	rawPassword := strings.SplitN(u.User.String(), ":", 2)[1]
	cases := []string{
		"dial " + testDSN() + " failed",
		"password=" + testPassword + " rejected",
		"userinfo " + rawPassword,
		"query " + url.QueryEscape(testPassword),
		"path " + url.PathEscape(testPassword),
		fmt.Sprintf("quoted %q", testPassword),
		"kv " + testKVDSN(),
		"kv value " + testKVPass,
		`kv quoted 'kv s3cr\'et'`,
		"token " + testToken + " end",
	}
	for _, text := range cases {
		got := s.String(text)
		assertClean(t, got)
		if !strings.Contains(got, redacted) {
			t.Fatalf("String(%q) = %q; want %s", text, got, redacted)
		}
	}
	if got := s.String("user sag on host db"); got != "user sag on host db" {
		t.Fatalf("String changed clean text: %q", got)
	}
}

func TestZeroScrubberPassesText(t *testing.T) {
	var s Scrubber
	if got := s.String("text"); got != "text" {
		t.Fatalf("String = %q; want text", got)
	}
	err := errors.New("cause")
	if got := s.Error(err); got == nil || got.Error() != "cause" {
		t.Fatalf("Error = %v; want cause", got)
	}
}

func TestNoDSNInError(t *testing.T) {
	s := testScrubber()
	if s.Error(nil) != nil {
		t.Fatal("Error(nil) != nil")
	}

	trace := core.Trace{Input: "SELECT 1"}
	driver := fmt.Errorf("connect %s: %w", testDSN(), errors.New("password authentication failed for "+testPassword))
	inner := &core.Error{Code: core.CodeResourceUnavailable, Message: core.PreflightConnectFailed, Cause: driver}
	outer := &core.Error{Code: core.CodeResourceUnavailable, Message: "resource unavailable " + testToken, Trace: trace, Cause: inner}

	got := s.Error(fmt.Errorf("wrapped: %w", outer))
	var ce *core.Error
	if !errors.As(got, &ce) {
		t.Fatalf("Error = %T; want *core.Error", got)
	}
	if ce.Code != core.CodeResourceUnavailable || ce.Trace != trace {
		t.Fatalf("Error = %+v; want same code and trace", ce)
	}
	if ce.Message != outer.Message {
		t.Fatalf("Message = %q; want the agent message unchanged", ce.Message)
	}
	if ce.Cause == nil {
		t.Fatal("Cause = nil")
	}
	cause := ce.Cause.Error()
	assertClean(t, cause)
	for _, want := range []string{core.PreflightConnectFailed, "password authentication failed"} {
		if !strings.Contains(cause, want) {
			t.Fatalf("Cause %q does not hold %q", cause, want)
		}
	}

	plain := s.Error(errors.New("open " + testKVDSN()))
	assertClean(t, plain.Error())
	if errors.As(plain, &ce) {
		t.Fatal("plain error became *core.Error")
	}

	noCause := s.Error(&core.Error{Code: core.CodeTimeout, Message: "request timed out"})
	if !errors.As(noCause, &ce) || ce.Cause != nil {
		t.Fatalf("Error = %+v; want nil Cause", noCause)
	}
}

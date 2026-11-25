package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

type preflightResource struct {
	report core.PreflightReport
	err    error
	closed bool
}

func (r *preflightResource) Preflight(context.Context) (core.PreflightReport, error) {
	return r.report, r.err
}

func (r *preflightResource) Execute(context.Context, string, string, json.RawMessage, core.Limits) (core.Result, error) {
	return core.Result{}, errors.ErrUnsupported
}

func (r *preflightResource) Close() error {
	r.closed = true
	return nil
}

type preflightAdapter struct {
	res     *preflightResource
	openErr error
}

func (a *preflightAdapter) Kind() string                    { return "fake" }
func (a *preflightAdapter) Capabilities() []core.Capability { return nil }
func (a *preflightAdapter) Open(context.Context, core.ResourceConfig, secret.Credential) (core.Resource, error) {
	if a.openErr != nil {
		return nil, a.openErr
	}
	return a.res, nil
}

func withPreflightAdapter(t *testing.T, a *preflightAdapter) {
	t.Helper()
	saved := loadResource
	t.Cleanup(func() { loadResource = saved })
	loadResource = func(name string) (resourceTarget, error) {
		if name != "main" {
			return resourceTarget{}, errors.New("resource is not in the config")
		}
		return resourceTarget{Adapter: a, Config: core.ResourceConfig{Name: name}}, nil
	}
}

func runResourceTest(args ...string) (int, string, string) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	std := IO{In: strings.NewReader(""), Out: out, Err: errOut, IsTerminal: func(any) bool { return false }}
	code := Run(append([]string{"resource", "test"}, args...), std)
	return code, out.String(), errOut.String()
}

func resultLines(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "result: ") {
			n++
		}
	}
	return n
}

func TestResourceTestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"a", "b"}} {
		if code, _, errOut := runResourceTest(args...); code != ExitUsage || !strings.Contains(errOut, "usage") {
			t.Fatalf("args %v: exit = %d, stderr %q; want %d with usage", args, code, errOut, ExitUsage)
		}
	}
}

func TestResourceTestPassedPrintsReport(t *testing.T) {
	res := &preflightResource{report: core.PreflightReport{
		Passed: true,
		Checks: []core.PreflightCheck{
			{Name: "engine", Passed: true, Detail: "PostgreSQL 17.12"},
			{Name: "operator_names", Passed: true, Detail: "<> = ~~*"},
		},
		Warnings: []string{"tls insecure-skip-verify: not verified"},
	}}
	withPreflightAdapter(t, &preflightAdapter{res: res})
	code, out, errOut := runResourceTest("main")
	if code != ExitOK {
		t.Fatalf("exit = %d; want %d; stdout %q stderr %q", code, ExitOK, out, errOut)
	}
	for _, want := range []string{"check engine: pass: PostgreSQL 17.12", "check operator_names: pass: <> = ~~*", "warning: tls insecure-skip-verify: not verified", "result: resource main passed preflight"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q; want %q", out, want)
		}
	}
	if resultLines(out) != 1 || !res.closed {
		t.Fatalf("stdout = %q, closed %v; want exactly one result line and a closed resource", out, res.closed)
	}
}

func TestResourceTestRefusedPrintsHint(t *testing.T) {
	hint := "resource main has risky objects dblink; to approve, run on the host: podman exec -it sag sag extension approve main"
	res := &preflightResource{report: core.PreflightReport{
		Checks: []core.PreflightCheck{{Name: "risky_objects", Passed: false, Detail: "not approved: dblink"}},
		Hint:   hint,
	}}
	withPreflightAdapter(t, &preflightAdapter{res: res})
	code, out, _ := runResourceTest("main")
	if code != ExitFailure {
		t.Fatalf("exit = %d; want %d", code, ExitFailure)
	}
	for _, want := range []string{"check risky_objects: FAIL: not approved: dblink", "hint: " + hint, "result: resource main refused by preflight"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q; want %q", out, want)
		}
	}
	if resultLines(out) != 1 {
		t.Fatalf("stdout = %q; want exactly one result line", out)
	}
}

func TestResourceTestErrorOmitsDriverText(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{errors.New("dial postgres://app:hunter2@db/app failed"), "result: resource main preflight failed: preflight failed"},
		{&core.Error{Code: core.CodeResourceUnavailable, Message: "preflight TLS handshake failed", Cause: errors.New("hunter2")}, "result: resource main preflight failed: resource_unavailable: preflight TLS handshake failed"},
	}
	for _, tc := range cases {
		res := &preflightResource{err: tc.err}
		withPreflightAdapter(t, &preflightAdapter{res: res})
		code, out, errOut := runResourceTest("main")
		if code != ExitFailure || !strings.Contains(out, tc.want) || resultLines(out) != 1 {
			t.Fatalf("error %v: exit %d, stdout %q; want %d and %q once", tc.err, code, out, ExitFailure, tc.want)
		}
		if strings.Contains(out+errOut, "hunter2") {
			t.Fatalf("error %v: output %q holds driver text", tc.err, out+errOut)
		}
		if !res.closed {
			t.Fatalf("error %v: resource not closed", tc.err)
		}
	}
}

func TestResourceTestOpenError(t *testing.T) {
	withPreflightAdapter(t, &preflightAdapter{openErr: errors.New("connection secret has query parameters or a fragment")})
	code, out, _ := runResourceTest("main")
	if code != ExitFailure || resultLines(out) != 1 || !strings.Contains(out, "query parameters") {
		t.Fatalf("exit %d, stdout %q; want %d with one result line naming the open error", code, out, ExitFailure)
	}
}

func TestResourceTestUnknownResource(t *testing.T) {
	withPreflightAdapter(t, &preflightAdapter{})
	if code, _, errOut := runResourceTest("other"); code != ExitFailure || !strings.Contains(errOut, "not in the config") {
		t.Fatalf("exit %d, stderr %q; want %d with the load error", code, errOut, ExitFailure)
	}
}

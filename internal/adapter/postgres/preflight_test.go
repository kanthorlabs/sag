package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

type catalogRows struct {
	values [][]any
	index  int
}

func (r *catalogRows) Columns() []core.Column { return nil }

func (r *catalogRows) Next() bool {
	r.index++
	return r.index <= len(r.values)
}

func (r *catalogRows) Values() ([]any, error) { return r.values[r.index-1], nil }
func (r *catalogRows) Err() error             { return nil }
func (r *catalogRows) Abort()                 {}
func (r *catalogRows) Close()                 {}

type catalogFake struct {
	answers map[string][][]any
	failOn  string
	queries []string
}

func (f *catalogFake) Query(ctx context.Context, text string, args ...any) (sql.Rows, error) {
	f.queries = append(f.queries, text)
	if text == f.failOn {
		return nil, errors.New("driver text with password=hunter2")
	}
	values, ok := f.answers[text]
	if !ok {
		return nil, fmt.Errorf("unexpected query %q", text)
	}
	return &catalogRows{values: values}, nil
}

func healthyCatalog() *catalogFake {
	return &catalogFake{answers: map[string][][]any{
		querySearchPath:         {{"pg_catalog"}},
		queryServer:             {{"PostgreSQL 17.12 on aarch64-unknown-linux-gnu", "170012"}},
		queryTLS:                {{true}},
		queryReadOnly:           {{"on", "on", "UTF8"}},
		queryRiskyExtensions:    nil,
		queryUntrustedLanguages: nil,
		queryOperators:          nil,
	}}
}

type approvalsFake struct {
	resource    string
	fingerprint string
	err         error
}

func (a *approvalsFake) Verify(resource, fingerprint string) error {
	a.resource = resource
	a.fingerprint = fingerprint
	return a.err
}

var testTarget = approval.Target{Resource: "main", Host: "pg.example.net", Port: 5432, Database: "app", User: "app"}

func testDialect(mode core.TLSMode, approvals core.ApprovalVerifier) *dialect {
	return newDialect(core.ResourceConfig{Name: "main", Kind: Kind, TLS: mode, Approvals: approvals}, testTarget)
}

func runPreflight(t *testing.T, d *dialect, r sql.CatalogReader) core.PreflightReport {
	t.Helper()
	report, err := d.Preflight(context.Background(), r)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	return report
}

func reportCheck(t *testing.T, report core.PreflightReport, name string) core.PreflightCheck {
	t.Helper()
	for _, c := range report.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("report has no check %q: %+v", name, report.Checks)
	return core.PreflightCheck{}
}

func TestPreflightPassesHealthyServer(t *testing.T) {
	report := runPreflight(t, testDialect(core.TLSVerifyFull, nil), healthyCatalog())
	if !report.Passed || len(report.Warnings) != 0 || report.Hint != "" {
		t.Fatalf("report = %+v; want passed without warnings and hint", report)
	}
	var names []string
	for _, c := range report.Checks {
		names = append(names, c.Name)
		if !c.Passed {
			t.Fatalf("check %+v failed", c)
		}
	}
	want := []string{checkEngine, checkVersion, checkTLSName, checkReadOnlyMode, checkRiskyObjects, checkOperatorNames}
	if !slices.Equal(names, want) {
		t.Fatalf("checks = %v; want %v", names, want)
	}
}

func TestPreflightRefusesOtherEngine(t *testing.T) {
	f := healthyCatalog()
	f.answers[queryServer] = [][]any{{"CockroachDB CCL v23.1", "130000"}}
	report := runPreflight(t, testDialect(core.TLSVerifyFull, nil), f)
	if report.Passed || reportCheck(t, report, checkEngine).Passed {
		t.Fatalf("report = %+v; want engine refused", report)
	}
	if !slices.Equal(f.queries, []string{querySearchPath, queryServer}) {
		t.Fatalf("queries = %v; want only the search_path and server queries after an engine refusal", f.queries)
	}
}

func TestPreflightPinsSearchPathFirst(t *testing.T) {
	f := healthyCatalog()
	runPreflight(t, testDialect(core.TLSVerifyFull, nil), f)
	if len(f.queries) == 0 || f.queries[0] != querySearchPath {
		t.Fatalf("queries = %v; want %q first", f.queries, querySearchPath)
	}
}

func TestPreflightVersionRange(t *testing.T) {
	cases := map[string]bool{
		"130020": false,
		"139999": false,
		"140000": true,
		"170012": true,
		"199999": true,
		"200000": false,
		"abc":    false,
		"":       false,
	}
	for num, want := range cases {
		f := healthyCatalog()
		f.answers[queryServer] = [][]any{{"PostgreSQL x", num}}
		report := runPreflight(t, testDialect(core.TLSVerifyFull, nil), f)
		if got := reportCheck(t, report, checkVersion).Passed; got != want || report.Passed != want {
			t.Errorf("server_version_num %q: version passed %v, report passed %v; want %v", num, got, report.Passed, want)
		}
	}
}

func TestPreflightRefusesTLSInactive(t *testing.T) {
	f := healthyCatalog()
	f.answers[queryTLS] = [][]any{{false}}
	report := runPreflight(t, testDialect(core.TLSVerifyFull, nil), f)
	if report.Passed || reportCheck(t, report, checkTLSName).Passed {
		t.Fatalf("report = %+v; want tls refused", report)
	}
}

func TestPreflightRefusesMissingSSLRow(t *testing.T) {
	f := healthyCatalog()
	f.answers[queryTLS] = nil
	report := runPreflight(t, testDialect(core.TLSVerifyFull, nil), f)
	if report.Passed || reportCheck(t, report, checkTLSName).Passed {
		t.Fatalf("report = %+v; want tls refused without a pg_stat_ssl row", report)
	}
}

func TestPreflightReadOnlyModeValues(t *testing.T) {
	bad := [][]any{
		{"off", "on", "UTF8"},
		{"on", "off", "UTF8"},
		{"on", "on", "SQL_ASCII"},
	}
	for _, row := range bad {
		f := healthyCatalog()
		f.answers[queryReadOnly] = [][]any{row}
		report := runPreflight(t, testDialect(core.TLSVerifyFull, nil), f)
		if report.Passed || reportCheck(t, report, checkReadOnlyMode).Passed {
			t.Errorf("settings %v: report = %+v; want read_only_mode refused", row, report)
		}
	}
}

func riskyCatalog() *catalogFake {
	f := healthyCatalog()
	f.answers[queryRiskyExtensions] = [][]any{{"file_fdw", "1.0", "public"}, {"dblink", "1.2", "public"}}
	f.answers[queryUntrustedLanguages] = [][]any{{"plperlu"}}
	return f
}

func TestPreflightRiskyWithoutApprovalsRefuses(t *testing.T) {
	report := runPreflight(t, testDialect(core.TLSVerifyFull, nil), riskyCatalog())
	if report.Passed || reportCheck(t, report, checkRiskyObjects).Passed {
		t.Fatalf("report = %+v; want risky_objects refused with nil Approvals", report)
	}
	want := "resource main has risky objects dblink, file_fdw, plperlu; to approve, run on the host: podman exec -it sag sag extension approve main"
	if report.Hint != want {
		t.Fatalf("hint = %q; want %q", report.Hint, want)
	}
}

func TestRiskyExtensionList(t *testing.T) {
	for _, name := range []string{"dblink", "pg_background", "adminpack", "file_fdw", "pg_cron", "http", "pg_net"} {
		if !strings.Contains(queryRiskyExtensions, "'"+name+"'") {
			t.Errorf("queryRiskyExtensions lacks %s", name)
		}
	}
}

func TestPreflightRefusesNetworkExtensions(t *testing.T) {
	for _, name := range []string{"pg_cron", "http", "pg_net"} {
		f := healthyCatalog()
		f.answers[queryRiskyExtensions] = [][]any{{name, "1.0", "public"}}
		report := runPreflight(t, testDialect(core.TLSVerifyFull, nil), f)
		if report.Passed || reportCheck(t, report, checkRiskyObjects).Passed {
			t.Errorf("%s: report = %+v; want risky_objects refused", name, report)
		}
		want := "resource main has risky objects " + name + "; to approve, run on the host: podman exec -it sag sag extension approve main"
		if report.Hint != want {
			t.Errorf("%s: hint = %q; want %q", name, report.Hint, want)
		}
		report = runPreflight(t, testDialect(core.TLSVerifyFull, &approvalsFake{}), f)
		if !report.Passed {
			t.Errorf("%s: report = %+v; want passed with an approval", name, report)
		}
	}
}

func TestPreflightRiskyVerifyErrorsRefuse(t *testing.T) {
	errs := []error{approval.ErrNotApproved, approval.ErrInvalidMAC, approval.ErrFingerprintMismatch, errors.New("record mode is 0644")}
	for _, verifyErr := range errs {
		a := &approvalsFake{err: verifyErr}
		report := runPreflight(t, testDialect(core.TLSVerifyFull, a), riskyCatalog())
		c := reportCheck(t, report, checkRiskyObjects)
		if report.Passed || c.Passed {
			t.Errorf("Verify error %v: report = %+v; want refused", verifyErr, report)
		}
		if !strings.Contains(report.Hint, "podman exec -it sag sag extension approve main") || !strings.Contains(report.Hint, "file_fdw") {
			t.Errorf("Verify error %v: hint = %q; want the approval hint with the objects", verifyErr, report.Hint)
		}
		if strings.Contains(c.Detail, verifyErr.Error()) && !errors.Is(verifyErr, approval.ErrNotApproved) {
			t.Errorf("Verify error %v: detail %q holds the error text; want a fixed text", verifyErr, c.Detail)
		}
	}
}

func TestPreflightRiskyExpiredDetail(t *testing.T) {
	report := runPreflight(t, testDialect(core.TLSVerifyFull, &approvalsFake{err: approval.ErrExpired}), riskyCatalog())
	c := reportCheck(t, report, checkRiskyObjects)
	if report.Passed || c.Passed || c.Detail != "approval record expired: dblink, file_fdw, plperlu" {
		t.Fatalf("check = %+v; want refused with the expiry detail", c)
	}
	report = runPreflight(t, testDialect(core.TLSVerifyFull, &approvalsFake{err: approval.ErrRevoked}), riskyCatalog())
	if c := reportCheck(t, report, checkRiskyObjects); report.Passed || c.Detail != "approval record is not valid for the current objects: dblink, file_fdw, plperlu" {
		t.Fatalf("check = %+v; want refused with the invalid detail after a revoke", c)
	}
}

func TestPreflightRiskyApprovedPasses(t *testing.T) {
	a := &approvalsFake{}
	report := runPreflight(t, testDialect(core.TLSVerifyFull, a), riskyCatalog())
	if !report.Passed || !reportCheck(t, report, checkRiskyObjects).Passed || report.Hint != "" {
		t.Fatalf("report = %+v; want passed with a valid approval", report)
	}
	want, err := approval.Fingerprint(testTarget, approval.Objects{
		Extensions: []approval.Extension{{Name: "dblink", Version: "1.2", Schema: "public"}, {Name: "file_fdw", Version: "1.0", Schema: "public"}},
		Languages:  []string{"plperlu"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.resource != "main" || a.fingerprint != want {
		t.Fatalf("Verify(%q, %q); want Verify(main, %q)", a.resource, a.fingerprint, want)
	}
}

func TestRiskyObjectsMatchCatalog(t *testing.T) {
	objects, err := riskyObjects(context.Background(), riskyCatalog())
	if err != nil {
		t.Fatalf("riskyObjects: %v", err)
	}
	want := approval.Objects{
		Extensions: []approval.Extension{{Name: "file_fdw", Version: "1.0", Schema: "public"}, {Name: "dblink", Version: "1.2", Schema: "public"}},
		Languages:  []string{"plperlu"},
	}
	if !slices.Equal(objects.Extensions, want.Extensions) || !slices.Equal(objects.Languages, want.Languages) {
		t.Fatalf("riskyObjects = %+v; want %+v", objects, want)
	}
}

func TestRiskyObjectsQueriesNameEachObject(t *testing.T) {
	for _, name := range []string{"'dblink'", "'pg_background'", "'adminpack'", "'file_fdw'"} {
		if !strings.Contains(queryRiskyExtensions, name) {
			t.Errorf("queryRiskyExtensions lacks %s", name)
		}
	}
	for _, part := range []string{"lanpltrusted", "'c'", "'internal'"} {
		if !strings.Contains(queryUntrustedLanguages, part) {
			t.Errorf("queryUntrustedLanguages lacks %s", part)
		}
	}
}

func TestPreflightNoRiskySkipsApprovals(t *testing.T) {
	a := &approvalsFake{err: approval.ErrNotApproved}
	report := runPreflight(t, testDialect(core.TLSVerifyFull, a), healthyCatalog())
	if !report.Passed || a.resource != "" {
		t.Fatalf("report = %+v, Verify called for %q; want passed without Verify", report, a.resource)
	}
}

func TestOperatorNamesReportOnly(t *testing.T) {
	f := healthyCatalog()
	f.answers[queryOperators] = [][]any{{"~~*"}, {"="}, {"<>"}, {"="}}
	report := runPreflight(t, testDialect(core.TLSVerifyFull, nil), f)
	c := reportCheck(t, report, checkOperatorNames)
	if !report.Passed || !c.Passed {
		t.Fatalf("report = %+v; want passed, operator names report only", report)
	}
	if c.Detail != "<> = ~~*" {
		t.Fatalf("operator_names detail = %q; want %q", c.Detail, "<> = ~~*")
	}
}

func TestOperatorCollisionsSortedUnique(t *testing.T) {
	f := healthyCatalog()
	f.answers[queryOperators] = [][]any{{"~~"}, {"="}, {"~~"}, {"<"}}
	names, err := operatorCollisions(context.Background(), f)
	if err != nil {
		t.Fatalf("operatorCollisions: %v", err)
	}
	if want := []string{"<", "=", "~~"}; !slices.Equal(names, want) {
		t.Fatalf("operatorCollisions = %v; want %v", names, want)
	}
	if !strings.Contains(queryOperators, "pg_catalog") {
		t.Fatalf("queryOperators = %q; want a filter on pg_catalog", queryOperators)
	}
}

func TestPreflightQueryErrorIsFixed(t *testing.T) {
	for _, q := range []string{queryServer, queryTLS, queryReadOnly, queryRiskyExtensions, queryUntrustedLanguages, queryOperators} {
		f := healthyCatalog()
		f.failOn = q
		_, err := testDialect(core.TLSVerifyFull, nil).Preflight(context.Background(), f)
		var coreErr *core.Error
		if !errors.As(err, &coreErr) || coreErr.Code != core.CodeResourceUnavailable {
			t.Fatalf("query %q failure: error = %v; want core.Error resource_unavailable", q, err)
		}
		if strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("query %q failure: error %q holds driver text", q, err)
		}
	}
}

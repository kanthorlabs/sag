package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/audit"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/pipeline"
	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/sql"
)

type oracleAudit struct{ records []audit.Record }

func (o *oracleAudit) Append(rec audit.Record) error {
	o.records = append(o.records, rec)
	return nil
}

func TestRejectionIsNotSecretOracle(t *testing.T) {
	loaded := "tr0ub4dor-x3"
	other := "wr0ngguess-1"
	if len(loaded) != len(other) {
		t.Fatal("the two values must have equal length")
	}
	dsn := "postgres://app:" + loaded + "@db.internal:5432/orders"
	inv := inventory.New()
	res := sql.NewExecutor(newDialect(core.ResourceConfig{}, approval.Target{}), nil)
	if err := inv.Add(inventory.Entry{Name: "r", Kind: Kind, Capabilities: []string{"sql"}, Agents: []string{"a"}, Limits: core.Limits{MaxRows: 100, MaxResultBytes: 32768, MaxCellBytes: 4096}, Resource: res, Status: inventory.StatusServed}); err != nil {
		t.Fatal(err)
	}
	records := &oracleAudit{}
	p := pipeline.New(pipeline.Options{
		Inventory:             inv,
		MaxConcurrentPerAgent: 20,
		Scrubber:              scrub.New([]secret.Credential{secret.NewCredential("sag-orders", []byte(dsn))}),
		Audit:                 records,
	})
	ask := func(stmt string) core.Error {
		t.Helper()
		in, err := json.Marshal(map[string]string{"resource": "r", "sql": stmt})
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Execute(context.Background(), pipeline.Request{ID: "req", Sub: "a", Resource: "r", Capability: "sql", Operation: "query", Input: in})
		var ce *core.Error
		if !errors.As(err, &ce) {
			t.Fatalf("Execute(%q) = %v; want *core.Error", stmt, err)
		}
		return core.Error{Code: ce.Code, Message: ce.Message}
	}
	for _, shape := range []string{
		`SELECT "%s".f()`,
		`SELECT "aaa"."%s".f(), 1`,
		`SELECT * FROM x."%s".y.z`,
		`SELECT 1 OPERATOR("%s".+) 1`,
		`EXPLAIN ("%s") SELECT 1`,
		`INSERT INTO "%s" VALUES (1)`,
		`SELECT 1; SELECT '%s'`,
		`SELECT * FROM t AS "%s" TABLESAMPLE SYSTEM (50)`,
		`SELECT * FROM "%s" FOR UPDATE`,
	} {
		withSecret := ask(strings.Replace(shape, "%s", loaded, 1))
		withOther := ask(strings.Replace(shape, "%s", other, 1))
		if withSecret != withOther {
			t.Errorf("shape %q: response with the loaded secret %+v differs from response with another value %+v", shape, withSecret, withOther)
		}
		if withSecret.Code != core.CodeQueryRejected || strings.Contains(withSecret.Message, other) || strings.Contains(withSecret.Message, "[REDACTED]") {
			t.Errorf("shape %q: response %+v; want query_rejected with fixed text", shape, withSecret)
		}
		secretRecord := records.records[len(records.records)-2]
		for _, text := range []string{secretRecord.SQL, secretRecord.Reason} {
			if strings.Contains(text, loaded) {
				t.Errorf("shape %q: audit record holds the loaded secret: %q", shape, text)
			}
		}
		if !strings.Contains(secretRecord.SQL, "[REDACTED]") {
			t.Errorf("shape %q: audit sql = %q; want the scrubbed marker", shape, secretRecord.SQL)
		}
	}
}

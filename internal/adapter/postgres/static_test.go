package postgres

import (
	"errors"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
)

func assertShapeReason(t *testing.T, text, want string) {
	t.Helper()
	err := checkStatementShape(mustParseTree(t, text))
	var rejection *Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("checkStatementShape(%q) = %v; want a Rejection %q", text, err, want)
	}
	if got := rejection.Reason(); got != want {
		t.Fatalf("checkStatementShape(%q) reason = %q; want %q", text, got, want)
	}
}

func assertShapeAccepted(t *testing.T, text string) {
	t.Helper()
	if err := checkStatementShape(mustParseTree(t, text)); err != nil {
		t.Fatalf("checkStatementShape(%q) = %v; want nil", text, err)
	}
}

func TestAC13_StaticRejects(t *testing.T) {
	cases := []struct {
		text   string
		reason string
	}{
		{"INSERT INTO t VALUES (1)", "statement:InsertStmt"},
		{"UPDATE t SET a = 1", "statement:UpdateStmt"},
		{"DELETE FROM t", "statement:DeleteStmt"},
		{"MERGE INTO t USING s ON t.id = s.id WHEN MATCHED THEN DELETE", "statement:MergeStmt"},
		{"TRUNCATE t", "statement:TruncateStmt"},
		{"DROP TABLE t", "statement:DropStmt"},
		{"CREATE TABLE t (id int)", "statement:CreateStmt"},
		{"CREATE TEMPORARY TABLE t (id int)", "statement:CreateStmt"},
		{"ALTER TABLE t ADD COLUMN b int", "statement:AlterTableStmt"},
		{"GRANT SELECT ON t TO PUBLIC", "statement:GrantStmt"},
		{"COPY t TO STDOUT", "statement:CopyStmt"},
		{"SET work_mem = '64MB'", "statement:VariableSetStmt"},
		{"BEGIN", "statement:TransactionStmt"},
		{"COMMIT", "statement:TransactionStmt"},
		{"CALL p()", "statement:CallStmt"},
		{"DO $$BEGIN END$$", "statement:DoStmt"},
		{"SELECT 1; DELETE FROM t", "statement_count:2"},
		{"WITH d AS (DELETE FROM t RETURNING *) SELECT * FROM d", "statement:DeleteStmt"},
		{"SELECT * INTO t2 FROM t", "SelectStmt.into_clause"},
		{"SELECT * FROM t FOR UPDATE", "SelectStmt.locking_clause"},
		{"EXPLAIN ANALYZE SELECT 1", "explain_option:analyze"},
		{"EXPLAIN (ANALYZE true) SELECT 1", "explain_option:analyze"},
		{"", "statement_count:0"},
	}
	for _, c := range cases {
		assertShapeReason(t, c.text, c.reason)
	}
	_, err := emptyDialect().Parse("SELEC 1")
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Code != core.CodeQueryRejected || coreErr.Message != msgParseFailed {
		t.Fatalf("Parse(unparsable) = %v; want query_rejected %q", err, msgParseFailed)
	}
	if len(cases)+1 != 24 {
		t.Fatalf("AC13 list has %d statements; want 24", len(cases)+1)
	}
}

func TestStatementCount(t *testing.T) {
	for text, want := range map[string]string{
		"   ":                         "statement_count:0",
		";":                           "statement_count:0",
		"/* only a comment */":        "statement_count:0",
		"-- only a comment":           "statement_count:0",
		"SELECT 1; SELECT 2":          "statement_count:2",
		"SELECT 1;;SELECT 2;":         "statement_count:2",
		"SELECT 1; SELECT 2; TABLE t": "statement_count:3",
	} {
		assertShapeReason(t, text, want)
	}
	for _, text := range []string{"SELECT 1", "SELECT 1;", " TABLE t ; ", "VALUES (1), (2)", "SELECT 1 UNION ALL SELECT 2"} {
		assertShapeAccepted(t, text)
	}
}

func TestOtherTopLevelStatementsRejected(t *testing.T) {
	for text, want := range map[string]string{
		"RESET ALL":                         "statement:VariableSetStmt",
		"SHOW work_mem":                     "statement:VariableShowStmt",
		"LOCK TABLE t":                      "statement:LockStmt",
		"PREPARE p AS SELECT 1":             "statement:PrepareStmt",
		"EXECUTE p":                         "statement:ExecuteStmt",
		"LISTEN c":                          "statement:ListenStmt",
		"NOTIFY c":                          "statement:NotifyStmt",
		"VACUUM t":                          "statement:VacuumStmt",
		"ANALYZE t":                         "statement:VacuumStmt",
		"ROLLBACK":                          "statement:TransactionStmt",
		"PREPARE TRANSACTION 'x'":           "statement:TransactionStmt",
		"CHECKPOINT":                        "statement:CheckPointStmt",
		"DECLARE c CURSOR FOR SELECT 1":     "statement:DeclareCursorStmt",
		"CREATE TABLE t2 AS SELECT 1":       "statement:CreateTableAsStmt",
		"COPY (SELECT 1) TO PROGRAM 'true'": "statement:CopyStmt",
		"CREATE VIEW v AS SELECT 1":         "statement:ViewStmt",
		"CREATE FUNCTION f() RETURNS int RETURN 1": "statement:CreateFunctionStmt",
	} {
		assertShapeReason(t, text, want)
	}
}

func TestExplainOptions(t *testing.T) {
	for _, text := range []string{
		"EXPLAIN SELECT 1",
		"EXPLAIN VERBOSE SELECT 1",
		"EXPLAIN (FORMAT JSON) SELECT 1",
		"EXPLAIN (COSTS off, VERBOSE, FORMAT yaml) SELECT * FROM t",
		"EXPLAIN (verbose false) SELECT 1",
		"EXPLAIN WITH c AS (SELECT 1) SELECT * FROM c",
		"EXPLAIN SELECT 1 UNION SELECT 2",
		"EXPLAIN TABLE t",
	} {
		assertShapeAccepted(t, text)
	}
	for text, want := range map[string]string{
		"EXPLAIN ANALYZE VERBOSE SELECT 1":                              "explain_option:analyze",
		"EXPLAIN (VERBOSE, ANALYZE) SELECT 1":                           "explain_option:analyze",
		"EXPLAIN (ANALYZE false) SELECT 1":                              "explain_option:analyze",
		"EXPLAIN (BUFFERS) SELECT 1":                                    "explain_option:buffers",
		"EXPLAIN (TIMING off) SELECT 1":                                 "explain_option:timing",
		"EXPLAIN (SETTINGS) SELECT 1":                                   "explain_option:settings",
		"EXPLAIN (WAL) SELECT 1":                                        "explain_option:wal",
		"EXPLAIN (SUMMARY) SELECT 1":                                    "explain_option:summary",
		"EXPLAIN (GENERIC_PLAN) SELECT 1":                               "explain_option:generic_plan",
		"EXPLAIN (SERIALIZE) SELECT 1":                                  "explain_option:serialize",
		"EXPLAIN (MEMORY) SELECT 1":                                     "explain_option:memory",
		"EXPLAIN (unknown_option) SELECT 1":                             "explain_option:unknown_option",
		`EXPLAIN ("ANALYZE") SELECT 1`:                                  "explain_option:ANALYZE",
		`EXPLAIN ("Format" json) SELECT 1`:                              "explain_option:Format",
		"EXPLAIN INSERT INTO t VALUES (1)":                              "statement:InsertStmt",
		"EXPLAIN UPDATE t SET a = 1":                                    "statement:UpdateStmt",
		"EXPLAIN DELETE FROM t":                                         "statement:DeleteStmt",
		"EXPLAIN CREATE TABLE t2 AS SELECT 1":                           "statement:CreateTableAsStmt",
		"EXPLAIN DECLARE c CURSOR FOR SELECT 1":                         "statement:DeclareCursorStmt",
		"EXPLAIN EXECUTE p":                                             "statement:ExecuteStmt",
		"EXPLAIN SELECT * FROM t FOR SHARE":                             "SelectStmt.locking_clause",
		"EXPLAIN SELECT * INTO t2 FROM t":                               "SelectStmt.into_clause",
		"EXPLAIN WITH d AS (DELETE FROM t RETURNING *) SELECT * FROM d": "statement:DeleteStmt",
	} {
		assertShapeReason(t, text, want)
	}
}

func TestWritableCTE(t *testing.T) {
	for _, text := range []string{
		"WITH a AS (SELECT 1 AS x), b AS (SELECT x FROM a) SELECT * FROM b",
		"WITH RECURSIVE r(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM r WHERE n < 3) SELECT * FROM r",
		"WITH a AS MATERIALIZED (SELECT 1) SELECT * FROM a",
		"SELECT * FROM (WITH a AS (SELECT 1) SELECT * FROM a) s",
		"WITH a AS (VALUES (1)) SELECT * FROM a",
	} {
		assertShapeAccepted(t, text)
	}
	for text, want := range map[string]string{
		"WITH i AS (INSERT INTO t VALUES (1) RETURNING *) SELECT * FROM i":                                     "statement:InsertStmt",
		"WITH u AS (UPDATE t SET a = 1 RETURNING *) SELECT * FROM u":                                           "statement:UpdateStmt",
		"WITH d AS (DELETE FROM t) SELECT 1":                                                                   "statement:DeleteStmt",
		"WITH m AS (MERGE INTO t USING s ON t.id = s.id WHEN MATCHED THEN DELETE RETURNING *) SELECT * FROM m": "statement:MergeStmt",
		"WITH a AS (SELECT 1), d AS (DELETE FROM t RETURNING *) SELECT * FROM a":                               "statement:DeleteStmt",
		"WITH a AS (WITH d AS (DELETE FROM t RETURNING *) SELECT * FROM d) SELECT * FROM a":                    "statement:DeleteStmt",
		"SELECT * FROM (WITH d AS (DELETE FROM t RETURNING *) SELECT * FROM d) s":                              "statement:DeleteStmt",
		"SELECT 1 WHERE 1 IN (WITH d AS (DELETE FROM t RETURNING id) SELECT id FROM d)":                        "statement:DeleteStmt",
		"SELECT (WITH d AS (DELETE FROM t RETURNING id) SELECT id FROM d)":                                     "statement:DeleteStmt",
		"SELECT 1 UNION SELECT * FROM (WITH d AS (DELETE FROM t RETURNING id) SELECT id FROM d) s":             "statement:DeleteStmt",
		"SELECT * FROM t, LATERAL (WITH d AS (DELETE FROM t RETURNING id) SELECT id FROM d) l":                 "statement:DeleteStmt",
	} {
		assertShapeReason(t, text, want)
	}
}

func TestRowLocksAndIntoAnyDepth(t *testing.T) {
	for text, want := range map[string]string{
		"SELECT * FROM t FOR NO KEY UPDATE":                      "SelectStmt.locking_clause",
		"SELECT * FROM t FOR SHARE":                              "SelectStmt.locking_clause",
		"SELECT * FROM t FOR KEY SHARE":                          "SelectStmt.locking_clause",
		"SELECT * FROM t FOR UPDATE OF t NOWAIT":                 "SelectStmt.locking_clause",
		"SELECT * FROM t FOR UPDATE SKIP LOCKED":                 "SelectStmt.locking_clause",
		"SELECT * FROM (SELECT * FROM t FOR UPDATE) s":           "SelectStmt.locking_clause",
		"WITH c AS (SELECT * FROM t FOR SHARE) SELECT * FROM c":  "SelectStmt.locking_clause",
		"SELECT 1 WHERE EXISTS (SELECT 1 FROM t FOR KEY SHARE)":  "SelectStmt.locking_clause",
		"SELECT 1 UNION ALL (SELECT 1 FROM t FOR UPDATE)":        "SelectStmt.locking_clause",
		"SELECT (SELECT a FROM t FOR NO KEY UPDATE)":             "SelectStmt.locking_clause",
		"SELECT * INTO TEMPORARY t2 FROM t":                      "SelectStmt.into_clause",
		"SELECT * INTO UNLOGGED t2 FROM t UNION SELECT * FROM t": "SelectStmt.into_clause",
	} {
		assertShapeReason(t, text, want)
	}
}

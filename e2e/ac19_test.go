//go:build e2e

package e2e

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
	"github.com/kanthorlabs/sag/internal/testenv"
)

const (
	ac19Sub          = "ac19"
	ac19Resource     = "ac19"
	ac19Database     = "sag_e2e"
	ac19ControlTable = "public.ac19_t"
)

const ac19ObjectQuery = `SELECT o.type AS kind, o.identity AS identity, d.detail AS detail
FROM (
  SELECT 'pg_namespace'::regclass AS classid, n.oid AS objid, coalesce(n.nspacl::text, '') AS detail FROM pg_namespace n
  UNION ALL SELECT 'pg_class'::regclass, c.oid, md5(concat_ws('|', c.relkind, c.relpersistence, c.relacl, c.reloptions,
    (SELECT string_agg(a.attname || ' ' || format_type(a.atttypid, a.atttypmod) || CASE WHEN a.attnotnull THEN ' not null' ELSE '' END, ',' ORDER BY a.attnum)
       FROM pg_attribute a WHERE a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped),
    CASE WHEN c.relkind IN ('v', 'm') THEN pg_get_viewdef(c.oid) END,
    CASE WHEN c.relkind = 'i' THEN pg_get_indexdef(c.oid) END)) FROM pg_class c WHERE c.oid >= 16384
  UNION ALL SELECT 'pg_proc'::regclass, p.oid, md5(concat_ws('|', p.prosrc, p.probin, p.provolatile, p.prosecdef, p.proconfig, p.proacl, p.prorettype)) FROM pg_proc p WHERE p.oid >= 16384
  UNION ALL SELECT 'pg_type'::regclass, t.oid, md5(concat_ws('|', t.typtype, t.typnotnull, t.typdefault, t.typacl)) FROM pg_type t WHERE t.oid >= 16384
  UNION ALL SELECT 'pg_constraint'::regclass, k.oid, pg_get_constraintdef(k.oid) FROM pg_constraint k WHERE k.oid >= 16384
  UNION ALL SELECT 'pg_attrdef'::regclass, ad.oid, pg_get_expr(ad.adbin, ad.adrelid) FROM pg_attrdef ad WHERE ad.oid >= 16384
  UNION ALL SELECT 'pg_trigger'::regclass, tg.oid, pg_get_triggerdef(tg.oid) FROM pg_trigger tg WHERE tg.oid >= 16384
  UNION ALL SELECT 'pg_rewrite'::regclass, r.oid, pg_get_ruledef(r.oid) FROM pg_rewrite r WHERE r.oid >= 16384
  UNION ALL SELECT 'pg_policy'::regclass, pol.oid, md5(concat_ws('|', pol.polcmd, pol.polpermissive, pol.polroles, pg_get_expr(pol.polqual, pol.polrelid), pg_get_expr(pol.polwithcheck, pol.polrelid))) FROM pg_policy pol WHERE pol.oid >= 16384
  UNION ALL SELECT 'pg_extension'::regclass, e.oid, e.extversion FROM pg_extension e WHERE e.oid >= 16384
  UNION ALL SELECT 'pg_cast'::regclass, ca.oid, concat_ws('|', ca.castfunc, ca.castcontext, ca.castmethod) FROM pg_cast ca WHERE ca.oid >= 16384
  UNION ALL SELECT 'pg_operator'::regclass, op.oid, op.oprcode::text FROM pg_operator op WHERE op.oid >= 16384
  UNION ALL SELECT 'pg_event_trigger'::regclass, et.oid, concat_ws('|', et.evtevent, et.evtfoid, et.evtenabled, et.evttags) FROM pg_event_trigger et WHERE et.oid >= 16384
  UNION ALL SELECT 'pg_largeobject'::regclass, lo.oid, coalesce(lo.lomacl::text, '') FROM pg_largeobject_metadata lo
) d
CROSS JOIN LATERAL pg_identify_object(d.classid, d.objid, 0) o
WHERE o.schema IS DISTINCT FROM 'pg_toast'`

const ac19DataQuery = `SELECT 'data' AS kind, format('%I.%I', n.nspname, c.relname) AS identity,
  (xpath('/row/d/text()', query_to_xml(format(
    CASE WHEN c.relkind = 'S'
      THEN 'SELECT count(*) || '':'' || md5(string_agg(concat_ws(''|'', t.last_value, t.log_cnt, t.is_called), '''')) AS d FROM %I.%I t'
      ELSE 'SELECT count(*) || '':'' || md5(coalesce(string_agg(t::text, ''|'' ORDER BY t::text), '''')) AS d FROM %I.%I t'
    END,
    n.nspname, c.relname), false, true, '')))[1]::text AS detail
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.oid >= 16384 AND n.nspname <> 'pg_toast' AND c.relpersistence <> 't'
  AND (c.relkind IN ('r', 'p', 'S') OR (c.relkind = 'm' AND c.relispopulated))`

const ac19StateQuery = ac19ObjectQuery + "\nUNION ALL\n" + ac19DataQuery

var (
	ac19StatementRecord = regexp.MustCompile(`(?s)LOG:\s+(?:statement|execute [^:]*):\s?(.*)$`)
	ac19CommitStatement = regexp.MustCompile(`(?i)\bCOMMIT\b|^\s*END\b`)
)

func init() {
	harness.Register(harness.AC{
		ID:     "AC19",
		Phase:  harness.PhaseSequential,
		Agents: []harness.AgentSpec{{Sub: ac19Sub, Name: "AC19"}},
		Resources: []harness.ResourceSpec{{
			Name:       ac19Resource,
			Containers: []string{harness.ContainerSAG},
			Database:   ac19Database,
			Agents:     []string{ac19Sub},
		}},
		Fixture: "CREATE TABLE " + ac19ControlTable + " (id int PRIMARY KEY, v text); " +
			"INSERT INTO " + ac19ControlTable + " VALUES (1, 'ac19_a'), (2, 'ac19_b'), (3, 'ac19_c')",
		Snapshot: ac19StateQuery,
		Run:      runAC19,
	})
}

type ac19Entry struct {
	Kind     string
	Identity string
	Detail   string
}

func ac19Compare(a, b ac19Entry) int {
	if c := strings.Compare(a.Kind, b.Kind); c != 0 {
		return c
	}
	if c := strings.Compare(a.Identity, b.Identity); c != 0 {
		return c
	}
	return strings.Compare(a.Detail, b.Detail)
}

func ac19Entries(t *testing.T, rows [][]string) []ac19Entry {
	t.Helper()
	entries := make([]ac19Entry, 0, len(rows))
	for _, r := range rows {
		if len(r) != 3 {
			t.Fatalf("state row has %d columns, want 3: %q", len(r), r)
		}
		entries = append(entries, ac19Entry{Kind: r[0], Identity: r[1], Detail: r[2]})
	}
	slices.SortFunc(entries, ac19Compare)
	return entries
}

func ac19Read(ctx context.Context, t *testing.T, pg *testenv.Postgres, database string) []ac19Entry {
	t.Helper()
	conn, err := harness.ConnectDB(ctx, pg, database)
	if err != nil {
		t.Fatalf("connect PostgreSQL %d database %s: %v", pg.Major, database, err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	rows, err := harness.QueryText(ctx, conn, ac19StateQuery)
	if err != nil {
		t.Fatalf("state query in PostgreSQL %d database %s: %v", pg.Major, database, err)
	}
	return ac19Entries(t, rows)
}

func ac19CheckDatabases(ctx context.Context, t *testing.T, env harness.Env, snapshots []harness.DatabaseSnapshot) {
	t.Helper()
	before := map[int][]string{}
	for _, s := range snapshots {
		before[s.Major] = append(before[s.Major], s.Database)
	}
	for major, names := range before {
		after, err := harness.Databases(ctx, env.Postgres(major))
		if err != nil {
			t.Fatalf("list databases of PostgreSQL %d: %v", major, err)
		}
		for _, db := range after {
			if !slices.Contains(names, db) {
				t.Errorf("PostgreSQL %d database %s exists after the suite and has no snapshot", major, db)
			}
		}
		for _, db := range names {
			if !slices.Contains(after, db) {
				t.Errorf("PostgreSQL %d database %s has a snapshot and does not exist after the suite", major, db)
			}
		}
	}
}

func ac19ControlEntry() ac19Entry {
	sum := md5.Sum([]byte("(1,ac19_a)|(2,ac19_b)|(3,ac19_c)"))
	return ac19Entry{Kind: "data", Identity: "public.ac19_t", Detail: "3:" + hex.EncodeToString(sum[:])}
}

func ac19Diff(before, after []ac19Entry) (missing, added []ac19Entry) {
	for _, e := range before {
		if _, found := slices.BinarySearchFunc(after, e, ac19Compare); !found {
			missing = append(missing, e)
		}
	}
	for _, e := range after {
		if _, found := slices.BinarySearchFunc(before, e, ac19Compare); !found {
			added = append(added, e)
		}
	}
	return missing, added
}

func ac19Statements(logs string) []string {
	var records []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.HasPrefix(line, "\t") && len(records) > 0 {
			records[len(records)-1] += "\n" + line
			continue
		}
		records = append(records, line)
	}
	var statements []string
	for _, r := range records {
		if m := ac19StatementRecord.FindStringSubmatch(r); m != nil {
			statements = append(statements, m[1])
		}
	}
	return statements
}

func runAC19(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	snapshots := env.Snapshots("AC19")
	control := ac19ControlEntry()
	var controlFound bool
	for _, s := range snapshots {
		before := ac19Entries(t, s.Rows)
		if !slices.ContainsFunc(before, func(e ac19Entry) bool { return e.Kind == "schema" && e.Identity == "public" }) {
			t.Fatalf("snapshot of PostgreSQL %d database %s has no entry for schema public: %+v", s.Major, s.Database, before)
		}
		if s.Major == env.Major() && s.Database == ac19Database {
			if _, found := slices.BinarySearchFunc(before, control, ac19Compare); !found {
				t.Fatalf("snapshot has no entry %+v; the snapshot query does not see the fixture tables: %+v", control, before)
			}
			if !slices.ContainsFunc(before, func(e ac19Entry) bool { return e.Kind == "table" && e.Identity == ac19ControlTable }) {
				t.Fatalf("snapshot has no object entry for table %s: %+v", ac19ControlTable, before)
			}
			controlFound = true
		}
	}
	if !controlFound {
		t.Fatalf("no snapshot of PostgreSQL %d database %s; snapshots: %d", env.Major(), ac19Database, len(snapshots))
	}

	ac19CheckDatabases(ctx, t, env, snapshots)
	for _, s := range snapshots {
		before := ac19Entries(t, s.Rows)
		after := ac19Read(ctx, t, env.Postgres(s.Major), s.Database)
		missing, added := ac19Diff(before, after)
		for _, e := range missing {
			t.Errorf("PostgreSQL %d database %s: entry changed or removed after the suite: %+v", s.Major, s.Database, e)
		}
		for _, e := range added {
			t.Errorf("PostgreSQL %d database %s: entry changed or added after the suite: %+v", s.Major, s.Database, e)
		}
	}

	pg := env.Postgres(env.Major())
	logs, err := pg.Logs(ctx, time.Time{})
	if err != nil {
		t.Fatalf("PostgreSQL logs: %v", err)
	}
	statements := ac19Statements(logs)
	if !slices.ContainsFunc(statements, func(s string) bool { return strings.Contains(s, ac19ControlTable) }) {
		t.Fatalf("PostgreSQL log parse found no statement with %s; the log check cannot see statements", ac19ControlTable)
	}
	for _, s := range statements {
		if ac19CommitStatement.MatchString(s) {
			t.Errorf("PostgreSQL log holds a commit: %q", s)
		}
	}
}

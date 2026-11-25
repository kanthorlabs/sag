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

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const (
	ac19Sub           = "ac19"
	ac19Resource      = "ac19"
	ac19Database      = "sag_e2e"
	ac19SnapshotTable = "sag_ac19.ac19_snapshot"
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
WHERE o.schema IS DISTINCT FROM 'sag_ac19' AND o.schema IS DISTINCT FROM 'pg_toast'
  AND NOT (o.type = 'schema' AND o.identity = 'sag_ac19')`

const ac19DataQuery = `SELECT 'data' AS kind, format('%I.%I', n.nspname, c.relname) AS identity,
  (xpath('/row/d/text()', query_to_xml(format(
    CASE WHEN c.relkind = 'S'
      THEN 'SELECT count(*) || '':'' || md5(string_agg(concat_ws(''|'', t.last_value, t.log_cnt, t.is_called), '''')) AS d FROM %I.%I t'
      ELSE 'SELECT count(*) || '':'' || md5(coalesce(string_agg(t::text, ''|'' ORDER BY t::text), '''')) AS d FROM %I.%I t'
    END,
    n.nspname, c.relname), false, true, '')))[1]::text AS detail
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.oid >= 16384 AND n.nspname NOT IN ('sag_ac19', 'pg_toast') AND c.relpersistence <> 't'
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
		Fixture: "CREATE TABLE public.ac19_t (id int PRIMARY KEY, v text); " +
			"INSERT INTO public.ac19_t VALUES (1, 'ac19_a'), (2, 'ac19_b'), (3, 'ac19_c'); " +
			"CREATE SCHEMA sag_ac19; " +
			"CREATE TABLE " + ac19SnapshotTable + " AS " + ac19StateQuery,
		Run: runAC19,
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

func ac19Read(ctx context.Context, t *testing.T, conn *pgx.Conn, query string) []ac19Entry {
	t.Helper()
	rows, err := conn.Query(ctx, query)
	if err != nil {
		t.Fatalf("query %s: %v", query, err)
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ac19Entry, error) {
		var e ac19Entry
		err := row.Scan(&e.Kind, &e.Identity, &e.Detail)
		return e, err
	})
	if err != nil {
		t.Fatalf("query %s: %v", query, err)
	}
	slices.SortFunc(entries, ac19Compare)
	return entries
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

	pg := env.Postgres(env.Major())
	conn, err := harness.ConnectDB(ctx, pg, ac19Database)
	if err != nil {
		t.Fatalf("connect %s: %v", ac19Database, err)
	}
	defer conn.Close(context.WithoutCancel(ctx))

	before := ac19Read(ctx, t, conn, "SELECT kind, identity, detail FROM "+ac19SnapshotTable)
	after := ac19Read(ctx, t, conn, ac19StateQuery)

	control := ac19ControlEntry()
	if _, found := slices.BinarySearchFunc(before, control, ac19Compare); !found {
		t.Fatalf("snapshot has no entry %+v; the snapshot query does not see the fixture tables: %+v", control, before)
	}
	if !slices.ContainsFunc(before, func(e ac19Entry) bool { return e.Kind == "table" && e.Identity == "public.ac19_t" }) {
		t.Fatalf("snapshot has no object entry for table public.ac19_t: %+v", before)
	}

	missing, added := ac19Diff(before, after)
	for _, e := range missing {
		t.Errorf("entry changed or removed after the suite: %+v", e)
	}
	for _, e := range added {
		t.Errorf("entry changed or added after the suite: %+v", e)
	}

	logs, err := pg.Logs(ctx, time.Time{})
	if err != nil {
		t.Fatalf("PostgreSQL logs: %v", err)
	}
	statements := ac19Statements(logs)
	if !slices.ContainsFunc(statements, func(s string) bool { return strings.Contains(s, ac19SnapshotTable) }) {
		t.Fatalf("PostgreSQL log parse found no statement with %s; the log check cannot see statements", ac19SnapshotTable)
	}
	for _, s := range statements {
		if ac19CommitStatement.MatchString(s) {
			t.Errorf("PostgreSQL log holds a commit: %q", s)
		}
	}
}

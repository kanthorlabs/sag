package sql

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/postexec"
)

func TestCellCutOnArrival(t *testing.T) {
	f := newFixture()
	long := strings.Repeat("é", 50000)
	f.conn.rows[testRendered] = &fakeRows{values: [][]any{{long, int64(7)}}}
	res, err := f.query(t, testText, core.Limits{MaxRows: 10, MaxCellBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	cell := res.Rows[0][0].(string)
	if len(cell) != 101 {
		t.Fatalf("stored cell = %d bytes; want 101", len(cell))
	}
	want := postexec.Apply(core.Result{Rows: [][]any{{long, int64(7)}}}, core.Limits{MaxCellBytes: 100}, nil)
	got := postexec.Apply(res, core.Limits{MaxCellBytes: 100}, nil)
	if !reflect.DeepEqual(got.Rows, want.Rows) {
		t.Fatalf("post-execute rows = %v; want %v", got.Rows, want.Rows)
	}
}

func TestByteStopReadsNoMoreRows(t *testing.T) {
	rowsOf := func() [][]any {
		values := make([][]any, 1000)
		for i := range values {
			values[i] = []any{strings.Repeat("x", 90)}
		}
		return values
	}
	lim := core.Limits{MaxRows: 1000, MaxResultBytes: 1000, MaxCellBytes: 4096}

	f := newFixture()
	rows := &fakeRows{ev: f.ev, values: rowsOf()}
	f.conn.rows[testRendered] = rows
	res, err := f.query(t, testText, lim)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated || res.TruncatedReason != core.TruncatedMaxResultBytes {
		t.Fatalf("result truncated %v %q; want max_result_bytes", res.Truncated, res.TruncatedReason)
	}
	if rows.valuesN != 11 || len(res.Rows) != 11 {
		t.Fatalf("Values %d, rows %d; want 11 rows read", rows.valuesN, len(res.Rows))
	}
	if !f.ev.has("rows:abort") || f.ev.count("conn:close") != 1 || f.ev.has("conn:release") {
		t.Fatalf("byte stop without a cursor did not abort and close: %v", f.ev.list)
	}

	f = newFixture()
	f.dialect.plan = RowCapPlan{Statement: testStmt, Fetch: testFetch}
	rows = &fakeRows{ev: f.ev, values: rowsOf()}
	f.conn.rows[testFetch] = rows
	res, err = f.query(t, testText, lim)
	if err != nil {
		t.Fatal(err)
	}
	if res.TruncatedReason != core.TruncatedMaxResultBytes || rows.valuesN != 11 {
		t.Fatalf("cursor byte stop: reason %q, Values %d", res.TruncatedReason, rows.valuesN)
	}
	if f.ev.has("rows:abort") || f.ev.count("conn:release") != 1 {
		t.Fatalf("cursor byte stop did not release: %v", f.ev.list)
	}
}

func TestReadCapsKeepPostExecuteResult(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	pieces := []string{"a", "é", "😀", "\xff", "\"", "<"}
	cell := func() any {
		switch rng.Intn(5) {
		case 0:
			return nil
		case 1:
			return int64(rng.Intn(100000))
		default:
			var b strings.Builder
			for n := rng.Intn(120); n > 0; n-- {
				b.WriteString(pieces[rng.Intn(len(pieces))])
			}
			return b.String()
		}
	}
	for i := 0; i < 3000; i++ {
		cols := 1 + rng.Intn(3)
		values := make([][]any, rng.Intn(30))
		for r := range values {
			values[r] = make([]any, cols)
			for c := range values[r] {
				values[r][c] = cell()
			}
		}
		columns := make([]core.Column, cols)
		for c := range columns {
			columns[c] = core.Column{Name: "c" + string(rune('a'+c)), Type: "text"}
		}
		lim := core.Limits{MaxRows: 1 + rng.Intn(25), MaxCellBytes: 1 + rng.Intn(80), MaxResultBytes: 40 + rng.Intn(3000)}
		redact := []string{}
		if rng.Intn(2) == 0 {
			redact = []string{"ca"}
		}

		full := core.Result{Columns: columns, Rows: values}
		if len(values) > lim.MaxRows {
			full = core.Result{Columns: columns, Rows: values[:lim.MaxRows], Truncated: true, TruncatedReason: core.TruncatedMaxRows}
		}
		want := postexec.Apply(full, lim, redact)

		for _, cursor := range []bool{false, true} {
			f := newFixture()
			text := testRendered
			if cursor {
				f.dialect.plan = RowCapPlan{Statement: testStmt, Fetch: testFetch}
				text = testFetch
			}
			f.conn.rows[text] = &fakeRows{columns: columns, values: values}
			res, err := f.query(t, testText, lim)
			if err != nil {
				t.Fatal(err)
			}
			got := postexec.Apply(res, lim, redact)
			gotJSON, _ := json.Marshal(got.Rows)
			wantJSON, _ := json.Marshal(want.Rows)
			if len(redact) > 0 && res.TruncatedReason == core.TruncatedMaxResultBytes {
				prefix, _ := json.Marshal(want.Rows[:min(len(got.Rows), len(want.Rows))])
				if string(gotJSON) != string(prefix) || (len(got.Rows) > 0 && got.Bytes > lim.MaxResultBytes) || !got.Truncated {
					t.Fatalf("case %d cursor %v: byte stop with redaction gave %s %d; want a prefix of %s under the cap", i, cursor, gotJSON, got.Bytes, wantJSON)
				}
				continue
			}
			if string(gotJSON) != string(wantJSON) || got.Truncated != want.Truncated || got.TruncatedReason != want.TruncatedReason || got.Bytes != want.Bytes {
				t.Fatalf("case %d cursor %v lim %+v redact %v:\ngot  %s %v %q %d\nwant %s %v %q %d", i, cursor, lim, redact, gotJSON, got.Truncated, got.TruncatedReason, got.Bytes, wantJSON, want.Truncated, want.TruncatedReason, want.Bytes)
			}
		}
	}
}

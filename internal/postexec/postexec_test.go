package postexec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
)

var prdDefaultPatterns = []string{"*password*", "*passwd*", "*secret*", "*token*", "*api_key*", "*apikey*", "*private_key*"}

func encodedLen(t *testing.T, res core.Result) int {
	t.Helper()
	out := output{Columns: res.Columns, Rows: res.Rows, RowCount: len(res.Rows), Truncated: res.Truncated, TruncatedReason: res.TruncatedReason}
	if out.Columns == nil {
		out.Columns = []core.Column{}
	}
	if out.Rows == nil {
		out.Rows = [][]any{}
	}
	text, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return len(text)
}

func columns(names ...string) []core.Column {
	var cols []core.Column
	for _, n := range names {
		cols = append(cols, core.Column{Name: n, Type: "text"})
	}
	return cols
}

func TestCellCapSuffix(t *testing.T) {
	long := strings.Repeat("a", 10)
	multi := "ab" + strings.Repeat("é", 4)
	res := core.Result{
		Columns: columns("a", "b", "c", "d", "e"),
		Rows:    [][]any{{long, "short", int64(1234567890), nil, multi}},
	}
	got := Apply(res, core.Limits{MaxCellBytes: 5}, nil)
	row := got.Rows[0]
	if row[0] != "aaaaa"+CellSuffix {
		t.Fatalf("long cell = %q; want %q", row[0], "aaaaa"+CellSuffix)
	}
	if row[1] != "short" || row[2] != int64(1234567890) || row[3] != nil {
		t.Fatalf("cells within the cap changed: %v", row)
	}
	if row[4] != "abé"+CellSuffix {
		t.Fatalf("multibyte cell = %q; want %q", row[4], "abé"+CellSuffix)
	}
	if got.Truncated || got.TruncatedReason != core.TruncatedNone {
		t.Fatalf("cell cap set truncated = %v %q", got.Truncated, got.TruncatedReason)
	}
	if res.Rows[0][0] != long {
		t.Fatalf("Apply changed the input row: %v", res.Rows[0])
	}
}

func TestRedactGlobCaseInsensitive(t *testing.T) {
	res := core.Result{
		Columns: columns("id", "API_Token", "MyPassword", "note"),
		Rows:    [][]any{{int64(1), "t1", "p1", "n1"}, {int64(2), nil, "p2", "n2"}},
	}
	got := Apply(res, core.Limits{}, []string{"*token*", "*PASSWORD"})
	want := [][]any{{int64(1), Redacted, Redacted, "n1"}, {int64(2), Redacted, Redacted, "n2"}}
	for i := range want {
		for j := range want[i] {
			if got.Rows[i][j] != want[i][j] {
				t.Fatalf("rows = %v; want %v", got.Rows, want)
			}
		}
	}
}

func TestDefaultPatterns(t *testing.T) {
	names := []string{"user_password", "PASSWD", "client_secret", "api_token", "x_api_key", "APIKEY", "ssh_private_key", "name"}
	row := make([]any, len(names))
	for i := range row {
		row[i] = "v"
	}
	res := core.Result{Columns: columns(names...), Rows: [][]any{row}}
	got := Apply(res, core.Limits{}, prdDefaultPatterns)
	for i, n := range names {
		want := any(Redacted)
		if n == "name" {
			want = "v"
		}
		if got.Rows[0][i] != want {
			t.Fatalf("column %s = %v; want %v", n, got.Rows[0][i], want)
		}
	}
	plain := Apply(res, core.Limits{}, nil)
	for i := range names {
		if plain.Rows[0][i] != "v" {
			t.Fatalf("no patterns: column %s = %v; postexec must add no patterns", names[i], plain.Rows[0][i])
		}
	}
}

func TestResourcePatternsAdd(t *testing.T) {
	res := core.Result{
		Columns: columns("api_token", "ssn", "email"),
		Rows:    [][]any{{"t", "123", "a@b"}},
	}
	got := Apply(res, core.Limits{}, append(append([]string{}, prdDefaultPatterns...), "SSN", "e?ail"))
	for i := range res.Columns {
		if got.Rows[0][i] != Redacted {
			t.Fatalf("rows = %v; want every column redacted", got.Rows)
		}
	}
}

func TestByteCapAfterEncoding(t *testing.T) {
	var rows [][]any
	for i := 0; i < 50; i++ {
		rows = append(rows, []any{int64(i), strings.Repeat("<", 20)})
	}
	res := core.Result{Columns: columns("id", "v"), Rows: rows, Truncated: true, TruncatedReason: core.TruncatedMaxRows}

	fits := Apply(res, core.Limits{MaxResultBytes: 1 << 20}, nil)
	if len(fits.Rows) != 50 || fits.TruncatedReason != core.TruncatedMaxRows || fits.Bytes != encodedLen(t, fits) {
		t.Fatalf("under the cap: rows %d reason %q bytes %d", len(fits.Rows), fits.TruncatedReason, fits.Bytes)
	}

	full := encodedLen(t, res)
	for _, limit := range []int{full - 1, full / 2, 300, 200} {
		got := Apply(res, core.Limits{MaxResultBytes: limit}, nil)
		if !got.Truncated || got.TruncatedReason != core.TruncatedMaxResultBytes {
			t.Fatalf("cap %d: truncated %v %q; want max_result_bytes", limit, got.Truncated, got.TruncatedReason)
		}
		if got.Bytes != encodedLen(t, got) || got.Bytes > limit {
			t.Fatalf("cap %d: Bytes %d, encoded %d", limit, got.Bytes, encodedLen(t, got))
		}
		for i := range got.Rows {
			if got.Rows[i][0] != int64(i) {
				t.Fatalf("cap %d: row %d = %v; want the first rows in order", limit, i, got.Rows[i])
			}
		}
		next := core.Result{Columns: got.Columns, Rows: rows[:len(got.Rows)+1], Truncated: true, TruncatedReason: core.TruncatedMaxResultBytes}
		if encodedLen(t, next) <= limit {
			t.Fatalf("cap %d: kept %d rows; one more row fits", limit, len(got.Rows))
		}
	}

	none := Apply(res, core.Limits{MaxResultBytes: 10}, nil)
	if len(none.Rows) != 0 || none.Rows == nil || none.Bytes != encodedLen(t, none) {
		t.Fatalf("tiny cap: rows %v bytes %d", none.Rows, none.Bytes)
	}

	empty := Apply(core.Result{}, core.Limits{}, nil)
	if empty.Bytes != len(`{"columns":[],"rows":[],"row_count":0,"truncated":false}`) {
		t.Fatalf("empty result Bytes = %d", empty.Bytes)
	}
}

func TestByteCapAfterRedaction(t *testing.T) {
	res := core.Result{Columns: columns("token"), Rows: [][]any{{strings.Repeat("x", 100)}}}
	got := Apply(res, core.Limits{MaxResultBytes: 120}, []string{"*token*"})
	if len(got.Rows) != 1 || got.Truncated || got.Bytes != encodedLen(t, got) {
		t.Fatalf("redacted result: rows %v truncated %v bytes %d", got.Rows, got.Truncated, got.Bytes)
	}
}

package postexec

import (
	"encoding/json"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kanthorlabs/sag/internal/core"
)

const (
	CellSuffix = "…[truncated]"
	Redacted   = "[REDACTED]"
)

type output struct {
	Columns         []core.Column        `json:"columns"`
	Rows            [][]any              `json:"rows"`
	RowCount        int                  `json:"row_count"`
	Truncated       bool                 `json:"truncated"`
	TruncatedReason core.TruncatedReason `json:"truncated_reason,omitempty"`
}

func Apply(res core.Result, lim core.Limits, redact []string) core.Result {
	redacted := redactedColumns(res.Columns, redact)
	rows := make([][]any, len(res.Rows))
	for i, row := range res.Rows {
		rows[i] = make([]any, len(row))
		for j, v := range row {
			if redacted[j] {
				rows[i][j] = Redacted
				continue
			}
			rows[i][j] = capCell(v, lim.MaxCellBytes)
		}
	}
	res.Rows = rows
	return capBytes(res, lim.MaxResultBytes)
}

func capCell(v any, limit int) any {
	s, ok := v.(string)
	if !ok || limit <= 0 || len(s) <= limit {
		return v
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + CellSuffix
}

func redactedColumns(cols []core.Column, patterns []string) map[int]bool {
	out := map[int]bool{}
	for i, c := range cols {
		name := strings.ToLower(c.Name)
		for _, p := range patterns {
			if ok, _ := path.Match(strings.ToLower(p), name); ok {
				out[i] = true
				break
			}
		}
	}
	return out
}

func capBytes(res core.Result, limit int) core.Result {
	res.Bytes = encode(res)
	if limit <= 0 || res.Bytes <= limit {
		return res
	}
	res.Truncated = true
	res.TruncatedReason = core.TruncatedMaxResultBytes
	base := encode(core.Result{Columns: res.Columns, Truncated: true, TruncatedReason: core.TruncatedMaxResultBytes}) - len("0")
	rowsLen := 0
	keep := 0
	for keep < len(res.Rows)-1 {
		add := encodedRow(res.Rows[keep])
		if keep > 0 {
			add += len(",")
		}
		if base+len(strconv.Itoa(keep+1))+rowsLen+add > limit {
			break
		}
		rowsLen += add
		keep++
	}
	res.Rows = res.Rows[:keep]
	res.Bytes = encode(res)
	return res
}

func encodedRow(row []any) int {
	text, err := json.Marshal(row)
	if err != nil {
		panic(err)
	}
	return len(text)
}

func encode(res core.Result) int {
	out := output{Columns: res.Columns, Rows: res.Rows, RowCount: len(res.Rows), Truncated: res.Truncated, TruncatedReason: res.TruncatedReason}
	if out.Columns == nil {
		out.Columns = []core.Column{}
	}
	if out.Rows == nil {
		out.Rows = [][]any{}
	}
	text, err := json.Marshal(out)
	if err != nil {
		panic(err)
	}
	return len(text)
}

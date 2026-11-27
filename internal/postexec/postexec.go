package postexec

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
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
			rows[i][j] = CapCell(v, lim.MaxCellBytes)
		}
	}
	res.Rows = rows
	return capBytes(res, lim.MaxResultBytes)
}

func CapCell(v any, limit int) any {
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

var errBadGlob = errors.New("bad glob")

func CompileGlob(pattern string) (*regexp.Regexp, error) {
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, err
	}
	p := []rune(strings.ToLower(pattern))
	var b strings.Builder
	b.WriteString(`(?s)^`)
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '*':
			b.WriteString(`.*`)
		case '?':
			b.WriteString(`.`)
		case '\\':
			i++
			if i >= len(p) {
				return nil, errBadGlob
			}
			b.WriteString(literal(p[i]))
		case '[':
			end, class, err := globClass(p, i+1)
			if err != nil {
				return nil, err
			}
			b.WriteString(class)
			i = end
		default:
			b.WriteString(literal(p[i]))
		}
	}
	b.WriteString(`$`)
	return regexp.Compile(b.String())
}

func globClass(p []rune, i int) (int, string, error) {
	var b strings.Builder
	b.WriteString(`[`)
	if i < len(p) && p[i] == '^' {
		b.WriteString(`^`)
		i++
	}
	ranges := 0
	for ; i < len(p); i++ {
		if p[i] == ']' && ranges > 0 {
			b.WriteString(`]`)
			return i, b.String(), nil
		}
		lo, next, err := classChar(p, i)
		if err != nil {
			return 0, "", err
		}
		i = next
		b.WriteString(literal(lo))
		if i+1 < len(p) && p[i+1] == '-' {
			hi, next, err := classChar(p, i+2)
			if err != nil {
				return 0, "", err
			}
			i = next
			b.WriteString(`-` + literal(hi))
		}
		ranges++
	}
	return 0, "", errBadGlob
}

func classChar(p []rune, i int) (rune, int, error) {
	if i >= len(p) || p[i] == '-' || p[i] == ']' {
		return 0, 0, errBadGlob
	}
	if p[i] == '\\' {
		i++
		if i >= len(p) {
			return 0, 0, errBadGlob
		}
	}
	return p[i], i, nil
}

func literal(r rune) string {
	return fmt.Sprintf(`\x{%x}`, r)
}

func redactedColumns(cols []core.Column, patterns []string) map[int]bool {
	globs := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		globs[i], _ = CompileGlob(p)
	}
	out := map[int]bool{}
	for i, c := range cols {
		name := strings.ToLower(c.Name)
		for _, g := range globs {
			if g == nil || g.MatchString(name) {
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

func CappedRowBytes(row []any, cellLimit int) int {
	capped := make([]any, len(row))
	for i, v := range row {
		capped[i] = CapCell(v, cellLimit)
	}
	return encodedRow(capped)
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

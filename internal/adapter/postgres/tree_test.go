package postgres

import (
	"errors"
	"slices"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func funcNames(t *testing.T, text string) []string {
	t.Helper()
	tree := mustParseTree(t, text)
	var names []string
	err := eachMessage(tree.Result.ProtoReflect(), func(m protoreflect.Message) error {
		if call, ok := m.Interface().(*pg_query.FuncCall); ok {
			for _, part := range call.GetFuncname() {
				names = append(names, part.GetString_().GetSval())
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("eachMessage(%q): %v", text, err)
	}
	slices.Sort(names)
	return names
}

func TestEachMessageVisitsEveryDepth(t *testing.T) {
	text := `WITH c AS (SELECT f_cte() AS x)
SELECT f_target(x), (SELECT f_sublink())
FROM c, LATERAL (SELECT f_lateral(c.x)) l
JOIN f_from() r ON f_join(r.a)
WHERE f_where(x) IN (SELECT f_in())
GROUP BY f_group(x)
HAVING f_having(x)
UNION ALL SELECT f_union(), sum(f_agg(1)) OVER (PARTITION BY f_window())
ORDER BY f_order(1)`
	want := []string{"f_agg", "f_cte", "f_from", "f_group", "f_having", "f_in", "f_join", "f_lateral", "f_order", "f_sublink", "f_target", "f_union", "f_where", "f_window", "sum"}
	if got := funcNames(t, text); !slices.Equal(got, want) {
		t.Fatalf("FuncCall names = %v; want %v", got, want)
	}
}

func TestEachMessageStopsAtFirstError(t *testing.T) {
	tree := mustParseTree(t, "SELECT f1(), f2(), f3()")
	stop := errors.New("stop")
	calls := 0
	err := eachMessage(tree.Result.ProtoReflect(), func(m protoreflect.Message) error {
		if _, ok := m.Interface().(*pg_query.FuncCall); ok {
			calls++
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("eachMessage() = %v after %d calls; want stop after 1 call", err, calls)
	}
}

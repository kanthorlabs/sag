package postgres

import "testing"

func TestRejectionReason(t *testing.T) {
	cases := []struct {
		rejection Rejection
		want      string
		message   string
	}{
		{Rejection{Node: "SelectStmt", Field: "locking_clause"}, "SelectStmt.locking_clause", "statement rejected: SelectStmt.locking_clause"},
		{Rejection{Rule: ruleFunctionDenied, Name: "set_config"}, "function_denied:set_config", "statement rejected: function_denied"},
		{Rejection{Node: "FuncCall", Field: "funcname", Rule: ruleRelation, Name: "pg_catalog.pg_authid"}, "relation:pg_catalog.pg_authid", "statement rejected: relation"},
	}
	for _, c := range cases {
		if got := c.rejection.Reason(); got != c.want {
			t.Fatalf("Reason() = %q; want %q", got, c.want)
		}
		if got := c.rejection.Message(); got != c.message {
			t.Fatalf("Message() = %q; want %q", got, c.message)
		}
		if got := c.rejection.Error(); got != "statement rejected: "+c.want {
			t.Fatalf("Error() = %q", got)
		}
	}
}

func TestQualifiedNameString(t *testing.T) {
	if got := (qualifiedName{Name: "now"}).String(); got != "now" {
		t.Fatalf("String() = %q", got)
	}
	if got := (qualifiedName{Schema: "pg_catalog", Name: "now"}).String(); got != "pg_catalog.now" {
		t.Fatalf("String() = %q", got)
	}
}

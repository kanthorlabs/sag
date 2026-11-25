package postgres

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestEveryAllowedTypeExists(t *testing.T) {
	if len(allowedNodeFields) == 0 {
		t.Fatal("allowedNodeFields is empty")
	}
	for typeName, fields := range allowedNodeFields {
		mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(pgQueryPackage + "." + typeName))
		if err != nil {
			t.Errorf("allowed type %s does not resolve in the pg_query descriptor: %v", typeName, err)
			continue
		}
		desc := mt.Descriptor()
		seen := map[string]bool{}
		for _, field := range fields {
			if seen[field] {
				t.Errorf("allowed field %s.%s is listed twice", typeName, field)
			}
			seen[field] = true
			if desc.Fields().ByName(protoreflect.Name(field)) == nil {
				t.Errorf("allowed field %s.%s does not resolve in the pg_query descriptor", typeName, field)
			}
		}
	}
}

func TestEveryAllowedTypeExistsInPlanList(t *testing.T) {
	plan := []string{
		"RawStmt", "SelectStmt", "ExplainStmt", "DefElem", "WithClause", "CommonTableExpr", "ResTarget",
		"ColumnRef", "A_Star", "A_Const", "String", "Integer", "Float", "Boolean", "A_Expr", "BoolExpr",
		"NullTest", "BooleanTest", "CaseExpr", "CaseWhen", "CoalesceExpr", "MinMaxExpr", "NullIfExpr",
		"SQLValueFunction", "FuncCall", "TypeCast", "TypeName", "CollateClause", "RangeVar", "RangeSubselect",
		"RangeFunction", "JoinExpr", "Alias", "SubLink", "SortBy", "WindowDef", "RowExpr", "A_ArrayExpr",
		"A_Indirection", "A_Indices", "GroupingSet", "List",
	}
	want := map[string]bool{"ParseResult": true}
	for _, name := range plan {
		want[name] = true
	}
	for name := range allowedNodeFields {
		if !want[name] {
			t.Errorf("allowed type %s is not in the T3.1 plan list", name)
		}
	}
	for name := range want {
		if _, ok := allowedNodeFields[name]; !ok {
			t.Errorf("plan type %s is missing from allowedNodeFields", name)
		}
	}
}

func TestEveryAllowedTypeExcludesSecurityFields(t *testing.T) {
	excluded := map[string][]string{
		"SelectStmt":       {"into_clause", "locking_clause"},
		"RangeVar":         {"catalogname"},
		"RangeFunction":    {"coldeflist"},
		"CommonTableExpr":  {"search_clause", "cycle_clause"},
		"A_Const":          {"bsval"},
		"DefElem":          {"defnamespace"},
		"ResTarget":        {"indirection"},
		"TypeName":         {"setof", "pct_type", "type_oid"},
		"JoinExpr":         {"rtindex"},
		"SubLink":          {"sub_link_id"},
		"SQLValueFunction": {"type"},
	}
	for typeName, fields := range excluded {
		for _, field := range fields {
			for _, allowed := range allowedNodeFields[typeName] {
				if allowed == field {
					t.Errorf("allowedNodeFields permits %s.%s", typeName, field)
				}
			}
		}
	}
}

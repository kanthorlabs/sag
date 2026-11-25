package postgres

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	pgQueryPackage = "pg_query"
	nodeOneofName  = "node"
)

var (
	nodeDescriptor        = (&pg_query.Node{}).ProtoReflect().Descriptor()
	defElemDescriptor     = (&pg_query.DefElem{}).ProtoReflect().Descriptor()
	parseResultDescriptor = (&pg_query.ParseResult{}).ProtoReflect().Descriptor()
)

var allowedNodeFields = map[string][]string{
	"ParseResult":      {"version", "stmts"},
	"RawStmt":          {"stmt", "stmt_location", "stmt_len"},
	"SelectStmt":       {"distinct_clause", "target_list", "from_clause", "where_clause", "group_clause", "group_distinct", "having_clause", "window_clause", "values_lists", "sort_clause", "limit_offset", "limit_count", "limit_option", "with_clause", "op", "all", "larg", "rarg"},
	"ExplainStmt":      {"query", "options"},
	"DefElem":          {"defname", "arg", "defaction", "location", "arg_location"},
	"WithClause":       {"ctes", "recursive", "location"},
	"CommonTableExpr":  {"ctename", "aliascolnames", "ctematerialized", "ctequery", "location"},
	"ResTarget":        {"name", "val", "location"},
	"ColumnRef":        {"fields", "location"},
	"A_Star":           {},
	"A_Const":          {"ival", "fval", "boolval", "sval", "isnull", "location"},
	"String":           {"sval"},
	"Integer":          {"ival"},
	"Float":            {"fval"},
	"Boolean":          {"boolval"},
	"A_Expr":           {"kind", "name", "lexpr", "rexpr", "location"},
	"BoolExpr":         {"boolop", "args", "location"},
	"NullTest":         {"arg", "nulltesttype", "location"},
	"BooleanTest":      {"arg", "booltesttype", "location"},
	"CaseExpr":         {"arg", "args", "defresult", "location"},
	"CaseWhen":         {"expr", "result", "location"},
	"CoalesceExpr":     {"args", "location"},
	"MinMaxExpr":       {"op", "args", "location"},
	"NullIfExpr":       {"args", "location"},
	"SQLValueFunction": {"op", "typmod", "location"},
	"FuncCall":         {"funcname", "args", "agg_order", "agg_filter", "over", "agg_within_group", "agg_star", "agg_distinct", "func_variadic", "funcformat", "location"},
	"TypeCast":         {"arg", "type_name", "location"},
	"TypeName":         {"names", "typmods", "typemod", "array_bounds", "location"},
	"CollateClause":    {"arg", "collname", "location"},
	"RangeVar":         {"schemaname", "relname", "inh", "relpersistence", "alias", "location"},
	"RangeSubselect":   {"lateral", "subquery", "alias"},
	"RangeFunction":    {"lateral", "ordinality", "is_rowsfrom", "functions", "alias"},
	"JoinExpr":         {"jointype", "is_natural", "larg", "rarg", "using_clause", "join_using_alias", "quals", "alias"},
	"Alias":            {"aliasname", "colnames"},
	"SubLink":          {"sub_link_type", "testexpr", "oper_name", "subselect", "location"},
	"SortBy":           {"node", "sortby_dir", "sortby_nulls", "use_op", "location"},
	"WindowDef":        {"name", "refname", "partition_clause", "order_clause", "frame_options", "start_offset", "end_offset", "location"},
	"RowExpr":          {"args", "row_format", "location"},
	"A_ArrayExpr":      {"elements", "location"},
	"A_Indirection":    {"arg", "indirection"},
	"A_Indices":        {"is_slice", "lidx", "uidx"},
	"GroupingSet":      {"kind", "content", "location"},
	"List":             {"items"},
}

var allowlist = buildAllowlist(allowedNodeFields)

func buildAllowlist(nodeFields map[string][]string) map[protoreflect.FullName]map[protoreflect.Name]bool {
	out := make(map[protoreflect.FullName]map[protoreflect.Name]bool, len(nodeFields))
	for typeName, fields := range nodeFields {
		set := make(map[protoreflect.Name]bool, len(fields))
		for _, field := range fields {
			set[protoreflect.Name(field)] = true
		}
		out[protoreflect.FullName(pgQueryPackage+"."+typeName)] = set
	}
	return out
}

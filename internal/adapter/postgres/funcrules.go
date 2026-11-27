package postgres

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const catalogSchema = "pg_catalog"

var prefixAllowedFunctions = map[string]bool{
	"pg_typeof":              true,
	"pg_size_pretty":         true,
	"pg_column_size":         true,
	"pg_relation_size":       true,
	"pg_table_size":          true,
	"pg_indexes_size":        true,
	"pg_total_relation_size": true,
	"pg_get_viewdef":         true,
	"pg_get_indexdef":        true,
	"pg_get_constraintdef":   true,
	"pg_get_expr":            true,
	"pg_get_userbyid":        true,
}

var deniedFunctionPrefixes = []string{"pg_", "lo_"}

var deniedFunctions = map[string]bool{
	"set_config":                true,
	"current_setting":           true,
	"nextval":                   true,
	"setval":                    true,
	"ts_stat":                   true,
	"ts_rewrite":                true,
	"brin_summarize_new_values": true,
	"brin_summarize_range":      true,
	"brin_desummarize_range":    true,
	"gin_clean_pending_list":    true,
	"loread":                    true,
	"lowrite":                   true,
}

var deniedFunctionFamilies = []string{
	"txid_current",
	"pg_current_xact_id",
	"query_to_xml",
	"cursor_to_xml",
	"table_to_xml",
	"schema_to_xml",
	"database_to_xml",
	"dblink",
	"binary_upgrade_",
}

var betweenOperators = map[pg_query.A_Expr_Kind][]string{
	pg_query.A_Expr_Kind_AEXPR_BETWEEN:         {">=", "<="},
	pg_query.A_Expr_Kind_AEXPR_BETWEEN_SYM:     {">=", "<="},
	pg_query.A_Expr_Kind_AEXPR_NOT_BETWEEN:     {"<", ">"},
	pg_query.A_Expr_Kind_AEXPR_NOT_BETWEEN_SYM: {"<", ">"},
}

func checkFunctionNames(t *pgTree) error {
	return eachMessage(t.Result.ProtoReflect(), func(m protoreflect.Message) error {
		switch n := m.Interface().(type) {
		case *pg_query.FuncCall:
			return addFunction(t, n.GetFuncname())
		case *pg_query.A_Expr:
			if ops, ok := betweenOperators[n.GetKind()]; ok {
				addBareOperators(t, ops...)
				return nil
			}
			return addOperator(t, n.GetName())
		case *pg_query.SubLink:
			if len(n.GetOperName()) == 0 && n.GetSubLinkType() == pg_query.SubLinkType_ANY_SUBLINK {
				addBareOperators(t, "=")
				return nil
			}
			return addOptionalOperator(t, n.GetOperName())
		case *pg_query.SortBy:
			return addOptionalOperator(t, n.GetUseOp())
		case *pg_query.CaseExpr:
			if n.GetArg() != nil {
				addBareOperators(t, "=")
			}
		case *pg_query.JoinExpr:
			if n.GetIsNatural() || len(n.GetUsingClause()) > 0 {
				addBareOperators(t, "=")
			}
		case *pg_query.A_Indirection:
			return addFieldNames(t, n.GetIndirection())
		case *pg_query.ColumnRef:
			if fields := n.GetFields(); len(fields) > 1 {
				return addFieldNames(t, fields[1:])
			}
		}
		return nil
	})
}

func addFunction(t *pgTree, parts []*pg_query.Node) error {
	name, err := splitName(parts)
	if err != nil {
		return err
	}
	if name.Schema != "" && name.Schema != catalogSchema {
		return &Rejection{Rule: ruleFunctionSchema, Name: name.String()}
	}
	if err := checkFunctionNameRules(name); err != nil {
		return err
	}
	t.Funcs = append(t.Funcs, name)
	return nil
}

func addFieldNames(t *pgTree, nodes []*pg_query.Node) error {
	for _, node := range nodes {
		field := node.GetString_()
		if field == nil {
			continue
		}
		name := qualifiedName{Name: field.GetSval()}
		if err := checkFunctionNameRules(name); err != nil {
			return err
		}
		t.FieldNames = append(t.FieldNames, name)
	}
	return nil
}

func checkFunctionNameRules(name qualifiedName) error {
	bare := strings.ToLower(name.Name)
	if isDeniedFunction(bare) {
		return &Rejection{Rule: ruleFunctionDenied, Name: name.String()}
	}
	if hasDeniedPrefix(bare) && !prefixAllowedFunctions[bare] {
		return &Rejection{Rule: ruleFunctionPrefix, Name: name.String()}
	}
	return nil
}

func isDeniedFunction(bare string) bool {
	if deniedFunctions[bare] {
		return true
	}
	for _, family := range deniedFunctionFamilies {
		if strings.HasPrefix(bare, family) {
			return true
		}
	}
	return false
}

func hasDeniedPrefix(bare string) bool {
	for _, prefix := range deniedFunctionPrefixes {
		if strings.HasPrefix(bare, prefix) {
			return true
		}
	}
	return false
}

func addOptionalOperator(t *pgTree, parts []*pg_query.Node) error {
	if len(parts) == 0 {
		return nil
	}
	return addOperator(t, parts)
}

func addOperator(t *pgTree, parts []*pg_query.Node) error {
	name, err := splitName(parts)
	if err != nil {
		return err
	}
	if name.Schema != "" && name.Schema != catalogSchema {
		return &Rejection{Rule: ruleOperatorSchema, Name: name.String()}
	}
	t.Operators = append(t.Operators, name)
	return nil
}

func addBareOperators(t *pgTree, ops ...string) {
	for _, op := range ops {
		t.Operators = append(t.Operators, qualifiedName{Name: op})
	}
}

func splitName(parts []*pg_query.Node) (qualifiedName, error) {
	words := make([]string, 0, len(parts))
	for _, part := range parts {
		words = append(words, part.GetString_().GetSval())
	}
	switch len(words) {
	case 1:
		if words[0] != "" {
			return qualifiedName{Name: words[0]}, nil
		}
	case 2:
		if words[0] != "" && words[1] != "" {
			return qualifiedName{Schema: words[0], Name: words[1]}, nil
		}
	}
	return qualifiedName{}, &Rejection{Rule: ruleNameParts, Name: strings.Join(words, ".")}
}

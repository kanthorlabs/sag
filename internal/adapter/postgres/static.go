package postgres

import (
	"strconv"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const unknownNodeType = "Node"

var allowedExplainOptions = map[string]bool{
	"format":  true,
	"verbose": true,
	"costs":   true,
}

func checkStatementShape(t *pgTree) error {
	stmts := t.Result.GetStmts()
	if len(stmts) != 1 {
		return &Rejection{Rule: ruleStatementCount, Name: strconv.Itoa(len(stmts))}
	}
	root := stmts[0].GetStmt()
	explain := root.GetExplainStmt()
	switch {
	case root.GetSelectStmt() != nil:
	case explain != nil:
		if err := checkExplain(explain); err != nil {
			return err
		}
	default:
		return &Rejection{Rule: ruleStatement, Name: nodeType(root)}
	}
	return eachMessage(root.ProtoReflect(), func(m protoreflect.Message) error {
		return checkNestedShape(m, explain)
	})
}

func checkExplain(s *pg_query.ExplainStmt) error {
	for _, option := range s.GetOptions() {
		name := option.GetDefElem().GetDefname()
		if !allowedExplainOptions[name] {
			return &Rejection{Rule: ruleExplainOption, Name: name}
		}
	}
	if s.GetQuery().GetSelectStmt() == nil {
		return &Rejection{Rule: ruleStatement, Name: nodeType(s.GetQuery())}
	}
	return nil
}

func checkNestedShape(m protoreflect.Message, explain *pg_query.ExplainStmt) error {
	switch node := m.Interface().(type) {
	case *pg_query.SelectStmt:
		if node.GetIntoClause() != nil {
			return &Rejection{Node: "SelectStmt", Field: "into_clause"}
		}
		if len(node.GetLockingClause()) > 0 {
			return &Rejection{Node: "SelectStmt", Field: "locking_clause"}
		}
	case *pg_query.ExplainStmt:
		if node != explain {
			return &Rejection{Rule: ruleStatement, Name: "ExplainStmt"}
		}
	default:
		if name := string(m.Descriptor().Name()); strings.HasSuffix(name, "Stmt") {
			return &Rejection{Rule: ruleStatement, Name: name}
		}
	}
	return nil
}

func nodeType(n *pg_query.Node) string {
	m := n.ProtoReflect()
	field := m.WhichOneof(m.Descriptor().Oneofs().ByName("node"))
	if field == nil || field.Message() == nil {
		return unknownNodeType
	}
	return string(field.Message().Name())
}

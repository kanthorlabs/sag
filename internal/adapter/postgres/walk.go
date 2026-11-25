package postgres

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const fieldUnknownFields = "unknown_fields"

type allowlistWalk struct {
	explainOptions map[proto.Message]bool
	emptyNodes     map[proto.Message]bool
}

func walkAllowlist(t *pgTree) error {
	if t == nil || t.Result == nil {
		return &Rejection{Rule: ruleNode, Name: string(parseResultDescriptor.Name())}
	}
	w := &allowlistWalk{
		explainOptions: map[proto.Message]bool{},
		emptyNodes:     map[proto.Message]bool{},
	}
	return eachMessage(t.Result.ProtoReflect(), w.visit)
}

func (w *allowlistWalk) visit(m protoreflect.Message) error {
	desc := m.Descriptor()
	if len(m.GetUnknown()) > 0 {
		return &Rejection{Node: string(desc.Name()), Field: fieldUnknownFields}
	}
	if desc.FullName() == nodeDescriptor.FullName() {
		return w.visitNode(m)
	}
	fields, ok := allowlist[desc.FullName()]
	if !ok || (desc.FullName() == defElemDescriptor.FullName() && !w.explainOptions[m.Interface()]) {
		return &Rejection{Rule: ruleNode, Name: string(desc.Name())}
	}
	var err error
	m.Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if !fields[fd.Name()] {
			err = &Rejection{Node: string(desc.Name()), Field: string(fd.Name())}
		}
		return err == nil
	})
	if err != nil {
		return err
	}
	w.markContext(m.Interface())
	return nil
}

func (w *allowlistWalk) visitNode(m protoreflect.Message) error {
	if m.WhichOneof(nodeDescriptor.Oneofs().ByName(nodeOneofName)) == nil && !w.emptyNodes[m.Interface()] {
		return &Rejection{Node: string(nodeDescriptor.Name()), Field: nodeOneofName}
	}
	return nil
}

func (w *allowlistWalk) markContext(msg proto.Message) {
	switch v := msg.(type) {
	case *pg_query.ExplainStmt:
		for _, option := range v.GetOptions() {
			if d := option.GetDefElem(); d != nil {
				w.explainOptions[d] = true
			}
		}
	case *pg_query.SelectStmt:
		if distinct := v.GetDistinctClause(); len(distinct) == 1 && distinct[0] != nil {
			w.emptyNodes[distinct[0]] = true
		}
	case *pg_query.RangeFunction:
		for _, function := range v.GetFunctions() {
			if items := function.GetList().GetItems(); len(items) == 2 && items[1] != nil {
				w.emptyNodes[items[1]] = true
			}
		}
	}
}

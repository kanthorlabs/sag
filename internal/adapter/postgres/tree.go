package postgres

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type qualifiedName struct {
	Schema string
	Name   string
}

func (n qualifiedName) String() string {
	if n.Schema == "" {
		return n.Name
	}
	return n.Schema + "." + n.Name
}

type pgTree struct {
	Result    *pg_query.ParseResult
	Funcs     []qualifiedName
	Operators []qualifiedName
	Relations []qualifiedName
	Validated bool
}

func eachMessage(m protoreflect.Message, fn func(protoreflect.Message) error) error {
	if err := fn(m); err != nil {
		return err
	}
	var err error
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsMap():
			err = &Rejection{Rule: ruleNode, Name: string(fd.FullName())}
		case fd.IsList() && fd.Message() != nil:
			list := v.List()
			for i := 0; i < list.Len() && err == nil; i++ {
				err = eachMessage(list.Get(i).Message(), fn)
			}
		case fd.Message() != nil:
			err = eachMessage(v.Message(), fn)
		}
		return err == nil
	})
	return err
}

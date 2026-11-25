package postgres

import (
	"strconv"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const maxTreeDepth = 500

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
	Result     *pg_query.ParseResult
	Funcs      []qualifiedName
	FieldNames []qualifiedName
	Operators  []qualifiedName
	Validated  bool
}

func eachMessage(m protoreflect.Message, fn func(protoreflect.Message) error) error {
	return eachMessageAt(m, fn, 1)
}

func eachMessageAt(m protoreflect.Message, fn func(protoreflect.Message) error, depth int) error {
	if depth > maxTreeDepth {
		return &Rejection{Rule: ruleDepth, Name: strconv.Itoa(maxTreeDepth)}
	}
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
				err = eachMessageAt(list.Get(i).Message(), fn, depth+1)
			}
		case fd.Message() != nil:
			err = eachMessageAt(v.Message(), fn, depth+1)
		}
		return err == nil
	})
	return err
}

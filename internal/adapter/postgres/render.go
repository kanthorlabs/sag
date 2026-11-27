package postgres

import (
	"strings"
	"unicode/utf8"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/kanthorlabs/sag/internal/core"
)

const (
	renderInvalidUTF8  = "invalid_utf8"
	renderReparse      = "reparse"
	renderTreeMismatch = "tree_mismatch"
	renderLexer        = "lexer"
)

var positionFields = []protoreflect.Name{"location", "stmt_location", "stmt_len"}

func deparseAndRevalidate(t *pgTree) (string, error) {
	if t == nil || t.Result == nil || !t.Validated {
		return "", core.NewError(core.CodeQueryRejected, msgTreeNotValidated)
	}
	rendered, again, err := renderAndReparse(t.Result)
	if err != nil {
		return "", err
	}
	if err := validateStaticTree(again); err != nil {
		return "", err
	}
	return rendered, nil
}

func renderAndReparse(original *pg_query.ParseResult) (string, *pgTree, error) {
	rendered, err := render(original)
	if err != nil {
		return "", nil, err
	}
	if !utf8.ValidString(rendered) || strings.IndexByte(rendered, 0) >= 0 {
		return "", nil, renderRejection(renderInvalidUTF8)
	}
	if err := checkLexerIndependent(rendered); err != nil {
		return "", nil, err
	}
	again, err := pg_query.Parse(rendered)
	if err != nil {
		return "", nil, renderRejection(renderReparse)
	}
	same, err := sameTree(original, again)
	if err != nil {
		return "", nil, err
	}
	if !same {
		return "", nil, renderRejection(renderTreeMismatch)
	}
	return rendered, &pgTree{Result: again}, nil
}

func checkLexerIndependent(text string) error {
	scan, err := pg_query.Scan(text)
	if err != nil {
		return renderRejection(renderLexer)
	}
	for _, token := range scan.GetTokens() {
		switch token.GetToken() {
		case pg_query.Token_SCONST:
			if !scsIndependentString(text[token.GetStart():token.GetEnd()]) {
				return renderRejection(renderLexer)
			}
		case pg_query.Token_USCONST, pg_query.Token_UIDENT, pg_query.Token_SQL_COMMENT, pg_query.Token_C_COMMENT:
			return renderRejection(renderLexer)
		}
	}
	return nil
}

func scsIndependentString(literal string) bool {
	switch {
	case strings.HasPrefix(literal, "E'"), strings.HasPrefix(literal, "e'"):
		return true
	case strings.HasPrefix(literal, "'"):
		return !strings.Contains(literal, `\`)
	default:
		return false
	}
}

func sameTree(a, b proto.Message) (bool, error) {
	na, err := withoutPositions(a)
	if err != nil {
		return false, err
	}
	nb, err := withoutPositions(b)
	if err != nil {
		return false, err
	}
	return proto.Equal(na, nb), nil
}

func withoutPositions(m proto.Message) (proto.Message, error) {
	clone := proto.Clone(m)
	err := eachMessage(clone.ProtoReflect(), func(msg protoreflect.Message) error {
		fields := msg.Descriptor().Fields()
		for _, name := range positionFields {
			if fd := fields.ByName(name); fd != nil && fd.Kind() == protoreflect.Int32Kind {
				msg.Clear(fd)
			}
		}
		return nil
	})
	return clone, err
}

func renderRejection(name string) error {
	return &Rejection{Rule: ruleRender, Name: name}
}

package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

const (
	sqlStateReadOnly    = "25006"
	msgForeignTree      = "statement tree is not a PostgreSQL parse tree"
	msgTreeNotValidated = "statement tree did not pass static validation"
	msgNULByte          = "statement holds a NUL byte"
	msgRejected         = "statement rejected"
)

var _ sql.Dialect = (*dialect)(nil)

type dialect struct {
	cfg    core.ResourceConfig
	target approval.Target
}

func newDialect(cfg core.ResourceConfig, target approval.Target) *dialect {
	return &dialect{cfg: cfg, target: target}
}

func (d *dialect) EngineGuard() sql.EngineGuard {
	return sql.EngineGuardTransaction
}

func (d *dialect) SessionGuard() []string {
	return sessionGuard()
}

func (d *dialect) RequestBegin(lim core.Limits) []string {
	return requestBegin(lim, d.cfg)
}

func (d *dialect) Parse(text string) (sql.Tree, error) {
	if strings.IndexByte(text, 0) >= 0 {
		return nil, core.NewError(core.CodeQueryRejected, msgNULByte)
	}
	result, err := parse(text)
	if err != nil {
		return nil, err
	}
	return &pgTree{Result: result}, nil
}

func (d *dialect) ValidateStatic(t sql.Tree) error {
	tree, err := asPGTree(t)
	if err != nil {
		return err
	}
	return queryRejected(validateStaticTree(tree))
}

func (d *dialect) Render(t sql.Tree) (string, error) {
	tree, err := validatedTree(t)
	if err != nil {
		return "", err
	}
	text, err := deparseAndRevalidate(tree)
	if err != nil {
		return "", queryRejected(err)
	}
	return text, nil
}

func (d *dialect) ValidateCatalog(ctx context.Context, tx sql.CatalogReader, t sql.Tree) error {
	tree, err := validatedTree(t)
	if err != nil {
		return err
	}
	return queryRejected(validateCatalog(ctx, tx, tree))
}

func (d *dialect) PlanRowCap(t sql.Tree, rendered string, maxRows int) (sql.RowCapPlan, error) {
	tree, err := validatedTree(t)
	if err != nil {
		return sql.RowCapPlan{}, err
	}
	plan, err := planRowCap(tree, rendered, maxRows)
	if err != nil {
		return sql.RowCapPlan{}, queryRejected(err)
	}
	return plan, nil
}

func (d *dialect) ViolationCodes() []string {
	return []string{sqlStateReadOnly}
}

func validateStaticTree(t *pgTree) error {
	t.Funcs, t.FieldNames, t.Operators, t.Validated = nil, nil, nil, false
	for _, check := range []func(*pgTree) error{
		checkStatementShape,
		walkAllowlist,
		checkRelations,
		checkFunctionNames,
	} {
		if err := check(t); err != nil {
			return err
		}
	}
	t.Validated = true
	return nil
}

func asPGTree(t sql.Tree) (*pgTree, error) {
	tree, ok := t.(*pgTree)
	if !ok || tree == nil || tree.Result == nil {
		return nil, core.NewError(core.CodeQueryRejected, msgForeignTree)
	}
	return tree, nil
}

func validatedTree(t sql.Tree) (*pgTree, error) {
	tree, err := asPGTree(t)
	if err != nil {
		return nil, err
	}
	if !tree.Validated {
		return nil, core.NewError(core.CodeQueryRejected, msgTreeNotValidated)
	}
	return tree, nil
}

func queryRejected(err error) error {
	if err == nil {
		return nil
	}
	var rejection *Rejection
	if errors.As(err, &rejection) {
		return &core.Error{Code: core.CodeQueryRejected, Message: rejection.Message(), Cause: rejection}
	}
	var coreErr *core.Error
	var engineErr *sql.EngineError
	if errors.As(err, &coreErr) || errors.As(err, &engineErr) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &core.Error{Code: core.CodeQueryRejected, Message: msgRejected, Cause: err}
}

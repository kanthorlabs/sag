package postgres

import (
	"context"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/kanthorlabs/sag/internal/approval"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

const (
	sqlStateReadOnly        = "25006"
	msgForeignTree          = "statement tree is not a PostgreSQL parse tree"
	msgCatalogCheckDisabled = "catalog validation is not available"
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
	return parse(text)
}

func (d *dialect) ValidateStatic(t sql.Tree) error {
	tree, err := parseTree(t)
	if err != nil {
		return err
	}
	return validateStatic(tree)
}

func (d *dialect) Render(t sql.Tree) (string, error) {
	tree, err := parseTree(t)
	if err != nil {
		return "", err
	}
	return render(tree)
}

func (d *dialect) ValidateCatalog(ctx context.Context, tx sql.CatalogReader, t sql.Tree) error {
	return core.NewError(core.CodeQueryRejected, msgCatalogCheckDisabled)
}

func (d *dialect) PlanRowCap(t sql.Tree, rendered string, maxRows int) (sql.RowCapPlan, error) {
	tree, err := parseTree(t)
	if err != nil {
		return sql.RowCapPlan{}, err
	}
	return planRowCap(tree, rendered, maxRows)
}

func (d *dialect) ViolationCodes() []string {
	return []string{sqlStateReadOnly}
}

func parseTree(t sql.Tree) (*pg_query.ParseResult, error) {
	tree, ok := t.(*pg_query.ParseResult)
	if !ok || tree == nil {
		return nil, core.NewError(core.CodeQueryRejected, msgForeignTree)
	}
	return tree, nil
}

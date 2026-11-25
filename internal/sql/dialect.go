package sql

import (
	"context"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

type EngineGuard string

const (
	EngineGuardTransaction EngineGuard = "transaction"
	EngineGuardAPI         EngineGuard = "api"
)

const CatalogTimeout = 10 * time.Second

type Tree any

type RowCapPlan struct {
	Statement string
	Fetch     string
}

type CatalogReader interface {
	Query(ctx context.Context, text string, args ...any) (Rows, error)
}

type Dialect interface {
	EngineGuard() EngineGuard
	SessionGuard() []string
	RequestBegin(lim core.Limits) []string
	Parse(text string) (Tree, error)
	ValidateStatic(t Tree) error
	Render(t Tree) (string, error)
	ValidateCatalog(ctx context.Context, tx CatalogReader, t Tree) error
	PlanRowCap(t Tree, rendered string, maxRows int) (RowCapPlan, error)
	Preflight(ctx context.Context, conn CatalogReader) (core.PreflightReport, error)
	ViolationCodes() []string
}

type Opener interface {
	OpenSQL(ctx context.Context, cfg core.ResourceConfig, cred secret.Credential) (Dialect, Pool, error)
}

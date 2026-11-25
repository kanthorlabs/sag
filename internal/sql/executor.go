package sql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
)

const (
	rollbackStatement = "ROLLBACK"
	rollbackTimeout   = 5 * time.Second
	timeoutSlack      = 2 * time.Second
	MaxSQLBytes       = 16 * 1024
)

const (
	messageTimeout     = "request timed out"
	messageUnavailable = "resource unavailable"
	messageInternal    = "internal error"
	messageUpstream    = "upstream error, SQLSTATE "
	messageRejected    = "statement rejected"
	messageSQLTooLarge = "sql exceeds 16384 bytes"
)

var errTruncatedRead = errors.New("truncated read without a cursor")

type executor struct {
	dialect Dialect
	pool    Pool
}

func NewExecutor(d Dialect, p Pool) core.Resource {
	return &executor{dialect: d, pool: p}
}

func (e *executor) Preflight(ctx context.Context) (core.PreflightReport, error) {
	var report core.PreflightReport
	err := runReadOnly(ctx, e.dialect, e.pool.Dedicated, core.Limits{Timeout: CatalogTimeout}, func(ctx context.Context, c Conn) error {
		var err error
		report, err = e.dialect.Preflight(ctx, c)
		return err
	})
	return report, err
}

func (e *executor) Execute(ctx context.Context, capability, operation string, input json.RawMessage, lim core.Limits) (core.Result, error) {
	if capability != CapabilityName {
		return core.Result{}, core.NewError(core.CodeInvalidArgument, "unknown capability")
	}
	text, err := operationText(operation, input)
	if err != nil {
		return core.Result{}, err
	}
	if len(text) > MaxSQLBytes {
		return core.Result{}, core.NewError(core.CodeInvalidArgument, messageSQLTooLarge)
	}
	trace := core.Trace{Input: text, InputSHA256: sha256Hex(text)}
	tree, rendered, err := e.validateStatic(text)
	if rendered != "" {
		trace.RenderedSHA256 = sha256Hex(rendered)
	}
	if err != nil {
		return core.Result{}, withTrace(rejected(err), trace)
	}
	var result core.Result
	err = runReadOnly(ctx, e.dialect, e.pool.Acquire, lim, func(ctx context.Context, c Conn) error {
		var err error
		result, err = e.run(ctx, c, tree, rendered, lim.MaxRows)
		return err
	})
	if err != nil && !errors.Is(err, errTruncatedRead) {
		return core.Result{}, withTrace(engineFailure(err), trace)
	}
	result.Trace = trace
	return result, nil
}

func (e *executor) Close() error {
	e.pool.Close()
	return nil
}

func (e *executor) validateStatic(text string) (Tree, string, error) {
	tree, err := e.dialect.Parse(text)
	if err != nil {
		return nil, "", err
	}
	if err := e.dialect.ValidateStatic(tree); err != nil {
		return nil, "", err
	}
	rendered, err := e.dialect.Render(tree)
	if err != nil {
		return nil, "", err
	}
	tree, err = e.dialect.Parse(rendered)
	if err != nil {
		return nil, rendered, err
	}
	if err := e.dialect.ValidateStatic(tree); err != nil {
		return nil, rendered, err
	}
	return tree, rendered, nil
}

func (e *executor) run(ctx context.Context, c Conn, tree Tree, rendered string, maxRows int) (core.Result, error) {
	if err := e.dialect.ValidateCatalog(ctx, c, tree); err != nil {
		return core.Result{}, rejected(err)
	}
	plan, err := e.dialect.PlanRowCap(tree, rendered, maxRows)
	if err != nil {
		return core.Result{}, rejected(err)
	}
	rows, err := openRows(ctx, c, plan)
	if err != nil {
		return core.Result{}, err
	}
	return readRows(rows, maxRows, plan.Fetch == "")
}

func openRows(ctx context.Context, c Conn, plan RowCapPlan) (Rows, error) {
	if plan.Fetch == "" {
		return c.Query(ctx, plan.Statement)
	}
	if err := c.Exec(ctx, plan.Statement); err != nil {
		return nil, err
	}
	return c.Query(ctx, plan.Fetch)
}

func readRows(rows Rows, maxRows int, cursorless bool) (core.Result, error) {
	defer rows.Close()
	result := core.Result{Columns: rows.Columns(), Rows: [][]any{}}
	for rows.Next() {
		if len(result.Rows) >= maxRows {
			result.Truncated = true
			result.TruncatedReason = core.TruncatedMaxRows
			if cursorless {
				rows.Abort()
				return result, errTruncatedRead
			}
			break
		}
		values, err := rows.Values()
		if err != nil {
			return core.Result{}, err
		}
		row := make([]any, len(values))
		for i, v := range values {
			row[i] = encodeValue(v)
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return core.Result{}, err
	}
	return result, nil
}

func RunReadOnly(ctx context.Context, d Dialect, p Pool, lim core.Limits, fn func(ctx context.Context, r CatalogReader) error) error {
	return runReadOnly(ctx, d, p.Acquire, lim, func(ctx context.Context, c Conn) error {
		return fn(ctx, c)
	})
}

func runReadOnly(ctx context.Context, d Dialect, acquire func(ctx context.Context) (Conn, error), lim core.Limits, fn func(ctx context.Context, c Conn) error) error {
	if lim.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, lim.Timeout+timeoutSlack)
		defer cancel()
	}
	conn, err := acquire(ctx)
	if err != nil {
		return err
	}
	finished := false
	defer func() {
		if !finished {
			_ = conn.Close()
		}
	}()
	err = requestBegin(ctx, conn, d.RequestBegin(lim))
	if err == nil {
		err = fn(ctx, conn)
	}
	if rollbackErr := rollback(ctx, conn); rollbackErr != nil && (err == nil || errors.Is(err, errTruncatedRead)) {
		err = rollbackErr
	}
	finished = true
	if err != nil {
		_ = conn.Close()
		return err
	}
	conn.Release()
	return nil
}

func requestBegin(ctx context.Context, c Conn, statements []string) error {
	for _, s := range statements {
		if err := c.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func rollback(ctx context.Context, c Conn) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	return c.Exec(ctx, rollbackStatement)
}

func rejected(err error) error {
	var ce *core.Error
	var ee *EngineError
	if errors.As(err, &ce) || errors.As(err, &ee) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &core.Error{Code: core.CodeQueryRejected, Message: messageRejected, Cause: err}
}

func engineFailure(err error) error {
	var ce *core.Error
	if errors.As(err, &ce) {
		switch ce.Code {
		case core.CodeTimeout:
			return &core.Error{Code: core.CodeTimeout, Message: messageTimeout, Cause: err}
		case core.CodeResourceUnavailable:
			return &core.Error{Code: core.CodeResourceUnavailable, Message: messageUnavailable, Cause: err}
		default:
			return err
		}
	}
	var ee *EngineError
	if errors.As(err, &ee) {
		switch ee.Class {
		case Timeout:
			return &core.Error{Code: core.CodeTimeout, Message: messageTimeout, Cause: err}
		case Connect:
			return &core.Error{Code: core.CodeResourceUnavailable, Message: messageUnavailable, Cause: err}
		default:
			return &core.Error{Code: core.CodeUpstreamError, Message: messageUpstream + ee.SQLState, Cause: err}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &core.Error{Code: core.CodeTimeout, Message: messageTimeout, Cause: err}
	}
	return &core.Error{Code: core.CodeInternal, Message: messageInternal, Cause: err}
}

func withTrace(err error, trace core.Trace) error {
	var ce *core.Error
	if !errors.As(err, &ce) {
		return err
	}
	traced := *ce
	traced.Trace = trace
	return &traced
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

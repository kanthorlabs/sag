package config

import (
	"fmt"
	"time"
)

const idleTimeoutMargin = 5 * time.Second

var defaultLimits = Limits{
	MaxRows:               100,
	MaxResultBytes:        32 << 10,
	MaxCellBytes:          4 << 10,
	StatementTimeout:      15 * time.Second,
	LockTimeout:           2 * time.Second,
	ConnectTimeout:        5 * time.Second,
	MaxConcurrentPerAgent: 20,
	PoolSize:              5,
}

var hardMaximums = Limits{
	MaxRows:                         1000,
	MaxResultBytes:                  2 << 20,
	MaxCellBytes:                    8 << 10,
	StatementTimeout:                30 * time.Second,
	LockTimeout:                     2 * time.Second,
	IdleInTransactionSessionTimeout: 35 * time.Second,
	ConnectTimeout:                  5 * time.Second,
	PoolSize:                        20,
}

const minLimitDuration = time.Millisecond

func checkLimits(path string, l rawLimits, inResource bool, p *problems) {
	checkCount(p, path+".max_rows", l.MaxRows, hardMaximums.MaxRows)
	checkCount(p, path+".max_result_bytes", l.MaxResultBytes, hardMaximums.MaxResultBytes)
	checkCount(p, path+".max_cell_bytes", l.MaxCellBytes, hardMaximums.MaxCellBytes)
	checkTimeout(p, path+".statement_timeout", l.StatementTimeout, hardMaximums.StatementTimeout)
	checkTimeout(p, path+".lock_timeout", l.LockTimeout, hardMaximums.LockTimeout)
	checkTimeout(p, path+".idle_in_transaction_session_timeout", l.IdleInTransactionSessionTimeout, hardMaximums.IdleInTransactionSessionTimeout)
	checkTimeout(p, path+".connect_timeout", l.ConnectTimeout, hardMaximums.ConnectTimeout)
	checkCount(p, path+".pool_size", l.PoolSize, hardMaximums.PoolSize)
	if inResource {
		if l.MaxConcurrentPerAgent != nil {
			p.add(path+".max_concurrent_per_agent", "is allowed only in defaults.limits")
		}
		return
	}
	checkCount(p, path+".max_concurrent_per_agent", l.MaxConcurrentPerAgent, 0)
}

func checkCount(p *problems, path string, v *int, max int) {
	switch {
	case v == nil:
	case *v < 1:
		p.add(path, "must be at least 1")
	case max > 0 && *v > max:
		p.add(path, fmt.Sprintf("%d exceeds the hard maximum %d", *v, max))
	}
}

func checkTimeout(p *problems, path string, v *time.Duration, max time.Duration) {
	switch {
	case v == nil:
	case *v < minLimitDuration:
		p.add(path, "must be at least "+minLimitDuration.String())
	case *v > max:
		p.add(path, fmt.Sprintf("%s exceeds the hard maximum %s", *v, max))
	}
}

func overlay[T any](over, base *T) *T {
	if over != nil {
		return over
	}
	return base
}

func mergeLimits(base, over rawLimits) rawLimits {
	return rawLimits{
		MaxRows:                         overlay(over.MaxRows, base.MaxRows),
		MaxResultBytes:                  overlay(over.MaxResultBytes, base.MaxResultBytes),
		MaxCellBytes:                    overlay(over.MaxCellBytes, base.MaxCellBytes),
		StatementTimeout:                overlay(over.StatementTimeout, base.StatementTimeout),
		LockTimeout:                     overlay(over.LockTimeout, base.LockTimeout),
		IdleInTransactionSessionTimeout: overlay(over.IdleInTransactionSessionTimeout, base.IdleInTransactionSessionTimeout),
		ConnectTimeout:                  overlay(over.ConnectTimeout, base.ConnectTimeout),
		MaxConcurrentPerAgent:           overlay(over.MaxConcurrentPerAgent, base.MaxConcurrentPerAgent),
		PoolSize:                        overlay(over.PoolSize, base.PoolSize),
	}
}

func valueOr[T any](v *T, fallback T) T {
	if v != nil {
		return *v
	}
	return fallback
}

func resolveLimits(l rawLimits) Limits {
	out := Limits{
		MaxRows:               valueOr(l.MaxRows, defaultLimits.MaxRows),
		MaxResultBytes:        valueOr(l.MaxResultBytes, defaultLimits.MaxResultBytes),
		MaxCellBytes:          valueOr(l.MaxCellBytes, defaultLimits.MaxCellBytes),
		StatementTimeout:      valueOr(l.StatementTimeout, defaultLimits.StatementTimeout),
		LockTimeout:           valueOr(l.LockTimeout, defaultLimits.LockTimeout),
		ConnectTimeout:        valueOr(l.ConnectTimeout, defaultLimits.ConnectTimeout),
		MaxConcurrentPerAgent: valueOr(l.MaxConcurrentPerAgent, defaultLimits.MaxConcurrentPerAgent),
		PoolSize:              valueOr(l.PoolSize, defaultLimits.PoolSize),
	}
	out.IdleInTransactionSessionTimeout = valueOr(l.IdleInTransactionSessionTimeout, out.StatementTimeout+idleTimeoutMargin)
	return out
}

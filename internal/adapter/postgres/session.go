package postgres

import (
	"fmt"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
)

func sessionGuard() []string {
	return []string{"SET default_transaction_read_only = on"}
}

func requestBegin(lim core.Limits, cfg core.ResourceConfig) []string {
	return []string{
		"BEGIN TRANSACTION READ ONLY",
		fmt.Sprintf("SET LOCAL statement_timeout = %d", milliseconds(lim.Timeout)),
		fmt.Sprintf("SET LOCAL lock_timeout = %d", milliseconds(cfg.LockTimeout)),
		fmt.Sprintf("SET LOCAL idle_in_transaction_session_timeout = %d", milliseconds(cfg.IdleInTransactionSessionTimeout)),
		"SET LOCAL standard_conforming_strings = on",
		"SET LOCAL client_encoding = 'UTF8'",
	}
}

func milliseconds(d time.Duration) int64 {
	ms := (d + time.Millisecond - 1) / time.Millisecond
	return max(int64(ms), 1)
}

package daemon

import (
	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

func ResourceConfig(name string, r config.Resource, creds map[string]secret.Credential, approvals core.ApprovalVerifier) core.ResourceConfig {
	rc := core.ResourceConfig{
		Name:                            name,
		Kind:                            r.Kind,
		TLS:                             r.TLS,
		PoolSize:                        r.Limits.PoolSize,
		ConnectTimeout:                  r.Limits.ConnectTimeout,
		LockTimeout:                     r.Limits.LockTimeout,
		IdleInTransactionSessionTimeout: r.Limits.IdleInTransactionSessionTimeout,
		Approvals:                       approvals,
	}
	if r.TLSCA != "" {
		rc.TLSCA = creds[string(r.TLSCA)].Reveal()
	}
	return rc
}

func resourceLimits(l config.Limits) core.Limits {
	return core.Limits{
		MaxRows:        l.MaxRows,
		MaxResultBytes: l.MaxResultBytes,
		MaxCellBytes:   l.MaxCellBytes,
		Timeout:        l.StatementTimeout,
	}
}

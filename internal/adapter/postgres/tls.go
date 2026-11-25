package postgres

import "github.com/kanthorlabs/sag/internal/core"

func checkTLS(mode core.TLSMode, host string, sessionSSL bool) (core.PreflightCheck, string) {
	return core.PreflightCheck{}, ""
}

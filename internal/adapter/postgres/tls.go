package postgres

import "github.com/kanthorlabs/sag/internal/core"

const checkTLSName = "tls"

const (
	tlsDetailLoopback    = "loopback IP literal host, TLS not required"
	tlsDetailActive      = "TLS active, verify-full"
	tlsDetailInactive    = "TLS not active for the session"
	tlsDetailSkipVerify  = "TLS opt-out insecure-skip-verify"
	tlsDetailDisable     = "TLS opt-out disable"
	tlsDetailUnknownMode = "unknown tls mode"
	tlsWarningSkipVerify = "tls insecure-skip-verify: the server certificate and host name are not verified"
	tlsWarningDisable    = "tls disable: the connection is not encrypted"
)

func checkTLS(mode core.TLSMode, host string, sessionSSL bool) (core.PreflightCheck, string) {
	switch mode {
	case core.TLSInsecureSkipVerify:
		return core.PreflightCheck{Name: checkTLSName, Passed: true, Detail: tlsDetailSkipVerify}, tlsWarningSkipVerify
	case core.TLSDisable:
		return core.PreflightCheck{Name: checkTLSName, Passed: true, Detail: tlsDetailDisable}, tlsWarningDisable
	case core.TLSVerifyFull, "":
		if isLoopbackIPLiteral(host) {
			return core.PreflightCheck{Name: checkTLSName, Passed: true, Detail: tlsDetailLoopback}, ""
		}
		if sessionSSL {
			return core.PreflightCheck{Name: checkTLSName, Passed: true, Detail: tlsDetailActive}, ""
		}
		return core.PreflightCheck{Name: checkTLSName, Passed: false, Detail: tlsDetailInactive}, ""
	default:
		return core.PreflightCheck{Name: checkTLSName, Passed: false, Detail: tlsDetailUnknownMode}, ""
	}
}

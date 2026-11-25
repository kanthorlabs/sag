package postgres

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

const defaultPort = 5432

var (
	errDSNFormat      = errors.New("connection secret is not a postgres URL with user, password, host, port, and database only")
	errDSNQuery       = errors.New("connection secret has query parameters or a fragment")
	errDSNMultiHost   = errors.New("connection secret has more than one host")
	errTLSCAEmpty     = errors.New("tls_ca has no PEM certificate")
	errTLSModeUnknown = errors.New("unknown tls mode")
)

type dsnFields struct {
	host     string
	port     uint16
	database string
	user     string
	password string
}

func connConfig(cfg core.ResourceConfig, cred secret.Credential) (*pgx.ConnConfig, error) {
	f, err := parseDSN(string(cred.Reveal()))
	if err != nil {
		return nil, err
	}
	tlsConfig, err := tlsConfigFor(cfg, f.host)
	if err != nil {
		return nil, err
	}
	cc, err := pgx.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("base pgx config: %w", err)
	}
	cc.Host = f.host
	cc.Port = f.port
	cc.Database = f.database
	cc.User = f.user
	cc.Password = f.password
	cc.TLSConfig = tlsConfig
	cc.Fallbacks = nil
	cc.RuntimeParams = map[string]string{"client_encoding": "UTF8"}
	cc.ConnectTimeout = cfg.ConnectTimeout
	cc.DefaultQueryExecMode = pgx.QueryExecModeExec
	return cc, nil
}

func parseDSN(dsn string) (dsnFields, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsnFields{}, errDSNFormat
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" {
		return dsnFields{}, errDSNQuery
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return dsnFields{}, errDSNFormat
	}
	if strings.Contains(u.Host, ",") {
		return dsnFields{}, errDSNMultiHost
	}
	if u.Opaque != "" || u.User == nil || u.User.Username() == "" {
		return dsnFields{}, errDSNFormat
	}
	host := u.Hostname()
	if host == "" {
		return dsnFields{}, errDSNFormat
	}
	port := uint16(defaultPort)
	if p := u.Port(); p != "" {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil || n == 0 {
			return dsnFields{}, errDSNFormat
		}
		port = uint16(n)
	}
	database := strings.TrimPrefix(u.Path, "/")
	if database == "" || strings.Contains(database, "/") {
		return dsnFields{}, errDSNFormat
	}
	password, _ := u.User.Password()
	return dsnFields{host: host, port: port, database: database, user: u.User.Username(), password: password}, nil
}

func tlsConfigFor(cfg core.ResourceConfig, host string) (*tls.Config, error) {
	switch cfg.TLS {
	case core.TLSDisable:
		return nil, nil
	case core.TLSInsecureSkipVerify:
		return &tls.Config{InsecureSkipVerify: true, ServerName: host, MinVersion: tls.VersionTLS12}, nil
	case core.TLSVerifyFull, "":
		if isLoopbackIPLiteral(host) {
			return nil, nil
		}
		roots, err := rootCAs(cfg.TLSCA)
		if err != nil {
			return nil, err
		}
		return &tls.Config{RootCAs: roots, ServerName: host, MinVersion: tls.VersionTLS12}, nil
	default:
		return nil, errTLSModeUnknown
	}
}

func isLoopbackIPLiteral(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func rootCAs(bundle []byte) (*x509.CertPool, error) {
	if len(bundle) == 0 {
		return nil, nil
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("system CA pool: %w", err)
	}
	if !roots.AppendCertsFromPEM(bundle) {
		return nil, errTLSCAEmpty
	}
	return roots, nil
}

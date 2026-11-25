package config

import (
	"errors"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
)

const DefaultPath = "/etc/sag/sag.yaml"

type SecretRef string

type Config struct {
	Version   int
	Server    Server
	Auth      Auth
	Preflight Preflight
	Defaults  Defaults
	Resources map[string]Resource
}

type Server struct {
	Listen         string
	TLSCert        SecretRef
	TLSKey         SecretRef
	AllowedOrigins []string
}

type Auth struct {
	Version       int
	TokenLifetime time.Duration
}

type Preflight struct {
	RecheckInterval time.Duration
}

type Defaults struct {
	Limits        Limits
	RedactColumns []string
}

type Limits struct {
	MaxRows                         int
	MaxResultBytes                  int
	MaxCellBytes                    int
	StatementTimeout                time.Duration
	LockTimeout                     time.Duration
	IdleInTransactionSessionTimeout time.Duration
	ConnectTimeout                  time.Duration
	MaxConcurrentPerAgent           int
	PoolSize                        int
}

type Resource struct {
	Name          string
	Kind          string
	Description   string
	Connection    SecretRef
	TLS           core.TLSMode
	TLSCA         SecretRef
	Agents        []string
	Limits        Limits
	RedactColumns []string
}

func Load(path string) (*Config, error) {
	return nil, errors.ErrUnsupported
}

func Parse(data []byte) (*Config, error) {
	return nil, errors.ErrUnsupported
}

func (c *Config) SecretNames() []string {
	return nil
}

func (c *Config) HasAgent(sub string) bool {
	return false
}

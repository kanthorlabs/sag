package config

import "time"

type rawConfig struct {
	Version   *int                   `yaml:"version"`
	Server    rawServer              `yaml:"server"`
	Auth      rawAuth                `yaml:"auth"`
	Preflight rawPreflight           `yaml:"preflight"`
	Defaults  rawDefaults            `yaml:"defaults"`
	Resources map[string]rawResource `yaml:"resources"`
}

type rawServer struct {
	Listen         *string  `yaml:"listen"`
	TLSCert        string   `yaml:"tls_cert"`
	TLSKey         string   `yaml:"tls_key"`
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type rawAuth struct {
	Version       *int           `yaml:"version"`
	TokenLifetime *time.Duration `yaml:"token_lifetime"`
}

type rawPreflight struct {
	RecheckInterval *time.Duration `yaml:"recheck_interval"`
}

type rawDefaults struct {
	Limits rawLimits `yaml:"limits"`
	Redact rawRedact `yaml:"redact"`
}

type rawRedact struct {
	Columns []string `yaml:"columns"`
}

type rawLimits struct {
	MaxRows                         *int           `yaml:"max_rows"`
	MaxResultBytes                  *int           `yaml:"max_result_bytes"`
	MaxCellBytes                    *int           `yaml:"max_cell_bytes"`
	StatementTimeout                *time.Duration `yaml:"statement_timeout"`
	LockTimeout                     *time.Duration `yaml:"lock_timeout"`
	IdleInTransactionSessionTimeout *time.Duration `yaml:"idle_in_transaction_session_timeout"`
	ConnectTimeout                  *time.Duration `yaml:"connect_timeout"`
	MaxConcurrentPerAgent           *int           `yaml:"max_concurrent_per_agent"`
	PoolSize                        *int           `yaml:"pool_size"`
}

type rawResource struct {
	Kind        string    `yaml:"kind"`
	Description string    `yaml:"description"`
	Connection  string    `yaml:"connection"`
	TLS         string    `yaml:"tls"`
	TLSCA       string    `yaml:"tls_ca"`
	Agents      []string  `yaml:"agents"`
	Limits      rawLimits `yaml:"limits"`
	Redact      rawRedact `yaml:"redact"`
}

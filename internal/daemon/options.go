package daemon

import (
	"io"

	"github.com/kanthorlabs/sag/internal/sql"
)

type Options struct {
	ConfigPath  string
	SecretsDir  string
	SecretUID   int
	StateDir    string
	Version     string
	Log         io.Writer
	WrapDialect func(sql.Dialect) sql.Dialect

	testAWSLoader AWSLoader
}

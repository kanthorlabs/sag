package bootstrap

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const sagYAMLTemplate = `version: 1
server:
  listen: 0.0.0.0:8443
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
auth:
  version: 1
  token_lifetime: 8760h
resources: {}
`

func createSAGConfig(cfgDir string) (bool, error) {
	path := filepath.Join(cfgDir, "sag.yaml")
	_, err := os.Stat(path)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.WriteFile(path, []byte(sagYAMLTemplate), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

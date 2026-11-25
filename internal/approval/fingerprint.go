package approval

import "errors"

type Target struct {
	Resource string
	Host     string
	Port     uint16
	Database string
	User     string
}

type Extension struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Schema  string `json:"schema"`
}

type Objects struct {
	Extensions []Extension `json:"extensions"`
	Languages  []string    `json:"languages"`
}

func Fingerprint(t Target, o Objects) (string, error) {
	return "", errors.ErrUnsupported
}

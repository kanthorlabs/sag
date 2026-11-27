package approval

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

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

type fingerprintDocument struct {
	Resource   string      `json:"resource"`
	Host       string      `json:"host"`
	Port       uint16      `json:"port"`
	Database   string      `json:"database"`
	User       string      `json:"user"`
	Extensions []Extension `json:"extensions"`
	Languages  []string    `json:"languages"`
}

func Fingerprint(t Target, o Objects) (string, error) {
	c := canonical(o)
	doc, err := json.Marshal(fingerprintDocument{
		Resource:   t.Resource,
		Host:       t.Host,
		Port:       t.Port,
		Database:   t.Database,
		User:       t.User,
		Extensions: c.Extensions,
		Languages:  c.Languages,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(doc)
	return hex.EncodeToString(sum[:]), nil
}

func canonical(o Objects) Objects {
	extensions := append([]Extension{}, o.Extensions...)
	slices.SortFunc(extensions, func(a, b Extension) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Version, b.Version), cmp.Compare(a.Schema, b.Schema))
	})
	languages := append([]string{}, o.Languages...)
	slices.Sort(languages)
	return Objects{Extensions: extensions, Languages: languages}
}

package deploy

import "github.com/kanthorlabs/sag/internal/config"

var ImageDigest = ""

type ProductFlags struct {
	ReadOnly        bool
	ReadOnlyTmpfs   bool
	UID             int
	GID             int
	CapDrop         string
	NoNewPrivileges bool
	PublishHost     string
	ContainerPort   int
	ConfigTarget    string
	StateTarget     string
	StateOptions    string
	SecretType      string
	SecretUID       int
	SecretMode      string
}

type ContainerFlags struct {
	Product  ProductFlags
	LogFlags LogFlags
}

type LogFlags struct {
	Driver  string
	MaxSize string
}

type RunOpts struct {
	Image        string
	ConfigSource string
	Autostart    bool
	RestartPolicy string
}

func RunArgs(cfg *config.Config, opts RunOpts) []string {
	panic("not implemented")
}

func Quadlet(cfg *config.Config, opts RunOpts) string {
	panic("not implemented")
}

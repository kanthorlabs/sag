package service

import "github.com/kanthorlabs/sag/internal/podman"

type Config struct {
	Podman  podman.Command
	CfgDir  string
	UID     string
	Home    string
	BinPath string
	Image   string
}

package daemon

import "io"

type Options struct {
	ConfigPath string
	SecretsDir string
	SecretUID  int
	StateDir   string
	Version    string
	Log        io.Writer
}

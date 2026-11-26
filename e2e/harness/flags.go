package harness

import (
	"github.com/kanthorlabs/sag/internal/deploy"
)

const (
	ConfigPath        = "/etc/sag/sag.yaml"
	StateDir          = "/var/lib/sag"
	AuditDir          = StateDir + "/audit"
	ListenPort        = 8443
	PublishHost       = "127.0.0.1"
	DefaultSecretMode = "0400"
)

var ContainerFlags = deploy.DefaultFlags

var TestOnlyFlags []string

func SecretFlag(source, target, mode string) string {
	return ContainerFlags.Product.Secret(source, target, mode)
}

type secretMount struct {
	Source string
	Target string
}

type runSpec struct {
	Name        string
	Network     string
	ConfigFile  string
	StateVolume string
	AuditDir    string
	Busybox     string
	Secrets     []secretMount
	SecretMode  string
	Image       string
	Command     []string
}

func runArgs(s runSpec) []string {
	p := ContainerFlags.Product
	l := ContainerFlags.LogFlags

	args := []string{"run", "-d", "--name", s.Name}
	if s.Network != "" {
		args = append(args, "--network", s.Network)
	}
	args = append(args, "--log-driver", l.Driver)
	args = append(args, "--log-opt", "max-size="+l.MaxSize)
	args = append(args, p.SecurityArgs()...)
	args = append(args, "-p", p.Publish(0))
	if s.ConfigFile != "" {
		args = append(args, "-v", p.ConfigVolume(s.ConfigFile))
	}
	if s.StateVolume != "" {
		args = append(args, "-v", p.StateVolume(s.StateVolume))
	}
	if s.AuditDir != "" {
		args = append(args, "-v", s.AuditDir+":"+AuditDir+":ro")
	}
	if s.Busybox != "" {
		args = append(args, "-v", s.Busybox+":"+BusyboxTarget+":ro")
	}
	for _, m := range s.Secrets {
		args = append(args, "--secret", SecretFlag(m.Source, m.Target, s.SecretMode))
	}
	args = append(args, s.Image)
	return append(args, s.Command...)
}

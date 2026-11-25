package harness

import (
	"strconv"

	"github.com/kanthorlabs/sag/internal/secret"
)

const (
	ConfigPath        = "/etc/sag/sag.yaml"
	StateDir          = "/var/lib/sag"
	AuditDir          = StateDir + "/audit"
	ListenPort        = 8443
	PublishHost       = "127.0.0.1"
	DefaultSecretMode = "0400"
)

var SecurityFlags = []string{
	"--read-only",
	"--cap-drop=all",
	"--security-opt=no-new-privileges",
	"--user", strconv.Itoa(secret.ContainerUID) + ":" + strconv.Itoa(secret.ContainerUID),
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
	Secrets     []secretMount
	SecretMode  string
	Image       string
	Command     []string
}

func SecretFlag(source, target, mode string) string {
	if mode == "" {
		mode = DefaultSecretMode
	}
	return source + ",type=mount,target=" + target + ",uid=" + strconv.Itoa(secret.ContainerUID) + ",mode=" + mode
}

func runArgs(s runSpec) []string {
	args := []string{"run", "-d", "--name", s.Name}
	if s.Network != "" {
		args = append(args, "--network", s.Network)
	}
	args = append(args, SecurityFlags...)
	args = append(args, "-p", PublishHost+"::"+strconv.Itoa(ListenPort))
	if s.ConfigFile != "" {
		args = append(args, "-v", s.ConfigFile+":"+ConfigPath+":ro")
	}
	if s.StateVolume != "" {
		args = append(args, "-v", s.StateVolume+":"+StateDir)
	}
	if s.AuditDir != "" {
		args = append(args, "-v", s.AuditDir+":"+AuditDir+":ro")
	}
	for _, m := range s.Secrets {
		args = append(args, "--secret", SecretFlag(m.Source, m.Target, s.SecretMode))
	}
	args = append(args, s.Image)
	return append(args, s.Command...)
}

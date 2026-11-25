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

var ContainerFlags = ProductFlags{
	ReadOnly:        true,
	ReadOnlyTmpfs:   false,
	UID:             secret.ContainerUID,
	GID:             secret.ContainerUID,
	CapDrop:         "all",
	NoNewPrivileges: true,
	PublishHost:     PublishHost,
	ContainerPort:   ListenPort,
	ConfigTarget:    ConfigPath,
	StateTarget:     StateDir,
	StateOptions:    "noexec",
	SecretType:      "mount",
	SecretUID:       secret.ContainerUID,
	SecretMode:      DefaultSecretMode,
}

var TestOnlyFlags = []string{"--log-driver", "k8s-file"}

func (f ProductFlags) SecurityArgs() []string {
	var args []string
	if f.ReadOnly {
		args = append(args, "--read-only")
	}
	args = append(args, "--read-only-tmpfs="+strconv.FormatBool(f.ReadOnlyTmpfs))
	args = append(args, "--cap-drop="+f.CapDrop)
	if f.NoNewPrivileges {
		args = append(args, "--security-opt=no-new-privileges")
	}
	return append(args, "--user", strconv.Itoa(f.UID)+":"+strconv.Itoa(f.GID))
}

func (f ProductFlags) Publish(hostPort int) string {
	host := ""
	if hostPort != 0 {
		host = strconv.Itoa(hostPort)
	}
	return f.PublishHost + ":" + host + ":" + strconv.Itoa(f.ContainerPort)
}

func (f ProductFlags) ConfigVolume(source string) string {
	return source + ":" + f.ConfigTarget + ":ro"
}

func (f ProductFlags) StateVolume(source string) string {
	return source + ":" + f.StateTarget + ":" + f.StateOptions
}

func (f ProductFlags) Secret(source, target, mode string) string {
	if mode == "" {
		mode = f.SecretMode
	}
	return source + ",type=" + f.SecretType + ",target=" + target + ",uid=" + strconv.Itoa(f.SecretUID) + ",mode=" + mode
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

func SecretFlag(source, target, mode string) string {
	return ContainerFlags.Secret(source, target, mode)
}

func runArgs(s runSpec) []string {
	args := []string{"run", "-d", "--name", s.Name}
	if s.Network != "" {
		args = append(args, "--network", s.Network)
	}
	args = append(args, TestOnlyFlags...)
	args = append(args, ContainerFlags.SecurityArgs()...)
	args = append(args, "-p", ContainerFlags.Publish(0))
	if s.ConfigFile != "" {
		args = append(args, "-v", ContainerFlags.ConfigVolume(s.ConfigFile))
	}
	if s.StateVolume != "" {
		args = append(args, "-v", ContainerFlags.StateVolume(s.StateVolume))
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

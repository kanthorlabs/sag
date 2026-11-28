package deploy

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/secret"
)

var ImageDigest = ""

const StateVolumeName = "sag-state"

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

func (f LogFlags) Args() []string {
	return []string{"--log-driver", f.Driver, "--log-opt", "max-size=" + f.MaxSize}
}

type RunOpts struct {
	Image         string
	ConfigSource  string
	Autostart     bool
	RestartPolicy string
}

var DefaultFlags = ContainerFlags{
	Product: ProductFlags{
		ReadOnly:        true,
		ReadOnlyTmpfs:   false,
		UID:             secret.ContainerUID,
		GID:             secret.ContainerUID,
		CapDrop:         "all",
		NoNewPrivileges: true,
		PublishHost:     "127.0.0.1",
		ContainerPort:   8443,
		ConfigTarget:    "/etc/sag/sag.yaml",
		StateTarget:     "/var/lib/sag",
		StateOptions:    "noexec",
		SecretType:      "mount",
		SecretUID:       secret.ContainerUID,
		SecretMode:      "0400",
	},
	LogFlags: LogFlags{
		Driver:  "k8s-file",
		MaxSize: "10mb",
	},
}

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

func RunArgs(cfg *config.Config, opts RunOpts) []string {
	p := DefaultFlags.Product
	l := DefaultFlags.LogFlags

	args := []string{"run", "-d", "--replace", "--pull=never", "--name", "sag"}
	args = append(args, "--log-driver", l.Driver)
	args = append(args, "--log-opt", "max-size="+l.MaxSize)
	args = append(args, p.SecurityArgs()...)
	args = append(args, "-p", p.Publish(p.ContainerPort))
	args = append(args, "-v", p.ConfigVolume(opts.ConfigSource))
	args = append(args, "-v", p.StateVolume(StateVolumeName))
	if opts.RestartPolicy != "" {
		args = append(args, "--restart="+opts.RestartPolicy)
	}
	for _, name := range SecretMounts(cfg) {
		args = append(args, "--secret", p.Secret(name, name, ""))
	}
	args = append(args, opts.Image)
	return append(args, "serve")
}

func Quadlet(cfg *config.Config, opts RunOpts) string {
	p := DefaultFlags.Product
	l := DefaultFlags.LogFlags

	var b strings.Builder

	fmt.Fprintln(&b, "[Unit]")
	fmt.Fprintln(&b, "Description=SAG Secure Access Gateway")
	fmt.Fprintln(&b, "Wants=network-online.target")
	fmt.Fprintln(&b, "After=network-online.target")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "[Container]")
	fmt.Fprintln(&b, "ContainerName=sag")
	fmt.Fprintln(&b, "Image="+opts.Image)
	fmt.Fprintln(&b, "Pull=never")
	fmt.Fprintln(&b, "Exec=serve")
	fmt.Fprintln(&b, "ReadOnly="+strconv.FormatBool(p.ReadOnly))
	fmt.Fprintln(&b, "ReadOnlyTmpfs="+strconv.FormatBool(p.ReadOnlyTmpfs))
	fmt.Fprintln(&b, "User="+strconv.Itoa(p.UID))
	fmt.Fprintln(&b, "Group="+strconv.Itoa(p.GID))
	fmt.Fprintln(&b, "DropCapability="+p.CapDrop)
	fmt.Fprintln(&b, "NoNewPrivileges="+strconv.FormatBool(p.NoNewPrivileges))
	fmt.Fprintln(&b, "PublishPort="+p.Publish(p.ContainerPort))
	fmt.Fprintln(&b, "LogDriver="+l.Driver)
	fmt.Fprintln(&b, "PodmanArgs=--log-opt=max-size="+l.MaxSize)
	fmt.Fprintln(&b, "Volume="+p.ConfigVolume(opts.ConfigSource))
	fmt.Fprintln(&b, "Volume="+p.StateVolume(StateVolumeName))
	for _, name := range SecretMounts(cfg) {
		fmt.Fprintln(&b, "Secret="+p.Secret(name, name, ""))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "[Service]")
	fmt.Fprintln(&b, "Restart="+opts.RestartPolicy)
	fmt.Fprintln(&b, "RestartSec=10")

	if opts.Autostart {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "[Install]")
		fmt.Fprintln(&b, "WantedBy=default.target")
	}

	return b.String()
}

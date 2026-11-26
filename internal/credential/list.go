package credential

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/stepline"
)

const (
	backendPodman      = "podman"
	roleConn           = "connection"
	roleTLSCA          = "tls_ca"
	renewThresholdDays = 30 * 24 * time.Hour
)

type SecretEntry struct {
	Name      string     `json:"name"`
	Backend   string     `json:"backend"`
	Role      string     `json:"role"`
	Exists    *bool      `json:"exists"`
	Owned     bool       `json:"owned"`
	CreatedAt *time.Time `json:"created_at"`
	Mounted   *bool      `json:"mounted,omitempty"`
}

type Entry struct {
	Name             string        `json:"name"`
	Kind             string        `json:"kind"`
	Description      string        `json:"description"`
	TLS              string        `json:"tls"`
	Agents           []string      `json:"agents"`
	Secrets          []SecretEntry `json:"secrets"`
	ContainerRunning *bool         `json:"container_running,omitempty"`
}

type podmanInspector interface {
	ContainerInspect(ctx context.Context, name string) (podman.ContainerInfo, error)
	Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error)
}

type ListOptions struct {
	SAGYaml string
	CAPem   string
	Backend secret.SecureStorage
	Podman  podmanInspector
	Stderr  io.Writer
}

type InspectOptions struct {
	SAGYaml string
	CAPem   string
	Backend secret.SecureStorage
	Podman  podmanInspector
	Stderr  io.Writer
}

func List(ctx context.Context, opts ListOptions) ([]Entry, error) {
	cfg, err := loadConfig(opts.SAGYaml)
	if err != nil {
		return nil, err
	}

	warnExpiry(opts.CAPem, opts.Stderr, time.Now())

	mountedSecrets, _ := getMountedSecrets(ctx, opts.Podman)

	names := slices.Sorted(maps.Keys(cfg.Resources))
	entries := make([]Entry, 0, len(names))
	for _, name := range names {
		r := cfg.Resources[name]
		e, err := buildEntry(ctx, name, r, opts.Backend, mountedSecrets, opts.Stderr)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func Inspect(ctx context.Context, name string, opts InspectOptions) (Entry, error) {
	cfg, err := loadConfig(opts.SAGYaml)
	if err != nil {
		return Entry{}, err
	}

	r, ok := cfg.Resources[name]
	if !ok {
		return Entry{}, fmt.Errorf("resource %s not found", name)
	}

	warnExpiry(opts.CAPem, opts.Stderr, time.Now())

	mountedSecrets, _ := getMountedSecrets(ctx, opts.Podman)

	e, err := buildEntry(ctx, name, r, opts.Backend, mountedSecrets, opts.Stderr)
	if err != nil {
		return Entry{}, err
	}

	running := containerRunning(ctx, opts.Podman)
	e.ContainerRunning = &running
	return e, nil
}

func loadConfig(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &config.Config{}, nil
		}
		return nil, err
	}
	return cfg, nil
}

func warnExpiry(caPEM string, w io.Writer, now time.Time) {
	if w == nil || caPEM == "" {
		return
	}
	data, err := os.ReadFile(caPEM)
	if err != nil {
		return
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return
	}
	if cert.NotAfter.Sub(now) < renewThresholdDays {
		date := cert.NotAfter.UTC().Format("2006-01-02")
		sw := stepline.New(w)
		sw.Line(stepline.StatusWarn, "endpoint", "certificate expires "+date+"; run sag bootstrap")
	}
}

func getMountedSecrets(ctx context.Context, pm podmanInspector) (map[string]bool, error) {
	if pm == nil {
		return nil, nil
	}
	info, err := pm.ContainerInspect(ctx, "sag")
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(info.Secrets))
	for _, s := range info.Secrets {
		m[s.Name] = true
	}
	return m, nil
}

func containerRunning(ctx context.Context, pm podmanInspector) bool {
	if pm == nil {
		return false
	}
	out, err := pm.Run(ctx, nil, "container", "inspect", "sag", "--format", "{{.State.Running}}")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func buildEntry(
	ctx context.Context,
	name string,
	r config.Resource,
	backend secret.SecureStorage,
	mountedSecrets map[string]bool,
	stderr io.Writer,
) (Entry, error) {
	agents := r.Agents
	if agents == nil {
		agents = []string{}
	}

	e := Entry{
		Name:        name,
		Kind:        r.Kind,
		Description: r.Description,
		TLS:         string(r.TLS),
		Agents:      agents,
		Secrets:     []SecretEntry{},
	}

	type secretSpec struct {
		name string
		role string
	}
	specs := []secretSpec{{name: string(r.Connection), role: roleConn}}
	if r.TLSCA != "" {
		specs = append(specs, secretSpec{name: string(r.TLSCA), role: roleTLSCA})
	}

	unreachableWarned := false
	for _, sp := range specs {
		se, warned, err := buildSecretEntry(ctx, sp.name, sp.role, name, backend, mountedSecrets)
		if err != nil {
			return Entry{}, err
		}
		if warned && !unreachableWarned {
			unreachableWarned = true
			if stderr != nil {
				sw := stepline.New(stderr)
				sw.Line(stepline.StatusWarn, "backend", backendPodman+" unreachable; secret metadata unavailable")
			}
		}
		e.Secrets = append(e.Secrets, se)
	}
	return e, nil
}

func buildSecretEntry(
	ctx context.Context,
	ref, role, resourceName string,
	backend secret.SecureStorage,
	mountedSecrets map[string]bool,
) (SecretEntry, bool, error) {
	secretName := ref
	b := backendPodman

	se := SecretEntry{
		Name:    secretName,
		Backend: b,
		Role:    role,
	}

	unreachable := false
	if backend != nil {
		meta, err := backend.Metadata(ctx, secretName)
		if err != nil {
			if errors.Is(err, secret.ErrNotFound) {
				exists := false
				se.Exists = &exists
			} else if errors.Is(err, secret.ErrUnavailable) {
				unreachable = true
			} else {
				return SecretEntry{}, false, err
			}
		} else {
			exists := true
			se.Exists = &exists
			se.CreatedAt = &meta.CreatedAt
			se.Owned = meta.Labels["sag.resource"] == resourceName
		}
	}

	if b == backendPodman {
		mounted := mountedSecrets[secretName]
		se.Mounted = &mounted
	}

	return se, unreachable, nil
}

package podman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type RunError struct {
	ExitCode int
	Stderr   string
}

func (e *RunError) Error() string {
	return fmt.Sprintf("exit status %d: %s", e.ExitCode, e.Stderr)
}

var ErrEmptyInspect = errors.New("podman inspect returned empty list")

type Runner struct {
	Path string
}

func (r Runner) Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.Path, args...)
	cmd.Stdin = stdin
	out, err := cmd.Output()
	if err != nil {
		return nil, runError(err)
	}
	return out, nil
}

func runError(err error) *RunError {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return &RunError{ExitCode: exit.ExitCode(), Stderr: strings.TrimSpace(string(exit.Stderr))}
	}
	return &RunError{ExitCode: -1, Stderr: err.Error()}
}

type runner struct {
	exec func(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error)
}

func fromRunner(r Runner) runner {
	return runner{exec: r.Run}
}

func (r Runner) SecretCreate(ctx context.Context, name string, value io.Reader, labels map[string]string) error {
	return fromRunner(r).SecretCreate(ctx, name, value, labels)
}

func (r Runner) SecretInspect(ctx context.Context, name string) (SecretInfo, error) {
	return fromRunner(r).SecretInspect(ctx, name)
}

func (r Runner) SecretList(ctx context.Context) ([]string, error) {
	return fromRunner(r).SecretList(ctx)
}

func (r Runner) SecretRemove(ctx context.Context, name string) error {
	return fromRunner(r).SecretRemove(ctx, name)
}

func (r Runner) ContainerInspect(ctx context.Context, name string) (ContainerInfo, error) {
	return fromRunner(r).ContainerInspect(ctx, name)
}

func (r runner) SecretCreate(ctx context.Context, name string, value io.Reader, labels map[string]string) error {
	args := []string{"secret", "create"}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--label", k+"="+labels[k])
	}
	args = append(args, name, "-")
	_, err := r.exec(ctx, value, args...)
	return err
}

func (r runner) SecretInspect(ctx context.Context, name string) (SecretInfo, error) {
	out, err := r.exec(ctx, nil, "secret", "inspect", name)
	if err != nil {
		return SecretInfo{}, err
	}
	var infos []rawSecretInfo
	if err := json.Unmarshal(out, &infos); err != nil {
		return SecretInfo{}, err
	}
	if len(infos) == 0 {
		return SecretInfo{}, ErrEmptyInspect
	}
	return infos[0].toSecretInfo(), nil
}

func (r runner) SecretList(ctx context.Context) ([]string, error) {
	out, err := r.exec(ctx, nil, "secret", "ls", "--format", "{{.Name}}")
	if err != nil {
		return nil, err
	}
	return parseNames(out), nil
}

func parseNames(b []byte) []string {
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (r runner) SecretRemove(ctx context.Context, name string) error {
	_, err := r.exec(ctx, nil, "secret", "rm", name)
	return err
}

func (r runner) ContainerInspect(ctx context.Context, name string) (ContainerInfo, error) {
	out, err := r.exec(ctx, nil, "container", "inspect", name)
	if err != nil {
		return ContainerInfo{}, err
	}
	var infos []rawContainerInfo
	if err := json.Unmarshal(out, &infos); err != nil {
		return ContainerInfo{}, err
	}
	if len(infos) == 0 {
		return ContainerInfo{}, ErrEmptyInspect
	}
	return infos[0].toContainerInfo(), nil
}

type rawSecretInfo struct {
	ID   string `json:"ID"`
	Spec struct {
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
	} `json:"Spec"`
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

func (r rawSecretInfo) toSecretInfo() SecretInfo {
	labels := r.Spec.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return SecretInfo{
		ID:        r.ID,
		Spec:      SecretSpec{Name: r.Spec.Name, Labels: labels},
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

type rawContainerInfo struct {
	Id      string            `json:"Id"`
	Name    string            `json:"Name"`
	Secrets []rawContainerSec `json:"Secrets"`
	Config  struct {
		Secrets []rawContainerSec `json:"Secrets"`
	} `json:"Config"`
}

type rawContainerSec struct {
	Name string `json:"Name"`
	UID  int    `json:"UID"`
	GID  int    `json:"GID"`
	Mode uint32 `json:"Mode"`
}

func (r rawContainerInfo) toContainerInfo() ContainerInfo {
	all := r.Secrets
	if len(all) == 0 {
		all = r.Config.Secrets
	}
	secrets := make([]ContainerSecret, len(all))
	for i, s := range all {
		secrets[i] = ContainerSecret{Name: s.Name, UID: s.UID, GID: s.GID, Mode: s.Mode}
	}
	name := r.Name
	name = strings.TrimPrefix(name, "/")
	return ContainerInfo{ID: r.Id, Name: name, Secrets: secrets}
}

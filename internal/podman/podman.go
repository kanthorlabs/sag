package podman

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"time"
)

type Runner struct {
	Path string
}

func (r Runner) Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.Path, args...)
	cmd.Stdin = stdin
	return cmd.Output()
}

type runner struct {
	exec func(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error)
}

func fromRunner(r Runner) runner {
	return runner{exec: func(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, r.Path, args...)
		cmd.Stdin = stdin
		out, err := cmd.Output()
		return out, err
	}}
}

func (r Runner) SecretCreate(ctx context.Context, name string, value io.Reader, labels map[string]string) error {
	return fromRunner(r).SecretCreate(ctx, name, value, labels)
}

func (r Runner) SecretInspect(ctx context.Context, name string) (SecretInfo, error) {
	return fromRunner(r).SecretInspect(ctx, name)
}

func (r Runner) SecretList(ctx context.Context) ([]SecretInfo, error) {
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
	for k, v := range labels {
		args = append(args, "--label", k+"="+v)
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
		return SecretInfo{}, io.EOF
	}
	return infos[0].toSecretInfo(), nil
}

func (r runner) SecretList(ctx context.Context) ([]SecretInfo, error) {
	out, err := r.exec(ctx, nil, "secret", "ls", "--format", "json")
	if err != nil {
		return nil, err
	}
	var raw []rawSecretInfo
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	infos := make([]SecretInfo, len(raw))
	for i, r := range raw {
		infos[i] = r.toSecretInfo()
	}
	return infos, nil
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
		return ContainerInfo{}, io.EOF
	}
	return infos[0].toContainerInfo(), nil
}

type rawSecretInfo struct {
	ID   string `json:"ID"`
	Spec struct {
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
	} `json:"Spec"`
	CreatedAt string `json:"CreatedAt"`
	UpdatedAt string `json:"UpdatedAt"`
}

func (r rawSecretInfo) toSecretInfo() SecretInfo {
	created, _ := time.Parse(time.RFC3339Nano, r.CreatedAt)
	updated, _ := time.Parse(time.RFC3339Nano, r.UpdatedAt)
	labels := r.Spec.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return SecretInfo{
		ID:        r.ID,
		Spec:      SecretSpec{Name: r.Spec.Name, Labels: labels},
		CreatedAt: created,
		UpdatedAt: updated,
	}
}

type rawContainerInfo struct {
	Id      string            `json:"Id"`
	Name    string            `json:"Name"`
	Secrets []rawContainerSec `json:"Secrets"`
}

type rawContainerSec struct {
	Name string `json:"Name"`
	UID  int    `json:"UID"`
	GID  int    `json:"GID"`
	Mode uint32 `json:"Mode"`
}

func (r rawContainerInfo) toContainerInfo() ContainerInfo {
	secrets := make([]ContainerSecret, len(r.Secrets))
	for i, s := range r.Secrets {
		secrets[i] = ContainerSecret{Name: s.Name, UID: s.UID, GID: s.GID, Mode: s.Mode}
	}
	name := r.Name
	name = strings.TrimPrefix(name, "/")
	return ContainerInfo{ID: r.Id, Name: name, Secrets: secrets}
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no secret with name") ||
		strings.Contains(msg, "no such secret") ||
		strings.Contains(msg, "not found")
}


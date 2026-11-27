package testenv

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	MajorEnv     = "SAG_TEST_PG_MAJOR"
	KeepEnv      = "SAG_E2E_KEEP"
	DefaultMajor = 17
)

const (
	minMajor = 14
	maxMajor = 19
)

type Run struct {
	ID      string
	Network string
	Start   time.Time
	CA      *CA

	mu         sync.Mutex
	password   string
	containers []string
	secrets    []string
}

func MajorFromEnv() (int, error) {
	value := os.Getenv(MajorEnv)
	if value == "" {
		return DefaultMajor, nil
	}
	major, err := strconv.Atoi(value)
	if err != nil || major < minMajor || major > maxMajor {
		return 0, fmt.Errorf("%s=%q: want an integer from %d to %d", MajorEnv, value, minMajor, maxMajor)
	}
	return major, nil
}

func NewRun(ctx context.Context) (*Run, error) {
	id, err := randomHex(4)
	if err != nil {
		return nil, err
	}
	ca, err := NewCA()
	if err != nil {
		return nil, err
	}
	r := &Run{ID: id, Network: "sag-e2e-" + id, Start: time.Now(), CA: ca}
	if err := podmanOK(ctx, nil, "network", "create", r.Network); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Run) CreateSecret(ctx context.Context, name string, data []byte) error {
	if err := podmanOK(ctx, bytes.NewReader(data), "secret", "create", name, "-"); err != nil {
		return err
	}
	r.mu.Lock()
	r.secrets = append(r.secrets, name)
	r.mu.Unlock()
	return nil
}

func (r *Run) Close(ctx context.Context) error {
	if os.Getenv(KeepEnv) == "1" {
		return nil
	}
	r.mu.Lock()
	containers := r.containers
	secrets := r.secrets
	r.containers, r.secrets = nil, nil
	r.mu.Unlock()
	var errs []error
	if len(containers) > 0 {
		errs = append(errs, podmanOK(ctx, nil, append([]string{"rm", "--force", "--ignore", "--volumes", "--time", "0"}, containers...)...))
	}
	errs = append(errs, podmanOK(ctx, nil, "network", "rm", "--force", r.Network))
	if len(secrets) > 0 {
		errs = append(errs, podmanOK(ctx, nil, append([]string{"secret", "rm"}, secrets...)...))
	}
	return errors.Join(errs...)
}

func (r *Run) addContainer(name string) {
	r.mu.Lock()
	r.containers = append(r.containers, name)
	r.mu.Unlock()
}

func (r *Run) postgresPassword(ctx context.Context) (string, string, error) {
	name := "pg-password-" + r.ID
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.password != "" {
		return name, r.password, nil
	}
	password, err := randomHex(32)
	if err != nil {
		return "", "", err
	}
	if err := podmanOK(ctx, bytes.NewReader([]byte(password)), "secret", "create", name, "-"); err != nil {
		return "", "", err
	}
	r.secrets = append(r.secrets, name)
	r.password = password
	return name, password, nil
}

func podmanOK(ctx context.Context, stdin io.Reader, args ...string) error {
	res, err := Podman(ctx, stdin, args...)
	if err != nil {
		return fmt.Errorf("podman %s: %w", args[0], err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("podman %s: exit %d: %s", args[0], res.ExitCode, res.Stderr)
	}
	return nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

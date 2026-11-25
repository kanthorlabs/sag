package testenv

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	containerPort  = 5432
	superuser      = "postgres"
	database       = "postgres"
	hostName       = "localhost"
	readyTimeout   = 2 * time.Minute
	readyInterval  = 500 * time.Millisecond
	secretDir      = "/run/secrets/"
	postgresUIDGID = "999"
)

var images = map[int]string{
	13: "docker.io/library/postgres:13-bookworm",
	14: "docker.io/library/postgres:14.25-bookworm",
	15: "docker.io/library/postgres:15.20-bookworm",
	16: "docker.io/library/postgres:16.16-bookworm",
	17: "docker.io/library/postgres:17.12-bookworm",
	18: "docker.io/library/postgres:18.7-bookworm",
	19: "docker.io/library/postgres:19.0-bookworm",
}

type Postgres struct {
	Major     int
	Container string
	HostPort  int
	CAPEM     []byte

	password string
	run      *Run
	ownsRun  bool
}

func StartPostgres(ctx context.Context, major int) (*Postgres, error) {
	run, err := NewRun(ctx)
	if err != nil {
		return nil, err
	}
	p, err := run.StartPostgres(ctx, major)
	if err != nil {
		return nil, errors.Join(err, run.Close(context.WithoutCancel(ctx)))
	}
	p.ownsRun = true
	return p, nil
}

func (r *Run) StartPostgres(ctx context.Context, major int) (*Postgres, error) {
	image, ok := images[major]
	if !ok {
		return nil, fmt.Errorf("no PostgreSQL image for major %d", major)
	}
	name := fmt.Sprintf("sag-e2e-%s-pg%d", r.ID, major)
	passwordSecret, password, err := r.postgresPassword(ctx)
	if err != nil {
		return nil, err
	}
	certPEM, keyPEM, err := r.CA.Issue(name, hostName)
	if err != nil {
		return nil, err
	}
	certSecret := fmt.Sprintf("pg-tls-cert-%s-pg%d", r.ID, major)
	keySecret := fmt.Sprintf("pg-tls-key-%s-pg%d", r.ID, major)
	if err := r.CreateSecret(ctx, certSecret, certPEM); err != nil {
		return nil, err
	}
	if err := r.CreateSecret(ctx, keySecret, keyPEM); err != nil {
		return nil, err
	}
	r.addContainer(name)
	err = podmanOK(ctx, nil,
		"run", "--detach",
		"--name", name,
		"--network", r.Network,
		"--publish", fmt.Sprintf("127.0.0.1::%d", containerPort),
		"--secret", secretMount(passwordSecret, "pg-password", "0400"),
		"--secret", secretMount(certSecret, "server.crt", "0644"),
		"--secret", secretMount(keySecret, "server.key", "0600"),
		"--env", "POSTGRES_PASSWORD_FILE="+secretDir+"pg-password",
		image,
		"-c", "ssl=on",
		"-c", "ssl_cert_file="+secretDir+"server.crt",
		"-c", "ssl_key_file="+secretDir+"server.key",
		"-c", "log_statement=all",
	)
	if err != nil {
		return nil, err
	}
	port, err := hostPort(ctx, name)
	if err != nil {
		return nil, err
	}
	p := &Postgres{Major: major, Container: name, HostPort: port, CAPEM: r.CA.CertPEM, password: password, run: r}
	if err := p.waitReady(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Postgres) HostDSN() string {
	return p.dsn(net.JoinHostPort(hostName, strconv.Itoa(p.HostPort)))
}

func (p *Postgres) NetworkDSN() string {
	return p.dsn(net.JoinHostPort(p.Container, strconv.Itoa(containerPort)))
}

func (p *Postgres) dsn(host string) string {
	u := url.URL{Scheme: "postgres", User: url.UserPassword(superuser, p.password), Host: host, Path: "/" + database}
	return u.String()
}

func (p *Postgres) Connect(ctx context.Context) (*pgx.Conn, error) {
	cfg, err := pgx.ParseConfig(p.HostDSN())
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(p.CAPEM) {
		return nil, errors.New("test CA PEM has no certificate")
	}
	cfg.TLSConfig = &tls.Config{RootCAs: roots, ServerName: hostName, MinVersion: tls.VersionTLS12}
	cfg.Fallbacks = nil
	return pgx.ConnectConfig(ctx, cfg)
}

func (p *Postgres) Logs(ctx context.Context, since time.Time) (string, error) {
	res, err := Podman(ctx, nil, "logs", "--since", since.UTC().Format(time.RFC3339Nano), p.Container)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("podman logs: exit %d: %s", res.ExitCode, res.Stderr)
	}
	return res.Stdout + res.Stderr, nil
}

func (p *Postgres) Close(ctx context.Context) error {
	if p.ownsRun {
		return p.run.Close(ctx)
	}
	if os.Getenv(KeepEnv) == "1" {
		return nil
	}
	return podmanOK(ctx, nil, "rm", "--force", "--ignore", "--time", "0", p.Container)
}

func (p *Postgres) waitReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	for {
		conn, err := p.Connect(ctx)
		if err == nil {
			return conn.Close(ctx)
		}
		select {
		case <-ctx.Done():
			logs, _ := p.Logs(context.WithoutCancel(ctx), p.run.Start)
			return fmt.Errorf("postgres %s not ready: %w\n%s", p.Container, err, logs)
		case <-time.After(readyInterval):
		}
	}
}

func secretMount(name, target, mode string) string {
	return strings.Join([]string{
		name, "type=mount", "target=" + target,
		"uid=" + postgresUIDGID, "gid=" + postgresUIDGID, "mode=" + mode,
	}, ",")
}

func hostPort(ctx context.Context, container string) (int, error) {
	res, err := Podman(ctx, nil, "port", container, fmt.Sprintf("%d/tcp", containerPort))
	if err != nil {
		return 0, err
	}
	if res.ExitCode != 0 {
		return 0, fmt.Errorf("podman port: exit %d: %s", res.ExitCode, res.Stderr)
	}
	line, _, _ := strings.Cut(strings.TrimSpace(res.Stdout), "\n")
	_, portText, err := net.SplitHostPort(line)
	if err != nil {
		return 0, fmt.Errorf("podman port output %q: %w", line, err)
	}
	return strconv.Atoi(portText)
}

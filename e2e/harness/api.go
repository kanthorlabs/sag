package harness

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	pathpkg "path"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/testenv"
)

type Phase int

const (
	PhaseParallel   Phase = 1
	PhaseSequential Phase = 2
)

const (
	ContainerSAG         = "sag"
	ContainerConformance = "sag-conformance"
	ContainerV2          = "sag-v2"
	ContainerROAudit     = "sag-ro-audit"
	ContainerRate        = "sag-rate"
)

type AgentSpec struct {
	Sub  string
	Name string
}

type ResourceSpec struct {
	Name          string
	Containers    []string
	PGMajor       int
	Database      string
	Role          string
	WrongPassword bool
	TLS           core.TLSMode
	NoTLSCA       bool
	Agents        []string
	Fragment      string
}

type AC struct {
	ID        string
	Phase     Phase
	Agents    []AgentSpec
	Resources []ResourceSpec
	Fixture   string
	Snapshot  string
	Run       func(t *testing.T, env Env)
}

type ContainerSpec struct {
	Name       string
	Args       []string
	Config     string
	MasterKey  []byte
	SecretMode string
}

type Container interface {
	Name() string
	Wait(ctx context.Context) (int, error)
	Healthz(ctx context.Context) error
	Logs(ctx context.Context) (string, error)
	Remove(ctx context.Context) error
}

type TTY interface {
	Expect(ctx context.Context, text string) error
	Send(text string) error
	Wait(ctx context.Context) (testenv.Result, error)
}

type Env interface {
	Major() int
	Image() string
	RepoRoot() string
	ContainerName(container string) string
	MasterKey() []byte
	Postgres(major int) *testenv.Postgres
	Podman(ctx context.Context, args ...string) (testenv.Result, error)
	Exec(ctx context.Context, container string, args ...string) (testenv.Result, error)
	ExecTTY(ctx context.Context, container string, args ...string) (TTY, error)
	StartSAG(ctx context.Context, spec ContainerSpec) (Container, error)
	URL(container string) string
	HTTPClient() *http.Client
	CAPEM() []byte
	Token(ctx context.Context, container, sub string) (string, error)
	Client(ctx context.Context, container, sub string) (*mcp.ClientSession, error)
	Logs(ctx context.Context, container string) (string, error)
	Audit(ctx context.Context, container string) ([]json.RawMessage, error)
	Snapshots(id string) []DatabaseSnapshot
}

type ClaudeRequest struct {
	MCPURL       string
	Token        string
	CAPEM        []byte
	Prompt       string
	AllowedTools []string
}

type ClaudeRunner func(ctx context.Context, req ClaudeRequest) (testenv.Result, error)

var ErrClaudePrerequisite = errors.New("prerequisite missing")

var _ ClaudeRunner = RunClaude

const (
	BusyboxImage  = "docker.io/library/busybox:1.37.0-musl"
	BusyboxTarget = "/e2e/busybox"
)

type BusyboxMounter func(ctx context.Context, dir string) (string, error)

var _ BusyboxMounter = BusyboxMount

type PublishHostCheck func(host string) bool

var _ PublishHostCheck = PublishHostAllowed

var registered []AC

func Register(ac AC) {
	registered = append(registered, ac)
}

func Registered() []AC {
	return slices.Clone(registered)
}

const ContainerUID = 65532

func ConnectDB(ctx context.Context, pg *testenv.Postgres, database string) (*pgx.Conn, error) {
	cfg, err := pgx.ParseConfig(pg.HostDSN())
	if err != nil {
		return nil, errors.New("harness: PostgreSQL host DSN does not parse")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pg.CAPEM) {
		return nil, errors.New("harness: PostgreSQL CA PEM has no certificate")
	}
	cfg.TLSConfig = &tls.Config{RootCAs: roots, ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	cfg.Fallbacks = nil
	cfg.Database = database
	return pgx.ConnectConfig(ctx, cfg)
}

func ReadContainerFile(ctx context.Context, podmanName, file string) ([]byte, error) {
	res, err := testenv.Podman(ctx, nil, "cp", podmanName+":"+file, "-")
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("harness: podman cp out of %s: exit %d: %s", podmanName, res.ExitCode, res.Stderr)
	}
	tr := tar.NewReader(strings.NewReader(res.Stdout))
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("harness: %s:%s: %w", podmanName, file, err)
		}
		if h.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
}

func WriteContainerFile(ctx context.Context, podmanName, file string, data []byte, mode fs.FileMode) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	h := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     pathpkg.Base(file),
		Mode:     int64(mode.Perm()),
		Uid:      ContainerUID,
		Gid:      ContainerUID,
		Size:     int64(len(data)),
	}
	if err := tw.WriteHeader(h); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	res, err := testenv.Podman(ctx, &buf, "cp", "-", podmanName+":"+pathpkg.Dir(file))
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("harness: podman cp into %s: exit %d: %s", podmanName, res.ExitCode, res.Stderr)
	}
	return nil
}

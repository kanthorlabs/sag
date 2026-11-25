package harness

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

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
	Token(ctx context.Context, container, sub string) (string, error)
	Client(ctx context.Context, container, sub string) (*mcp.ClientSession, error)
	Logs(ctx context.Context, container string) (string, error)
	Audit(ctx context.Context, container string) ([]json.RawMessage, error)
}

var registered []AC

func Register(ac AC) {
	registered = append(registered, ac)
}

func Registered() []AC {
	return slices.Clone(registered)
}

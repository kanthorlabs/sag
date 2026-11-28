package daemon

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
)

func TestDaemonLoadsThroughBackend(t *testing.T) {
	fake := resetFake(t)
	r := openWith(t, fakeKind, "backend-loaded-9z4x", fakeApprovals{}, nil)
	if fake.opened != 1 {
		t.Fatalf("adapter.Open calls = %d; want 1", fake.opened)
	}
	if len(fake.creds) == 0 || fake.creds[0] != "backend-loaded-9z4x" {
		t.Fatalf("adapter received cred %q; want the value from the secret backend", fake.creds)
	}
	_ = r
}

func TestAWSResourceAddedUnavailableAndScheduled(t *testing.T) {
	fake := resetFake(t)
	const awsConfig = `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
storage:
  aws:
    region: us-east-1
    credentials: secret:sag-aws-cred
resources:
  remote:
    kind: daemon-fake
    connection: aws-sm:sag/remote/conn
    agents: [skeleton]
`
	cfg, err := config.Parse([]byte(awsConfig))
	if err != nil {
		t.Fatal(err)
	}
	creds := map[string]secret.Credential{
		"sag-aws-cred": secret.NewCredential("sag-aws-cred", []byte("cred-val-7x9z")),
	}
	loader := &fakeAWSLoader{cred: secret.NewCredential("remote", []byte("secret-9a3b"))}
	inv := inventory.New()
	sched := &fakeScheduler{}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	_, err = openResources(context.Background(), log, cfg, creds, loader, scrub.New(nil), inv, fakeApprovals{}, nil, sched)
	if err != nil {
		t.Fatalf("openResources: %v", err)
	}
	if fake.opened != 0 {
		t.Fatalf("aws resource should not be opened synchronously; got %d opens", fake.opened)
	}
	if len(sched.unopened) != 1 || sched.unopened[0].name != "remote" {
		t.Fatalf("expected 1 immediate unopened for 'remote'; got %#v", sched.unopened)
	}
	if len(sched.added) != 0 {
		t.Fatalf("expected no synchronous scheduled resources; got %d", len(sched.added))
	}
}

type fakeAWSLoader struct {
	cred secret.Credential
	err  error
}

func (l *fakeAWSLoader) Load(_ context.Context, _ string) (secret.Credential, error) {
	return l.cred, l.err
}

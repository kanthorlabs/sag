package daemon

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
	"github.com/kanthorlabs/sag/internal/scrub"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/testenv"
)

func awsRunConfig(addr string) string {
	return fmt.Sprintf(`version: 1
server:
  listen: %s
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
`, addr)
}

const awsUnitConfig = `version: 1
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

func awsCreds() map[string]secret.Credential {
	return map[string]secret.Credential{
		"sag-aws-cred": secret.NewCredential("sag-aws-cred", []byte("aws-cred-4p7x")),
	}
}

func noopLog() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

type blockingLoader struct {
	block <-chan struct{}
	cred  secret.Credential
	err   error
}

func (l *blockingLoader) Load(ctx context.Context, _ string) (secret.Credential, error) {
	select {
	case <-l.block:
		return l.cred, l.err
	case <-ctx.Done():
		return secret.Credential{}, ctx.Err()
	}
}

func TestListenerBeforeAWS(t *testing.T) {
	master := bytes.Repeat([]byte{7}, 32)
	f := newFixture(t, master)
	addr := freeAddr(t)
	if err := os.WriteFile(f.opts.ConfigPath, []byte(awsRunConfig(addr)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.opts.SecretsDir, "sag-aws-cred"), []byte("aws-cred-7q9z"), 0o400); err != nil {
		t.Fatal(err)
	}
	ca, err := testenv.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := ca.Issue("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	replaceSecret(t, f.opts.SecretsDir, "sag-tls-cert", certPEM)
	replaceSecret(t, f.opts.SecretsDir, "sag-tls-key", keyPEM)

	blocked := make(chan struct{})
	f.opts.testAWSLoader = &blockingLoader{block: blocked}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, f.opts) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca.CertPEM)
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}

	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, herr := client.Get("https://" + addr + "/healthz")
		if herr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("healthz did not return 200 within 30s while AWS loader blocked")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestUnavailableUntilLoad(t *testing.T) {
	resetFake(t)
	cfg, err := config.Parse([]byte(awsUnitConfig))
	if err != nil {
		t.Fatal(err)
	}
	loader := &fakeAWSLoader{cred: secret.NewCredential("remote", []byte("secret-9a3b"))}
	inv := inventory.New()
	sched := &fakeScheduler{}
	_, err = openResources(context.Background(), noopLog(), cfg, awsCreds(), loader, scrub.New(nil), inv, fakeApprovals{}, nil, sched)
	if err != nil {
		t.Fatalf("openResources: %v", err)
	}

	_, lookupErr := inv.Lookup("skeleton", "remote")
	var ce *core.Error
	if !errors.As(lookupErr, &ce) || ce.Code != core.CodeResourceUnavailable {
		t.Fatalf("Lookup before load = %v; want resource_unavailable", lookupErr)
	}

	if len(sched.unopened) != 1 {
		t.Fatalf("expected 1 immediate opener; got %d", len(sched.unopened))
	}
	res, openErr := sched.unopened[0].open(context.Background())
	if openErr != nil {
		t.Fatalf("open: %v", openErr)
	}
	if res == nil {
		t.Fatal("open returned nil resource")
	}
	if err := inv.SetStatus("remote", inventory.StatusServed); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.Lookup("skeleton", "remote"); err != nil {
		t.Fatalf("Lookup after open: %v", err)
	}
}

func TestRetryBringsBack(t *testing.T) {
	resetFake(t)
	cfg, err := config.Parse([]byte(awsUnitConfig))
	if err != nil {
		t.Fatal(err)
	}
	loader := &fakeAWSLoader{err: fmt.Errorf("aws unavailable")}
	inv := inventory.New()
	sched := &fakeScheduler{}
	_, err = openResources(context.Background(), noopLog(), cfg, awsCreds(), loader, scrub.New(nil), inv, fakeApprovals{}, nil, sched)
	if err != nil {
		t.Fatalf("openResources: %v", err)
	}

	if len(sched.unopened) != 1 {
		t.Fatalf("expected 1 immediate opener; got %d", len(sched.unopened))
	}
	_, openErr := sched.unopened[0].open(context.Background())
	if openErr == nil {
		t.Fatal("first open should fail while loader errors")
	}

	loader.err = nil
	loader.cred = secret.NewCredential("remote", []byte("recovered-secret-7c3x"))
	res, openErr := sched.unopened[0].open(context.Background())
	if openErr != nil {
		t.Fatalf("retry open: %v", openErr)
	}
	if res == nil {
		t.Fatal("retry returned nil resource")
	}
	if err := inv.SetStatus("remote", inventory.StatusServed); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.Lookup("skeleton", "remote"); err != nil {
		t.Fatalf("Lookup after retry: %v", err)
	}
}

func TestScrubAddOnLoad(t *testing.T) {
	resetFake(t)
	cfg, err := config.Parse([]byte(awsUnitConfig))
	if err != nil {
		t.Fatal(err)
	}
	const secretVal = "db-secret-9x3b"
	loader := &fakeAWSLoader{cred: secret.NewCredential("remote", []byte(secretVal))}
	s := scrub.New(nil)
	inv := inventory.New()
	sched := &fakeScheduler{}
	_, err = openResources(context.Background(), noopLog(), cfg, awsCreds(), loader, s, inv, fakeApprovals{}, nil, sched)
	if err != nil {
		t.Fatalf("openResources: %v", err)
	}

	text := "connect to " + secretVal + " failed"
	if strings.Contains(s.String(text), "[REDACTED]") {
		t.Fatal("scrubber should not contain secret before open")
	}

	if len(sched.unopened) != 1 {
		t.Fatalf("expected 1 opener; got %d", len(sched.unopened))
	}
	if _, err := sched.unopened[0].open(context.Background()); err != nil {
		t.Fatalf("open: %v", err)
	}

	if strings.Contains(s.String(text), secretVal) {
		t.Fatalf("secret value %q not scrubbed after open; got %q", secretVal, s.String(text))
	}
}

func TestCredentialsFileScrubbed(t *testing.T) {
	cfg, err := config.Parse([]byte(awsUnitConfig))
	if err != nil {
		t.Fatal(err)
	}
	const keyID, secretKey = "AKIAEXAMPLE7Q3X", "wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY"
	file := "[default]\naws_access_key_id = " + keyID + "\naws_secret_access_key=" + secretKey + "\n"
	creds := map[string]secret.Credential{"sag-aws-cred": secret.NewCredential("sag-aws-cred", []byte(file))}
	s := newScrubber(cfg, creds)
	for _, v := range []string{keyID, secretKey, file} {
		if got := s.String("aws error: " + v); strings.Contains(got, v) {
			t.Errorf("value %q not redacted; got %q", v, got)
		}
	}
}

func TestScrubAddOnLoadTLSCA(t *testing.T) {
	resetFake(t)
	cfg, err := config.Parse([]byte(awsCAConfig("secret:sag-res-remote.conn")))
	if err != nil {
		t.Fatal(err)
	}
	creds := map[string]secret.Credential{
		"sag-aws-cred":        secret.NewCredential("sag-aws-cred", []byte("aws-cred-4p7x")),
		"sag-res-remote.conn": secret.NewCredential("sag-res-remote.conn", []byte("podman-conn-8m1d")),
	}
	s := scrub.New(nil)
	sched := &fakeScheduler{}
	loader := mapAWSLoader{"sag/remote/ca": "aws-ca-pem-3v6w"}
	if _, err := openResources(context.Background(), noopLog(), cfg, creds, loader, s, inventory.New(), fakeApprovals{}, nil, sched); err != nil {
		t.Fatalf("openResources: %v", err)
	}
	if _, err := sched.unopened[0].open(context.Background()); err != nil {
		t.Fatalf("open: %v", err)
	}
	if got := s.String("ca aws-ca-pem-3v6w"); strings.Contains(got, "aws-ca-pem-3v6w") {
		t.Fatalf("aws-sm tls_ca not scrubbed after load; got %q", got)
	}
}

func TestAWSBackendNeverStopsStart(t *testing.T) {
	cases := map[string]struct {
		storage   string
		resources string
		wantCalls int
	}{
		"region_without_refs": {
			storage:   "storage:\n  aws:\n    region: us-east-1\n",
			wantCalls: 0,
		},
		"backend_error": {
			storage:   "storage:\n  aws:\n    region: us-east-1\n    credentials: secret:sag-aws-cred\n",
			resources: "resources:\n  remote:\n    kind: daemon-fake\n    connection: aws-sm:sag/remote/conn\n    agents: [skeleton]\n",
			wantCalls: 1,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resetFake(t)
			calls := 0
			old := newAWSDaemon
			newAWSDaemon = func(context.Context, string, string) (AWSLoader, error) {
				calls++
				return nil, errors.New("aws config broken")
			}
			t.Cleanup(func() { newAWSDaemon = old })

			master := bytes.Repeat([]byte{7}, 32)
			f := newFixture(t, master)
			addr := freeAddr(t)
			conf := fmt.Sprintf("version: 1\nserver:\n  listen: %s\n  tls_cert: secret:sag-tls-cert\n  tls_key: secret:sag-tls-key\n%s%s", addr, tc.storage, tc.resources)
			if err := os.WriteFile(f.opts.ConfigPath, []byte(conf), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(f.opts.SecretsDir, "sag-aws-cred"), []byte("aws-cred-7q9z"), 0o400); err != nil {
				t.Fatal(err)
			}
			ca, err := testenv.NewCA()
			if err != nil {
				t.Fatal(err)
			}
			certPEM, keyPEM, err := ca.Issue("127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			replaceSecret(t, f.opts.SecretsDir, "sag-tls-cert", certPEM)
			replaceSecret(t, f.opts.SecretsDir, "sag-tls-key", keyPEM)

			ctx, cancel := context.WithCancel(context.Background())
			exited := make(chan struct{})
			var runErr error
			go func() {
				runErr = Run(ctx, f.opts)
				close(exited)
			}()
			t.Cleanup(func() {
				cancel()
				<-exited
			})

			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(ca.CertPEM)
			client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
			deadline := time.Now().Add(30 * time.Second)
			for {
				select {
				case <-exited:
					t.Fatalf("Run returned %v; want a running daemon", runErr)
				default:
				}
				resp, herr := client.Get("https://" + addr + "/healthz")
				if herr == nil {
					_ = resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						break
					}
				}
				if time.Now().After(deadline) {
					t.Fatal("healthz did not return 200 within 30s")
				}
				time.Sleep(50 * time.Millisecond)
			}
			if calls != tc.wantCalls {
				t.Fatalf("aws backend built %d times; want %d", calls, tc.wantCalls)
			}
		})
	}
}

type mapAWSLoader map[string]string

func (l mapAWSLoader) Load(_ context.Context, name string) (secret.Credential, error) {
	v, ok := l[name]
	if !ok {
		return secret.Credential{}, secret.ErrNotFound
	}
	return secret.NewCredential(name, []byte(v)), nil
}

func awsCAConfig(conn string) string {
	return fmt.Sprintf(`version: 1
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
    connection: %s
    tls: verify-full
    tls_ca: aws-sm:sag/remote/ca
    agents: [skeleton]
`, conn)
}

func TestAWSTLSCAReachesAdapter(t *testing.T) {
	cases := map[string]struct {
		conn     string
		creds    map[string]secret.Credential
		wantConn string
	}{
		"aws_conn": {
			conn:     "aws-sm:sag/remote/conn",
			creds:    awsCreds(),
			wantConn: "aws-conn-5h2k",
		},
		"podman_conn": {
			conn: "secret:sag-res-remote.conn",
			creds: map[string]secret.Credential{
				"sag-aws-cred":        secret.NewCredential("sag-aws-cred", []byte("aws-cred-4p7x")),
				"sag-res-remote.conn": secret.NewCredential("sag-res-remote.conn", []byte("podman-conn-8m1d")),
			},
			wantConn: "podman-conn-8m1d",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := resetFake(t)
			cfg, err := config.Parse([]byte(awsCAConfig(tc.conn)))
			if err != nil {
				t.Fatal(err)
			}
			loader := mapAWSLoader{"sag/remote/conn": "aws-conn-5h2k", "sag/remote/ca": "aws-ca-pem-3v6w"}
			sched := &fakeScheduler{}
			if _, err := openResources(context.Background(), noopLog(), cfg, tc.creds, loader, scrub.New(nil), inventory.New(), fakeApprovals{}, nil, sched); err != nil {
				t.Fatalf("openResources: %v", err)
			}
			if len(sched.unopened) != 1 {
				t.Fatalf("expected 1 immediate opener; got %d", len(sched.unopened))
			}
			if _, err := sched.unopened[0].open(context.Background()); err != nil {
				t.Fatalf("open: %v", err)
			}
			if len(fake.configs) != 1 || string(fake.configs[0].TLSCA) != "aws-ca-pem-3v6w" {
				t.Fatalf("adapter TLSCA = %q; want the aws-sm CA", tlsCAs(fake.configs))
			}
			if len(fake.creds) != 1 || fake.creds[0] != tc.wantConn {
				t.Fatalf("adapter connection = %q; want %q", fake.creds, tc.wantConn)
			}
		})
	}
}

func TestAWSTLSCALoadFailureKeepsUnavailable(t *testing.T) {
	fake := resetFake(t)
	cfg, err := config.Parse([]byte(awsCAConfig("aws-sm:sag/remote/conn")))
	if err != nil {
		t.Fatal(err)
	}
	loader := mapAWSLoader{"sag/remote/conn": "aws-conn-5h2k"}
	sched := &fakeScheduler{}
	if _, err := openResources(context.Background(), noopLog(), cfg, awsCreds(), loader, scrub.New(nil), inventory.New(), fakeApprovals{}, nil, sched); err != nil {
		t.Fatalf("openResources: %v", err)
	}
	if _, err := sched.unopened[0].open(context.Background()); !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("open = %v; want ErrNotFound for the missing CA", err)
	}
	if fake.opened+fake.openedSQL != 0 {
		t.Fatal("adapter opened without the aws-sm CA")
	}
}

func tlsCAs(cfgs []core.ResourceConfig) []string {
	out := make([]string, 0, len(cfgs))
	for _, c := range cfgs {
		out = append(out, string(c.TLSCA))
	}
	return out
}

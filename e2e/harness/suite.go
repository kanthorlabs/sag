package harness

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/testenv"
)

const (
	ImageEnv            = "SAG_E2E_IMAGE"
	ConformanceImageEnv = "SAG_E2E_CONFORMANCE_IMAGE"
	defaultDatabase     = "postgres"
	fixtureHost         = "localhost"
)

var (
	Image            = envOr(ImageEnv, "localhost/sag:e2e")
	ConformanceImage = envOr(ConformanceImageEnv, "localhost/sag-conformance:e2e")
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var startOrder = []string{ContainerSAG, ContainerConformance, ContainerV2, ContainerROAudit, ContainerRate}

var _ Env = (*Suite)(nil)

type Suite struct {
	acs       []AC
	run       *testenv.Run
	major     int
	root      string
	dir       string
	masterKey []byte
	pgs       map[int]*testenv.Postgres
	http      *http.Client
	secrets   []secretMount
	snapshots map[string][]DatabaseSnapshot

	mu         sync.Mutex
	containers map[string]*sagContainer
	tokens     map[string]string
}

func NewSuite(ctx context.Context, acs []AC) (s *Suite, err error) {
	if err := Validate(acs); err != nil {
		return nil, err
	}
	major, err := testenv.MajorFromEnv()
	if err != nil {
		return nil, err
	}
	root, err := findRepoRoot()
	if err != nil {
		return nil, err
	}
	run, err := testenv.NewRun(ctx)
	if err != nil {
		return nil, err
	}
	s = &Suite{
		acs:        acs,
		run:        run,
		major:      major,
		root:       root,
		pgs:        map[int]*testenv.Postgres{},
		containers: map[string]*sagContainer{},
		tokens:     map[string]string{},
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.Close(context.WithoutCancel(ctx)))
			s = nil
		}
	}()

	if s.dir, err = os.MkdirTemp("", run.Network+"-"); err != nil {
		return s, err
	}
	if err = os.Chmod(s.dir, 0o755); err != nil {
		return s, err
	}
	if s.http, err = newHTTPClient(run.CA.CertPEM); err != nil {
		return s, err
	}
	if err = s.startPostgres(ctx); err != nil {
		return s, err
	}
	if err = s.createServerSecrets(ctx); err != nil {
		return s, err
	}
	if err = s.prepareResources(ctx); err != nil {
		return s, err
	}
	if err = s.runFixtures(ctx); err != nil {
		return s, err
	}
	if err = s.takeSnapshots(ctx); err != nil {
		return s, err
	}
	if err = s.startContainers(ctx); err != nil {
		return s, err
	}
	return s, nil
}

func (s *Suite) Close(ctx context.Context) error {
	if os.Getenv(testenv.KeepEnv) == "1" {
		return nil
	}
	s.mu.Lock()
	containers := make([]*sagContainer, 0, len(s.containers))
	for _, c := range s.containers {
		containers = append(containers, c)
	}
	s.containers = map[string]*sagContainer{}
	s.mu.Unlock()

	var errs []error
	for _, c := range containers {
		errs = append(errs, c.Remove(ctx))
	}
	if s.dir != "" {
		errs = append(errs, os.RemoveAll(s.dir))
	}
	errs = append(errs, s.run.Close(ctx))
	return errors.Join(errs...)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("harness: no go.mod above the working directory")
		}
		dir = parent
	}
}

func (s *Suite) resourceMajor(r ResourceSpec) int {
	if r.PGMajor == 0 {
		return s.major
	}
	return r.PGMajor
}

func resourceDatabase(r ResourceSpec) string {
	if r.Database == "" {
		return defaultDatabase
	}
	return r.Database
}

func (s *Suite) startPostgres(ctx context.Context) error {
	majors := []int{s.major}
	for _, ac := range s.acs {
		for _, r := range ac.Resources {
			majors = append(majors, s.resourceMajor(r))
		}
	}
	slices.Sort(majors)
	for _, m := range slices.Compact(majors) {
		pg, err := s.run.StartPostgres(ctx, m)
		if err != nil {
			return fmt.Errorf("harness: start PostgreSQL %d: %w", m, err)
		}
		s.pgs[m] = pg
	}
	return nil
}

func (s *Suite) secretName(target string) string {
	return s.run.Network + "-" + target
}

func (s *Suite) addSecret(ctx context.Context, target string, data []byte) error {
	source := s.secretName(target)
	if err := s.run.CreateSecret(ctx, source, data); err != nil {
		return fmt.Errorf("harness: create secret %s: %w", target, err)
	}
	s.secrets = append(s.secrets, secretMount{Source: source, Target: target})
	return nil
}

func (s *Suite) createServerSecrets(ctx context.Context) error {
	s.masterKey = make([]byte, secret.MasterKeySize)
	if _, err := rand.Read(s.masterKey); err != nil {
		return err
	}
	certPEM, keyPEM, err := s.run.CA.Issue(fixtureHost)
	if err != nil {
		return err
	}
	if err := s.addSecret(ctx, secret.MasterKeyName, []byte(base64.StdEncoding.EncodeToString(s.masterKey))); err != nil {
		return err
	}
	if err := s.addSecret(ctx, TLSCertSecret, certPEM); err != nil {
		return err
	}
	return s.addSecret(ctx, TLSKeySecret, keyPEM)
}

func (s *Suite) prepareResources(ctx context.Context) error {
	created := map[string]bool{}
	for _, ac := range s.acs {
		for _, r := range ac.Resources {
			pg := s.pgs[s.resourceMajor(r)]
			db := resourceDatabase(r)
			key := fmt.Sprintf("%d/%s", pg.Major, db)
			if db != defaultDatabase && !created[key] {
				if err := s.exec(ctx, pg, defaultDatabase, "CREATE DATABASE "+pgx.Identifier{db}.Sanitize()); err != nil {
					return fmt.Errorf("harness: %s: create database %s: %w", ac.ID, db, err)
				}
				created[key] = true
			}
			dsn, err := s.resourceDSN(pg, db, r.WrongPassword)
			if err != nil {
				return err
			}
			if err := s.addSecret(ctx, ConnectionSecret(r.Name), []byte(dsn)); err != nil {
				return err
			}
			if err := s.addSecret(ctx, CASecret(r.Name), pg.CAPEM); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Suite) resourceDSN(pg *testenv.Postgres, db string, wrongPassword bool) (string, error) {
	u, err := url.Parse(pg.NetworkDSN())
	if err != nil {
		return "", errors.New("harness: PostgreSQL network DSN does not parse")
	}
	u.Path = "/" + db
	if wrongPassword {
		wrong := make([]byte, 16)
		if _, err := rand.Read(wrong); err != nil {
			return "", err
		}
		u.User = url.UserPassword(u.User.Username(), hex.EncodeToString(wrong))
	}
	return u.String(), nil
}

func (s *Suite) runFixtures(ctx context.Context) error {
	for _, ac := range s.acs {
		if ac.Fixture == "" {
			continue
		}
		pg, db := s.pgs[s.major], defaultDatabase
		if len(ac.Resources) > 0 {
			pg, db = s.pgs[s.resourceMajor(ac.Resources[0])], resourceDatabase(ac.Resources[0])
		}
		if err := s.exec(ctx, pg, db, ac.Fixture); err != nil {
			return fmt.Errorf("harness: %s fixture: %w", ac.ID, err)
		}
	}
	return nil
}

func (s *Suite) exec(ctx context.Context, pg *testenv.Postgres, db, sql string) error {
	cfg, err := pgx.ParseConfig(pg.HostDSN())
	if err != nil {
		return errors.New("harness: PostgreSQL host DSN does not parse")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pg.CAPEM) {
		return errors.New("harness: PostgreSQL CA PEM has no certificate")
	}
	cfg.TLSConfig = &tls.Config{RootCAs: roots, ServerName: fixtureHost, MinVersion: tls.VersionTLS12}
	cfg.Fallbacks = nil
	cfg.Database = db
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close(context.WithoutCancel(ctx))
	_, err = conn.PgConn().Exec(ctx, sql).ReadAll()
	return err
}

func (s *Suite) neededContainers() []string {
	needed := []string{ContainerSAG}
	for _, ac := range s.acs {
		for _, r := range ac.Resources {
			needed = append(needed, resourceContainers(r)...)
		}
	}
	var out []string
	for _, c := range startOrder {
		if slices.Contains(needed, c) {
			out = append(out, c)
		}
	}
	return out
}

func (s *Suite) startContainers(ctx context.Context) error {
	var started []Container
	for _, name := range s.neededContainers() {
		c, err := s.StartSAG(ctx, ContainerSpec{Name: name})
		if err != nil {
			return err
		}
		started = append(started, c)
	}
	for _, c := range started {
		if err := c.(*sagContainer).waitHealthy(ctx); err != nil {
			logs, _ := c.Logs(context.WithoutCancel(ctx))
			return fmt.Errorf("%w\n%s logs:\n%s", err, c.Name(), logs)
		}
	}
	return nil
}

func (s *Suite) Major() int {
	return s.major
}

func (s *Suite) Image() string {
	return Image
}

func (s *Suite) RepoRoot() string {
	return s.root
}

func (s *Suite) ContainerName(container string) string {
	return s.run.Network + "-" + container
}

func (s *Suite) MasterKey() []byte {
	return slices.Clone(s.masterKey)
}

func (s *Suite) Postgres(major int) *testenv.Postgres {
	if major == 0 {
		major = s.major
	}
	return s.pgs[major]
}

func (s *Suite) Podman(ctx context.Context, args ...string) (testenv.Result, error) {
	return testenv.Podman(ctx, nil, args...)
}

func (s *Suite) Exec(ctx context.Context, container string, args ...string) (testenv.Result, error) {
	return testenv.Podman(ctx, nil, append([]string{"exec", s.ContainerName(container)}, args...)...)
}

func (s *Suite) ExecTTY(ctx context.Context, container string, args ...string) (TTY, error) {
	return startTTY(ctx, "podman", append([]string{"exec", "-it", s.ContainerName(container)}, args...)...)
}

func (s *Suite) StartSAG(ctx context.Context, spec ContainerSpec) (Container, error) {
	if !namePattern.MatchString(spec.Name) {
		return nil, fmt.Errorf("harness: container name %q does not match %s", spec.Name, namePattern)
	}
	name := s.ContainerName(spec.Name)
	s.mu.Lock()
	_, exists := s.containers[spec.Name]
	s.mu.Unlock()
	if exists {
		return nil, fmt.Errorf("harness: container %s exists", spec.Name)
	}

	config := spec.Config
	if config == "" {
		var err error
		if config, err = Compose(s.acs, spec.Name); err != nil {
			return nil, err
		}
	}
	configFile := filepath.Join(s.dir, spec.Name+".yaml")
	if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
		return nil, err
	}
	if err := os.Chmod(configFile, 0o644); err != nil {
		return nil, err
	}

	mounts := slices.Clone(s.secrets)
	if spec.MasterKey != nil {
		source := s.secretName(spec.Name + "-" + secret.MasterKeyName)
		if err := s.run.CreateSecret(ctx, source, []byte(base64.StdEncoding.EncodeToString(spec.MasterKey))); err != nil {
			return nil, fmt.Errorf("harness: create master key secret of %s: %w", spec.Name, err)
		}
		for i := range mounts {
			if mounts[i].Target == secret.MasterKeyName {
				mounts[i].Source = source
			}
		}
	}

	var auditDir string
	if containerProfiles[spec.Name].readOnlyAudit {
		auditDir = filepath.Join(s.dir, spec.Name+"-audit")
		if err := os.Mkdir(auditDir, 0o755); err != nil {
			return nil, err
		}
	}

	image := Image
	if spec.Name == ContainerConformance {
		image = ConformanceImage
	}
	c := &sagContainer{name: name, volume: name + "-state", client: s.http}
	s.mu.Lock()
	s.containers[spec.Name] = c
	s.mu.Unlock()

	if _, err := podman(ctx, runArgs(runSpec{
		Name:        name,
		Network:     s.run.Network,
		ConfigFile:  configFile,
		StateVolume: c.volume,
		AuditDir:    auditDir,
		Secrets:     mounts,
		SecretMode:  spec.SecretMode,
		Image:       image,
		Command:     spec.Args,
	})...); err != nil {
		return nil, err
	}
	if port, err := publishedPort(ctx, name); err == nil {
		c.port = port
	}
	return c, nil
}

func (s *Suite) container(name string) (*sagContainer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.containers[name]
	if !ok {
		return nil, fmt.Errorf("harness: no container %s", name)
	}
	return c, nil
}

func (s *Suite) URL(container string) string {
	c, err := s.container(container)
	if err != nil || c.port == 0 {
		return ""
	}
	return c.url()
}

func (s *Suite) HTTPClient() *http.Client {
	return s.http
}

func (s *Suite) agentName(sub string) string {
	for _, ac := range s.acs {
		for _, a := range ac.Agents {
			if a.Sub == sub {
				return a.Name
			}
		}
	}
	return ""
}

func (s *Suite) Token(ctx context.Context, container, sub string) (string, error) {
	key := container + "/" + sub
	s.mu.Lock()
	token, ok := s.tokens[key]
	s.mu.Unlock()
	if ok {
		return token, nil
	}

	args := []string{"sag", "jwt", "generate", sub}
	if name := s.agentName(sub); name != "" {
		args = append(args, "--name", name)
	}
	tty, err := s.ExecTTY(ctx, container, args...)
	if err != nil {
		return "", err
	}
	res, err := tty.Wait(ctx)
	if err != nil {
		return "", fmt.Errorf("harness: jwt generate %s on %s: %w", sub, container, err)
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("harness: jwt generate %s on %s: exit %d: %q", sub, container, res.ExitCode, res.Stdout)
	}
	token, err = parseToken(res.Stdout)
	if err != nil {
		return "", fmt.Errorf("harness: jwt generate %s on %s: output has no JWT", sub, container)
	}
	s.mu.Lock()
	s.tokens[key] = token
	s.mu.Unlock()
	return token, nil
}

func (s *Suite) Client(ctx context.Context, container, sub string) (*mcp.ClientSession, error) {
	token, err := s.Token(ctx, container, sub)
	if err != nil {
		return nil, err
	}
	base := s.URL(container)
	if base == "" {
		return nil, fmt.Errorf("harness: container %s has no URL", container)
	}
	return connectMCP(ctx, base+"/mcp", bearerClient(s.http, token))
}

func (s *Suite) Logs(ctx context.Context, container string) (string, error) {
	c, err := s.container(container)
	if err != nil {
		return "", err
	}
	return c.Logs(ctx)
}

func (s *Suite) Audit(ctx context.Context, container string) ([]json.RawMessage, error) {
	tmp, err := os.MkdirTemp(s.dir, "audit-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if _, err := podman(ctx, "cp", s.ContainerName(container)+":"+AuditDir, tmp); err != nil {
		return nil, err
	}
	var files []string
	err = filepath.WalkDir(tmp, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var records []json.RawMessage
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			if !json.Valid(line) {
				return nil, fmt.Errorf("harness: audit file %s has a line that is not JSON", filepath.Base(f))
			}
			records = append(records, json.RawMessage(slices.Clone(line)))
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	return records, nil
}

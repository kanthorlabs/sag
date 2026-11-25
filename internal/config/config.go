package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/kanthorlabs/sag/internal/core"
)

const DefaultPath = "/etc/sag/sag.yaml"

type SecretRef string

type Config struct {
	Version   int
	Server    Server
	Auth      Auth
	Preflight Preflight
	Defaults  Defaults
	Resources map[string]Resource
}

type Server struct {
	Listen         string
	TLSCert        SecretRef
	TLSKey         SecretRef
	AllowedOrigins []string
}

type Auth struct {
	Version       int
	TokenLifetime time.Duration
}

type Preflight struct {
	RecheckInterval time.Duration
}

type Defaults struct {
	Limits        Limits
	RedactColumns []string
}

type Limits struct {
	MaxRows                         int
	MaxResultBytes                  int
	MaxCellBytes                    int
	StatementTimeout                time.Duration
	LockTimeout                     time.Duration
	IdleInTransactionSessionTimeout time.Duration
	ConnectTimeout                  time.Duration
	MaxConcurrentPerAgent           int
	PoolSize                        int
}

type Resource struct {
	Name          string
	Kind          string
	Description   string
	Connection    SecretRef
	TLS           core.TLSMode
	TLSCA         SecretRef
	Agents        []string
	Limits        Limits
	RedactColumns []string
}

const (
	schemaVersion          = 1
	defaultListen          = "0.0.0.0:8443"
	defaultAuthVersion     = 1
	defaultTokenLifetime   = 8760 * time.Hour
	minTokenLifetime       = time.Second
	minRecheckInterval     = 10 * time.Second
	maxRecheckInterval     = 10 * time.Minute
	defaultRecheckInterval = maxRecheckInterval
	maxDescriptionRunes    = 500
	secretRefPrefix        = "secret:"
)

var (
	defaultRedactColumns = []string{"*password*", "*passwd*", "*secret*", "*token*", "*api_key*", "*apikey*", "*private_key*"}
	namePattern          = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	secretNamePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,252}$`)
	yamlValuePattern     = regexp.MustCompile("`[^`]*`")
	tlsModes             = []core.TLSMode{core.TLSVerifyFull, core.TLSInsecureSkipVerify, core.TLSDisable}
)

type problems []string

func (p *problems) add(path, msg string) {
	*p = append(*p, path+": "+msg)
}

func (p problems) err() error {
	if len(p) == 0 {
		return nil
	}
	return errors.New("config: " + strings.Join(p, "; "))
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	raw, err := decode(data)
	if err != nil {
		return nil, err
	}
	var p problems
	cfg := build(raw, &p)
	if err := p.err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func decode(data []byte) (rawConfig, error) {
	var raw rawConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return raw, errors.New("config: empty file")
		}
		return raw, fmt.Errorf("config: %s", yamlValuePattern.ReplaceAllString(err.Error(), "value"))
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return raw, errors.New("config: the file must hold exactly one YAML document")
	}
	return raw, nil
}

func build(raw rawConfig, p *problems) *Config {
	cfg := &Config{
		Version:   valueOr(raw.Version, 0),
		Server:    buildServer(raw.Server, p),
		Auth:      buildAuth(raw.Auth, p),
		Preflight: buildPreflight(raw.Preflight, p),
		Resources: map[string]Resource{},
	}
	if cfg.Version != schemaVersion {
		p.add("version", "is required and must be 1")
	}

	checkLimits("defaults.limits", raw.Defaults.Limits, false, p)
	checkGlobs(p, "defaults.redact.columns", raw.Defaults.Redact.Columns)
	cfg.Defaults = Defaults{
		Limits:        resolveLimits(raw.Defaults.Limits),
		RedactColumns: defaultRedactColumns,
	}
	if raw.Defaults.Redact.Columns != nil {
		cfg.Defaults.RedactColumns = raw.Defaults.Redact.Columns
	}

	if len(raw.Resources) == 0 {
		p.add("resources", "must hold at least one resource")
	}
	for _, name := range slices.Sorted(maps.Keys(raw.Resources)) {
		cfg.Resources[name] = buildResource(name, raw.Resources[name], raw.Defaults.Limits, cfg.Defaults.RedactColumns, p)
	}
	return cfg
}

func buildServer(raw rawServer, p *problems) Server {
	s := Server{
		Listen:         valueOr(raw.Listen, defaultListen),
		TLSCert:        secretRef(p, "server.tls_cert", raw.TLSCert, true),
		TLSKey:         secretRef(p, "server.tls_key", raw.TLSKey, true),
		AllowedOrigins: raw.AllowedOrigins,
	}
	if !validListen(s.Listen) {
		p.add("server.listen", "must be <host>:<port>")
	}
	return s
}

func validListen(addr string) bool {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

func buildAuth(raw rawAuth, p *problems) Auth {
	a := Auth{
		Version:       valueOr(raw.Version, defaultAuthVersion),
		TokenLifetime: valueOr(raw.TokenLifetime, defaultTokenLifetime),
	}
	if a.Version < 1 {
		p.add("auth.version", "must be a positive integer")
	}
	if a.TokenLifetime < minTokenLifetime {
		p.add("auth.token_lifetime", "must be at least "+minTokenLifetime.String())
	}
	return a
}

func buildPreflight(raw rawPreflight, p *problems) Preflight {
	interval := valueOr(raw.RecheckInterval, defaultRecheckInterval)
	if interval < minRecheckInterval || interval > maxRecheckInterval {
		p.add("preflight.recheck_interval", fmt.Sprintf("must be from %s to %s", minRecheckInterval, maxRecheckInterval))
	}
	return Preflight{RecheckInterval: interval}
}

func buildResource(name string, raw rawResource, defaults rawLimits, redact []string, p *problems) Resource {
	path := "resources." + name
	if !namePattern.MatchString(name) {
		p.add(path, "name must match "+namePattern.String())
	}
	r := Resource{
		Name:          name,
		Kind:          raw.Kind,
		Description:   raw.Description,
		Connection:    secretRef(p, path+".connection", raw.Connection, true),
		TLS:           core.TLSMode(raw.TLS),
		TLSCA:         secretRef(p, path+".tls_ca", raw.TLSCA, false),
		Agents:        raw.Agents,
		Limits:        resolveLimits(mergeLimits(defaults, raw.Limits)),
		RedactColumns: slices.Concat(redact, raw.Redact.Columns),
	}
	if r.Kind == "" {
		p.add(path+".kind", "is required")
	}
	if utf8.RuneCountInString(r.Description) > maxDescriptionRunes {
		p.add(path+".description", fmt.Sprintf("must be at most %d characters", maxDescriptionRunes))
	}
	if r.TLS == "" {
		r.TLS = core.TLSVerifyFull
	}
	if !slices.Contains(tlsModes, r.TLS) {
		p.add(path+".tls", "must be verify-full, insecure-skip-verify, or disable")
	}
	if r.Agents == nil {
		p.add(path+".agents", "is required")
	}
	for i, sub := range r.Agents {
		if !namePattern.MatchString(sub) {
			p.add(fmt.Sprintf("%s.agents[%d]", path, i), "must match "+namePattern.String())
		}
	}
	checkLimits(path+".limits", raw.Limits, true, p)
	checkGlobs(p, path+".redact.columns", raw.Redact.Columns)
	return r
}

func secretRef(p *problems, path, value string, required bool) SecretRef {
	if value == "" {
		if required {
			p.add(path, "is required")
		}
		return ""
	}
	name, ok := strings.CutPrefix(value, secretRefPrefix)
	if !ok || !secretNamePattern.MatchString(name) {
		p.add(path, "must be a secret reference secret:<name>; inline values are rejected")
		return ""
	}
	return SecretRef(name)
}

func checkGlobs(p *problems, key string, patterns []string) {
	for i, pattern := range patterns {
		if _, err := path.Match(pattern, ""); err != nil {
			p.add(fmt.Sprintf("%s[%d]", key, i), "is not a valid glob")
		}
	}
}

func (c *Config) SecretNames() []string {
	names := []string{string(c.Server.TLSCert), string(c.Server.TLSKey)}
	for _, r := range c.Resources {
		names = append(names, string(r.Connection))
		if r.TLSCA != "" {
			names = append(names, string(r.TLSCA))
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func (c *Config) HasAgent(sub string) bool {
	for _, r := range c.Resources {
		if slices.Contains(r.Agents, sub) {
			return true
		}
	}
	return false
}

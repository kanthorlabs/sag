package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/postexec"
)

const DefaultPath = "/etc/sag/sag.yaml"

type SecretRef string

type Config struct {
	Version   int
	Server    Server
	Auth      Auth
	Preflight Preflight
	Defaults  Defaults
	Storage   Storage
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
	awsSMRefPrefix         = "aws-sm:"
)

var (
	defaultRedactColumns = []string{"*password*", "*passwd*", "*secret*", "*token*", "*api_key*", "*apikey*", "*private_key*"}
	namePattern          = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	secretNamePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,252}$`)
	awsNamePattern       = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)
	yamlLinePattern      = regexp.MustCompile(`^(?:yaml: )?line (\d+): `)
	originLabelPattern   = regexp.MustCompile(`^[a-z0-9-]+$`)
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
		return raw, decodeError(err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return raw, errors.New("config: the file must hold exactly one YAML document")
	}
	return raw, nil
}

func decodeError(err error) error {
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return errors.New("config: " + yamlProblem(err.Error(), "invalid YAML syntax"))
	}
	var p problems
	for _, msg := range typeErr.Errors {
		p = append(p, yamlProblem(msg, yamlClass(msg)))
	}
	return p.err()
}

func yamlClass(msg string) string {
	switch {
	case strings.Contains(msg, " not found in type "):
		return "unknown key"
	case strings.Contains(msg, " already defined at line "):
		return "duplicate key"
	case strings.Contains(msg, " cannot unmarshal "):
		return "wrong type"
	default:
		return "invalid value"
	}
}

func yamlProblem(msg, class string) string {
	if m := yamlLinePattern.FindStringSubmatch(msg); m != nil {
		return "line " + m[1] + ": " + class
	}
	return class
}

func build(raw rawConfig, p *problems) *Config {
	cfg := &Config{
		Version:   valueOr(raw.Version, 0),
		Server:    buildServer(raw.Server, p),
		Auth:      buildAuth(raw.Auth, p),
		Preflight: buildPreflight(raw.Preflight, p),
		Storage:   buildStorage(raw.Storage, p),
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

	for _, name := range slices.Sorted(maps.Keys(raw.Resources)) {
		cfg.Resources[name] = buildResource(name, raw.Resources[name], raw.Defaults.Limits, cfg.Defaults.RedactColumns, p)
	}

	if cfg.Storage.AWS.Region != "" || cfg.Storage.AWS.Credentials != "" {
		for resName, r := range cfg.Resources {
			if hasAWSSMRef(r.Connection) || hasAWSSMRef(r.TLSCA) {
				if cfg.Storage.AWS.Region == "" {
					p.add("storage.aws.region", "is required when an aws-sm: reference is present (resource "+resName+")")
				}
				if cfg.Storage.AWS.Credentials == "" {
					p.add("storage.aws.credentials", "is required when an aws-sm: reference is present (resource "+resName+")")
				}
				break
			}
		}
	} else {
		for resName, r := range cfg.Resources {
			if hasAWSSMRef(r.Connection) || hasAWSSMRef(r.TLSCA) {
				p.add("storage.aws", "region and credentials are required when an aws-sm: reference is present (resource "+resName+")")
				break
			}
		}
	}

	return cfg
}

func buildStorage(raw rawStorage, p *problems) Storage {
	s := Storage{}
	if raw.AWS.Region == "" && raw.AWS.Credentials == "" {
		return s
	}
	s.AWS.Region = raw.AWS.Region
	s.AWS.Credentials = secretRef(p, "storage.aws.credentials", raw.AWS.Credentials, false)
	return s
}

func hasAWSSMRef(ref SecretRef) bool {
	return strings.HasPrefix(string(ref), awsSMRefPrefix)
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
	for i, origin := range s.AllowedOrigins {
		if !validOrigin(origin) {
			p.add(fmt.Sprintf("server.allowed_origins[%d]", i), "must be a serialized origin <scheme>://<host>[:<port>] with scheme http or https, a lowercase host, no default port, and no path")
		}
	}
	return s
}

func validOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Hostname() == "" {
		return false
	}
	if u.Scheme+"://"+u.Host != origin || strings.HasSuffix(u.Host, ":") || !validOriginHost(u) {
		return false
	}
	port := u.Port()
	if port == "" {
		return true
	}
	n, err := strconv.Atoi(port)
	defaultPort := u.Scheme == "http" && n == 80 || u.Scheme == "https" && n == 443
	return err == nil && strconv.Itoa(n) == port && n >= 1 && n <= 65535 && !defaultPort
}

func validOriginHost(u *url.URL) bool {
	host := u.Hostname()
	if strings.HasPrefix(u.Host, "[") {
		addr, err := netip.ParseAddr(host)
		return err == nil && addr.Is6() && addr.Zone() == "" && addr.String() == host
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Is4() && addr.String() == host
	}
	labels := strings.Split(host, ".")
	last := labels[len(labels)-1]
	if strings.Trim(last, "0123456789") == "" || strings.HasPrefix(last, "0x") {
		return false
	}
	for _, label := range labels {
		if !originLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
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
		path = "resources.<invalid name>"
		p.add(path, "name must match "+namePattern.String())
	}
	r := Resource{
		Name:          name,
		Kind:          raw.Kind,
		Description:   raw.Description,
		Connection:    secretOrAWSRef(p, path+".connection", raw.Connection, true),
		TLS:           core.TLSMode(raw.TLS),
		TLSCA:         secretOrAWSRef(p, path+".tls_ca", raw.TLSCA, false),
		Agents:        raw.Agents,
		Limits:        resolveLimits(mergeLimits(defaults, raw.Limits)),
		RedactColumns: slices.Concat(redact, raw.Redact.Columns),
	}
	if r.Kind == "" {
		p.add(path+".kind", "is required")
	} else if !namePattern.MatchString(r.Kind) {
		p.add(path+".kind", "must match "+namePattern.String())
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

func secretOrAWSRef(p *problems, path, value string, required bool) SecretRef {
	if value == "" {
		if required {
			p.add(path, "is required")
		}
		return ""
	}
	if name, ok := strings.CutPrefix(value, secretRefPrefix); ok {
		if !secretNamePattern.MatchString(name) {
			p.add(path, "must be a secret reference secret:<name>; inline values are rejected")
			return ""
		}
		return SecretRef(name)
	}
	if name, ok := strings.CutPrefix(value, awsSMRefPrefix); ok {
		if !awsNamePattern.MatchString(name) {
			p.add(path, "must be a valid aws-sm reference aws-sm:<name> matching "+awsNamePattern.String())
			return ""
		}
		return SecretRef(awsSMRefPrefix + name)
	}
	p.add(path, "must be a secret reference secret:<name> or aws-sm:<name>; inline values are rejected")
	return ""
}

func checkGlobs(p *problems, key string, patterns []string) {
	for i, pattern := range patterns {
		if _, err := postexec.CompileGlob(pattern); err != nil {
			p.add(fmt.Sprintf("%s[%d]", key, i), "is not a valid glob")
		}
	}
}

func (c *Config) SecretNames() []string {
	var names []string
	for _, ref := range []SecretRef{c.Server.TLSCert, c.Server.TLSKey} {
		if ref != "" && !hasAWSSMRef(ref) {
			names = append(names, string(ref))
		}
	}
	if c.Storage.AWS.Credentials != "" && !hasAWSSMRef(c.Storage.AWS.Credentials) {
		names = append(names, string(c.Storage.AWS.Credentials))
	}
	for _, r := range c.Resources {
		if r.Connection != "" && !hasAWSSMRef(r.Connection) {
			names = append(names, string(r.Connection))
		}
		if r.TLSCA != "" && !hasAWSSMRef(r.TLSCA) {
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

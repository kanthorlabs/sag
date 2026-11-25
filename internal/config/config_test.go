package config

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
)

const minimal = `version: 1
server:
  tls_cert: secret:sag-tls-cert
  tls_key: secret:sag-tls-key
resources:
  main:
    kind: postgres
    connection: secret:sag-main
    agents: [agent-a]
`

func withDefaults(block string) string {
	return minimal + "defaults:\n" + indent(block, "  ")
}

func withResourceKeys(block string) string {
	return minimal + indent(block, "    ")
}

func indent(block, prefix string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(block, "\n"), "\n") {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

func mustParse(t *testing.T, text string) *Config {
	t.Helper()
	cfg, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, text)
	}
	return cfg
}

func mustReject(t *testing.T, text, want string) {
	t.Helper()
	_, err := Parse([]byte(text))
	if err == nil {
		t.Fatalf("Parse accepted:\n%s", text)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Parse error = %q; want it to contain %q", err, want)
	}
}

func TestAC01_ConfigCheckRejects(t *testing.T) {
	for _, name := range []string{"ac01-inline-dsn", "ac01-unknown-key", "ac01-max-rows", "ac01-recheck-5s"} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(filepath.Join("testdata", name+".yaml"))
			if err == nil {
				t.Fatal("Load accepted the fixture")
			}
			if strings.Contains(err.Error(), "postgres://") || strings.Contains(err.Error(), "p4ss") {
				t.Fatalf("error holds the inline DSN: %v", err)
			}
		})
	}
}

func TestAC01_ValidWithoutSecretsDir(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "ac01-valid.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Resources["partner-ledger"].Limits.MaxRows; got != 500 {
		t.Fatalf("partner-ledger max_rows = %d; want 500", got)
	}
	if got := cfg.Resources["orders"].Limits.MaxRows; got != 100 {
		t.Fatalf("orders max_rows = %d; want 100", got)
	}
}

func TestDefaults(t *testing.T) {
	cfg := mustParse(t, minimal)
	want := Limits{
		MaxRows:                         100,
		MaxResultBytes:                  32 << 10,
		MaxCellBytes:                    4 << 10,
		StatementTimeout:                15 * time.Second,
		LockTimeout:                     2 * time.Second,
		IdleInTransactionSessionTimeout: 20 * time.Second,
		ConnectTimeout:                  5 * time.Second,
		MaxConcurrentPerAgent:           20,
		PoolSize:                        5,
	}
	if cfg.Defaults.Limits != want {
		t.Fatalf("defaults limits = %+v; want %+v", cfg.Defaults.Limits, want)
	}
	r := cfg.Resources["main"]
	if r.Limits != want {
		t.Fatalf("resource limits = %+v; want %+v", r.Limits, want)
	}
	if cfg.Version != 1 || cfg.Server.Listen != "0.0.0.0:8443" || len(cfg.Server.AllowedOrigins) != 0 {
		t.Fatalf("server = %+v; want listen 0.0.0.0:8443 and no origins", cfg.Server)
	}
	if cfg.Server.TLSCert != "sag-tls-cert" || cfg.Server.TLSKey != "sag-tls-key" {
		t.Fatalf("server secrets = %q %q", cfg.Server.TLSCert, cfg.Server.TLSKey)
	}
	if cfg.Auth.Version != 1 || cfg.Auth.TokenLifetime != 31536000*time.Second {
		t.Fatalf("auth = %+v; want version 1 and one year", cfg.Auth)
	}
	if cfg.Preflight.RecheckInterval != 10*time.Minute {
		t.Fatalf("recheck_interval = %v; want 10m", cfg.Preflight.RecheckInterval)
	}
	if r.Name != "main" || r.Kind != "postgres" || r.Connection != "sag-main" || r.TLS != core.TLSVerifyFull || r.TLSCA != "" {
		t.Fatalf("resource = %+v", r)
	}
	patterns := []string{"*password*", "*passwd*", "*secret*", "*token*", "*api_key*", "*apikey*", "*private_key*"}
	if !slices.Equal(cfg.Defaults.RedactColumns, patterns) || !slices.Equal(r.RedactColumns, patterns) {
		t.Fatalf("redact = %q, %q; want %q", cfg.Defaults.RedactColumns, r.RedactColumns, patterns)
	}
}

func TestDefaultsOverride(t *testing.T) {
	cfg := mustParse(t, minimal+`  other:
    kind: postgres
    connection: secret:sag-other
    tls: disable
    tls_ca: secret:sag-other-ca
    agents: []
    limits:
      statement_timeout: 30s
      max_rows: 1000
    redact:
      columns: ["*iban*"]
defaults:
  limits:
    statement_timeout: 10s
    max_concurrent_per_agent: 4
    pool_size: 4
  redact:
    columns: ["*ssn*"]
`)
	if got := cfg.Defaults.Limits; got.StatementTimeout != 10*time.Second || got.IdleInTransactionSessionTimeout != 15*time.Second || got.MaxConcurrentPerAgent != 4 || got.PoolSize != 4 {
		t.Fatalf("defaults limits = %+v", got)
	}
	main := cfg.Resources["main"]
	if main.Limits != cfg.Defaults.Limits {
		t.Fatalf("main limits = %+v; want defaults %+v", main.Limits, cfg.Defaults.Limits)
	}
	other := cfg.Resources["other"]
	if other.Limits.StatementTimeout != 30*time.Second || other.Limits.IdleInTransactionSessionTimeout != 35*time.Second || other.Limits.MaxRows != 1000 || other.Limits.PoolSize != 4 || other.Limits.MaxConcurrentPerAgent != 4 {
		t.Fatalf("other limits = %+v", other.Limits)
	}
	if other.TLS != core.TLSDisable || other.TLSCA != "sag-other-ca" || len(other.Agents) != 0 {
		t.Fatalf("other = %+v", other)
	}
	if !slices.Equal(cfg.Defaults.RedactColumns, []string{"*ssn*"}) || !slices.Equal(main.RedactColumns, []string{"*ssn*"}) {
		t.Fatalf("redact = %q, %q", cfg.Defaults.RedactColumns, main.RedactColumns)
	}
	if !slices.Equal(other.RedactColumns, []string{"*ssn*", "*iban*"}) {
		t.Fatalf("other redact = %q", other.RedactColumns)
	}
}

func TestExplicitIdleTimeoutWins(t *testing.T) {
	cfg := mustParse(t, minimal+`    limits:
      statement_timeout: 20s
defaults:
  limits:
    idle_in_transaction_session_timeout: 7s
`)
	if got := cfg.Resources["main"].Limits.IdleInTransactionSessionTimeout; got != 7*time.Second {
		t.Fatalf("idle timeout = %v; want 7s", got)
	}
}

func TestHardMaximums(t *testing.T) {
	cases := []struct {
		key   string
		max   string
		over  string
		zero  string
		minus string
	}{
		{"max_rows", "1000", "1001", "0", "-1"},
		{"max_result_bytes", "2097152", "2097153", "0", "-1"},
		{"max_cell_bytes", "8192", "8193", "0", "-1"},
		{"statement_timeout", "30s", "30001ms", "0s", "-1s"},
		{"lock_timeout", "2s", "2001ms", "0s", "-1s"},
		{"idle_in_transaction_session_timeout", "35s", "35001ms", "0s", "-1s"},
		{"connect_timeout", "5s", "5001ms", "0s", "-1s"},
		{"pool_size", "20", "21", "0", "-1"},
	}
	scopes := map[string]func(string) string{
		"defaults": func(line string) string { return withDefaults("limits:\n  " + line + "\n") },
		"resource": func(line string) string { return withResourceKeys("limits:\n  " + line + "\n") },
	}
	for scope, build := range scopes {
		for _, c := range cases {
			t.Run(scope+"/"+c.key, func(t *testing.T) {
				mustParse(t, build(c.key+": "+c.max))
				mustReject(t, build(c.key+": "+c.over), c.key)
				mustReject(t, build(c.key+": "+c.zero), c.key)
				mustReject(t, build(c.key+": "+c.minus), c.key)
			})
		}
	}
	mustParse(t, withDefaults("limits:\n  max_concurrent_per_agent: 100000\n"))
	mustReject(t, withDefaults("limits:\n  max_concurrent_per_agent: 0\n"), "max_concurrent_per_agent")
}

func TestRecheckInterval(t *testing.T) {
	for _, ok := range []string{"10s", "1m", "10m"} {
		cfg := mustParse(t, minimal+"preflight:\n  recheck_interval: "+ok+"\n")
		want, _ := time.ParseDuration(ok)
		if cfg.Preflight.RecheckInterval != want {
			t.Fatalf("recheck_interval = %v; want %v", cfg.Preflight.RecheckInterval, want)
		}
	}
	for _, bad := range []string{"5s", "9999ms", "10m1s", "0s", "-10s"} {
		mustReject(t, minimal+"preflight:\n  recheck_interval: "+bad+"\n", "recheck_interval")
	}
}

func TestLimitKeysUnderLimits(t *testing.T) {
	keys := []string{"max_rows: 5", "max_result_bytes: 5", "max_cell_bytes: 5", "statement_timeout: 5s", "lock_timeout: 1s", "idle_in_transaction_session_timeout: 5s", "connect_timeout: 1s", "pool_size: 1"}
	for _, line := range keys {
		mustReject(t, withResourceKeys(line), "line 10: unknown key")
		mustReject(t, withDefaults(line), "line 11: unknown key")
		mustReject(t, minimal+line+"\n", "line 10: unknown key")
		mustParse(t, withResourceKeys("limits:\n  "+line))
		mustParse(t, withDefaults("limits:\n  "+line))
	}
	mustReject(t, minimal+"max_concurrent_per_agent: 5\n", "line 10: unknown key")
}

func TestMaxConcurrentOnlyInDefaults(t *testing.T) {
	cfg := mustParse(t, withDefaults("limits:\n  max_concurrent_per_agent: 4\n"))
	if got := cfg.Defaults.Limits.MaxConcurrentPerAgent; got != 4 {
		t.Fatalf("max_concurrent_per_agent = %d; want 4", got)
	}
	mustReject(t, withResourceKeys("limits:\n  max_concurrent_per_agent: 4"), "resources.main.limits.max_concurrent_per_agent")
	mustReject(t, withResourceKeys("max_concurrent_per_agent: 4"), "line 10: unknown key")
}

func TestRejectsInlineSecrets(t *testing.T) {
	values := []string{"postgres://u:p4ss@h/d", "p4ss", "secret:", "secret:../p4ss", "secret:a/b", "secret:.hidden", "SECRET:p4ss", " secret:p4ss"}
	for _, v := range values {
		for _, text := range []string{
			strings.Replace(minimal, "secret:sag-main", `"`+v+`"`, 1),
			strings.Replace(minimal, "secret:sag-tls-cert", `"`+v+`"`, 1),
			strings.Replace(minimal, "secret:sag-tls-key", `"`+v+`"`, 1),
			withResourceKeys(`tls_ca: "` + v + `"`),
		} {
			_, err := Parse([]byte(text))
			if err == nil {
				t.Fatalf("Parse accepted secret reference %q", v)
			}
			if strings.Contains(err.Error(), "p4ss") || strings.Contains(err.Error(), "postgres://") {
				t.Fatalf("error holds the value: %v", err)
			}
		}
	}
}

func TestTypeErrorHidesValue(t *testing.T) {
	_, err := Parse([]byte(strings.Replace(minimal, "[agent-a]", `"postgres://u:p4ss@h/d"`, 1)))
	if err == nil {
		t.Fatal("Parse accepted a string for agents")
	}
	if strings.Contains(err.Error(), "p4ss") || strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("error holds the value: %v", err)
	}
}

func TestOperatorTextHiddenInErrors(t *testing.T) {
	dsn := "postgres://u:" + "s3cr3t-pw" + "@db.example:5432/app"
	cases := map[string]struct {
		text string
		want string
	}{
		"dsn as kind":          {strings.Replace(minimal, "kind: postgres", "kind: "+dsn, 1), "resources.main.kind: must match"},
		"dsn as unknown key":   {withResourceKeys(`"` + dsn + `": 1`), "line 10: unknown key"},
		"dsn as duplicate key": {minimal + `"` + dsn + "\": 1\n\"" + dsn + "\": 2\n", "line 11: duplicate key"},
		"dsn as resource name": {strings.Replace(minimal, "  main:", `  "`+dsn+`":`, 1), "resources.<invalid name>: name must match"},
		"syntax error":         {minimal + dsn + ": [\n", "invalid YAML syntax"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.text))
			if err == nil {
				t.Fatal("Parse accepted the file")
			}
			if strings.Contains(err.Error(), "s3cr3t") || strings.Contains(err.Error(), "db.example") {
				t.Fatalf("error holds operator text: %v", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %q; want it to contain %q", err, c.want)
			}
		})
	}
}

func TestSchemaRejects(t *testing.T) {
	cases := map[string]struct {
		text string
		want string
	}{
		"empty":               {"", "empty"},
		"version absent":      {strings.Replace(minimal, "version: 1\n", "", 1), "version"},
		"version 2":           {strings.Replace(minimal, "version: 1", "version: 2", 1), "version"},
		"unknown top key":     {minimal + "debug: true\n", "line 10: unknown key"},
		"unknown server key":  {strings.Replace(minimal, "server:\n", "server:\n  port: 1\n", 1), "line 3: unknown key"},
		"unknown resource":    {withResourceKeys("read_only: false"), "line 10: unknown key"},
		"unknown limit":       {withDefaults("limits:\n  max_rows_x: 1\n"), "line 12: unknown key"},
		"unknown redact":      {withDefaults("redact:\n  rows: [a]\n"), "line 12: unknown key"},
		"server absent":       {strings.Replace(minimal, "server:\n  tls_cert: secret:sag-tls-cert\n  tls_key: secret:sag-tls-key\n", "", 1), "server.tls_cert"},
		"tls_key absent":      {strings.Replace(minimal, "  tls_key: secret:sag-tls-key\n", "", 1), "server.tls_key"},
		"bad listen":          {strings.Replace(minimal, "server:\n", "server:\n  listen: nope\n", 1), "server.listen"},
		"auth version 0":      {minimal + "auth:\n  version: 0\n", "auth.version"},
		"lifetime 0":          {minimal + "auth:\n  token_lifetime: 0s\n", "auth.token_lifetime"},
		"lifetime integer":    {minimal + "auth:\n  token_lifetime: 3600\n", "line 11: wrong type"},
		"no resources":        {strings.SplitN(minimal, "resources:", 2)[0], "resources"},
		"bad resource name":   {strings.Replace(minimal, "  main:", "  Main:", 1), "resources.<invalid name>: name must match"},
		"long resource name":  {strings.Replace(minimal, "  main:", "  a"+strings.Repeat("b", 63)+":", 1), "resources"},
		"kind absent":         {strings.Replace(minimal, "    kind: postgres\n", "", 1), "resources.main.kind"},
		"connection absent":   {strings.Replace(minimal, "    connection: secret:sag-main\n", "", 1), "resources.main.connection"},
		"agents absent":       {strings.Replace(minimal, "    agents: [agent-a]\n", "", 1), "resources.main.agents"},
		"bad agent":           {strings.Replace(minimal, "[agent-a]", "[Agent-A]", 1), "resources.main.agents"},
		"bad tls":             {withResourceKeys("tls: require"), "resources.main.tls"},
		"long description":    {withResourceKeys("description: " + strings.Repeat("x", 501)), "resources.main.description"},
		"bad glob":            {withResourceKeys("redact:\n  columns: [\"[\"]"), "redact.columns"},
		"duplicate key":       {minimal + "version: 1\n", "line 10: duplicate key"},
		"second document":     {minimal + "---\nversion: 1\n", "document"},
		"string limit":        {withDefaults("limits:\n  max_rows: lots\n"), "line 12: wrong type"},
		"integer duration":    {withDefaults("limits:\n  statement_timeout: 15\n"), "line 12: wrong type"},
		"sub-ms timeout":      {withDefaults("limits:\n  lock_timeout: 500us\n"), "lock_timeout"},
		"resources not a map": {strings.SplitN(minimal, "resources:", 2)[0] + "resources: [a]\n", "line 5: wrong type"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			mustReject(t, c.text, c.want)
		})
	}
	mustParse(t, withResourceKeys("description: "+strings.Repeat("é", 500)))
}

func TestSecretNames(t *testing.T) {
	cfg := mustParse(t, minimal+`  other:
    kind: postgres
    connection: secret:sag-other
    tls_ca: secret:sag-ca
    agents: [agent-b]
  third:
    kind: postgres
    connection: secret:sag-main
    tls_ca: secret:sag-ca
    agents: [agent-a]
`)
	want := []string{"sag-ca", "sag-main", "sag-other", "sag-tls-cert", "sag-tls-key"}
	if got := cfg.SecretNames(); !slices.Equal(got, want) {
		t.Fatalf("SecretNames = %q; want %q", got, want)
	}
}

func TestHasAgent(t *testing.T) {
	cfg := mustParse(t, minimal+`  other:
    kind: postgres
    connection: secret:sag-other
    agents: [agent-b]
`)
	for sub, want := range map[string]bool{"agent-a": true, "agent-b": true, "agent-c": false, "": false} {
		if got := cfg.HasAgent(sub); got != want {
			t.Fatalf("HasAgent(%q) = %v; want %v", sub, got, want)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("Load accepted a missing file")
	}
}

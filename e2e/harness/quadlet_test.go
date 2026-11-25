package harness

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/secret"
)

type quadletKey struct {
	Section string
	Key     string
	Value   string
}

func readQuadlet(t *testing.T) []quadletKey {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "deploy", "sag.container"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var keys []quadletKey
	section := ""
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		switch {
		case text == "" || strings.HasPrefix(text, "#") || strings.HasPrefix(text, ";"):
		case strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]"):
			section = text[1 : len(text)-1]
		default:
			key, value, ok := strings.Cut(text, "=")
			if !ok || section == "" {
				t.Fatalf("deploy/sag.container line %d: %q is not a key in a section", line, text)
			}
			keys = append(keys, quadletKey{Section: section, Key: key, Value: value})
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return keys
}

func containerValues(keys []quadletKey, key string) []string {
	var values []string
	for _, k := range keys {
		if k.Section == "Container" && k.Key == key {
			values = append(values, k.Value)
		}
	}
	return values
}

func singleValue(t *testing.T, keys []quadletKey, key string) string {
	t.Helper()
	values := containerValues(keys, key)
	if len(values) != 1 {
		t.Fatalf("[Container] %s occurs %d times; want 1", key, len(values))
	}
	return values[0]
}

func TestQuadletMatchesHarnessFlags(t *testing.T) {
	keys := readQuadlet(t)
	f := ContainerFlags

	scalar := []struct {
		key  string
		want string
	}{
		{"ReadOnly", strconv.FormatBool(f.ReadOnly)},
		{"User", strconv.Itoa(f.UID)},
		{"Group", strconv.Itoa(f.GID)},
		{"DropCapability", f.CapDrop},
		{"NoNewPrivileges", strconv.FormatBool(f.NoNewPrivileges)},
		{"PublishPort", f.Publish(f.ContainerPort)},
	}
	for _, s := range scalar {
		if got := singleValue(t, keys, s.key); got != s.want {
			t.Errorf("[Container] %s = %q; want %q", s.key, got, s.want)
		}
	}

	volumes := containerValues(keys, "Volume")
	if len(volumes) != 2 {
		t.Fatalf("[Container] Volume lines = %q; want the config volume and the state volume", volumes)
	}
	var config, state []string
	for _, v := range volumes {
		source, rest, _ := strings.Cut(v, ":")
		target, _, _ := strings.Cut(rest, ":")
		switch target {
		case f.ConfigTarget:
			config = append(config, v)
			if want := f.ConfigVolume(source); v != want {
				t.Errorf("config Volume = %q; want %q", v, want)
			}
		case f.StateTarget:
			state = append(state, v)
			if want := f.StateVolume(source); v != want {
				t.Errorf("state Volume = %q; want %q", v, want)
			}
		default:
			t.Errorf("Volume %q has target %q; want %s or %s", v, target, f.ConfigTarget, f.StateTarget)
		}
	}
	if len(config) != 1 || len(state) != 1 {
		t.Errorf("config volumes %q, state volumes %q; want one of each", config, state)
	}

	for _, s := range containerValues(keys, "Secret") {
		name, _, _ := strings.Cut(s, ",")
		if want := f.Secret(name, name, ""); s != want {
			t.Errorf("Secret = %q; want %q", s, want)
		}
	}

	for _, k := range keys {
		if k.Section != "Container" {
			continue
		}
		switch k.Key {
		case "LogDriver", "PodmanArgs", "Network", "AddCapability", "SecurityLabelDisable":
			t.Errorf("[Container] %s=%s is not a product flag", k.Key, k.Value)
		}
		if strings.Contains(k.Value, BusyboxTarget) || strings.Contains(k.Value, "busybox") {
			t.Errorf("[Container] %s=%s holds the test-only busybox", k.Key, k.Value)
		}
	}
}

func TestPublishPortLoopbackOrTailnet(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"100.64.0.0", true},
		{"100.101.102.103", true},
		{"100.127.255.255", true},
		{"100.63.255.255", false},
		{"100.128.0.0", false},
		{"127.0.0.2", false},
		{"0.0.0.0", false},
		{"::", false},
		{"::1", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:100.64.0.1", false},
		{"", false},
		{"localhost", false},
		{"203.0.113.10", false},
		{"10.0.0.1", false},
	}
	for _, c := range cases {
		if got := PublishHostAllowed(c.host); got != c.want {
			t.Errorf("PublishHostAllowed(%q) = %v; want %v", c.host, got, c.want)
		}
	}

	if !PublishHostAllowed(ContainerFlags.PublishHost) {
		t.Errorf("ContainerFlags.PublishHost %q is not allowed", ContainerFlags.PublishHost)
	}
	for _, p := range containerValues(readQuadlet(t), "PublishPort") {
		host, _, err := net.SplitHostPort(strings.TrimSuffix(p, ":"+strconv.Itoa(ContainerFlags.ContainerPort)))
		if err != nil {
			t.Fatalf("PublishPort %q: %v", p, err)
		}
		if !PublishHostAllowed(host) {
			t.Errorf("PublishPort %q has host %q; want 127.0.0.1 or an address in 100.64.0.0/10", p, host)
		}
	}
}

func TestOneSecretLinePerSecret(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "deploy", "dev", "sag.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{secret.MasterKeyName}, cfg.SecretNames()...)
	slices.Sort(want)

	var got []string
	for _, s := range containerValues(readQuadlet(t), "Secret") {
		fields := strings.Split(s, ",")
		source := fields[0]
		target := ""
		for _, field := range fields[1:] {
			if v, ok := strings.CutPrefix(field, "target="); ok {
				target = v
			}
		}
		if target != source {
			t.Errorf("Secret %q: target %q; want the source name %q", s, target, source)
		}
		got = append(got, target)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("Secret targets = %q; want %q", got, want)
	}
	if len(slices.Compact(slices.Clone(got))) != len(got) {
		t.Errorf("Secret targets %q have a duplicate", got)
	}
}

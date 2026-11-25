package harness

import (
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/kanthorlabs/sag/internal/core"
)

func composeFixture() []AC {
	return []AC{
		{
			ID:     "AC90",
			Agents: []AgentSpec{{Sub: "ac90", Name: "Agent 90"}},
			Resources: []ResourceSpec{{
				Name:     "ac90-main",
				Agents:   []string{"ac90"},
				Fragment: "description: Main resource.\nlimits:\n  max_rows: 5\n",
			}},
		},
		{
			ID:     "AC91",
			Agents: []AgentSpec{{Sub: "ac91"}},
			Resources: []ResourceSpec{
				{Name: "ac91-skip", Agents: []string{"ac91"}, TLS: core.TLSInsecureSkipVerify, NoTLSCA: true},
				{Name: "ac91-v2", Containers: []string{ContainerSAG, ContainerV2}, Agents: []string{"ac91"}},
				{Name: "ac91-rate", Containers: []string{ContainerRate}, Agents: []string{"ac91"}},
			},
		},
	}
}

func parseComposed(t *testing.T, text string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("composed config is not YAML: %v\n%s", err, text)
	}
	return doc
}

func path(t *testing.T, doc map[string]any, keys ...string) any {
	t.Helper()
	var cur any = doc
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("%s: not a mapping at %q", strings.Join(keys, "."), k)
		}
		cur, ok = m[k]
		if !ok {
			return nil
		}
	}
	return cur
}

func TestComposeConfigFromFragments(t *testing.T) {
	text, err := Compose(composeFixture(), ContainerSAG)
	if err != nil {
		t.Fatal(err)
	}
	doc := parseComposed(t, text)

	checks := []struct {
		keys []string
		want any
	}{
		{[]string{"version"}, 1},
		{[]string{"server", "tls_cert"}, "secret:sag-tls-cert"},
		{[]string{"server", "tls_key"}, "secret:sag-tls-key"},
		{[]string{"preflight", "recheck_interval"}, "10s"},
		{[]string{"auth"}, nil},
		{[]string{"resources", "ac90-main", "kind"}, "postgres"},
		{[]string{"resources", "ac90-main", "connection"}, "secret:conn-ac90-main"},
		{[]string{"resources", "ac90-main", "tls_ca"}, "secret:ca-ac90-main"},
		{[]string{"resources", "ac90-main", "tls"}, nil},
		{[]string{"resources", "ac90-main", "agents"}, []any{"ac90"}},
		{[]string{"resources", "ac90-main", "description"}, "Main resource."},
		{[]string{"resources", "ac90-main", "limits", "max_rows"}, 5},
		{[]string{"resources", "ac91-skip", "tls"}, "insecure-skip-verify"},
		{[]string{"resources", "ac91-skip", "tls_ca"}, nil},
		{[]string{"resources", "ac91-v2", "connection"}, "secret:conn-ac91-v2"},
		{[]string{"resources", "ac91-rate"}, nil},
	}
	for _, c := range checks {
		if got := path(t, doc, c.keys...); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s = %#v, want %#v", strings.Join(c.keys, "."), got, c.want)
		}
	}
}

func TestComposeContainerProfiles(t *testing.T) {
	v2, err := Compose(composeFixture(), ContainerV2)
	if err != nil {
		t.Fatal(err)
	}
	doc := parseComposed(t, v2)
	if got := path(t, doc, "auth", "version"); got != 2 {
		t.Errorf("sag-v2 auth.version = %v, want 2", got)
	}
	resources, _ := path(t, doc, "resources").(map[string]any)
	if len(resources) != 1 || resources["ac91-v2"] == nil {
		t.Errorf("sag-v2 resources = %v, want only ac91-v2", resources)
	}

	rate, err := Compose(composeFixture(), ContainerRate)
	if err != nil {
		t.Fatal(err)
	}
	doc = parseComposed(t, rate)
	if got := path(t, doc, "defaults", "limits", "max_concurrent_per_agent"); got != 4 {
		t.Errorf("sag-rate max_concurrent_per_agent = %v, want 4", got)
	}
}

func TestComposeUnknownContainerUsesSAG(t *testing.T) {
	sag, err := Compose(composeFixture(), ContainerSAG)
	if err != nil {
		t.Fatal(err)
	}
	other, err := Compose(composeFixture(), "ac02-mode")
	if err != nil {
		t.Fatal(err)
	}
	if sag != other {
		t.Errorf("config of an unknown container differs from sag:\n%s\n---\n%s", other, sag)
	}
}

func TestComposeRejectsFragmentConflict(t *testing.T) {
	acs := []AC{{
		ID:        "AC90",
		Agents:    []AgentSpec{{Sub: "ac90"}},
		Resources: []ResourceSpec{{Name: "r", Agents: []string{"ac90"}, Fragment: "connection: secret:other\n"}},
	}}
	if _, err := Compose(acs, ContainerSAG); err == nil {
		t.Fatal("Compose accepted a fragment that overrides connection")
	}
}

func TestUniqueAgents(t *testing.T) {
	if err := Validate(composeFixture()); err != nil {
		t.Fatalf("Validate(valid) = %v", err)
	}
	cases := map[string][]AC{
		"agent in two ACs": {
			{ID: "AC90", Agents: []AgentSpec{{Sub: "shared"}}},
			{ID: "AC91", Agents: []AgentSpec{{Sub: "shared"}}},
		},
		"agent twice in one AC": {
			{ID: "AC90", Agents: []AgentSpec{{Sub: "ac90"}, {Sub: "ac90"}}},
		},
		"resource agent of another AC": {
			{ID: "AC90", Agents: []AgentSpec{{Sub: "ac90"}}},
			{ID: "AC91", Agents: []AgentSpec{{Sub: "ac91"}}, Resources: []ResourceSpec{{Name: "r", Agents: []string{"ac90"}}}},
		},
		"resource in two ACs": {
			{ID: "AC90", Agents: []AgentSpec{{Sub: "ac90"}}, Resources: []ResourceSpec{{Name: "r", Agents: []string{"ac90"}}}},
			{ID: "AC91", Agents: []AgentSpec{{Sub: "ac91"}}, Resources: []ResourceSpec{{Name: "r", Agents: []string{"ac91"}}}},
		},
		"invalid sub": {
			{ID: "AC90", Agents: []AgentSpec{{Sub: "AC90"}}},
		},
		"invalid resource name": {
			{ID: "AC90", Agents: []AgentSpec{{Sub: "ac90"}}, Resources: []ResourceSpec{{Name: "Main", Agents: []string{"ac90"}}}},
		},
		"unknown container": {
			{ID: "AC90", Agents: []AgentSpec{{Sub: "ac90"}}, Resources: []ResourceSpec{{Name: "r", Containers: []string{"nope"}, Agents: []string{"ac90"}}}},
		},
		"duplicate AC ID": {
			{ID: "AC90", Agents: []AgentSpec{{Sub: "ac90"}}},
			{ID: "AC90", Agents: []AgentSpec{{Sub: "ac91"}}},
		},
	}
	for name, acs := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(acs); err == nil {
				t.Fatal("Validate accepted the set")
			}
		})
	}
}

func TestComposeValidates(t *testing.T) {
	acs := []AC{
		{ID: "AC90", Agents: []AgentSpec{{Sub: "shared"}}},
		{ID: "AC91", Agents: []AgentSpec{{Sub: "shared"}}},
	}
	if _, err := Compose(acs, ContainerSAG); err == nil {
		t.Fatal("Compose accepted duplicate agents")
	}
}

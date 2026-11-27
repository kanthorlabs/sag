package harness

import (
	"fmt"
	"regexp"
	"slices"

	"go.yaml.in/yaml/v3"
)

const (
	TLSCertSecret   = "sag-tls-cert"
	TLSKeySecret    = "sag-tls-key"
	RecheckInterval = "10s"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type containerProfile struct {
	authVersion   int
	maxConcurrent int
	readOnlyAudit bool
}

var containerProfiles = map[string]containerProfile{
	ContainerSAG:         {},
	ContainerConformance: {},
	ContainerV2:          {authVersion: 2},
	ContainerROAudit:     {readOnlyAudit: true},
	ContainerRate:        {maxConcurrent: 4},
}

func ConnectionSecret(resource string) string {
	return "conn-" + resource
}

func CASecret(resource string) string {
	return "ca-" + resource
}

func resourceContainers(r ResourceSpec) []string {
	if len(r.Containers) == 0 {
		return []string{ContainerSAG}
	}
	return r.Containers
}

func Validate(acs []AC) error {
	acIDs := map[string]bool{}
	agentOwner := map[string]string{}
	resourceOwner := map[string]string{}
	for _, ac := range acs {
		if ac.ID == "" {
			return fmt.Errorf("harness: AC without ID")
		}
		if acIDs[ac.ID] {
			return fmt.Errorf("harness: AC %s registered twice", ac.ID)
		}
		acIDs[ac.ID] = true
		own := map[string]bool{}
		for _, a := range ac.Agents {
			if !namePattern.MatchString(a.Sub) {
				return fmt.Errorf("harness: %s: agent sub %q does not match %s", ac.ID, a.Sub, namePattern)
			}
			if prev, ok := agentOwner[a.Sub]; ok {
				return fmt.Errorf("harness: agent %q in %s and %s", a.Sub, prev, ac.ID)
			}
			agentOwner[a.Sub] = ac.ID
			own[a.Sub] = true
		}
		for _, r := range ac.Resources {
			if !namePattern.MatchString(r.Name) {
				return fmt.Errorf("harness: %s: resource name %q does not match %s", ac.ID, r.Name, namePattern)
			}
			if prev, ok := resourceOwner[r.Name]; ok {
				return fmt.Errorf("harness: resource %q in %s and %s", r.Name, prev, ac.ID)
			}
			resourceOwner[r.Name] = ac.ID
			for _, c := range resourceContainers(r) {
				if _, ok := containerProfiles[c]; !ok {
					return fmt.Errorf("harness: %s: resource %q names unknown container %q", ac.ID, r.Name, c)
				}
			}
			for _, sub := range r.Agents {
				if !own[sub] {
					return fmt.Errorf("harness: %s: resource %q lists agent %q that %s does not declare", ac.ID, r.Name, sub, ac.ID)
				}
			}
		}
	}
	return nil
}

func Compose(acs []AC, container string) (string, error) {
	if err := Validate(acs); err != nil {
		return "", err
	}
	profile, known := containerProfiles[container]
	if !known {
		container = ContainerSAG
	}

	doc := map[string]any{
		"version": 1,
		"server": map[string]any{
			"tls_cert": "secret:" + TLSCertSecret,
			"tls_key":  "secret:" + TLSKeySecret,
		},
		"preflight": map[string]any{"recheck_interval": RecheckInterval},
	}
	if profile.authVersion != 0 {
		doc["auth"] = map[string]any{"version": profile.authVersion}
	}
	if profile.maxConcurrent != 0 {
		doc["defaults"] = map[string]any{"limits": map[string]any{"max_concurrent_per_agent": profile.maxConcurrent}}
	}

	resources := map[string]any{}
	for _, ac := range acs {
		for _, r := range ac.Resources {
			if !slices.Contains(resourceContainers(r), container) {
				continue
			}
			body, err := resourceBody(r)
			if err != nil {
				return "", fmt.Errorf("harness: %s: resource %q: %w", ac.ID, r.Name, err)
			}
			resources[r.Name] = body
		}
	}
	doc["resources"] = resources

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func resourceBody(r ResourceSpec) (map[string]any, error) {
	body := map[string]any{
		"kind":       "postgres",
		"connection": "secret:" + ConnectionSecret(r.Name),
		"agents":     slices.Clone(r.Agents),
	}
	if body["agents"] == nil {
		body["agents"] = []string{}
	}
	if r.TLS != "" {
		body["tls"] = string(r.TLS)
	}
	if !r.NoTLSCA {
		body["tls_ca"] = "secret:" + CASecret(r.Name)
	}
	if r.Fragment == "" {
		return body, nil
	}
	var fragment map[string]any
	if err := yaml.Unmarshal([]byte(r.Fragment), &fragment); err != nil {
		return nil, fmt.Errorf("fragment: %w", err)
	}
	for k, v := range fragment {
		if _, taken := body[k]; taken {
			return nil, fmt.Errorf("fragment key %q conflicts with a harness key", k)
		}
		body[k] = v
	}
	return body, nil
}

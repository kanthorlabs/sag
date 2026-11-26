//go:build devxe2e

package devx_test

import (
	"context"
	"encoding/json"
	"maps"
	"sort"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	credentialTests["DXAC17_ListInspect"] = func(t *testing.T, ctx context.Context) {
		DXAC17_ListInspect(t, ctx)
	}
}

func DXAC17_ListInspect(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	canary := dxh.ActiveCanaryParts

	t.Log("text list")
	listOut, listErr := runSAGBothCapture(t, ctx, "credential", "list")
	if strings.Contains(listOut+listErr, canary.Password) {
		t.Fatal("credential list (text): output contains canary password")
	}
	if strings.Contains(listOut+listErr, canary.User) && canary.User != "" {
		t.Logf("credential list (text): contains canary user; checking context")
	}

	t.Log("text inspect canary")
	inspectOut, inspectErr := runSAGBothCapture(t, ctx, "credential", "inspect", "canary")
	_ = inspectErr
	if strings.Contains(inspectOut+inspectErr, canary.Password) {
		t.Fatal("credential inspect (text): output contains canary password")
	}

	t.Log("json list")
	jsonListOut, jsonListErr := runSAGBothCapture(t, ctx, "credential", "list", "--json")
	_ = jsonListErr
	if strings.Contains(jsonListOut+jsonListErr, canary.Password) {
		t.Fatal("credential list --json: output contains canary password")
	}

	var listResult struct {
		Credentials []map[string]any `json:"credentials"`
	}
	if err := json.Unmarshal([]byte(jsonListOut), &listResult); err != nil {
		t.Fatalf("credential list --json: parse: %v\noutput: %s", err, jsonListOut)
	}
	if len(listResult.Credentials) == 0 {
		t.Fatal("credential list --json: no credentials")
	}
	checkEntryFieldSet(t, "list", listResult.Credentials[0], false)
	for _, cred := range listResult.Credentials {
		if secrets, ok := cred["secrets"].([]any); ok {
			for _, s := range secrets {
				if sm, ok := s.(map[string]any); ok {
					checkSecretEntryFieldSet(t, "list secret", sm)
				}
			}
		}
	}

	t.Log("json inspect canary")
	jsonInspectOut, jsonInspectErr := runSAGBothCapture(t, ctx, "credential", "inspect", "canary", "--json")
	_ = jsonInspectErr
	if strings.Contains(jsonInspectOut+jsonInspectErr, canary.Password) {
		t.Fatal("credential inspect --json: output contains canary password")
	}

	var inspectResult map[string]any
	if err := json.Unmarshal([]byte(jsonInspectOut), &inspectResult); err != nil {
		t.Fatalf("credential inspect --json: parse: %v\noutput: %s", err, jsonInspectOut)
	}
	checkEntryFieldSet(t, "inspect", inspectResult, true)
	if secrets, ok := inspectResult["secrets"].([]any); ok {
		for _, s := range secrets {
			if sm, ok := s.(map[string]any); ok {
				checkSecretEntryFieldSet(t, "inspect secret", sm)
			}
		}
	}
}

func checkEntryFieldSet(t *testing.T, label string, obj map[string]any, isInspect bool) {
	t.Helper()
	wantKeys := map[string]bool{
		"name": true, "kind": true, "description": true,
		"tls": true, "agents": true, "secrets": true,
	}
	if isInspect {
		wantKeys["container_running"] = true
	}
	gotKeys := map[string]bool{}
	for k := range obj {
		gotKeys[k] = true
	}
	if !maps.Equal(wantKeys, gotKeys) {
		t.Errorf("%s: entry keys mismatch: got %v, want %v", label, sortedBoolKeys(gotKeys), sortedBoolKeys(wantKeys))
	}
}

func checkSecretEntryFieldSet(t *testing.T, label string, obj map[string]any) {
	t.Helper()
	requiredKeys := map[string]bool{
		"name": true, "backend": true, "role": true,
		"exists": true, "owned": true, "created_at": true,
	}
	for k := range requiredKeys {
		if _, ok := obj[k]; !ok {
			t.Errorf("%s: missing key %q", label, k)
		}
	}
	allowedExtras := map[string]bool{"mounted": true}
	for k := range obj {
		if !requiredKeys[k] && !allowedExtras[k] {
			t.Errorf("%s: unexpected key %q", label, k)
		}
	}
}

func runSAGBothCapture(t *testing.T, ctx context.Context, args ...string) (stdout, stderr string) {
	t.Helper()
	return runSAGWithStderr(t, ctx, args...)
}

func sortedBoolKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

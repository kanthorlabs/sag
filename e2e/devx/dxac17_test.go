//go:build devxe2e

package devx_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
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
	uid := fmt.Sprintf("%d", os.Getuid())

	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("waitLaunchAgentIdle: %v", err)
	}

	t.Log("text list")
	listOut, listErr, listRunErr := runSAGWithStderr(t, ctx, "credential", "list")
	if listRunErr != nil {
		t.Fatalf("credential list (text): %v", listRunErr)
	}
	listText := listOut + listErr
	assertNoCanaryPart(t, "credential list (text)", listText, canary)
	assertSection8FieldsInText(t, "credential list (text)", listText, false)

	t.Log("text inspect canary")
	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("waitLaunchAgentIdle: %v", err)
	}
	inspectOut, inspectErr, inspectRunErr := runSAGWithStderr(t, ctx, "credential", "inspect", "canary")
	if inspectRunErr != nil {
		t.Fatalf("credential inspect canary (text): %v", inspectRunErr)
	}
	inspectText := inspectOut + inspectErr
	assertNoCanaryPart(t, "credential inspect (text)", inspectText, canary)
	assertSection8FieldsInText(t, "credential inspect (text)", inspectText, true)

	t.Log("json list")
	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("waitLaunchAgentIdle: %v", err)
	}
	jsonListOut, jsonListErr, jsonListRunErr := runSAGWithStderr(t, ctx, "credential", "list", "--json")
	if jsonListRunErr != nil {
		t.Fatalf("credential list --json: %v", jsonListRunErr)
	}
	assertNoCanaryPart(t, "credential list --json", jsonListOut+jsonListErr, canary)

	var listResult struct {
		Credentials []map[string]any `json:"credentials"`
	}
	if err := json.Unmarshal([]byte(jsonListOut), &listResult); err != nil {
		t.Fatalf("credential list --json: parse: %v", err)
	}
	if len(listResult.Credentials) == 0 {
		t.Fatal("credential list --json: no credentials")
	}

	canaryEntry := findEntryByName(listResult.Credentials, "canary")
	if canaryEntry == nil {
		t.Fatal("credential list --json: no entry with name=canary")
	}
	checkEntryFieldSet(t, "list", canaryEntry, false)
	if secrets, ok := canaryEntry["secrets"].([]any); ok {
		for _, s := range secrets {
			if sm, ok := s.(map[string]any); ok {
				checkSecretEntryFieldSet(t, "list secret", sm)
				assertSecretValues(t, "list secret", sm)
			}
		}
	}

	t.Log("json inspect canary")
	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("waitLaunchAgentIdle: %v", err)
	}
	jsonInspectOut, jsonInspectErr, jsonInspectRunErr := runSAGWithStderr(t, ctx, "credential", "inspect", "canary", "--json")
	if jsonInspectRunErr != nil {
		t.Fatalf("credential inspect --json: %v", jsonInspectRunErr)
	}
	assertNoCanaryPart(t, "credential inspect --json", jsonInspectOut+jsonInspectErr, canary)

	var inspectResult map[string]any
	if err := json.Unmarshal([]byte(jsonInspectOut), &inspectResult); err != nil {
		t.Fatalf("credential inspect --json: parse: %v", err)
	}
	checkEntryFieldSet(t, "inspect", inspectResult, true)
	if cr, ok := inspectResult["container_running"].(bool); !ok || !cr {
		t.Errorf("inspect: container_running must be true; got %v", inspectResult["container_running"])
	}
	if secrets, ok := inspectResult["secrets"].([]any); ok {
		for _, s := range secrets {
			if sm, ok := s.(map[string]any); ok {
				checkSecretEntryFieldSet(t, "inspect secret", sm)
				assertSecretValues(t, "inspect secret", sm)
			}
		}
	}
}

func findEntryByName(entries []map[string]any, name string) map[string]any {
	for _, e := range entries {
		if n, ok := e["name"].(string); ok && n == name {
			return e
		}
	}
	return nil
}

func assertNoCanaryPart(t *testing.T, label, text string, canary dxh.CanaryParts) {
	t.Helper()
	parts := map[string]string{
		"password": canary.Password,
		"host":     canary.Host,
		"user":     canary.User,
		"database": canary.Database,
	}
	for partName, value := range parts {
		if value != "" && strings.Contains(text, value) {
			t.Errorf("%s: contains canary %s (value not logged per security rules)", label, partName)
		}
	}
}

func assertSection8FieldsInText(t *testing.T, label, text string, isInspect bool) {
	t.Helper()
	labels := []string{
		"name: ", "kind: ", "description: ", "tls: ", "agents: ",
		".name: ", ".backend: ", ".role: ", ".exists: ", ".owned: ", ".created_at: ", ".mounted: ",
	}
	for _, lbl := range labels {
		if !strings.Contains(text, lbl) {
			t.Errorf("%s: text output missing label %q", label, lbl)
		}
	}
	if isInspect {
		if !strings.Contains(text, "container_running: ") {
			t.Errorf("%s: text output missing field %q", label, "container_running")
		}
	}
}

func assertSecretValues(t *testing.T, label string, sm map[string]any) {
	t.Helper()
	if exists, ok := sm["exists"].(bool); !ok || !exists {
		t.Errorf("%s: exists must be true; got %v", label, sm["exists"])
	}
	if owned, ok := sm["owned"].(bool); !ok || !owned {
		t.Errorf("%s: owned must be true; got %v", label, sm["owned"])
	}
	if mounted, ok := sm["mounted"].(bool); !ok || !mounted {
		t.Errorf("%s: mounted must be true; got %v", label, sm["mounted"])
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
	wantKeys := map[string]bool{
		"name": true, "backend": true, "role": true,
		"exists": true, "owned": true, "created_at": true, "mounted": true,
	}
	gotKeys := map[string]bool{}
	for k := range obj {
		gotKeys[k] = true
	}
	if !maps.Equal(wantKeys, gotKeys) {
		t.Errorf("%s: secret keys mismatch: got %v, want %v", label, sortedBoolKeys(gotKeys), sortedBoolKeys(wantKeys))
	}
}

func sortedBoolKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

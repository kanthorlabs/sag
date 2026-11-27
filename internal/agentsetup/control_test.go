package agentsetup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func testWriteJSON(t *testing.T) func() {
	t.Helper()
	old := writeJSONFn
	writeJSONFn = func(path string, keyPath []string, value any) error {
		t.Helper()
		data, err := os.ReadFile(path)
		var top map[string]json.RawMessage
		if err == nil {
			json.Unmarshal(data, &top)
		}
		if top == nil {
			top = map[string]json.RawMessage{}
		}
		if len(keyPath) == 2 {
			inner := map[string]json.RawMessage{}
			if existing, ok := top[keyPath[0]]; ok {
				json.Unmarshal(existing, &inner)
			}
			raw, _ := json.Marshal(value)
			inner[keyPath[1]] = raw
			top[keyPath[0]], _ = json.Marshal(inner)
		}
		out, _ := json.Marshal(top)
		return os.WriteFile(path, out, 0o600)
	}
	return func() { writeJSONFn = old }
}

func TestDenyRulesNoDuplicate(t *testing.T) {
	dir := t.TempDir()
	restore := testWriteJSON(t)
	defer restore()

	opts := Options{DenyRules: "yes", CfgDir: dir}
	ctx := context.Background()

	if err := applyDenyRules(ctx, opts, dir); err != nil {
		t.Fatalf("first applyDenyRules: %v", err)
	}

	rules1 := mustReadDenyRules(t, dir)
	for _, rule := range sagDenyRules {
		if !slices.Contains(rules1, rule) {
			t.Errorf("rule %q missing after first apply", rule)
		}
	}
	if len(rules1) != len(sagDenyRules) {
		t.Errorf("deny rules count = %d; want %d", len(rules1), len(sagDenyRules))
	}

	if err := applyDenyRules(ctx, opts, dir); err != nil {
		t.Fatalf("second applyDenyRules: %v", err)
	}

	rules2 := mustReadDenyRules(t, dir)
	if len(rules2) != len(sagDenyRules) {
		t.Errorf("deny rules count after second apply = %d; want %d (duplicate added)", len(rules2), len(sagDenyRules))
	}
}

func TestDenyRulesNoKeepsFile(t *testing.T) {
	dir := t.TempDir()
	restore := testWriteJSON(t)
	defer restore()

	settings := filepath.Join(dir, "settings.json")
	initial := `{"other":"value"}`
	if err := os.WriteFile(settings, []byte(initial), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	opts := Options{DenyRules: "no", CfgDir: dir}
	ctx := context.Background()

	if err := applyDenyRules(ctx, opts, dir); err != nil {
		t.Fatalf("applyDenyRules: %v", err)
	}

	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if string(data) != initial {
		t.Errorf("settings.json changed; got %s; want %s", data, initial)
	}
}

func TestDenyRulesPreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	restore := testWriteJSON(t)
	defer restore()

	settings := filepath.Join(dir, "settings.json")
	initial := `{"other":"value","permissions":{"allow":["*"]}}`
	if err := os.WriteFile(settings, []byte(initial), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	opts := Options{DenyRules: "yes", CfgDir: dir}
	if err := applyDenyRules(context.Background(), opts, dir); err != nil {
		t.Fatalf("applyDenyRules: %v", err)
	}

	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	if _, ok := top["other"]; !ok {
		t.Error("key 'other' was removed")
	}
}

func TestGuardMarkerOnly(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "extensions")
	if err := os.MkdirAll(extDir, 0o700); err != nil {
		t.Fatal(err)
	}
	guardPath := filepath.Join(extDir, "sag-guard.ts")
	if err := os.WriteFile(guardPath, []byte("// some other content\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := Options{Guard: "yes"}
	err := applyGuard(context.Background(), opts, dir, []byte(guardMarker+"\n"))
	if err == nil {
		t.Fatal("applyGuard: expected error for file without marker; got nil")
	}

	data, _ := os.ReadFile(guardPath)
	if string(data) != "// some other content\n" {
		t.Error("applyGuard overwrote file that lacked the marker")
	}
}

func TestGuardWritesNewFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte(guardMarker + "\nsome content\n")

	opts := Options{Guard: "yes"}
	if err := applyGuard(context.Background(), opts, dir, content); err != nil {
		t.Fatalf("applyGuard: %v", err)
	}

	guardPath := filepath.Join(dir, "extensions", "sag-guard.ts")
	data, err := os.ReadFile(guardPath)
	if err != nil {
		t.Fatalf("read guard: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("guard content = %q; want %q", data, content)
	}
}

func TestGuardOverwritesWithMarker(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "extensions")
	if err := os.MkdirAll(extDir, 0o700); err != nil {
		t.Fatal(err)
	}
	guardPath := filepath.Join(extDir, "sag-guard.ts")
	old := []byte(guardMarker + "\nold content\n")
	if err := os.WriteFile(guardPath, old, 0o600); err != nil {
		t.Fatal(err)
	}

	newContent := []byte(guardMarker + "\nnew content\n")
	opts := Options{Guard: "yes"}
	if err := applyGuard(context.Background(), opts, dir, newContent); err != nil {
		t.Fatalf("applyGuard: %v", err)
	}

	data, err := os.ReadFile(guardPath)
	if err != nil {
		t.Fatalf("read guard: %v", err)
	}
	if string(data) != string(newContent) {
		t.Errorf("guard content = %q; want %q", data, newContent)
	}
}

func TestGuardNoOpWhenAnswerNo(t *testing.T) {
	dir := t.TempDir()
	opts := Options{Guard: "no"}
	err := applyGuard(context.Background(), opts, dir, []byte("content"))
	if err != nil {
		t.Fatalf("applyGuard with 'no': %v", err)
	}
	guardPath := filepath.Join(dir, "extensions", "sag-guard.ts")
	if _, err := os.Stat(guardPath); !os.IsNotExist(err) {
		t.Error("guard file created when answer is 'no'")
	}
}

func mustReadDenyRules(t *testing.T, dir string) []string {
	t.Helper()
	rules, err := readDenyRules(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("readDenyRules: %v", err)
	}
	return rules
}

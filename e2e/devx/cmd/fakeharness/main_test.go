package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordsArgsEnvDir(t *testing.T) {
	dir := t.TempDir()
	wd := t.TempDir()

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(wd); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(prev) }()

	t.Setenv("HOME", t.TempDir())

	recordCall(dir, []string{"mcp", "list"})

	data, err := os.ReadFile(filepath.Join(dir, "stub_records.json"))
	if err != nil {
		t.Fatalf("read stub_records.json: %v", err)
	}
	var rec callRecord
	if err := json.Unmarshal(bytes.TrimSpace(data), &rec); err != nil {
		t.Fatalf("parse record: %v", err)
	}
	if len(rec.Argv) != 2 || rec.Argv[0] != "mcp" || rec.Argv[1] != "list" {
		t.Errorf("argv = %v; want [mcp list]", rec.Argv)
	}
	if _, ok := rec.Env["HOME"]; !ok {
		t.Error("env missing HOME")
	}
	wdReal, _ := filepath.EvalSymlinks(wd)
	if rec.Dir != wd && rec.Dir != wdReal {
		t.Errorf("dir = %q; want %q (or %q)", rec.Dir, wd, wdReal)
	}
}

func TestClaudeLineMatchesRecord(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	code := runClaude(&stdout, &bytes.Buffer{}, dir, []string{"mcp", "list"})
	if code != 0 {
		t.Fatalf("exit %d; want 0", code)
	}
	got := strings.TrimSpace(stdout.String())
	if got != claudeSagLine {
		t.Errorf("output = %q; want %q", got, claudeSagLine)
	}

	harnessForms, err := os.ReadFile("../../../../e2e/devx/testdata/harness-forms.txt")
	if err != nil {
		t.Fatalf("read harness-forms.txt: %v", err)
	}
	var sagLine string
	for _, line := range strings.Split(string(harnessForms), "\n") {
		if strings.HasPrefix(line, "sag: ") {
			sagLine = strings.TrimSpace(line)
			break
		}
	}
	if sagLine == "" {
		t.Fatal("harness-forms.txt has no sag: line")
	}
	if got != sagLine {
		t.Errorf("fakeharness claude output %q != harness-forms.txt sag line %q", got, sagLine)
	}
}

func TestPiConnectedState(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	code := runPi(&stdout, &bytes.Buffer{}, dir, []string{"mcp", "list", "--json"})
	if code != 0 {
		t.Fatalf("exit %d; want 0", code)
	}
	var result struct {
		Servers []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if len(result.Servers) != 1 || result.Servers[0].Name != "sag" || result.Servers[0].State != "connected" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestPiDisconnectedState(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pi_sag_state"), []byte("disconnected"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	code := runPi(&stdout, &bytes.Buffer{}, dir, []string{"mcp", "list", "--json"})
	if code == 0 {
		t.Fatal("exit 0; want non-zero for disconnected state")
	}
	var result struct {
		Servers []struct {
			State string `json:"state"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if len(result.Servers) != 1 || result.Servers[0].State != "disconnected" {
		t.Errorf("state = %q; want disconnected", result.Servers[0].State)
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const claudeSagLine = "sag: https://localhost:8443/mcp (HTTP) - ✔ Connected"

type callRecord struct {
	Argv       []string          `json:"argv"`
	Env        map[string]string `json:"env"`
	Dir        string            `json:"dir"`
	DirEntries []string          `json:"dir_entries"`
}

func main() {
	os.Exit(run(os.Stdout, os.Stderr, os.Args))
}

func run(stdout, stderr io.Writer, argv []string) int {
	dir, err := stateDir()
	if err != nil {
		fmt.Fprintf(stderr, "fakeharness: %v\n", err)
		return 1
	}
	recordCall(dir, argv[1:])
	name := filepath.Base(argv[0])
	switch name {
	case "claude":
		return runClaude(stdout, stderr, dir, argv[1:])
	case "pi":
		return runPi(stdout, stderr, dir, argv[1:])
	default:
		fmt.Fprintf(stderr, "fakeharness: unknown harness name %q\n", name)
		return 1
	}
}

func stateDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	target, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	return filepath.Dir(target), nil
}

func recordCall(dir string, args []string) {
	wd, _ := os.Getwd()
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			env[k] = v
		}
	}
	var dirEntries []string
	if entries, err := os.ReadDir(wd); err == nil {
		for _, e := range entries {
			dirEntries = append(dirEntries, e.Name())
		}
	}
	rec := callRecord{Argv: args, Env: env, Dir: wd, DirEntries: dirEntries}
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "stub_records.json"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

func runClaude(stdout, stderr io.Writer, dir string, args []string) int {
	if len(args) == 2 && args[0] == "mcp" && args[1] == "list" {
		line := claudeSagLine
		if data, err := os.ReadFile(filepath.Join(dir, "claude_sag_line")); err == nil {
			trimmed := strings.TrimSpace(string(data))
			if trimmed != "" {
				line = trimmed
			}
		}
		fmt.Fprintln(stdout, line)
		return 0
	}
	fmt.Fprintf(stderr, "fakeharness claude: unknown command %v\n", args)
	return 1
}

func runPi(stdout, stderr io.Writer, dir string, args []string) int {
	if len(args) == 3 && args[0] == "mcp" && args[1] == "list" && args[2] == "--json" {
		state := "connected"
		if data, err := os.ReadFile(filepath.Join(dir, "pi_sag_state")); err == nil {
			trimmed := strings.TrimSpace(string(data))
			if trimmed != "" {
				state = trimmed
			}
		}
		result := map[string]any{
			"servers": []map[string]string{
				{"name": "sag", "state": state},
			},
		}
		data, _ := json.Marshal(result)
		fmt.Fprintln(stdout, string(data))
		if state != "connected" {
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "fakeharness pi: unknown command %v\n", args)
	return 1
}

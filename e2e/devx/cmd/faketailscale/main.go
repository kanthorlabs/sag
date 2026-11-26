package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Stdout, os.Stderr, os.Args[1:]))
}

func run(stdout, stderr io.Writer, args []string) int {
	dir, err := stateDir()
	if err != nil {
		fmt.Fprintf(stderr, "faketailscale: %v\n", err)
		return 1
	}
	recordArgs(dir, args)
	return dispatch(stdout, stderr, dir, args)
}

func stateDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

func recordArgs(dir string, args []string) {
	line := strings.Join(args, " ") + "\n"
	f, err := os.OpenFile(filepath.Join(dir, "ts_args"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line)
	_ = f.Close()
}

func dispatch(stdout, stderr io.Writer, dir string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "faketailscale: no command\n")
		return 1
	}
	switch args[0] {
	case "ip":
		return handleIP(stdout, dir, args[1:])
	case "serve":
		return handleServe(stdout, stderr, dir, args[1:])
	default:
		fmt.Fprintf(stderr, "faketailscale: unknown command: %s\n", args[0])
		return 1
	}
}

func handleIP(stdout io.Writer, dir string, args []string) int {
	if len(args) < 1 || args[0] != "-4" {
		return 1
	}
	data, err := os.ReadFile(filepath.Join(dir, "ts_ip4"))
	if err != nil || strings.TrimSpace(string(data)) == "exit:1" {
		return 1
	}
	fmt.Fprintln(stdout, strings.TrimSpace(string(data)))
	return 0
}

func handleServe(stdout, stderr io.Writer, dir string, args []string) int {
	if len(args) >= 2 && args[0] == "status" && args[1] == "--json" {
		return handleServeStatus(stdout, dir)
	}
	if isServeRule(args) {
		return handleServeApply(stderr, dir)
	}
	fmt.Fprintf(stderr, "faketailscale: unknown serve args: %v\n", args)
	return 1
}

func isServeRule(args []string) bool {
	return len(args) == 4 &&
		args[0] == "--bg" &&
		args[1] == "--tcp" &&
		args[2] == "8443" &&
		args[3] == "tcp://127.0.0.1:8443"
}

type tcpEntry struct {
	TCPForward string `json:"TCPForward"`
}

type serveStatus struct {
	TCP         map[string]tcpEntry `json:"TCP,omitempty"`
	AllowFunnel map[string]bool     `json:"AllowFunnel,omitempty"`
}

func handleServeStatus(stdout io.Writer, dir string) int {
	var status serveStatus
	if data, err := os.ReadFile(filepath.Join(dir, "ts_serve_status")); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &status)
	}
	if _, err := os.Stat(filepath.Join(dir, "ts_rule")); err == nil {
		if status.TCP == nil {
			status.TCP = map[string]tcpEntry{}
		}
		status.TCP["8443"] = tcpEntry{TCPForward: "127.0.0.1:8443"}
	}
	out, _ := json.Marshal(status)
	fmt.Fprintln(stdout, string(out))
	return 0
}

func handleServeApply(stderr io.Writer, dir string) int {
	if err := os.WriteFile(filepath.Join(dir, "ts_rule"), []byte("served\n"), 0o600); err != nil {
		fmt.Fprintf(stderr, "faketailscale: write ts_rule: %v\n", err)
		return 1
	}
	return 0
}

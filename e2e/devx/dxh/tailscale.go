package dxh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"
)

type TCPEntry struct {
	TCPForward string `json:"TCPForward"`
}

type ServeStatus struct {
	TCP         map[string]TCPEntry `json:"TCP,omitempty"`
	AllowFunnel map[string]bool     `json:"AllowFunnel,omitempty"`
}

func ParseServeStatus(data []byte) (ServeStatus, error) {
	var s ServeStatus
	if err := json.Unmarshal(data, &s); err != nil {
		return ServeStatus{}, fmt.Errorf("parse tailscale serve status: %w", err)
	}
	return s, nil
}

func HasTCP8443(s ServeStatus) bool {
	_, ok := s.TCP["8443"]
	return ok
}

func HasSAGRule(s ServeStatus) bool {
	entry, ok := s.TCP["8443"]
	if !ok {
		return false
	}
	if entry.TCPForward != "127.0.0.1:8443" {
		return false
	}
	for k, v := range s.AllowFunnel {
		if strings.HasSuffix(k, ":8443") && v {
			return false
		}
	}
	return true
}

func EqualServeStatus(a, b []byte) error {
	var sa, sb ServeStatus
	if err := json.Unmarshal(a, &sa); err != nil {
		return fmt.Errorf("parse first status: %w", err)
	}
	if err := json.Unmarshal(b, &sb); err != nil {
		return fmt.Errorf("parse second status: %w", err)
	}
	if !reflect.DeepEqual(sa, sb) {
		return fmt.Errorf("serve status differs")
	}
	return nil
}

func MaskTailscaleIP(text, ip string) string {
	return strings.ReplaceAll(text, ip, "<tailscale IP>")
}

func ReadTailscaleIPv4(ctx context.Context, r Runner) (string, error) {
	out, err := r.Output(ctx, "tailscale", "ip", "-4")
	if err != nil {
		return "", fmt.Errorf("prerequisite missing: tailscale ip -4: %w", err)
	}
	ip := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("prerequisite missing: tailscale ip invalid: %q", ip)
	}
	_, tsRange, _ := net.ParseCIDR("100.64.0.0/10")
	if !tsRange.Contains(parsed) {
		return "", fmt.Errorf("prerequisite missing: tailscale ip not in 100.64.0.0/10: %q", ip)
	}
	return ip, nil
}

func ReadServeStatusJSON(ctx context.Context, r Runner) ([]byte, error) {
	out, err := r.Output(ctx, "tailscale", "serve", "status", "--json")
	if err != nil {
		return nil, fmt.Errorf("tailscale serve status --json: %w", err)
	}
	return bytes.TrimSpace(out), nil
}

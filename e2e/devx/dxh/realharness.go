package dxh

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const ClaudeCodeVersion = "2.1.332"
const PiVersion = "1.6.1"

func VersionGate(ctx context.Context, r Runner, name, required string) error {
	out, err := r.Output(ctx, name, "--version")
	if err != nil {
		return fmt.Errorf("prerequisite missing: %s %s required, cannot run: %w", name, required, err)
	}
	text := strings.TrimSpace(string(out))
	for _, tok := range strings.Fields(text) {
		if tok == required {
			return nil
		}
	}
	return fmt.Errorf("prerequisite missing: %s %s required, got %s", name, required, text)
}

func ParseClaudeMCPListSAG(output string) error {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "sag:") {
			continue
		}
		if strings.Contains(line, "Failed to connect") || strings.Contains(line, "Needs authentication") {
			return fmt.Errorf("sag mcp server not connected: %s", strings.TrimSpace(line))
		}
		if strings.Contains(line, "Connected") {
			return nil
		}
		return fmt.Errorf("sag mcp server unknown state: %s", strings.TrimSpace(line))
	}
	return fmt.Errorf("sag mcp server not found in claude mcp list output")
}

func ParseClaudeListResources(output, name string) error {
	var resp struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		return fmt.Errorf("parse claude output: %w", err)
	}
	if resp.IsError {
		return fmt.Errorf("claude -p returned is_error=true")
	}
	if !strings.Contains(resp.Result, name) {
		return fmt.Errorf("resource %q not found in claude output result", name)
	}
	return nil
}

func ParseClaudePermissionDenials(output, command string) error {
	var resp struct {
		PermissionDenials []struct {
			ToolName  string `json:"tool_name"`
			ToolInput struct {
				Command string `json:"command"`
			} `json:"tool_input"`
		} `json:"permission_denials"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		return fmt.Errorf("parse claude output: %w", err)
	}
	for _, d := range resp.PermissionDenials {
		if d.ToolName == "Bash" && strings.Contains(d.ToolInput.Command, command) {
			return nil
		}
	}
	return fmt.Errorf("no permission denial for Bash command %q in permission_denials", command)
}

func ParseCredentialListNoName(output, name string) error {
	var resp struct {
		Credentials []struct {
			Name string `json:"name"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		return fmt.Errorf("parse sag credential list --json: %w", err)
	}
	for _, c := range resp.Credentials {
		if c.Name == name {
			return fmt.Errorf("credential %q found; want absent", name)
		}
	}
	return nil
}

func ParsePiMCPListSAG(output string) error {
	var resp struct {
		Servers []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"servers"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		return fmt.Errorf("parse pi mcp list --json: %w", err)
	}
	for _, s := range resp.Servers {
		if s.Name == "sag" {
			if s.State == "connected" {
				return nil
			}
			return fmt.Errorf("sag mcp server state = %q; want connected", s.State)
		}
	}
	return fmt.Errorf("sag mcp server not found in pi mcp list --json output")
}

func ParsePiGuardResult(output string) error {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event struct {
			Type     string `json:"type"`
			ToolName string `json:"toolName"`
			IsError  bool   `json:"isError"`
			Result   struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type != "tool_execution_end" || event.ToolName != "bash" || !event.IsError {
			continue
		}
		if len(event.Result.Content) > 0 && strings.HasPrefix(event.Result.Content[0].Text, "sag-guard:") {
			return nil
		}
	}
	return fmt.Errorf("no tool_execution_end event with sag-guard: reason found in pi --mode json output")
}

var jwtPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)

func FilterNoJWT(s string) string {
	return jwtPattern.ReplaceAllString(s, "<JWT>")
}

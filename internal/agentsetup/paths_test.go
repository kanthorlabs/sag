package agentsetup

import (
	"path/filepath"
	"testing"
)

func TestClaudeCodePathsDefaultDir(t *testing.T) {
	p := ClaudeCodePaths("", "/home/user")
	want := "/home/user/.claude"
	if p.Dir != want {
		t.Errorf("Dir = %q; want %q", p.Dir, want)
	}
}

func TestClaudeCodePathsDefaultMCPFile(t *testing.T) {
	p := ClaudeCodePaths("", "/home/user")
	want := "/home/user/.claude.json"
	if p.MCPFile != want {
		t.Errorf("MCPFile = %q; want %q", p.MCPFile, want)
	}
}

func TestClaudeCodePathsDefaultSkillFile(t *testing.T) {
	p := ClaudeCodePaths("", "/home/user")
	want := filepath.Join("/home/user/.claude", "skills", "sag", "SKILL.md")
	if p.SkillFile != want {
		t.Errorf("SkillFile = %q; want %q", p.SkillFile, want)
	}
}

func TestClaudeCodePathsDefaultSettingsFile(t *testing.T) {
	p := ClaudeCodePaths("", "/home/user")
	want := filepath.Join("/home/user/.claude", "settings.json")
	if p.SettingsFile != want {
		t.Errorf("SettingsFile = %q; want %q", p.SettingsFile, want)
	}
}

func TestClaudeCodePathsWithEnvDir(t *testing.T) {
	p := ClaudeCodePaths("/tmp/testcfg", "/home/user")
	if p.Dir != "/tmp/testcfg" {
		t.Errorf("Dir = %q; want /tmp/testcfg", p.Dir)
	}
}

func TestClaudeCodePathsWithEnvMCPFile(t *testing.T) {
	p := ClaudeCodePaths("/tmp/testcfg", "/home/user")
	want := "/tmp/testcfg/.claude.json"
	if p.MCPFile != want {
		t.Errorf("MCPFile = %q; want %q", p.MCPFile, want)
	}
}

func TestClaudeCodePathsWithEnvSkillFile(t *testing.T) {
	p := ClaudeCodePaths("/tmp/testcfg", "/home/user")
	want := filepath.Join("/tmp/testcfg", "skills", "sag", "SKILL.md")
	if p.SkillFile != want {
		t.Errorf("SkillFile = %q; want %q", p.SkillFile, want)
	}
}

func TestClaudeCodePathsWithEnvSettingsFile(t *testing.T) {
	p := ClaudeCodePaths("/tmp/testcfg", "/home/user")
	want := filepath.Join("/tmp/testcfg", "settings.json")
	if p.SettingsFile != want {
		t.Errorf("SettingsFile = %q; want %q", p.SettingsFile, want)
	}
}

func TestPiPathsDefaultDir(t *testing.T) {
	p := PiPaths("", "/home/user")
	want := "/home/user/.pi/agent"
	if p.Dir != want {
		t.Errorf("Dir = %q; want %q", p.Dir, want)
	}
}

func TestPiPathsDefaultMCPFile(t *testing.T) {
	p := PiPaths("", "/home/user")
	want := filepath.Join("/home/user/.pi/agent", "mcp.json")
	if p.MCPFile != want {
		t.Errorf("MCPFile = %q; want %q", p.MCPFile, want)
	}
}

func TestPiPathsDefaultSkillFile(t *testing.T) {
	p := PiPaths("", "/home/user")
	want := filepath.Join("/home/user/.pi/agent", "skills", "sag", "SKILL.md")
	if p.SkillFile != want {
		t.Errorf("SkillFile = %q; want %q", p.SkillFile, want)
	}
}

func TestPiPathsDefaultGuardFile(t *testing.T) {
	p := PiPaths("", "/home/user")
	want := filepath.Join("/home/user/.pi/agent", "extensions", "sag-guard.ts")
	if p.GuardFile != want {
		t.Errorf("GuardFile = %q; want %q", p.GuardFile, want)
	}
}

func TestPiPathsWithEnvDir(t *testing.T) {
	p := PiPaths("/tmp/pidir", "/home/user")
	if p.Dir != "/tmp/pidir" {
		t.Errorf("Dir = %q; want /tmp/pidir", p.Dir)
	}
}

func TestPiPathsWithEnvMCPFile(t *testing.T) {
	p := PiPaths("/tmp/pidir", "/home/user")
	want := filepath.Join("/tmp/pidir", "mcp.json")
	if p.MCPFile != want {
		t.Errorf("MCPFile = %q; want %q", p.MCPFile, want)
	}
}

func TestPiPathsWithEnvGuardFile(t *testing.T) {
	p := PiPaths("/tmp/pidir", "/home/user")
	want := filepath.Join("/tmp/pidir", "extensions", "sag-guard.ts")
	if p.GuardFile != want {
		t.Errorf("GuardFile = %q; want %q", p.GuardFile, want)
	}
}

func TestShellProfileZsh(t *testing.T) {
	profile, ok := ShellProfile("darwin", "/bin/zsh", "", "/home/user")
	if !ok {
		t.Fatal("ok = false; want true")
	}
	want := "/home/user/.zshrc"
	if profile != want {
		t.Errorf("profile = %q; want %q", profile, want)
	}
}

func TestShellProfileZshWithZdotdir(t *testing.T) {
	profile, ok := ShellProfile("darwin", "/bin/zsh", "/tmp/zdot", "/home/user")
	if !ok {
		t.Fatal("ok = false; want true")
	}
	want := "/tmp/zdot/.zshrc"
	if profile != want {
		t.Errorf("profile = %q; want %q", profile, want)
	}
}

func TestShellProfileBashMacOS(t *testing.T) {
	profile, ok := ShellProfile("darwin", "/bin/bash", "", "/home/user")
	if !ok {
		t.Fatal("ok = false; want true")
	}
	want := "/home/user/.bash_profile"
	if profile != want {
		t.Errorf("profile = %q; want %q", profile, want)
	}
}

func TestShellProfileBashLinux(t *testing.T) {
	profile, ok := ShellProfile("linux", "/bin/bash", "", "/home/user")
	if !ok {
		t.Fatal("ok = false; want true")
	}
	want := "/home/user/.bashrc"
	if profile != want {
		t.Errorf("profile = %q; want %q", profile, want)
	}
}

func TestShellProfileFishRefused(t *testing.T) {
	_, ok := ShellProfile("darwin", "/usr/local/bin/fish", "", "/home/user")
	if ok {
		t.Fatal("ok = true; want false for unsupported shell")
	}
}

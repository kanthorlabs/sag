package agentsetup

import "path/filepath"

type HarnessPaths struct {
	Dir          string
	MCPFile      string
	SkillFile    string
	SettingsFile string
	GuardFile    string
}

func ClaudeCodePaths(claudeConfigDir, home string) HarnessPaths {
	dir := filepath.Join(home, ".claude")
	mcpFile := filepath.Join(home, ".claude.json")
	if claudeConfigDir != "" {
		dir = claudeConfigDir
		mcpFile = filepath.Join(claudeConfigDir, ".claude.json")
	}
	return HarnessPaths{
		Dir:          dir,
		MCPFile:      mcpFile,
		SkillFile:    filepath.Join(dir, "skills", "sag", "SKILL.md"),
		SettingsFile: filepath.Join(dir, "settings.json"),
	}
}

func PiPaths(piAgentDir, home string) HarnessPaths {
	dir := filepath.Join(home, ".pi", "agent")
	if piAgentDir != "" {
		dir = piAgentDir
	}
	return HarnessPaths{
		Dir:       dir,
		MCPFile:   filepath.Join(dir, "mcp.json"),
		SkillFile: filepath.Join(dir, "skills", "sag", "SKILL.md"),
		GuardFile: filepath.Join(dir, "extensions", "sag-guard.ts"),
	}
}

func ShellProfile(goos, shell, zdotdir, home string) (string, bool) {
	base := filepath.Base(shell)
	switch base {
	case "zsh":
		dir := home
		if zdotdir != "" {
			dir = zdotdir
		}
		return filepath.Join(dir, ".zshrc"), true
	case "bash":
		if goos == "darwin" {
			return filepath.Join(home, ".bash_profile"), true
		}
		return filepath.Join(home, ".bashrc"), true
	default:
		return "", false
	}
}

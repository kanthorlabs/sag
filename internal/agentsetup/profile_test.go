package agentsetup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileByShell(t *testing.T) {
	cases := []struct {
		goos       string
		shell      string
		wantSuffix string
	}{
		{"darwin", "/bin/bash", ".bash_profile"},
		{"linux", "/bin/bash", ".bashrc"},
		{"darwin", "/bin/zsh", ".zshrc"},
	}
	for _, tc := range cases {
		t.Run(tc.goos+"-"+filepath.Base(tc.shell), func(t *testing.T) {
			home := t.TempDir()
			cfgDir := filepath.Join(home, ".config", "sag")
			if err := os.MkdirAll(cfgDir, 0o700); err != nil {
				t.Fatal(err)
			}
			opts := Options{
				ShellProfile: "yes",
				GOOS:         tc.goos,
				Home:         home,
				CfgDir:       cfgDir,
				Env:          map[string]string{"SHELL": tc.shell},
			}
			if err := applyShellProfile(context.Background(), opts); err != nil {
				t.Fatalf("applyShellProfile: %v", err)
			}
			profilePath := filepath.Join(home, tc.wantSuffix)
			data, err := os.ReadFile(profilePath)
			if err != nil {
				t.Fatalf("read profile: %v", err)
			}
			exportLine := `export NODE_EXTRA_CA_CERTS="` + filepath.Join(cfgDir, "ca.pem") + `"`
			if !strings.Contains(string(data), exportLine) {
				t.Errorf("profile does not contain export line; got %q", data)
			}
		})
	}
}

func TestProfileZshWithZDOTDIR(t *testing.T) {
	home := t.TempDir()
	zdotdir := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "sag")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		ShellProfile: "yes",
		GOOS:         "darwin",
		Home:         home,
		CfgDir:       cfgDir,
		Env:          map[string]string{"SHELL": "/bin/zsh", "ZDOTDIR": zdotdir},
	}
	if err := applyShellProfile(context.Background(), opts); err != nil {
		t.Fatalf("applyShellProfile: %v", err)
	}
	profilePath := filepath.Join(zdotdir, ".zshrc")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if !strings.Contains(string(data), "export NODE_EXTRA_CA_CERTS=") {
		t.Errorf("profile does not contain export line; got %q", data)
	}
	homeProfile := filepath.Join(home, ".zshrc")
	if _, err := os.Stat(homeProfile); !os.IsNotExist(err) {
		t.Error("home .zshrc was written but ZDOTDIR was set")
	}
}

func TestProfileLineOnce(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "sag")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		ShellProfile: "yes",
		GOOS:         "darwin",
		Home:         home,
		CfgDir:       cfgDir,
		Env:          map[string]string{"SHELL": "/bin/zsh"},
	}
	ctx := context.Background()
	if err := applyShellProfile(ctx, opts); err != nil {
		t.Fatalf("first applyShellProfile: %v", err)
	}
	if err := applyShellProfile(ctx, opts); err != nil {
		t.Fatalf("second applyShellProfile: %v", err)
	}
	profilePath := filepath.Join(home, ".zshrc")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	exportLine := `export NODE_EXTRA_CA_CERTS="` + filepath.Join(cfgDir, "ca.pem") + `"`
	count := strings.Count(string(data), exportLine)
	if count != 1 {
		t.Errorf("export line appears %d times; want 1\n%s", count, data)
	}
}

func TestProfileNoTrailingNewlineAppend(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "sag")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(profilePath, []byte("existing content"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		ShellProfile: "yes",
		GOOS:         "darwin",
		Home:         home,
		CfgDir:       cfgDir,
		Env:          map[string]string{"SHELL": "/bin/zsh"},
	}
	if err := applyShellProfile(context.Background(), opts); err != nil {
		t.Fatalf("applyShellProfile: %v", err)
	}
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "existing content\n") {
		t.Errorf("file does not start with existing content + newline; got %q", content)
	}
}

func TestFishRefused(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "sag")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		ShellProfile: "yes",
		GOOS:         "darwin",
		Home:         home,
		CfgDir:       cfgDir,
		Env:          map[string]string{"SHELL": "/usr/local/bin/fish"},
	}
	if err := applyShellProfile(context.Background(), opts); err == nil {
		t.Fatal("applyShellProfile with fish: expected error; got nil")
	}
}

func TestProfileNoOpWhenAnswerNo(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "sag")
	opts := Options{
		ShellProfile: "no",
		GOOS:         "darwin",
		Home:         home,
		CfgDir:       cfgDir,
		Env:          map[string]string{"SHELL": "/bin/zsh"},
	}
	if err := applyShellProfile(context.Background(), opts); err != nil {
		t.Fatalf("applyShellProfile with 'no': %v", err)
	}
	profilePath := filepath.Join(home, ".zshrc")
	if _, err := os.Stat(profilePath); !os.IsNotExist(err) {
		t.Error("profile file created when answer is 'no'")
	}
}

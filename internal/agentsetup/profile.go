package agentsetup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kanthorlabs/sag/internal/stepline"
)

const (
	profileCommentLine = "# sag agent setup"
)

func applyShellProfile(ctx context.Context, opts Options) error {
	out := stepline.New(agentOut)
	if opts.Harness != HarnessPi {
		return nil
	}
	shell := opts.Env["SHELL"]
	zdotdir := opts.Env["ZDOTDIR"]
	profilePath, ok := ShellProfile(opts.GOOS, shell, zdotdir, opts.Home)
	if !ok {
		if opts.ShellProfile == "yes" {
			return fmt.Errorf("SHELL=%s is not supported; add the export line by hand and run with --shell-profile no", shell)
		}
		out.Line(stepline.StatusSkip, "shell-profile", "")
		return nil
	}
	if opts.CfgDir == "" {
		return fmt.Errorf("CfgDir is empty")
	}
	caPath := filepath.Join(opts.CfgDir, "ca.pem")
	exportLine := `export NODE_EXTRA_CA_CERTS="` + caPath + `"`
	if fileContainsLine(profilePath, exportLine) {
		out.Line(stepline.StatusOK, "shell-profile", "")
		return nil
	}
	if profileHasAcceptedValue(profilePath, caPath) {
		out.Line(stepline.StatusOK, "shell-profile", "")
		return nil
	}
	if opts.ShellProfile != "yes" {
		out.Line(stepline.StatusSkip, "shell-profile", "")
		return nil
	}
	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open profile %s: %w", profilePath, err)
	}
	defer f.Close()
	if needsLeadingNewline(f) {
		if _, err := fmt.Fprintln(f); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(f, profileCommentLine); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, exportLine); err != nil {
		return err
	}
	out.Line(stepline.StatusOK, "shell-profile", "")
	return nil
}

func needsLeadingNewline(f *os.File) bool {
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return false
	}
	if _, err := f.Seek(-1, io.SeekEnd); err != nil {
		return false
	}
	var b [1]byte
	if _, err := f.Read(b[:]); err != nil {
		return false
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return false
	}
	return b[0] != '\n'
}

func fileContainsLine(path, line string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimRight(sc.Text(), "\r") == line {
			return true
		}
	}
	return false
}

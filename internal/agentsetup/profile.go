package agentsetup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	profileCommentLine = "# sag agent setup"
)

func applyShellProfile(ctx context.Context, opts Options) error {
	if opts.ShellProfile != "yes" {
		return nil
	}
	shell := opts.Env["SHELL"]
	zdotdir := opts.Env["ZDOTDIR"]
	profilePath, ok := ShellProfile(opts.GOOS, shell, zdotdir, opts.Home)
	if !ok {
		return fmt.Errorf("SHELL=%s is not supported; add the export line by hand and run with --shell-profile no", shell)
	}
	if opts.CfgDir == "" {
		return fmt.Errorf("CfgDir is empty")
	}
	exportLine := `export NODE_EXTRA_CA_CERTS="` + filepath.Join(opts.CfgDir, "ca.pem") + `"`
	if fileContainsLine(profilePath, exportLine) {
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

func ReadShellProfileLines(profilePath string) []string {
	f, err := os.Open(profilePath)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	return lines
}

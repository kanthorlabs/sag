package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var pgLogDropped = []string{"statement:", "STATEMENT:", "PASSWORD", "password"}

func (s *Suite) SaveLogs(ctx context.Context, reportDir string) error {
	dir := filepath.Join(reportDir, fmt.Sprintf("pg%d", s.major), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.mu.Lock()
	containers := make([]*sagContainer, 0, len(s.containers))
	for _, c := range s.containers {
		containers = append(containers, c)
	}
	s.mu.Unlock()

	var errs []error
	for _, c := range containers {
		out, err := c.Logs(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("logs %s: %w", c.name, err))
			continue
		}
		errs = append(errs, os.WriteFile(filepath.Join(dir, c.name+".log"), []byte(out), 0o644))
	}
	for major, pg := range s.pgs {
		out, err := pg.Logs(ctx, time.Time{})
		if err != nil {
			errs = append(errs, fmt.Errorf("logs postgres %d: %w", major, err))
			continue
		}
		errs = append(errs, os.WriteFile(filepath.Join(dir, fmt.Sprintf("postgres-%d.log", major)), []byte(keepSafePGLines(out)), 0o644))
	}
	return errors.Join(errs...)
}

func keepSafePGLines(log string) string {
	var b strings.Builder
	drop := false
	for _, line := range strings.SplitAfter(log, "\n") {
		continued := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		if !continued {
			drop = containsAny(line, pgLogDropped)
		}
		if !drop && !containsAny(line, pgLogDropped) {
			b.WriteString(line)
		}
	}
	return b.String()
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

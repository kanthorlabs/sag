package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/internal/testenv"
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
	errs = append(errs, s.writeNetworkInfo(ctx, dir))
	errs = append(errs, s.writeEvents(ctx, dir))
	return errors.Join(errs...)
}

func (s *Suite) writeNetworkInfo(ctx context.Context, dir string) error {
	type inspectNet struct {
		IPAddress string `json:"IPAddress"`
	}
	type inspectState struct {
		Status string `json:"Status"`
	}
	type inspectItem struct {
		Name            string       `json:"Name"`
		State           inspectState `json:"State"`
		NetworkSettings struct {
			Networks map[string]inspectNet `json:"Networks"`
		} `json:"NetworkSettings"`
	}

	names := make([]string, 0)
	s.mu.Lock()
	for _, c := range s.containers {
		names = append(names, c.name)
	}
	s.mu.Unlock()
	for _, pg := range s.pgs {
		names = append(names, pg.Container)
	}

	var sb strings.Builder
	for _, name := range names {
		res, err := testenv.Podman(ctx, nil, "container", "inspect", "--format", "json", name)
		if err != nil || res.ExitCode != 0 {
			fmt.Fprintf(&sb, "%s error=%v\n", name, err)
			continue
		}
		var items []inspectItem
		if err := json.Unmarshal([]byte(res.Stdout), &items); err != nil || len(items) == 0 {
			fmt.Fprintf(&sb, "%s parse-error\n", name)
			continue
		}
		it := items[0]
		ip := ""
		if net, ok := it.NetworkSettings.Networks[s.run.Network]; ok {
			ip = net.IPAddress
		}
		fmt.Fprintf(&sb, "%s ip=%s status=%s\n", it.Name, ip, it.State.Status)
	}
	return os.WriteFile(filepath.Join(dir, "network.txt"), []byte(sb.String()), 0o644)
}

func (s *Suite) writeEvents(ctx context.Context, dir string) error {
	since := fmt.Sprintf("%d", s.run.Start.Add(-2*time.Minute).Unix())
	until := fmt.Sprintf("%d", time.Now().Unix())
	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	res, err := testenv.Podman(tctx, nil, "events", "--since", since, "--until", until, "--format", "json")
	if err != nil && tctx.Err() == nil {
		return fmt.Errorf("podman events: %w", err)
	}
	prefix := s.run.Network
	filtered := filterEventLines(res.Stdout, prefix)
	return os.WriteFile(filepath.Join(dir, "events.json"), filtered, 0o644)
}

func filterEventLines(raw, prefix string) []byte {
	type eventName struct {
		Name string `json:"Name"`
	}
	var sb strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev eventName
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if !strings.HasPrefix(ev.Name, prefix) {
			continue
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
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

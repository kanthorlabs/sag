package cli

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/service"
)

func withFakePodmanStream(t *testing.T, capture *[][]string) {
	t.Helper()
	old := runPodmanStream
	runPodmanStream = func(_ context.Context, _, _ io.Writer, _ string, args []string) error {
		cp := make([]string, len(args))
		copy(cp, args)
		*capture = append(*capture, cp)
		return nil
	}
	t.Cleanup(func() { runPodmanStream = old })
}

func withFakeUTCToday(t *testing.T, date string) {
	t.Helper()
	old := utcToday
	utcToday = func() string { return date }
	t.Cleanup(func() { utcToday = old })
}

func logsFakeCfg() service.Config {
	return service.Config{Podman: podman.Runner{Path: "/dev/null"}}
}

func TestDaemonFlagsPassed(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantArgs []string
	}{
		{
			name:     "no flags",
			args:     nil,
			wantArgs: []string{"logs", "sag"},
		},
		{
			name:     "follow only",
			args:     []string{"--follow"},
			wantArgs: []string{"logs", "--follow", "sag"},
		},
		{
			name:     "since only",
			args:     []string{"--since", "1h"},
			wantArgs: []string{"logs", "--since", "1h", "sag"},
		},
		{
			name:     "tail only",
			args:     []string{"--tail", "50"},
			wantArgs: []string{"logs", "--tail", "50", "sag"},
		},
		{
			name:     "all flags",
			args:     []string{"--follow", "--since", "30m", "--tail", "100"},
			wantArgs: []string{"logs", "--follow", "--since", "30m", "--tail", "100", "sag"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured [][]string
			withFakeNoLockCfg(t, logsFakeCfg())
			withFakePodmanStream(t, &captured)

			std := IO{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
			code := logsDaemon(tc.args, std)
			if code != ExitOK {
				t.Fatalf("exit %d; want %d; stderr: %s", code, ExitOK, std.Err.(*bytes.Buffer).String())
			}
			if len(captured) != 1 {
				t.Fatalf("stream called %d times; want 1", len(captured))
			}
			got := captured[0]
			if len(got) != len(tc.wantArgs) {
				t.Fatalf("args %v; want %v", got, tc.wantArgs)
			}
			for i, w := range tc.wantArgs {
				if got[i] != w {
					t.Fatalf("arg[%d] = %q; want %q (full: %v)", i, got[i], w, got)
				}
			}
		})
	}
}

func TestAuditDefaultUTCDate(t *testing.T) {
	fixedDate := "2025-07-04"
	withFakeUTCToday(t, fixedDate)

	var captured [][]string
	withFakeNoLockCfg(t, logsFakeCfg())
	withFakePodmanStream(t, &captured)

	std := IO{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	code := logsAudit(nil, std)
	if code != ExitOK {
		t.Fatalf("exit %d; want %d; stderr: %s", code, ExitOK, std.Err.(*bytes.Buffer).String())
	}
	if len(captured) != 1 {
		t.Fatalf("stream called %d times; want 1", len(captured))
	}

	args := captured[0]
	wantDate := fixedDate
	found := false
	for i, a := range args {
		if a == "--date" && i+1 < len(args) && args[i+1] == wantDate {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("args %v do not contain --date %s", args, wantDate)
	}
}

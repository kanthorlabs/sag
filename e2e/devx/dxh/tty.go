package dxh

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"os"
)

const ptyDrainTimeout = 2 * time.Second

type TTY struct {
	cmd    *exec.Cmd
	master *os.File

	mu      sync.Mutex
	output  strings.Builder
	matched int
	changed chan struct{}
	done    chan struct{}
	redact  []string
}

func StartTTY(ctx context.Context, name string, args ...string) (*TTY, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 200})
	if err != nil {
		return nil, fmt.Errorf("dxh: start %s on a pseudo-terminal: %w", name, err)
	}
	t := &TTY{
		cmd:     cmd,
		master:  master,
		changed: make(chan struct{}),
		done:    make(chan struct{}),
	}
	go t.read()
	return t, nil
}

func (t *TTY) read() {
	defer close(t.done)
	buf := make([]byte, 4096)
	for {
		n, err := t.master.Read(buf)
		if n > 0 {
			t.mu.Lock()
			t.output.Write(buf[:n])
			close(t.changed)
			t.changed = make(chan struct{})
			t.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (t *TTY) Redact(values ...string) {
	t.mu.Lock()
	t.redact = append(t.redact, values...)
	t.mu.Unlock()
}

func (t *TTY) redactString(s string) string {
	for _, v := range t.redact {
		if v != "" {
			s = strings.ReplaceAll(s, v, "[REDACTED]")
		}
	}
	return s
}

func (t *TTY) Expect(ctx context.Context, text string) error {
	finished := false
	for {
		t.mu.Lock()
		out := t.output.String()
		changed := t.changed
		if i := strings.Index(out[t.matched:], text); i >= 0 {
			t.matched += i + len(text)
			t.mu.Unlock()
			return nil
		}
		t.mu.Unlock()
		if finished {
			return fmt.Errorf("dxh: pseudo-terminal closed before %q; output %q", text, t.redactString(out))
		}
		select {
		case <-changed:
		case <-t.done:
			finished = true
		case <-ctx.Done():
			return fmt.Errorf("dxh: wait for %q: %w; output %q", text, ctx.Err(), t.redactString(out))
		}
	}
}

func (t *TTY) Send(text string) error {
	_, err := t.master.Write([]byte(text))
	return err
}

func (t *TTY) Wait(ctx context.Context) (int, error) {
	waited := make(chan error, 1)
	go func() { waited <- t.cmd.Wait() }()
	var err error
	select {
	case err = <-waited:
	case <-ctx.Done():
		_ = t.cmd.Process.Kill()
		<-waited
		err = ctx.Err()
	}
	select {
	case <-t.done:
	case <-time.After(ptyDrainTimeout):
	}
	_ = t.master.Close()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}

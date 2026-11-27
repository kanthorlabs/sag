package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"

	"github.com/kanthorlabs/sag/internal/testenv"
)

const ptyDrainTimeout = 2 * time.Second

type ptyTTY struct {
	cmd    *exec.Cmd
	master *os.File

	mu      sync.Mutex
	output  strings.Builder
	matched int
	changed chan struct{}
	done    chan struct{}
}

func startTTY(ctx context.Context, name string, args ...string) (*ptyTTY, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 200})
	if err != nil {
		return nil, fmt.Errorf("harness: start %s on a pseudo-terminal: %w", name, err)
	}
	t := &ptyTTY{
		cmd:     cmd,
		master:  master,
		changed: make(chan struct{}),
		done:    make(chan struct{}),
	}
	go t.read()
	return t, nil
}

func (t *ptyTTY) read() {
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

func (t *ptyTTY) Expect(ctx context.Context, text string) error {
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
			return fmt.Errorf("harness: pseudo-terminal closed before %q; output %q", text, out)
		}
		select {
		case <-changed:
		case <-t.done:
			finished = true
		case <-ctx.Done():
			return fmt.Errorf("harness: wait for %q: %w; output %q", text, ctx.Err(), out)
		}
	}
}

func (t *ptyTTY) Send(text string) error {
	_, err := t.master.Write([]byte(text))
	return err
}

func (t *ptyTTY) Wait(ctx context.Context) (testenv.Result, error) {
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

	t.mu.Lock()
	res := testenv.Result{Stdout: t.output.String()}
	t.mu.Unlock()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, err
}

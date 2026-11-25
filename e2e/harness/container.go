package harness

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/internal/testenv"
)

const (
	healthzPoll    = 500 * time.Millisecond
	healthzTimeout = 2 * time.Minute
)

type sagContainer struct {
	name   string
	volume string
	port   int
	client *http.Client
}

func (c *sagContainer) Name() string {
	return c.name
}

func (c *sagContainer) url() string {
	return "https://localhost:" + strconv.Itoa(c.port)
}

func (c *sagContainer) Wait(ctx context.Context) (int, error) {
	res, err := podman(ctx, "wait", c.name)
	if err != nil {
		return 0, err
	}
	code, err := strconv.Atoi(strings.TrimSpace(res.Stdout))
	if err != nil {
		return 0, fmt.Errorf("harness: podman wait %s: %q", c.name, res.Stdout)
	}
	return code, nil
}

func (c *sagContainer) Healthz(ctx context.Context) error {
	if c.port == 0 {
		return fmt.Errorf("harness: %s publishes no port", c.name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url()+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("harness: %s /healthz status %d", c.name, resp.StatusCode)
	}
	return nil
}

func (c *sagContainer) Logs(ctx context.Context) (string, error) {
	res, err := podman(ctx, "logs", c.name)
	if err != nil {
		return "", err
	}
	return res.Stdout + res.Stderr, nil
}

func (c *sagContainer) Remove(ctx context.Context) error {
	if _, err := podman(ctx, "rm", "--force", "--ignore", "--time", "0", c.name); err != nil {
		return err
	}
	_, err := podman(ctx, "volume", "rm", "--force", c.volume)
	return err
}

func (c *sagContainer) running(ctx context.Context) bool {
	res, err := testenv.Podman(ctx, nil, "inspect", "--format", "{{.State.Running}}", c.name)
	return err == nil && strings.TrimSpace(res.Stdout) == "true"
}

func (c *sagContainer) waitHealthy(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, healthzTimeout)
	defer cancel()
	var last error
	for {
		probe, cancelProbe := context.WithTimeout(ctx, 5*time.Second)
		last = c.Healthz(probe)
		cancelProbe()
		if last == nil {
			return nil
		}
		if !c.running(ctx) {
			return fmt.Errorf("harness: %s stopped before /healthz answered: %w", c.name, last)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("harness: %s /healthz: %w", c.name, last)
		case <-time.After(healthzPoll):
		}
	}
}

func publishedPort(ctx context.Context, name string) (int, error) {
	res, err := podman(ctx, "port", name, strconv.Itoa(ListenPort)+"/tcp")
	if err != nil {
		return 0, err
	}
	line := strings.TrimSpace(strings.SplitN(res.Stdout, "\n", 2)[0])
	i := strings.LastIndex(line, ":")
	if i < 0 {
		return 0, fmt.Errorf("harness: podman port %s: %q", name, res.Stdout)
	}
	return strconv.Atoi(line[i+1:])
}

func podman(ctx context.Context, args ...string) (testenv.Result, error) {
	res, err := testenv.Podman(ctx, nil, args...)
	if err != nil {
		return res, fmt.Errorf("harness: podman %s: %w", strings.Join(args, " "), err)
	}
	if res.ExitCode != 0 {
		return res, fmt.Errorf("harness: podman %s: exit %d: %s", strings.Join(args, " "), res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return res, nil
}

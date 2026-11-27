//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
)

const ac04Sub = "ac04"

func init() {
	harness.Register(harness.AC{
		ID:     "AC04",
		Phase:  harness.PhaseParallel,
		Agents: []harness.AgentSpec{{Sub: ac04Sub, Name: "AC04"}},
		Resources: []harness.ResourceSpec{{
			Name:       "ac04",
			Containers: []string{harness.ContainerSAG},
			Agents:     []string{ac04Sub},
		}},
		Run: runAC04,
	})
}

func ac04HasJWT(output string) bool {
	return strings.Contains(output, "eyJ")
}

func runAC04(t *testing.T, env harness.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Run("control", func(t *testing.T) {
		raw, err := env.Token(ctx, harness.ContainerSAG, ac04Sub)
		if err != nil {
			t.Fatalf("jwt generate %s on a terminal: %v", ac04Sub, err)
		}
		if strings.Count(raw, ".") != 2 {
			t.Fatalf("jwt generate %s on a terminal printed no JWT", ac04Sub)
		}
	})

	t.Run("no terminal", func(t *testing.T) {
		res, err := env.Podman(ctx, "exec", env.ContainerName(harness.ContainerSAG), "sag", "jwt", "generate", ac04Sub)
		if err != nil {
			t.Fatalf("podman exec: %v", err)
		}
		if res.ExitCode == 0 {
			t.Fatal("jwt generate without a terminal exited 0")
		}
		if !strings.Contains(res.Stderr, "must be terminals") {
			t.Fatalf("jwt generate without a terminal: stderr %q; want the terminal cause", res.Stderr)
		}
		if ac04HasJWT(res.Stdout) || ac04HasJWT(res.Stderr) {
			t.Fatal("jwt generate without a terminal printed a JWT")
		}
	})

	t.Run("unknown sub", func(t *testing.T) {
		tty, err := env.ExecTTY(ctx, harness.ContainerSAG, "sag", "jwt", "generate", "nobody")
		if err != nil {
			t.Fatalf("podman exec -it: %v", err)
		}
		res, err := tty.Wait(ctx)
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
		if res.ExitCode == 0 {
			t.Fatal("jwt generate nobody exited 0")
		}
		if !strings.Contains(res.Stdout+res.Stderr, "not in the agents list") {
			t.Fatalf("jwt generate nobody: output %q; want the agents list cause", res.Stdout+res.Stderr)
		}
		if ac04HasJWT(res.Stdout) || ac04HasJWT(res.Stderr) {
			t.Fatal("jwt generate nobody printed a JWT")
		}
	})
}

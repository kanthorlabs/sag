package bootstrap

import (
	"context"
	"fmt"
	"io"

	"github.com/kanthorlabs/sag/internal/agentsetup"
)

func (d Deps) printEndpoint(ctx context.Context, s *State, as *agentRunState) error {
	fmt.Fprintln(d.Print, "https://localhost:8443/mcp")

	cmd := "sag credential create <name> --kind postgres"
	if len(as.completed) > 0 {
		for _, c := range as.completed {
			cmd += " --agent " + c.sub
		}
	} else {
		cmd += " --agent <sub>"
	}
	fmt.Fprintln(d.Print, cmd)

	completedSet := make(map[agentsetup.Harness]bool)
	for _, c := range as.completed {
		completedSet[c.harness] = true
	}
	for _, det := range as.detected {
		if !completedSet[det.harness] {
			fmt.Fprintln(d.Print, "sag agent setup "+string(det.harness))
		}
	}

	if s.TailscaleState.Address != "" {
		printTailscaleSteps(d.Print, s.TailscaleState.Address)
	}

	return nil
}

func printTailscaleSteps(w io.Writer, addr string) {
	steps := []string{
		"Second machine steps:",
		"1. Install sag (see docs/plan/dev-setup.md), without sag bootstrap.",
		"2. Create <cfg> with mode 0700. Copy <cfg>/ca.pem of this machine to <cfg>/ca.pem on the second machine.",
		"3. On this machine: sag credential create <name> --kind <kind> --agent <sub>",
		"4. On this machine: sag jwt generate <sub>",
		"5. On the second machine: sag agent setup <harness> --url https://" + addr + ":8443/mcp --sub <sub>",
		"6. After each certificate renewal, copy <cfg>/ca.pem again.",
	}
	for _, step := range steps {
		fmt.Fprintln(w, step)
	}
}

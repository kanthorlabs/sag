package agentsetup

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kanthorlabs/sag/internal/secret"
)

var execPodmanExec = func(ctx context.Context, podmanPath string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, podmanPath, args...)
	return cmd.Output()
}

func issueJWT(ctx context.Context, opts Options) (secret.Credential, error) {
	out, err := execPodmanExec(ctx, opts.PodmanPath,
		[]string{"exec", "sag", "sag", "agent", "issue", opts.Sub})
	if err != nil {
		return secret.Credential{}, fmt.Errorf("agent issue %s: %w", opts.Sub, err)
	}
	raw := strings.TrimSpace(string(bytes.TrimRight(out, "\r\n")))
	if raw == "" {
		return secret.Credential{}, fmt.Errorf("agent issue %s: empty output", opts.Sub)
	}
	return secret.NewCredential("jwt-"+opts.Sub, []byte(raw)), nil
}

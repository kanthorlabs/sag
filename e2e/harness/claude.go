package harness

import (
	"context"
	"errors"

	"github.com/kanthorlabs/sag/internal/testenv"
)

var errClaudeNotImplemented = errors.New("harness: RunClaude is not implemented")

func RunClaude(ctx context.Context, req ClaudeRequest) (testenv.Result, error) {
	return testenv.Result{}, errClaudeNotImplemented
}

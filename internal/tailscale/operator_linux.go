//go:build linux

package tailscale

import (
	"context"
	"encoding/json"

	"github.com/kanthorlabs/sag/internal/stepline"
)

type debugPrefs struct {
	OperatorUser string `json:"OperatorUser"`
}

func EnsureOperator(ctx context.Context, cmd Command, currentUser string, hasTerminal bool, ask func() (bool, error), out stepline.Writer) error {
	data, err := cmd.Output(ctx, "debug", "prefs")
	if err != nil {
		return err
	}
	var prefs debugPrefs
	if err := json.Unmarshal(data, &prefs); err != nil {
		return err
	}
	if prefs.OperatorUser == currentUser {
		return nil
	}
	if !hasTerminal {
		out.Line(stepline.StatusSkip, "tailscale", SkipOperatorNotSet+"; loopback only")
		return ErrOperatorSkip
	}
	approved, err := ask()
	if err != nil || !approved {
		out.Line(stepline.StatusSkip, "tailscale", SkipOperatorNotSet+"; loopback only")
		return ErrOperatorSkip
	}
	if _, err := cmd.Output(ctx, "set", "--operator="+currentUser); err != nil {
		return err
	}
	return nil
}

package dxh

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func CheckRunLock(devDir string) error {
	envToken := os.Getenv("DEVX_RUN_TOKEN")
	if envToken == "" {
		return fmt.Errorf("no run token in environment; run through make e2e-devx")
	}
	owner := filepath.Join(devDir, "claims", "devx-e2e.lock", "owner")
	data, err := os.ReadFile(owner)
	if err != nil {
		return fmt.Errorf("no DX run lock at %s; run through make e2e-devx", owner)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return fmt.Errorf("DX run lock %s has invalid format", owner)
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return fmt.Errorf("DX run lock %s has an invalid owner PID %q", owner, fields[0])
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return fmt.Errorf("DX run lock owner %d does not run", pid)
	}
	fileToken := fields[1]
	if fileToken != envToken {
		return fmt.Errorf("DX run lock token mismatch; run through make e2e-devx")
	}
	return nil
}

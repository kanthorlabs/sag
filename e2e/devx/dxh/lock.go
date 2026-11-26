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
	owner := filepath.Join(devDir, "claims", "devx-e2e.lock", "owner")
	data, err := os.ReadFile(owner)
	if err != nil {
		return fmt.Errorf("no DX run lock at %s; run through make e2e-devx", owner)
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return fmt.Errorf("DX run lock %s has no owner PID", owner)
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return fmt.Errorf("DX run lock %s has an invalid owner PID %q", owner, fields[0])
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return fmt.Errorf("DX run lock owner %d does not run", pid)
	}
	return nil
}

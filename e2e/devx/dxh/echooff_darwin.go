package dxh

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

func waitEchoOff(ctx context.Context, master *os.File) error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		var t syscall.Termios
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGETA, uintptr(unsafe.Pointer(&t))); errno != 0 {
			return fmt.Errorf("TIOCGETA: %w", errno)
		}
		if t.Lflag&syscall.ECHO == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("echo did not turn off within 2s")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

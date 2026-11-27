package cli

import (
	"fmt"
	"os"
)

type Mode int

const (
	ModeUnset     Mode = 0
	ModeContainer Mode = 1
	ModeHost      Mode = 2
)

type commandKind int

const (
	kindAny           commandKind = 0
	kindContainerOnly commandKind = 1
	kindHostOnly      commandKind = 2
	kindInternal      commandKind = 3
)

var uid = os.Getuid

func resolveMode(m Mode) Mode {
	if m != ModeUnset {
		return m
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return ModeContainer
	}
	return ModeHost
}

func commandGuard(m Mode, c command, std IO) (int, bool) {
	if m != ModeHost {
		return 0, false
	}
	if c.kind == kindInternal {
		fmt.Fprintln(std.Err, "sag: command runs only in the container")
		return ExitUsage, true
	}
	if c.kind == kindHostOnly && uid() == 0 {
		fmt.Fprintln(std.Err, "sag: host commands refuse root")
		return ExitFailure, true
	}
	return 0, false
}

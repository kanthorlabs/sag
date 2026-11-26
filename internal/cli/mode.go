package cli

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

func resolveMode(m Mode) Mode {
	if m == ModeUnset {
		return ModeContainer
	}
	return m
}

func commandGuard(m Mode, c command, std IO) (int, bool) {
	return 0, false
}

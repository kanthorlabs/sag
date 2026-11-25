package daemon

import (
	"log/slog"

	"github.com/kanthorlabs/sag/internal/scrub"
)

func scrubHandler(h slog.Handler, _ scrub.Scrubber) slog.Handler {
	return h
}

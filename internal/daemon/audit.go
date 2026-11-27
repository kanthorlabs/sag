package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/kanthorlabs/sag/internal/audit"
	"github.com/kanthorlabs/sag/internal/endpoint"
)

func authAuditHook(w audit.Writer, log *slog.Logger) endpoint.AuditHook {
	return func(_ context.Context, remoteAddr, cause string) {
		rec := audit.AuthFailure(time.Now(), ulid.Make().String(), remoteAddr, cause)
		if err := w.Append(rec); err != nil {
			log.Error("audit append failed", "request_id", rec.RequestID, "error", err.Error())
		}
	}
}

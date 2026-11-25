package endpoint

import "context"

type AuditHook func(ctx context.Context, remoteAddr, cause string)

# Foundation

This file holds the decisions that all layers share. The PRD wins on any conflict.

## 1. Toolchain

`go.mod` sets `module github.com/kanthorlabs/sag`, `go 1.27`, and `toolchain go1.27.4`. A local Go 1.25 downloads go1.27.4 by itself, because `GOTOOLCHAIN=auto`.

## 2. Dependencies

Module versions from the Go module proxy, read on 2026-11-25:

| Module | Version | Added by | Use |
|---|---|---|---|
| `github.com/modelcontextprotocol/go-sdk` | v1.8.0 | `T1.0` | MCP server (`mcp`), bearer token bridge (`auth`), e2e client |
| `github.com/jackc/pgx/v5` | v5.13.0 | `T1.0` | PostgreSQL driver, `pgxpool` |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | `T1.0` | HS256 sign and verify |
| `github.com/oklog/ulid/v2` | v2.1.2 | `T1.0` | `jti`, `request_id` |
| `go.yaml.in/yaml/v3` | v3.0.6 | `T1.0` | `sag.yaml` with `KnownFields(true)` |
| `github.com/creack/pty` | v1.1.24 | `T1.0` | Test only: drive `podman exec -it` |
| `github.com/pganalyze/pg_query_go/v6` | v6.2.5 | `T2.0` | Parse and deparse (PostgreSQL 17 grammar) |
| `google.golang.org/protobuf` | the version that `go mod tidy` selects, minimum v1.33.0 | `T3.0` | `protoreflect` walk |

Standard library: `crypto/hkdf`, `crypto/hmac`, `crypto/sha256`, `crypto/tls`, `crypto/x509`, `net/http`, `encoding/json`, `log/slog`, `syscall`.

Images:

| Image | Tag | Use |
|---|---|---|
| Builder | `docker.io/library/golang:1.27.4-bookworm` | cgo build |
| Runtime | `gcr.io/distroless/base-debian12:nonroot` | glibc, CA certificates, UID 65532 |
| PostgreSQL 13 | `docker.io/library/postgres:13-bookworm` | AC9 only |
| PostgreSQL 14 to 19 | `postgres:14.25-bookworm`, `15.20-bookworm`, `16.16-bookworm`, `17.12-bookworm`, `18.7-bookworm`, `19.0-bookworm` | Integration and e2e |
| busybox | `docker.io/library/busybox:1.37.0-musl` | e2e only: static `stat` for AC33 |

## 3. Module layout

One responsibility per package.

| Path | Responsibility |
|---|---|
| `cmd/sag/main.go` | Product entry point. Blank-imports the adapters. Calls `cli.Run`. |
| `internal/cli` | Command dispatch, terminal check, exit codes. One file per command. |
| `internal/config` | `sag.yaml` schema, defaults, hard maximums, unknown-key rejection. |
| `internal/secret` | Podman secret file load, owner and mode check, `Credential` type, master key decode. |
| `internal/keys` | HKDF derivation of the JWT key and the approval HMAC key. |
| `internal/token` | JWT issue and JWT verification in the PRD order, refusal causes. |
| `internal/core` | `Adapter`, `Resource`, `Capability`, `Operation`, `Limits`, `Result`, `Trace`, error codes, compile-time registry. No SQL. |
| `internal/inventory` | Opened resources, preflight status, tag authorization (`Visible`, `Lookup`). |
| `internal/preflight` | Preflight schedule: start, 60 s retry, `preflight.recheck_interval` re-check, injectable clock. |
| `internal/approval` | Fingerprint, approval record, HMAC, file store. |
| `internal/pipeline` | Request pipeline: authorize, resolve limits, semaphore, `Resource.Execute`, post-execute, audit. |
| `internal/postexec` | Cell cap, column redaction, byte cap. |
| `internal/scrub` | Removal of secret values from errors and log text. |
| `internal/audit` | Audit record type and append-only daily file writer. |
| `internal/endpoint` | TLS server, `/healthz`, body and header limits, `Origin` check, JWT middleware. |
| `internal/mcpserver` | MCP tools from the registry, input and output schemas, error results. |
| `internal/daemon` | `sag serve` wiring: environment scrub, `RLIMIT_CORE`, load, open, schedule, serve. |
| `internal/sql` | `Dialect`, `Conn`, `Pool`, `Tree`, `RowCapPlan`, `EngineError`; SQL executor; `sql` capability operations. |
| `internal/adapter/postgres` | `postgres` adapter: connection config, dialect, allowlist walk, render, catalog rules, preflight. |
| `internal/testenv` | Test only: Podman driver, PostgreSQL containers, test CA and certificates, Podman secrets. |
| `e2e/harness` | Test only: AC registry, SAG container start, MCP client, JWT mint, pseudo-terminal driver, log and audit readers. |
| `e2e/conformance` | Test only: `Containerfile`, `cmd/sag-conformance`, `passthrough` dialect wrapper. |
| `e2e/cmd/acreport` | Test only: AC report from `go test -json` events. |
| `e2e` | Test only: `acs.go` (AC table) and one test file for each AC (`ac<NN>_test.go`). |
| `deploy` | `Containerfile`, `sag.container` (Quadlet), `dev/sag.yaml`. |
| `scripts` | `dev-up.sh`, `dev-down.sh`. |
| `Makefile` | Build, test, e2e, and dev targets. |
| `.github/workflows/ci.yml` | CI (AC35). |

## 4. Core boundary

- `core.Resource.Execute(ctx, capability, operation, input, lim)` is the only call from the core into an adapter.
- The core pipeline order is PRD section 14.3: JWT, authorize, resolve limits, one semaphore for each JWT `sub` shared across all resources, `Resource.Execute`, post-execute, audit.
- `sql.Executor` implements `Execute` for every SQL adapter. It owns static validation, the sequence, and `ROLLBACK`.
- Inside `Execute`: `Parse`, `ValidateStatic`, `Render`, second parse and validation, then take a connection, `RequestBegin`, `ValidateCatalog`, `PlanRowCap`, execute `Statement` and `Fetch`, `ROLLBACK`.
- The adapter supplies `sql.Dialect` and `sql.Pool`. `sql.Pool` hands out `sql.Conn`. The adapter maps engine errors to `sql.EngineError{SQLState, Class}` in its `Conn`.
- `Result` is tabular: `columns`, `rows`, `truncated`, `truncated_reason`. `Result` and `core.Error` carry `Trace{Input, InputSHA256, RenderedSHA256}`. The audit writer maps `Trace` to the fields `sql`, `sql_sha256`, `rendered_sha256`.

## 5. Fixed design decisions

- The MCP handler runs with `StreamableHTTPOptions{Stateless: true, JSONResponse: true}`.
- `internal/endpoint` verifies the JWT in the PRD order and writes the `401` with `WWW-Authenticate: Bearer`. It then hands the identity to go-sdk `auth.RequireBearerToken` through a verifier that returns `auth.TokenInfo{UserID: sub, Expiration: exp}`. Tool handlers read `req.Extra.TokenInfo`.
- JWT parse: `jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithoutClaimsValidation(), jwt.WithJSONNumber())` with `jwt.MapClaims`. `internal/token` checks the claims in the PRD order.
- `sag jwt generate` prints the bare JWT and one newline.
- The terminal check uses `os.File.Stat` and `os.ModeCharDevice`.
- The pgx config starts from `pgx.ParseConfig("")` after the daemon removes each `PG*` variable. SAG then sets host, port, database, user, password, TLS, `ConnectTimeout`, `DefaultQueryExecMode = QueryExecModeExec`, and `Fallbacks = nil`.
- `max_concurrent_per_agent` (default 20) lives only in `defaults.limits`. The semaphore is non-blocking: an excess request gets `rate_limited`.
- `pool_size` is per resource. The executor acquires a connection under the request context deadline (`statement_timeout`). An exhausted pool returns `timeout` at the deadline.
- `pgxpool` `AfterConnect` runs `SessionGuard`. A connection that returns an error, a timeout, or a failed `ROLLBACK` is closed and leaves the pool.
- Every SAG statement on a connection, also preflight and `sag extension approve`, runs inside `RequestBegin` and ends with `ROLLBACK`. Only `SessionGuard` runs outside a transaction.
- Tool errors return `isError: true`. The text content is the JSON object `{"code","message","request_id"}`.
- `sql_list_tables` and `sql_describe_table` build a fixed `information_schema` text. Input values enter as string literals with each `'` doubled. A value longer than 63 bytes or with a NUL byte returns `invalid_argument`. The text then runs through the full `sql_query` path, so C1, C2, and C3 apply.
- Pass-through validator: `e2e/conformance/passthrough` wraps a `sql.Dialect`. `Parse`, `ValidateStatic`, `Render`, and `ValidateCatalog` pass the text unchanged. `PlanRowCap` returns the text as `Statement` with an empty `Fetch`. Only `e2e/conformance/cmd/sag-conformance` imports it. `daemon.Options.WrapDialect` is the injection point. No flag or config key sets it.
- `sag serve` requires secret owner 65532. `secret.Load` takes the expected UID as a parameter, so unit tests pass the test user UID.
- The e2e harness sets `preflight.recheck_interval: 10s`.

## 6. Critical security tasks

| Control | Task | Files | AC that proves it |
|---|---|---|---|
| C1 validator: node and field allowlist, `protoreflect` walk, single statement, credential catalogs | `T3.1`, `T3.2`, `T3.3` | `allowlist.go`, `walk.go`, `static.go`, `relations.go`, `funcrules.go` | AC13, AC14, AC15 |
| C2 read-only transaction setup: `SessionGuard`, `BEGIN TRANSACTION READ ONLY`, 5 `SET LOCAL` values, extended protocol only | `T2.2` | `session.go`, `conn.go`, `connconfig.go` | AC5, AC6, AC16, AC18 |
| C3 rollback only: executor owns `ROLLBACK`, closes the connection on failure | `T2.1` | `internal/sql/executor.go` | AC8 |
| Volatility gate: reject volatile functions outside the 4-name list | `T3.4` | `catalog.go` | AC15 |
| Deparse-only execution: send only `pg_query.Deparse` output, validate it again | `T3.5` | `render.go`, `rowcap.go` | AC17, AC16 |
| No pass-through code in the product binary | `T2.7` | `cmd/sag/guard_test.go`, `e2e/conformance` | AC7 |
| Secret handling: owner and mode, master key length, no inline DSN, scrub, fixed connect error, `Credential` redaction, no JWT in audit | `T1.1`, `T1.2`, `T4.3`, `T4.4`, `T6.2` | `internal/config`, `internal/secret`, `internal/scrub`, `internal/audit` | AC1, AC2, AC24, AC26, AC33 |
| JWT verification: HS256 only, closed claims, PRD order, `auth.version` key, terminal-only issue | `T1.3`, `T1.4`, `T5.1`, `T6.1` | `internal/token`, `internal/endpoint/auth.go`, `internal/cli/jwt_generate.go` | AC3, AC4, AC28, AC34 |
| Authorization: default deny, `not_found` for foreign resources | `T1.5`, `T5.2` | `internal/inventory`, `internal/mcpserver` | AC29 |
| Preflight and approval: version, TLS, risky objects, HMAC, re-check | `T2.3`, `T2.4`, `T2.5` | `preflight.go`, `tls.go`, `internal/approval`, `internal/preflight` | AC9, AC10, AC11 |
| Audit before data: `internal` on append failure | `T4.4` | `internal/pipeline`, `internal/audit` | AC27 |

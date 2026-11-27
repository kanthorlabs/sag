# Layer 1 review

- Reviewer: Aelita. Owner: Ulrich.
- Code: `.dev/integration`, branch `feat/v1-postgres`, head `936e80b`.
- Spec: `docs/PRD.md` revision 5, `docs/plan/layer-1.md`, `docs/plan/foundation.md`, `docs/plan/testing.md`.
- Method: Aelita read each layer-1 source file. Aelita ran probes in a copy outside the repository. Aelita changed no repository file and made no commit.
- Baseline at head: `go vet ./...`, `go test -count=1 ./...`, `go mod verify`, and the e2e suite on PostgreSQL 17 pass.
- Marks: `[tested]` = Aelita reproduced the behavior. `[read]` = Aelita found it in the source text only.
- Cleanup: Aelita removed each Podman object of the probes (`rev-*`, one `sag-e2e-*` run). The worktree is clean.

## Findings

- B1 - status:FIXED - action:YES - AC01 e2e invalid cases cannot fail - [tested] `e2e/ac01_test.go:53-68` asserts only exit 1 and the absence of `postgres://`. Podman mounts `/run/secrets` into the container, so `ac01-valid.yaml` also exits 1 there (`lstat /run/secrets/sag-orders: no such file or directory`). A validator that accepts all four invalid fixtures still gives AC01 `PASS`. The unit tests in `internal/cli/config_check_test.go:31-54` discriminate; the e2e test does not. - fix:For each invalid fixture, assert the cause in stderr (the config key path of that fixture) and assert that stderr contains no `/run/secrets` text. Add an in-image control: run `ac01-valid.yaml` with its secrets mounted with `uid=65532,mode=0400`, and expect exit 0 and `config ok`. - why:The AC report shows AC01 `PASS` from four cases that pass for every input. The gate evidence must fail when the behavior is wrong. Fixed in `d8a725b`; test: `TestE2E/parallel/AC01_ConfigCheckRejects` subtests `ac01-inline-dsn`, `ac01-unknown-key`, `ac01-max-rows`, `ac01-recheck-5s` (cause and no `/run/secrets` text), `ac01-valid-in-image` (secrets uid 65532 mode 0400, exit 0, `config ok`). A mutation that accepts schema problems fails three cases.

- B2 - status:FIXED - action:YES - Config errors print a DSN with its password - [tested] `sag config check` prints the full DSN when the DSN is the value of `kind` (`internal/cli/config_check.go:47`), an unknown key or a duplicate key (`internal/config/config.go:145`), or a resource name (`internal/config/config.go:230-232`). [read] `sag serve` prints the same text to the container log (`internal/daemon/resources.go:22`, `internal/config/config.go:145`). `yamlValuePattern` masks only text between backticks. - fix:Print no operator text in config errors. Validate `kind` against `namePattern` in `internal/config` and print a fixed message. Replace each YAML decode error with its line number and a fixed class (`unknown key`, `duplicate key`, `wrong type`). Print a resource name only after it matches `namePattern`. Add the four cases next to `TestTypeErrorHidesValue`. - why:PRD section 10.2 states that SAG never puts a DSN string into an error or a log line. Asset A1 has the highest priority. The exposure needs an operator edit error, but the container log then keeps the password. Fixed in `2428695`; test: `TestOperatorTextHiddenInErrors` (kind, unknown key, duplicate key, resource name, syntax error), `TestConfigErrorHidesDSN` (daemon error and log), updated `TestSchemaRejects`. `sag serve` in `localhost/sag:e2e` logs no DSN text for the three cases.

- S1 - status:FIXED - action:YES - Preflight has no deadline after connect - [tested] A server that completes authentication and then sends no data blocks `Preflight` without limit (`internal/adapter/postgres/preflight.go:34-39`). Aelita stopped the probe after 20 s; `connect_timeout` is 5 s. `openResources` runs each preflight in sequence before the listener opens (`internal/daemon/resources.go:46`, `internal/daemon/run.go:73`). One stalled database stops `sag serve` with no log line and no `/healthz`. - fix:In `daemon.preflight`, wrap the call in `context.WithTimeout` (for example `connect_timeout` plus 10 s). Close the connection with a bounded context. Add a unit test with a stalled fake server (`pgproto3.Backend`). - why:Actor T5 or a network fault controls the availability of the daemon for all resources. PRD section 11.3 expects a failed resource to become `resource_unavailable`, not to stop the other resources. Fixed in `3d15f8c`; test: `TestPreflightStalledServerTimesOut` (`pgproto3.Backend` fake server, deadline `connect_timeout` + 10 s, bounded close 5 s).

- S2 - status:FIXED - action:YES - `conn.Exec` without arguments uses the simple protocol - [tested] With the layer-1 config (`DefaultQueryExecMode = QueryExecModeExec`, `internal/adapter/postgres/connconfig.go:59`), `conn.Exec` with the text `CREATE TEMP TABLE ...; INSERT ...; SELECT 1` succeeds and writes one row. pgx v5.13.0 forces the simple protocol when `Exec` has no arguments (pgx `conn.go:524-527`). `conn.Query` with two statements fails with SQLSTATE `42601`. `readVersion` uses `conn.Exec` (`internal/adapter/postgres/preflight.go:34`, `:39`). `TestExecModeExec` (`connconfig_test.go:107-112`) asserts only the config field. - fix:Send `BEGIN` and `ROLLBACK` through `conn.PgConn().ExecParams` or `conn.Query`. Add an integration test: multi-statement text through the adapter connection path returns `42601`. Record the rule "no `conn.Exec` without arguments" in the T2.0 contract. - why:C2 and AC6 in layer 2 depend on the extended protocol. The current test gives false confidence, and layer 2 builds on the layer-1 pattern. Fixed in `3d15f8c`; test: `TestExtendedProtocolRejectsMultiStatement` (integration, SQLSTATE 42601). No `conn.Exec` without arguments remains. The rule is in `.dev/local/layer-1-contract.md` for T2.0.

- S3 - status:FIXED - action:YES - Unauthenticated redirect for non-canonical paths - [tested] `GET //mcp`, `POST //mcp`, `GET /a/../mcp`, and `GET //healthz` without a token return `307` with a `Location` header, not `401`. The `sag:e2e` container gives the same result. `ServeMux` cleans the path before `authenticate` runs (`internal/endpoint/server.go:58-66`). - fix:Put `authenticate` outside the mux. One outer handler serves `GET /healthz` on the exact path and sends each other request to `authenticate` first. Add the paths to `Test401HasWWWAuthenticate`. - why:PRD section 6 states that all paths other than `/healthz` need a JWT, and each failure returns `401`. In layer 4, these requests also write no audit record for the authentication failure. Fixed in `24b6574`; test: `Test401HasWWWAuthenticate` cases `//mcp`, `/a/../mcp`, `//healthz`, `/healthz/`, `/a/../healthz`, `/%68ealthz`, and POST `//mcp`, `/a/../mcp`, `/healthz`.

- S4 - status:FIXED - action:YES - Endpoint has no idle timeout, body timeout, or header size limit - [tested] After one `401`, an unauthenticated connection stays open and usable after 12 s without traffic. A request with a valid JWT and a partial body holds its handler for more than 12 s. [read] `http.Server` sets only `ReadHeaderTimeout` (`internal/endpoint/server.go:95-105`). `MaxHeaderBytes` is the 1 MiB default, and `token.Verify` decodes a header of that size before the signature check. - fix:Set `IdleTimeout` (for example 60 s), `ReadTimeout` (for example 10 s), and `MaxHeaderBytes` (for example 16 KiB). Add the values to `TestServerTimeouts`. - why:Actor T2 reaches the port without a JWT and holds file descriptors without limit. PRD section 7.5 names the timeouts as a T2 mitigation, and rate limits come only in phase 2. Fixed in `24b6574`; test: `TestServerTimeouts` (`ReadTimeout` 10 s, `IdleTimeout` 60 s, `MaxHeaderBytes` 16 KiB).

- S5 - status:FIXED - action:YES - `.dev/` is not in `.gitignore` - [read] `scripts/dev-up.sh:5-26` writes `master-key`, `pg-password`, and TLS keys to `.dev/local/dev`. Only `.git/info/exclude` hides that directory (`docs/plan/dev-setup.md:3`), and that file is local to one clone. `.gitignore:1-2` lists only `/bin/` and `/e2e/report/`. - fix:Add `/.dev/` to `.gitignore`. - why:In a new clone, `git add -A` after `make dev-up` stages the development master key, the database password, and the mkcert leaf keys. Fixed in `1175550`; test: `git check-ignore -v .dev/local/x` matches `.gitignore:3:/.dev/`. No Go test.

- S6 - status:FIXED - action:YES - Base images use tags, not digests - [read] `deploy/Containerfile:1` and `deploy/Containerfile:16` name `golang:1.27.4-bookworm` and `gcr.io/distroless/base-debian12:nonroot`. The `nonroot` tag moves with each distroless rebuild. [tested] The two action SHAs in `.github/workflows/ci.yml:16` and `:19` are release commits (`actions/checkout` v7.0.1, `actions/setup-go` v7.1.0). - fix:Pin both `FROM` lines with `@sha256:<index digest>` and keep the tag in the reference. - why:The plan pins actions by commit SHA, and the product image needs the same rule. A changed base image changes the runtime of the credential holder without a diff. Fixed in `1175550`; test: `TestContainerfileBaseImagesPinned`. golang `sha256:ce59b414...`, distroless `sha256:7f0c72cd...`.

- S7 - status:FIXED - action:YES - Process rules apply to `sag serve` only - [read] `removePGEnv` and `disableCoreDumps` run only in `daemon.Run` (`internal/daemon/run.go:38-43`). `sag jwt generate` loads the master key without `RLIMIT_CORE` 0 (`internal/cli/jwt_generate.go:36-45`). [tested] Without the environment scrub, `connConfig` keeps `ValidateConnect`, `SSLNegotiation`, and `ChannelBinding` from `PG*` variables (`internal/adapter/postgres/connconfig.go:46-59`). Host, password, TLS, fallbacks, and runtime parameters stay correct. - fix:Call both functions in `cli.Run` before the dispatch, for each command. - why:`sag resource test` and `sag extension approve` in layer 2 open database connections outside the daemon. One call site removes the dependency on each command author. Fixed in `5b01d22`; test: `TestRunAppliesProcessRules` (`daemon.ApplyProcessRules` in `cli.Run`).

- S8 - status:FIXED - action:YES - Daemon log drops the cause of a failed resource - [tested] In the `sag:e2e` container, a wrong password gives only `resource preflight failed` and the resource name. A DSN secret with a final newline fails `Open` with a fixed text. [read] `internal/daemon/resources.go:34` and `:70` discard `err`. The `Open` errors are fixed sentinel texts without secret data (`internal/adapter/postgres/connconfig.go:22-26`). - fix:Log `err` for `Open` failures. For `Preflight` failures, log a fixed class (connect, TLS, query) until the scrub of T4.3 exists. - why:The operator cannot tell a format error from a network error. The sentinel texts contain no part of the DSN (`TestOpenErrorOmitsSecret`). Fixed in `3d15f8c`; test: `TestOpenFailureLogsCause`; `assertPreflightCause` in `TestPreflightRefusesUnknownCA` (TLS) and `TestPreflightWrongPasswordOmitsPassword` (connect).

- S9 - status:FIXED - action:YES - `sag config check` ignores the master key secret - [read] `checkSecretFiles` checks only `cfg.SecretNames()` (`internal/cli/config_check.go:59`, `internal/config/config.go:293-303`). `sag-master-key` is not in that list; the daemon adds it (`internal/daemon/run.go:51-54`). - fix:When the secrets directory exists, also call `secret.CheckFile` for `secret.MasterKeyName`. - why:`config check` prints `config ok`, and then `sag serve` refuses to start for an absent or wrong-mode master key. Fixed in `5b01d22`; test: `TestConfigCheckMasterKey` (absent, mode 0444, valid).

- S10 - status:FIXED - action:YES - JWT parser accepts non-canonical base64 - [tested] A valid token with a different last signature character (`g` to `h`) passes `token.Verify`. The parser in `internal/token/token.go:114-118` has no strict decode option. - fix:Add `jwt.WithStrictDecoding()` and one test case. - why:One signed token has more than one accepted text form. An exact-text control in a later layer (audit search of AC26, secret scrub) then does not find the variant. Fixed in `09ab43e`; test: `TestAC03_Rejects/non-canonical_signature_base64`.

- S11 - status:FIXED - action:YES - AC03 and AC04 e2e assert the status only - [read] `e2e/ac03_test.go:134` accepts each `WWW-Authenticate` value with the prefix `Bearer` and does not require an empty body. `e2e/ac04_test.go:53` and `:70` accept each non-zero exit. The control cases keep both tests valid. - fix:Require `WWW-Authenticate` equal to `Bearer` and an empty body. Require the stderr texts `not a terminal` and `not in the agents list`. - why:An exact assertion detects a `401` from the go-sdk bridge and an exit for a different cause. Fixed in `d8a725b`; test: `TestE2E/parallel/AC03_JWTRejects` (exactly `[Bearer]`, empty body); `TestE2E/parallel/AC04_JWTGenerateRefuses/no_terminal` (`not a terminal`) and `/unknown_sub` (`not in the agents list`).

- S12 - status:FIXED - action:YES - Stub skip path stays in the daemon tests - [read] `internal/daemon/run_test.go:93-95` calls `t.Skipf` when `Run` returns `errors.ErrUnsupported`. The stubs of T1.0 no longer exist. - fix:Delete the branch. - why:A later stub or an `ENOTSUP` system error makes `TestRefusesBadMasterKey` and `TestRefusesSecretMode` skip, and `go test` still prints `ok`. Fixed in `2428695`; test: `TestRefusesBadMasterKey` and `TestRefusesSecretMode` cannot skip.

- S13 - status:FIXED - action:YES - Unit case `auth.version change` repeats `wrong signature` - [read] `internal/token/verify_test.go:123-125` signs with `otherKey`, the same as the case in `:97-99`. No unit test verifies a token of `keys.JWTKey(master, 1)` against `keys.JWTKey(master, 2)`. - fix:Derive both keys from one master key in that case. - why:The AC3 unit table lists nine cases and proves eight. The e2e case with `sag-v2` covers the behavior. Fixed in `09ab43e`; test: `TestAC03_Rejects/auth.version_change` (keys of one master key, versions 1 and 2).

- S14 - status:FIXED - action:YES - AC report shows 0 ms for an AC with parallel subtests - [tested] `AC02_StartRefusesBadSecrets` reports `0.00s`, and its subtests report 1.28 s and 1.34 s. Go excludes parallel subtests from the parent time. `mergeMajors` reads only the parent event (`e2e/cmd/acreport/report.go:121`). - fix:Use the maximum `Elapsed` of the AC subtest and its descendants. Add a case to `report_test.go`. - why:The `Duration` column is wrong for each AC that uses `t.Parallel()` in subtests. The gate result does not change. Fixed in `19fce85`; test: `TestDurationIncludesParallelSubtests`.

- S15 - status:FIXED - action:YES - AC report cannot show that `Harness_Skeleton` passed - [read] `buildReport` lists non-AC tests only when they fail (`e2e/cmd/acreport/report.go:90-100`). A run without `Harness_Skeleton` (for example `SAG_AC_SKIP=Harness_Skeleton`) passes the gate. - fix:Add a line to `ac-report.md` for each non-AC subtest with its status, and fail the gate when `Harness_Skeleton` did not run. - why:`docs/plan/layer-1.md` T1.10 expects the report to show that `Harness_Skeleton` passed. Fixed in `19fce85`; test: `TestOtherSubtestsListed`, `TestGateFailsOnEarlierLayer/skeleton_not_run`.

- S16 - status:OPEN - action:NO - AC01 valid case runs on the host - [tested] The Podman machine has `/usr/share/containers/mounts.conf` with `/usr/share/rhel/secrets:/run/secrets`, so `/run/secrets` exists in each container. The host run in `e2e/ac01_test.go:33-51` is the only way to meet "without `/run/secrets`" on this platform. - fix:None for the host run. The in-image control of B1 covers the image binary. - why:PRD section 13.1 names `sag config check` as a host command for macOS and Linux, and the PRD check for AC1 is a Go test with five fixtures. The case is a valid e2e for that clause. Limit: it proves the host build, not the image binary.

- S17 - status:OPEN - action:NO - Terminal check does not stop the capture of a JWT - [tested] `podman exec -t <sag> sag jwt generate <sub>` with host stdout on a pipe exits 0, and the pipe receives the JWT. `isTerminal` also returns true for `/dev/null` (`internal/cli/tty.go:14`). - fix:None. - why:The PRD rule covers standard output inside the container, and `-t` gives it a pseudo-terminal. `docs/plan/dev-setup.md:6` uses this form on purpose. The image has no shell, so no in-container redirect to a character device exists. AC4 and AC34 stop accidents, not a script.

- S18 - status:OPEN - action:NO - `Origin` check is not active - [tested] A request with `Origin: https://evil.example` and a valid JWT returns `200`. `endpoint.Options.AllowedOrigins` has no reader (`internal/endpoint/server.go:28`). - fix:None in layer 1. T5.1 adds the check with AC28. - why:PRD section 11.4 is outside the layer-1 sections. A browser cannot add `Authorization` to a cross-origin request without a preflight, and `OPTIONS` without a JWT returns `401`. The go-sdk handler also rejects a non-local `Host` on a loopback connection with `403`.

- S19 - status:OPEN - action:NO - go-sdk bridge checks the expiry a second time - [read] `auth.RequireBearerToken` compares `TokenInfo.Expiration` with `time.Now()` and answers `401` with the body `token expired` and no `WWW-Authenticate` (go-sdk `auth/auth.go:170-171`, `:118`). SAG reaches that branch only when a token expires between the two checks (`internal/endpoint/auth.go:26`, `:41`). - fix:None. - why:The window is some microseconds, one time for each token. The token holder already reads `exp` from the payload, so the body gives no new fact.

- S20 - status:OPEN - action:NO - `core.ResourceConfig` has no lock timeout and no idle timeout - [read] `core.Limits` holds only `Timeout` (`internal/core/adapter.go:35-40`), and `resourceConfig` copies only `PoolSize` and `ConnectTimeout` (`internal/daemon/resources.go:53-60`). `RequestBegin` in layer 2 needs `lock_timeout` and `idle_in_transaction_session_timeout` for the five `SET LOCAL` values. - fix:None in layer 1. T2.0 adds the two fields to `core.ResourceConfig`. - why:The layer-1 code has no reader for the values. The core has no SQL, and the registry fits the adapter contract of PRD section 14.

- S21 - status:OPEN - action:NO - `os/exec` is in the product binary - [tested] `go list -deps ./cmd/sag` contains `os/exec`. The only importer is `github.com/modelcontextprotocol/go-sdk/mcp`. - fix:None. - why:No SAG product package imports `os/exec`, so the daemon starts no child process (PRD section 10.3). A later symbol check must not use `os/exec` as its signal.

## Checks without a finding

- Code comments: none in Go, shell, YAML, `Makefile`, and `Containerfile`. Only `//go:build` lines exist.
- Repository content: no private key, no real JWT, no real DSN. The DSN and JWT literals are test dummies. `scripts/dev-up.sh:35` holds a `printf` template, not a password.
- AC report: it holds statuses only and no credential.
- Daemon log of the e2e container [tested]: no master key, no database password, no DSN text, no JWT.
- Image [tested]: user `65532:65532`, `/var/lib/sag`, `audit`, and `approvals` with mode 0700 and owner 65532, binary with owner root and mode 0755, no shell, no busybox.
- Harness container flags [tested]: read-only root filesystem, `--cap-drop=all`, `no-new-privileges`, publish on `127.0.0.1`.
- JWT [tested]: `alg` values `none`, `HS512`, and `hs256` fail. Unknown, absent, and wrong-type claims fail in the PRD order. The `401` has an empty body and `WWW-Authenticate: Bearer`. The signature compare uses `hmac.Equal`.
- Secrets [read, unit tests pass]: `O_NOFOLLOW` open, then `fstat` on the same descriptor, so no TOCTOU exists. The loader checks owner, mode 0400, regular file, and a name without a path separator.
- PostgreSQL connection config [tested]: query parameters, fragments, and multiple hosts fail. The error text never contains the URL. `Fallbacks` is empty. The TLS exemption applies to loopback IP literals only; `localhost` gets `verify-full`.
- Inventory [tested in the container]: an agent sees only its served resources. A resource with a failed preflight, a resource of a different agent, and a resource without agents stay hidden.

## Verdict

Yes. The layer 1 gate can close after the fixes for B1, B2, and each `action:YES` suggestion merge, and `make e2e-matrix LAYER=1 PGS="14 17 19"` passes again. No finding needs a design change. No finding breaks the product behavior of AC1 to AC4; B1 is a defect of the proof, and B2 is a defect of error text.

## Final AC report

- Commit: `eb9a58d75893ec9de3bb9035901fa9a8179c54fe`
- Image: `sha256:3ee5b41b6c40a3b4652f18a097e1066b49718eac08ec4e317e30a8e01f5161aa`
- Generated: 2026-11-25T07:48:49.294Z
- Gate layer 1: PASS

| AC | Layer | Test | Status | PG14 | PG17 | PG19 | Duration | Message |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| AC01 | 1 | `TestE2E/parallel/AC01_ConfigCheckRejects` | PASS | PASS | PASS | PASS | 8570 ms |  |
| AC02 | 1 | `TestE2E/parallel/AC02_StartRefusesBadSecrets` | PASS | PASS | PASS | PASS | 7550 ms |  |
| AC03 | 1 | `TestE2E/parallel/AC03_JWTRejects` | PASS | PASS | PASS | PASS | 5740 ms |  |
| AC04 | 1 | `TestE2E/parallel/AC04_JWTGenerateRefuses` | PASS | PASS | PASS | PASS | 5800 ms |  |
| AC05 | 2 | `AC05_C2BlocksWrites` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC06 | 2 | `AC06_SingleStatement` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC07 | 2 | `AC07_NoPassThrough` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC08 | 2 | `AC08_RollbackOnly` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC09 | 2 | `AC09_RefusesPG13` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC10 | 2 | `AC10_TLSVerifyFull` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC11 | 2 | `AC11_ExtensionApproval` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC12 | 2 | `AC12_OperatorCollision` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC13 | 3 | `AC13_StaticRejects` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC14 | 3 | `AC14_FieldAllowlist` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC15 | 3 | `AC15_C1OnlyEffects` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC16 | 3 | `AC16_LexerSettings` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC17 | 3 | `AC17_RenderedTextOnly` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC18 | 3 | `AC18_ViewWriteBlocked` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC19 | 3 | `AC19_Integrity` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC20 | 4 | `AC20_MaxRows` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC21 | 4 | `AC21_MaxResultBytes` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC22 | 4 | `AC22_LockTimeout` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC23 | 4 | `AC23_RateLimit` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC24 | 4 | `AC24_ConnectErrorScrub` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC25 | 4 | `AC25_ColumnRedaction` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC26 | 4 | `AC26_AuditRecord` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC27 | 4 | `AC27_AuditFailure` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC28 | 5 | `AC28_EndpointAuthOrigin` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC29 | 5 | `AC29_Authorization` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC30 | 5 | `AC30_ToolShapes` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC31 | 5 | `AC31_ClaudeCode` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC32 | 6 | `AC32_ContainerInspect` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC33 | 6 | `AC33_SecretMounts` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC34 | 6 | `AC34_JWTGenerateNoTTY` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC35 | 6 | `AC35_CI` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |

### Other subtests

| Test | Status | PG14 | PG17 | PG19 |
| --- | --- | --- | --- | --- |
| `TestE2E/parallel/Harness_Skeleton` | PASS | PASS | PASS | PASS |

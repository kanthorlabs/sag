# Layer 4: Limits, errors, redaction, audit

| Item | Value |
|---|---|
| Goal | Each request runs inside the limits of PRD section 10.1. Each `tools/call` request and each authentication failure produces exactly one audit record. No data leaves SAG without that record. |
| ACs | AC20, AC21, AC22, AC23, AC24, AC25, AC26, AC27 |
| PRD sections | 10.1, 10.2, 10.4, 10.5, 14.3 |
| Gate | `T4.5`: `make e2e-matrix LAYER=4 PGS="14 17 19"`, review in `docs/reviews/layer-4.md` (README section 7) |
| Available parallelism | 4 (`T4.1` to `T4.4`) |
| Agents at once | 3 (README section 3.1). Start order: `T4.1`, `T4.3`, `T4.4`, `T4.2` |
| Wall-clock with pool of 3 | `T4.0` 1 h + parallel phase 3.5 h + `T4.5` 3 h = 7.5 h |
| Critical path | `T4.0` (1 h) -> `T4.1` (2.5 h) -> `T4.5` (3 h) = 6.5 h |

A task is done only when its verify command passes and the task is merged (README section 6).

## File ownership

| Task | Owns |
|---|---|
| `T4.0` | `internal/pipeline/pipeline.go`, `internal/audit/record.go`, `internal/sql/conn.go` (amendments), all layer-4 stub files until their owner task takes them |
| `T4.1` | `internal/pipeline/limits.go`, `internal/pipeline/semaphore.go`, `e2e/ac20_test.go`, `e2e/ac23_test.go` |
| `T4.2` | `internal/postexec/*`, `e2e/ac21_test.go`, `e2e/ac25_test.go` |
| `T4.3` | `internal/scrub/*`, `internal/adapter/postgres/conn.go`, `internal/daemon/logging.go`, `e2e/ac22_test.go`, `e2e/ac24_test.go` |
| `T4.4` | `internal/audit/writer.go`, `internal/endpoint/auth.go`, `e2e/ac26_test.go`, `e2e/ac27_test.go` |

## T4.0 Contract

- Goal: Freeze the pipeline stage order and the stage signatures.
- Owns: see the ownership table.
- Reads: PRD sections 10, 14.3.
- Depends on: `T3.7`.
- Content:
  - `pipeline.go`: the fixed order of PRD section 14.3. Each stage is a call to a stub: `resolveLimits`, `acquire`, `Resource.Execute`, `postexec.Apply`, `scrub.Error`, `audit.Append`. On an `audit.Append` error, the pipeline returns `internal` and no data.
  - `audit/record.go`: the `Record` type with the fields of PRD section 10.5.
  - Stubs: `postexec.Apply(Result, Limits, redact []string) Result`, `scrub.New(secrets []secret.Credential) Scrubber`, `audit.Writer`, `endpoint.AuditHook`.
- Tests first: `pipeline/order_test.go`: `TestStageOrder` (fake stages record their order), `TestAuditFailureReturnsInternal`.
- ACs: contributes to AC27.
- Verify: `make contract-check && go test -count=1 ./internal/pipeline/... -run 'TestStageOrder|TestAuditFailure'`. Expected: exit 0 and `ok`.
- Done when: every later task of layer 4 can compile against the stubs.

## T4.1 Limits and semaphore

- Goal: Resolve limits from defaults, resource values, and request values with the hard maximums. Hold one non-blocking semaphore for each JWT `sub`, shared across all resources.
- Owns: `internal/pipeline/limits.go`, `internal/pipeline/semaphore.go`, `e2e/ac20_test.go`, `e2e/ac23_test.go`.
- Reads: `internal/config`, `internal/core`.
- Depends on: `T4.0`.
- Tests first: `limits_test.go`: `TestRequestAboveHardMaxInvalidArgument`, `TestIdleTimeoutIsStatementPlus5s`; `semaphore_test.go`: `TestFifthRequestRateLimited`, `TestSharedAcrossResources`, `TestReleaseOnError`.
- e2e: `AC20_MaxRows`: on `t10k`, no `max_rows` gives 100 rows and `truncated: true`; `max_rows: 1000` gives 1000 rows; `max_rows: 1001` gives `invalid_argument`. `AC23_RateLimit` (on `sag-rate` with `max_concurrent_per_agent: 4`): fixture locks hold two requests of `ac23` on resource `rate-a` and two on `rate-b`; a fifth request on either resource gives `rate_limited`.
- ACs: AC20, AC23.
- Verify: `go test -count=1 ./internal/pipeline/... -run 'TestRequestAbove|TestIdleTimeout|TestFifth|TestSharedAcross|TestRelease'`. Expected: `ok`.
- Done when: the semaphore runs before `Resource.Execute`.

## T4.2 Post-execute

- Goal: Apply the cell cap, the column redaction, and the byte cap after JSON encoding.
- Owns: `internal/postexec/*`, `e2e/ac21_test.go`, `e2e/ac25_test.go`.
- Reads: `internal/core`.
- Depends on: `T4.0`.
- Tests first: `TestCellCapSuffix`, `TestRedactGlobCaseInsensitive`, `TestDefaultPatterns`, `TestResourcePatternsAdd`, `TestByteCapAfterEncoding`.
- e2e: `AC21_MaxResultBytes`: a result larger than 32 KiB gives `truncated_reason: max_result_bytes`. `AC25_ColumnRedaction`: column `api_token` shows `[REDACTED]`.
- ACs: AC21, AC25.
- Verify: `go test -count=1 ./internal/postexec/...`. Expected: `ok`.
- Done when: the byte cap measures the encoded JSON.

## T4.3 Errors and scrub

- Goal: Map SQLSTATE `57014` and `55P03` to `timeout`. Map an exhausted pool at the deadline to `timeout`. Map a connect error to `resource_unavailable` with a fixed message. Scrub secret values from errors and daemon logs.
- Owns: `internal/scrub/*`, `internal/adapter/postgres/conn.go`, `internal/daemon/logging.go`, `e2e/ac22_test.go`, `e2e/ac24_test.go`.
- Reads: `internal/secret`, `internal/sql/conn.go`.
- Depends on: `T4.0`.
- Tests first: `scrub_test.go`: `TestRemovesSecretAndPassword`, `TestNoDSNInError`; `conn_integration_test.go`: `TestLockTimeoutClass`, `TestStatementTimeoutClass`, `TestPoolExhaustedTimeout`, `TestConnectErrorFixedMessage`; `daemon/logging_test.go`: `TestLogHandlerScrubs`.
- e2e: `AC22_LockTimeout`: a fixture session holds `ACCESS EXCLUSIVE` on `t_lock22`; `SELECT * FROM t_lock22` gives `timeout` in less than 3 s. `AC24_ConnectErrorScrub`: resource `badpass` gives `resource_unavailable` with the fixed message; the response, `podman logs sag`, and the audit copy do not contain the password.
- ACs: AC22, AC24.
- Verify: `make test-integration PG=17 PKG='./internal/scrub/... ./internal/adapter/postgres/... ./internal/daemon/...' RUN='TestRemoves|TestNoDSN|TestLockTimeout|TestStatementTimeout|TestPoolExhausted|TestConnectError|TestLogHandler'`. Expected: `ok` for each package.
- Done when: no error path returns a driver error text to the agent.

## T4.4 Audit

- Goal: Append one audit line for each `tools/call` request and for each authentication failure. Write nothing for `initialize` and `tools/list`.
- Owns: `internal/audit/writer.go`, `internal/endpoint/auth.go`, `e2e/ac26_test.go`, `e2e/ac27_test.go`.
- Reads: `internal/audit/record.go`, `internal/pipeline/pipeline.go`.
- Depends on: `T4.0`.
- Tests first: `audit/writer_test.go`: `TestDailyFileMode0600`, `TestAllFields`, `TestNoJWTNoValues`, `TestAppendError`; `endpoint/auth_test.go`: `TestAuthFailureAudited`, `TestInitializeNotAudited`.
- e2e: `AC26_AuditRecord`: each `tools/call` of `ac26` and each authentication failure has exactly one audit line with all fields of PRD section 10.5; `initialize` and `tools/list` add none; the audit copy does not contain the JWT. `AC27_AuditFailure`: `sag-ro-audit` has a read-only bind mount at `/var/lib/sag/audit`; a valid query gives `internal` and no rows.
- ACs: AC26, AC27.
- Verify: `go test -count=1 ./internal/audit/... ./internal/endpoint/...`. Expected: `ok` for both packages.
- Done when: the audit writer opens files with `O_APPEND` and mode 0600.

## T4.5 Integration and gate

- Goal: Prove AC20 to AC27 end to end with no regression of AC1 to AC19, and close the layer 4 gate.
- Owns: every layer-4 file after all other layer-4 tasks merge. `docs/reviews/layer-4.md`.
- Depends on: `T4.1` to `T4.4`.
- Steps: README section 7.
- ACs: AC20 to AC27.
- Verify: `make e2e-matrix LAYER=4 PGS="14 17 19"`. Expected: exit 0. AC1 to AC27 `PASS`. AC28 to AC35 `NOT_IMPLEMENTED`.
- Done when: README section 7 step 11 is complete.

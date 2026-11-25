# Layer 4 review

- Reviewer: Aelita. Owner: Ulrich.
- Scope: layer 4. Limits and per-agent semaphore, post-execute (cell cap, redaction, byte cap), error mapping and secret scrub (errors and daemon log), audit log, e2e AC20 to AC27, harness changes, T4.5 integration.
- Code: `.dev/integration`, branch `feat/v1-postgres`, commit `af8f112`. Diff: `sag-layer-3..af8f112`.
- Spec: `docs/PRD.md` sections 6, 7, 10, 13.4, 14.3, 15.2. `docs/plan/layer-4.md`. `.dev/local/layer-4-contract.md`. `.dev/local/layer-4-ac-report.md`.
- Verdict: NOT READY for the layer-4 tag. 3 blockers are open (B1 to B3). 7 suggestions have `action:YES`.
- Method: Aelita read each layer-4 source file, the e2e files of AC20 to AC27, and the harness diff. Aelita ran probes and mutations in a copy outside the repository. Aelita changed no repository file and made no commit.
- Baseline at `af8f112`: `go vet ./...`, `go vet -tags e2e ./e2e/...`, `go vet -tags integration ./...`, and `go test -count=1 ./...` pass. `go test -race` passes for the 8 layer-4 packages. `gofmt -l` is empty.
- Marks: `[tested]` = Aelita reproduced the behavior on 2026-11-25. `[read]` = Aelita found it in the source text only.
- Probe targets: (1) an in-process stack of `endpoint`, `mcpserver`, and `pipeline` with a fake resource and raw HTTP requests. (2) The pipeline with the product dialect and the product scrubber. (3) The product executor on PostgreSQL 17 from `internal/testenv`. (4) The daemon in one probe image on PostgreSQL 17 through the e2e harness.
- Probe image: Aelita built one image, `localhost/sag:l4review`, from a copy of `af8f112`. The copy had mutation branches that stay off without the variable `SAG_MUTANT`. Without the variable, AC20 to AC27 pass on PostgreSQL 17.
- Limits: Aelita did not run the matrix on PostgreSQL 14 and 19. Aelita did not run AC01 to AC19 again; the AC report shows PASS for them. Aelita did not read `docs/plan/testing.md` and did not read each unit test line by line. 42 unit mutations and 4 e2e mutation runs replaced that read.
- Note on the AC report: the report image `sha256:0bd90d08…` is not on the machine, so Aelita did not examine it. See S12.
- Cleanup: Aelita removed `localhost/sag:l4review`. The harness and `testenv` removed their containers, volumes, and network. `podman ps -a` shows no `sag-e2e` object. Aelita touched no object of another project. The worktree is clean. The probe files are in `<scratchpad>`.

## Checks without a finding

- e2e mutations [tested]: on PostgreSQL 17, each of these 7 mutants fails its AC. No hard maximum check (AC20). No byte cap (AC21). `55P03` not mapped to `timeout` (AC22). Semaphore key `sub` plus resource (AC23). Case-sensitive column name (AC25). No record for an unknown tool (AC26). Data returned on append failure (AC27).
- AC24 [tested]: with an injected driver error that holds the password, AC24 passes with the scrubber and fails with a no-op scrubber. The limit of AC24 is finding B3.
- Unit mutations [tested]: of 42 mutations, the unit tier fails for 36. 2 more (no audit writer, no audit hook in `run.go`) change the record count that AC26 asserts [read]. 1 is equivalent: the idle timeout cap at 35 s is not reachable. The last 3 are finding B3.
- Request limits [tested]: 30 argument cases over HTTP. `max_rows` 1000, `max_result_bytes` 2097152, and `timeout_seconds` 30 pass. 1001, 2097153, 31, 0, -1, `1e3`, `1000.0`, `"1000"`, `null`, `true`, 2^63, and 2^64+5 give `invalid_argument`. `SQL`, `max_cell_bytes`, and a nested `limits` object give `invalid_argument`. A NUL byte in a value or in a key gives `invalid_argument`. For a duplicate key, the limit check and the adapter both use the last value.
- Semaphore [tested]: the key is `sub`. With a limit of 2, the third request gets `rate_limited`. The slot returns after an error and after 5 panics. `list_resources` does not take a slot.
- Audit count [tested]: exactly 1 record for allow, deny, `not_found`, arguments that do not decode, an unknown tool, an empty tool name, a panic, and `list_resources`. 0 records for `tools/list`. 1 record for each 401: no bearer, 2 `Authorization` headers, a false JWT, `POST /healthz`, and a path other than `/mcp`.
- Audit failure [tested]: with a writer that fails, `sql_query`, `not_found`, `list_resources`, a panic, and bad arguments return `internal` with no data. An unknown tool returns the SDK error with no data.
- Audit file [tested]: 2,000 concurrent appends of 16 KB give 2,000 valid lines. In the daemon container, the directory has mode 0700 and the file has mode 0600, owner 65532. `json.Marshal` escapes a newline, U+2028, a NUL byte, and invalid UTF-8, so one record stays on one line.
- Error mapping [read]: `engineError` keeps only the SQLSTATE and the class (`internal/adapter/postgres/conn.go:225-238`). Each agent message is a constant, except the rejection text of B1. An error that is not a `*core.Error` gives `internal` (`internal/mcpserver/sql_tools.go:140-145`).
- Byte cap [tested]: 20,000 random results. `Result.Bytes` equals the size of the 5-key JSON object. The result is never above the cap when the header fits. The kept rows are the largest prefix that fits.
- Cell cap [tested]: the cut is at a rune boundary, and invalid UTF-8 causes no panic. On the daemon, `jsonb`, arrays, and `numeric` arrive as text, so the cap applies. `NaN` arrives as the string `NaN`.
- Redaction [tested]: the match ignores case in the pattern and in the name. It applies to an alias and to `SELECT *`. `sag config check` refuses a malformed glob. An alias or a transform of the value passes; PRD 10.4 states this.
- Secrets in the log [read]: after `secret.Load`, each daemon log call uses the scrub handler (`run.go:81`, `resources.go:66`, `:89`, `preflight/scheduler.go:163-173`, `pipeline.go:180`, `endpoint/server.go:115`). The SDK logger discards its lines. `token.Verify` returns a cause from a fixed list. No code path logs the `Authorization` header.
- Deep nesting [tested]: arguments with 9,000 to 30,000 array levels give HTTP 400 from the SDK depth limit of 1,000. The process stays up.
- Earlier tests [read]: the diff of the layer 1 to 3 test files adds assertions and removes none.
- Code comments [tested]: a search of the added lines of the diff finds none.

## Findings

- B1 - status:FIXED - action:YES - The scrubber confirms a guessed secret to an agent - [tested] A rejected statement returns its rejected name to the agent (`internal/adapter/postgres/dialect.go:140`, `rejection.go:30-39`). The pipeline scrubs that message (`internal/pipeline/pipeline.go:126`, `internal/scrub/scrub.go:70`). The scrubber holds each credential of each resource (`internal/daemon/run.go:80`). For `SELECT "<guess>".f()` the agent receives `statement rejected: function_schema:<guess>.f`. When the guess equals a loaded password, the agent receives `function_schema:[REDACTED].f`. Aelita reproduced this in the pipeline with the product dialect and on the daemon with PostgreSQL 17. On the daemon, an agent of resource `rotated` confirmed the password of resource `badpass`. One statement of 16,009 bytes holds 1,555 names (`funcrules.go:203`), and the position of `[REDACTED]` names the match. In the pipeline, 200 such requests took 0.5 s on one thread and used no database connection. After the correct guess, the audit record holds the password in `sql` and `reason` (`pipeline.go:161`, `:171`). A password that is part of a constant changes that constant: the password `resource` gives `[REDACTED] unavailable`. - fix:Return a constant message for each rejection: the rule or the `Node.Field` text without the rejected name. Keep the name in the audit `reason`. Send `sql` and `reason` through the scrubber before the append. Add a unit test: a statement with a loaded secret value and a statement with a different value of equal length give equal responses. - why:Asset A1 has the highest priority. Actor T1 reads error text. The oracle permits a dictionary attack on each database password with 1,555 guesses for each request and no PostgreSQL log line. PRD 10.5 forbids credentials in the audit log.

- B2 - status:FIXED - action:YES - One `sql_query` request ends the daemon through memory exhaustion - [tested] `readRows` keeps each row with its full cell text until the statement ends (`internal/sql/executor.go:135-162`, `internal/adapter/postgres/conn.go:161-173`, `:221`). The cell cap and the byte cap run after `Resource.Execute` returns (`internal/pipeline/pipeline.go:116-120`, `internal/postexec/postexec.go:26-41`). On PostgreSQL 17 with the product dialect and the default limits, `SELECT repeat('x', 20000000) AS v FROM generate_series(1, 20)` held 381 MiB before the caps ran. The post-execute result was 28,923 bytes. On the daemon in a container with `--memory 384m`, one request with `SELECT repeat('x', 50000000) AS v FROM generate_series(1, 100)` ended the process in about 4.4 s: `status=exited exit=137 oom=true`. Without a memory limit, the daemon takes host memory up to `max_rows` times the row size. One agent can send 20 such requests at the same time. - fix:In `readRows`, cut each text cell at `max_cell_bytes` when the row arrives. Keep a count of the encoded size and stop at `max_result_bytes` the same way as the `max_rows` stop. On each connection, call `PgConn().Frontend().SetMaxBodyLen` with a fixed value, for example 16 MiB, so one row has a limit. Add an integration test that asserts a memory bound. - why:PRD 10.1 states that the core stops at the cap. A 62-byte request stops the service for each agent, and a restart does not prevent the next request. Layer 3 treated a one-request daemon stop as blocker B2.

- B3 - status:FIXED - action:YES - No test fails when the daemon does not connect the scrubber - [tested] In a copy, Aelita removed the `scrubHandler` wrap (`internal/daemon/run.go:81`). `go test ./internal/daemon/` passes. Aelita removed `Scrubber: scrubber` from the pipeline options (`run.go:130`). The unit tests of `daemon`, `pipeline`, and `mcpserver` pass. Aelita gave `openResources` a logger without the scrub handler (`run.go:115`). `go test ./internal/daemon/` passes. With a no-op `Scrubber.String` in the daemon image, AC20 to AC27 pass on PostgreSQL 17. The driver text of today holds no password, so AC24 searches text that never held the secret (`e2e/ac24_test.go:78-85`, `:151`, `:231`). `TestLogHandlerScrubs`, `TestNoDSNInError`, and `TestFatalErrorIsScrubbed` test the parts and the fatal path, not the log and pipeline connection in `Run`. - fix:Add a daemon test with a fake adapter. Its `Open` error and its `resource_unavailable` cause hold the DSN password. Start `daemon.Run`, send one request, and assert that the log holds `[REDACTED]` and that the log and the response do not hold the password. - why:The secret scrub is the layer-4 control for asset A1. A later change can remove it without a red test. Layer 3 treated the same gap as blocker B4.

- S1 - status:FIXED - action:YES - A `tools/call` request with malformed parameters writes no audit record - [tested] With a valid JWT, a `tools/call` request with `params: null`, `params: []`, or `params: {"name": 5}` returns a JSON-RPC error and writes no record. A request without `params`, without `id`, or in a batch returns HTTP 400 and writes no record. The SDK rejects these requests before the receiving middleware (`internal/mcpserver/server.go:69-80`). No tool runs and no data returns. - fix:Add this exception to PRD 10.5: a request that the JSON-RPC layer rejects before dispatch writes no record. Add a unit test that pins the 6 cases. - why:PRD 10.5 and AC26 state one record for each `tools/call` request. Goal G4 holds, because no data leaves. The PRD text and the behavior must agree before the tag.

- S2 - status:FIXED - action:YES - An audit append failure on `tools/call` writes no daemon log line - [tested] With a writer that fails, `Pipeline.Execute` and `Pipeline.Listed` return `internal`, and the daemon log stays empty (`internal/pipeline/pipeline.go:129-131`, `:138-143`). `logCause` logs only `resource_unavailable` (`:177-182`). The authentication hook does log `audit append failed` (`internal/daemon/audit.go:17-19`). - fix:In `Pipeline.append`, log `audit append failed` with `request_id` and the error text at level error. - why:With a full or read-only audit volume, each agent gets `internal`. The operator needs the cause in the log.

- S3 - status:FIXED - action:YES - One agent can fill the audit volume, and then each request fails - [tested] The record holds `resource` and the tool name without a length limit (`internal/pipeline/pipeline.go:152-154`, `internal/mcpserver/server.go:74`, `sql_tools.go:95`). `json.Marshal` writes `<` as 6 bytes. Over HTTP with an in-memory writer, 3,000 `not_found` requests of 60 KB from one agent gave 1,031 MiB of records in 2.5 s. `authorize` fails before the semaphore, so no request was `rate_limited`. A full volume gives `internal` for each `tools/call` of each agent. - fix:Record at most the first 63 bytes of `resource` and of `operation`. Add a residual risk to PRD 7.6: audit growth has no limit, and the operator must monitor the free space of the state volume. - why:The resource name pattern permits 63 bytes at most. The bound removes most of the growth for one small change. A rate limit is outside v1.

- S4 - status:FIXED - action:YES - A failed write leaves a partial line, and the next record joins it - [tested] Aelita put a partial line without a newline at the end of the daily file. The next `Append` wrote its record on the same line (`internal/audit/writer.go:36-41`). A line reader then loses the first record after the failure. - fix:Before the write, read the file size with `Seek`. On a write error, call `Truncate` with that size. - why:The first record after a disk-full event has high value for the operator. The change is about 6 lines.

- S5 - status:FIXED - action:YES - A column name with `/` passes the redaction glob - [tested] `path.Match` does not let `*` match `/` (`internal/postexec/postexec.go:60`). On the daemon with PostgreSQL 17, a table column with the name `user/password` returned its value. In the unit probe, `a/b/api_token` and `password/x` returned their values with the default patterns. - fix:At config load, compile each pattern to a regular expression in which `*` matches each character. Match the lowercase name with it. Add the 3 names to `TestRedactGlobCaseInsensitive`. - why:PRD 10.4 states a glob on the output name. A real column with `/` in its name shows its value without an agent action.

- S6 - status:FIXED - action:NO - The column header can exceed `max_result_bytes` - [tested] `capBytes` keeps the header when the header alone is above the cap (`internal/postexec/postexec.go:76-90`). On the daemon with PostgreSQL 17, `SELECT *` on a table with 1,500 columns returned a result text of 133,651 bytes. This occurred with the default cap of 32,768 and with `max_result_bytes: 1`. The result had 0 rows and `truncated_reason: max_result_bytes`. - fix:Change no code. Add to PRD 10.1: the column list is outside the row cut, and its size is at most 1,664 columns of 63 bytes plus the type names. - why:The excess has a fixed upper limit of about 170 KB, below the hard maximum of 2 MiB. An error result needs a change of the `postexec.Apply` signature of D16.

- S7 - status:FIXED - action:YES - PRD 10.1 names a hard maximum for each resource, and the code has one global table - [tested] A resource with `max_rows: 100` and a timeout of 15 s accepted `max_rows: 1000`, `max_result_bytes: 2097152`, and `timeout_seconds: 30` from the request (`internal/pipeline/limits.go:42-53`, `internal/config/limits.go:21-30`). `sag.yaml` has no key for a hard maximum (`docs/PRD.md:443-444`, section 13.4). Decision E7 accepts the global table. - fix:Change `docs/PRD.md:443-444`: `sag.yaml` changes the default for each resource, and a request can set the 3 keys up to the hard maximum of the table. - why:An operator who sets `statement_timeout: 5s` for a primary server reads the PRD as a ceiling. An agent can raise the value to 30 s.

- S8 - status:OPEN - action:NO - A client disconnect does not cancel the request - [tested] In the in-process probe, the handler context was not cancelled 4 s after the client closed the connection. The semaphore slot stayed in use, and the next request got `rate_limited`. `mcpserver.Handler` does not set `PropagateRequestCancellation` (`internal/mcpserver/server.go:53-58`). The SDK applies that option only to protocol 2026-07-28 and later. - fix:Change no code in layer 4. In layer 5, set `PropagateRequestCancellation: true` and test it with the client of AC31. - why:The statement ends at `statement_timeout` plus 2 s at the latest, so one slot stays in use for 32 s at most. PRD R7 covers the database load.

- S9 - status:FIXED - action:YES - AC27 passes when the query does not run - [read] AC27 has no wait for the served state (`e2e/ac27_test.go:45-64`). Before preflight ends, the request gets `resource_unavailable`, the append fails, and the result is `internal` too. [tested] In the probe run the resource was served: the mutant that returns data on append failure failed AC27. - fix:After the calls, search the PostgreSQL log for `SELECT 27 AS n`. Fail the test when the text is absent. - why:PRD AC27 names a valid query. The test must prove that the statement ran and that no row returned.

- S10 - status:FIXED - action:YES - The audit writer keeps wrong modes, follows a symbolic link, and does not call `fsync` - [tested] `NewFileWriter` keeps a directory that exists with mode 0755 (`internal/audit/writer.go:20`). `Append` keeps a file that exists with mode 0644 and writes through a symbolic link (`:36`). [read] `Append` does not call `Sync`, so a host crash can lose a record of a result that the agent already has. - fix:Change no code. Optional: add `O_NOFOLLOW` to `OpenFile`. - why:PRD 7.1 gives the state volume to the operator only. The image creates the directory with mode 0700. `fsync` under the mutex adds latency to each request.

- S11 - status:OPEN - action:NO - The SDK bearer check can return 401 without an audit record at the expiry second - [read] `token.Verify` and `auth.RequireBearerToken` read the clock at different times (`internal/endpoint/auth.go:31`, `:61`). A JWT that expires between the 2 reads gets 401 from the SDK, and `refuse` does not run. - fix:Change no code. - why:The window is some microseconds at one second of the JWT life. No data returns.

- S12 - status:FIXED - action:YES - The AC report does not bind the image to the commit, and the default e2e tag is stale - [tested] The report reads the commit from `git rev-parse HEAD` and the digest from the image tag (`e2e/cmd/acreport/main.go:156-158`). The report image `sha256:0bd90d08…` is not on the machine. `localhost/sag:e2e` has the creation time 12:23 UTC and prints the version `sag-layer-3-5-g116b0a0`. - fix:Add the output of `sag version` from the image to the report. Rebuild or remove the 2 `:e2e` tags after the fixes. - why:A direct `go test -tags e2e` run without `make image` tests old code under the default tag. `make e2e` builds the image first, so the gate itself is not affected.

## Resolution

- `T4.6` (`b141c68`): B1, B3, S1, S2, S3, S4, S10 (`O_NOFOLLOW`).
- `T4.7` (`bc9f88e`): B2, S5, S6 (PRD 10.1 text), S7, S9, S12.
- `T4.8` (`40fdc62`): the agent message keeps a detail only from a closed set (node type, depth limit, statement count, `Node.Field`). Names that the agent writes go to the audit `reason` only.
- S8 moves to layer 5. S11 needs no change.

## Final AC report

- Gate run: `make e2e-matrix LAYER=4 PGS="14 17 19"` on commit `9c5f93e` with a clean tree. The run included `make test-integration` for PostgreSQL 14, 17, and 19. `acreport` exited 0.
- Image version: `sag sag-layer-3-23-g9c5f93e`.
- The tag commit adds only this review to `9c5f93e`.

| AC | Layer | Test | Status | PG14 | PG17 | PG19 | Duration | Message |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| AC01 | 1 | `TestE2E/parallel/AC01_ConfigCheckRejects` | PASS | PASS | PASS | PASS | 6940 ms |  |
| AC02 | 1 | `TestE2E/parallel/AC02_StartRefusesBadSecrets` | PASS | PASS | PASS | PASS | 2590 ms |  |
| AC03 | 1 | `TestE2E/parallel/AC03_JWTRejects` | PASS | PASS | PASS | PASS | 1970 ms |  |
| AC04 | 1 | `TestE2E/parallel/AC04_JWTGenerateRefuses` | PASS | PASS | PASS | PASS | 3740 ms |  |
| AC05 | 2 | `TestE2E/parallel/AC05_C2BlocksWrites` | PASS | PASS | PASS | PASS | 1320 ms |  |
| AC06 | 2 | `TestE2E/parallel/AC06_SingleStatement` | PASS | PASS | PASS | PASS | 660 ms |  |
| AC07 | 2 | `TestE2E/parallel/AC07_NoPassThrough` | PASS | PASS | PASS | PASS | 5490 ms |  |
| AC08 | 2 | `TestE2E/sequential/AC08_RollbackOnly` | PASS | PASS | PASS | PASS | 270 ms |  |
| AC09 | 2 | `TestE2E/parallel/AC09_RefusesPG13` | PASS | PASS | PASS | PASS | 3010 ms |  |
| AC10 | 2 | `TestE2E/parallel/AC10_TLSVerifyFull` | PASS | PASS | PASS | PASS | 2860 ms |  |
| AC11 | 2 | `TestE2E/parallel/AC11_ExtensionApproval` | PASS | PASS | PASS | PASS | 68970 ms |  |
| AC12 | 2 | `TestE2E/parallel/AC12_OperatorCollision` | PASS | PASS | PASS | PASS | 2380 ms |  |
| AC13 | 3 | `TestE2E/parallel/AC13_StaticRejects` | PASS | PASS | PASS | PASS | 3260 ms |  |
| AC14 | 3 | `TestE2E/parallel/AC14_FieldAllowlist` | PASS | PASS | PASS | PASS | 3180 ms |  |
| AC15 | 3 | `TestE2E/parallel/AC15_C1OnlyEffects` | PASS | PASS | PASS | PASS | 3090 ms |  |
| AC16 | 3 | `TestE2E/sequential/AC16_LexerSettings` | PASS | PASS | PASS | PASS | 160 ms |  |
| AC17 | 3 | `TestE2E/parallel/AC17_RenderedTextOnly` | PASS | PASS | PASS | PASS | 2290 ms |  |
| AC18 | 3 | `TestE2E/parallel/AC18_ViewWriteBlocked` | PASS | PASS | PASS | PASS | 2180 ms |  |
| AC19 | 3 | `TestE2E/sequential/AC19_Integrity` | PASS | PASS | PASS | PASS | 300 ms |  |
| AC20 | 4 | `TestE2E/parallel/AC20_MaxRows` | PASS | PASS | PASS | PASS | 1940 ms |  |
| AC21 | 4 | `TestE2E/parallel/AC21_MaxResultBytes` | PASS | PASS | PASS | PASS | 1770 ms |  |
| AC22 | 4 | `TestE2E/parallel/AC22_LockTimeout` | PASS | PASS | PASS | PASS | 4330 ms |  |
| AC23 | 4 | `TestE2E/parallel/AC23_RateLimit` | PASS | PASS | PASS | PASS | 4800 ms |  |
| AC24 | 4 | `TestE2E/parallel/AC24_ConnectErrorScrub` | PASS | PASS | PASS | PASS | 7840 ms |  |
| AC25 | 4 | `TestE2E/parallel/AC25_ColumnRedaction` | PASS | PASS | PASS | PASS | 1750 ms |  |
| AC26 | 4 | `TestE2E/sequential/AC26_AuditRecord` | PASS | PASS | PASS | PASS | 350 ms |  |
| AC27 | 4 | `TestE2E/parallel/AC27_AuditFailure` | PASS | PASS | PASS | PASS | 1960 ms |  |
| AC28 | 5 | `AC28_EndpointAuthOrigin` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC29 | 5 | `AC29_Authorization` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC30 | 5 | `AC30_ToolShapes` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC31 | 5 | `AC31_ClaudeCode` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC32 | 6 | `AC32_ContainerInspect` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC33 | 6 | `AC33_SecretMounts` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC34 | 6 | `AC34_JWTGenerateNoTTY` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |
| AC35 | 6 | `AC35_CI` | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED | NOT_IMPLEMENTED |  |  |

## Other subtests

| Test | Status | PG14 | PG17 | PG19 |
| --- | --- | --- | --- | --- |
| `TestE2E/parallel/Harness_Skeleton` | PASS | PASS | PASS | PASS |
| `TestE2E/parallel/Product_C1ServesReadsOnly` | PASS | PASS | PASS | PASS |
| `TestE2E/parallel/Product_SQLSizeLimit` | PASS | PASS | PASS | PASS |

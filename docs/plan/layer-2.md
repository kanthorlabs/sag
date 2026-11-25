# Layer 2: SQL executor with C2 and C3, PostgreSQL contract, preflight, approval

| Item | Value |
|---|---|
| Goal | `sql_query` runs through the SQL executor with the full PostgreSQL contract. Preflight and extension approval decide which resources SAG serves. The `sag-conformance` image proves C2 and C3 alone. In the product image, `ValidateStatic` rejects every statement until layer 3. |
| ACs | AC5, AC6, AC7, AC8, AC9, AC10, AC11, AC12 |
| PRD sections | 8.1, 8.2, 8.4 (except function rules), 9, 10.1 value encoding, 12.2, 14.2, 14.3 |
| Gate | `T2.8`: `make e2e-matrix LAYER=2 PGS="14 17 19"`, review in `docs/reviews/layer-2.md` (README section 7) |
| Available parallelism | 7 (`T2.1` to `T2.7`) |
| Agents at once | 3 (README section 3.1). Start order: `T2.2`, `T2.4`, `T2.1`, `T2.3`, `T2.6`, `T2.5`, `T2.7` |
| Wall-clock with pool of 3 | `T2.0` 1 h + parallel phase 6 h + `T2.8` 3 h = 10 h |
| Critical path | `T2.0` (1 h) -> `T2.2` (3 h) -> `T2.3` verify (1 h after `T2.2`) -> `T2.8` (3 h) = 8 h |

A task is done only when its verify command passes and the task is merged (README section 6).

## File ownership

| Task | Owns |
|---|---|
| `T2.0` | `go.mod`, `go.sum`, `Makefile`, `internal/sql/dialect.go`, `internal/sql/conn.go`, `internal/core/*` (amendments), `internal/daemon/options.go`, `e2e/harness/api.go` (amendments), all layer-2 stub files until their owner task takes them |
| `T2.1` | `internal/sql/executor.go`, `values.go`, `operations.go`, `e2e/ac08_test.go` |
| `T2.2` | `internal/adapter/postgres/dialect.go`, `session.go`, `conn.go`, `rowcap.go`, `parse.go`, `static.go`, `connconfig.go` |
| `T2.3` | `internal/adapter/postgres/preflight.go`, `tls.go`, `operators.go`, `internal/cli/resource_test_cmd.go`, `e2e/ac09_test.go`, `e2e/ac10_test.go`, `e2e/ac12_test.go` |
| `T2.4` | `internal/approval/*`, `internal/cli/extension.go`, `e2e/ac11_test.go` |
| `T2.5` | `internal/preflight/*`, `internal/inventory/*` |
| `T2.6` | `internal/pipeline/*`, `internal/mcpserver/sql_tools.go`, `internal/daemon/run.go` |
| `T2.7` | `e2e/conformance/*`, `cmd/sag/guard_test.go`, `e2e/ac05_test.go`, `e2e/ac06_test.go`, `e2e/ac07_test.go` |

## T2.0 Contract

- Goal: Freeze the SQL interfaces and the layer-2 signatures, so that `T2.1` to `T2.7` run in parallel.
- Owns: see the ownership table.
- Reads: PRD sections 8.2, 8.4, 9, 14.2, 14.3; `foundation.md` section 4.
- Depends on: `T1.10`.
- Content:
  - `go.mod`: add `github.com/pganalyze/pg_query_go/v6@v6.2.5`.
  - `internal/sql/dialect.go`: `Dialect` and `RowCapPlan` exactly as PRD section 14.2, `EngineGuard`, `Tree`, `CatalogReader`.
  - `internal/sql/conn.go`: `Pool`, `Conn` (`Exec`, `Query`, `ExecParams`, `Close`), `Rows`, `EngineError{SQLState, Class}`, `Class` values `ReadOnlyViolation`, `Timeout`, `Connect`, `Other`.
  - Stubs: `sql.NewExecutor(d Dialect, p Pool) core.Resource`, the `sql` capability operations, postgres intra-package functions (`requestBegin`, `sessionGuard`, `planRowCap`, `parse`, `render`, `validateStatic`, `checkTLS`, `riskyObjects`, `operatorCollisions`), `approval.Fingerprint`, `approval.Store`, `preflight.Scheduler`, `preflight.Clock`, `pipeline.New`.
  - `internal/daemon/options.go`: `Options{WrapDialect func(sql.Dialect) sql.Dialect}`.
  - `Makefile`: target `image-conformance`; `e2e` builds both images.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: every later task of layer 2 can compile against the stubs.

## T2.1 SQL executor

- Goal: Implement `Execute` for the `sql` capability in the PRD section 14.3 order. Own `ROLLBACK`. Encode values per PRD section 10.1.
- Owns: `internal/sql/executor.go`, `values.go`, `operations.go`, `e2e/ac08_test.go`.
- Reads: `internal/sql/dialect.go`, `conn.go`, `internal/core`.
- Depends on: `T2.0`.
- Tests first (fake `Conn` and fake `Dialect`): `TestStaticValidationBeforeConnection`, `TestRollbackAlways`, `TestNeverCommit`, `TestCloseOnRollbackFailure`, `TestCloseOnError`, `TestFetchEmptyReadsStatementRows`, `TestStopsAtMaxRowsPlusOne`, `TestValueEncoding`, `TestListTablesQuotesLiteral`, `TestIdentifierTooLong`.
- e2e: `AC08_RollbackOnly` (phase 2): the PostgreSQL log of the run has no `COMMIT`. The count of `ROLLBACK` equals the count of `BEGIN TRANSACTION READ ONLY`.
- ACs: AC8.
- Verify: `go test -count=1 ./internal/sql/...`. Expected: `ok`.
- Done when: no code path in `internal/sql` sends `COMMIT`.

## T2.2 PostgreSQL C2 path

- Goal: Implement `SessionGuard`, `RequestBegin`, the pgx `Pool` and `Conn` with error classes, `Parse` and `Render` with `pg_query`, and `PlanRowCap`. `ValidateStatic` rejects every statement until layer 3.
- Owns: `internal/adapter/postgres/dialect.go`, `session.go`, `conn.go`, `rowcap.go`, `parse.go`, `static.go`, `connconfig.go`.
- Reads: `internal/sql`, `internal/testenv`.
- Depends on: `T2.0`.
- Tests first: `session_test.go`: `TestRequestBeginText` (5 `SET LOCAL` values); `static_test.go`: `TestFailClosedRejectsAll`; `rowcap_test.go`: `TestSelectPlanUsesCursor`, `TestExplainPlanNoFetch`; `conn_integration_test.go`: `TestSessionGuardReadOnly`, `Test25006IsReadOnlyViolation`, `TestExtendedProtocolOneStatement`, `TestAfterConnectRunsSessionGuard`.
- ACs: contributes to AC5, AC6, AC8.
- Verify: `make test-integration PG=17 PKG=./internal/adapter/postgres/... RUN='TestRequestBegin|TestFailClosed|TestSelectPlan|TestExplainPlan|TestSessionGuard|Test25006|TestExtendedProtocol|TestAfterConnect'`. Expected: `ok`.
- Done when: a write through the pass-through path returns `EngineError{Class: ReadOnlyViolation}`.

## T2.3 PostgreSQL preflight

- Goal: Implement every check of PRD section 9.1, the TLS modes of PRD section 12.2, the operator name report, and `sag resource test`.
- Owns: `internal/adapter/postgres/preflight.go`, `tls.go`, `operators.go`, `internal/cli/resource_test_cmd.go`, `e2e/ac09_test.go`, `e2e/ac10_test.go`, `e2e/ac12_test.go`.
- Reads: `internal/sql`, `internal/approval` signatures, `internal/testenv`.
- Depends on: `T2.0`. Verify after `T2.2` and `T2.4`.
- Tests first: `tls_test.go`: `TestLoopbackIPLiteralExempt`, `TestLocalhostNameNotExempt`, `TestOneWarningPerPreflight`; `preflight_integration_test.go`: `TestRefusesPG13`, `TestRefusesUnknownCA`, `TestSkipVerifyWarns`, `TestRefusesDblink`, `TestApprovalAccepts`, `TestReportsCitextOperator`, `TestReadOnlyModeCheck`.
- e2e: `AC09_RefusesPG13`: `sag resource test pg13` exits non-zero, `list_resources` excludes `pg13`, `sql_query` on `pg13` returns `resource_unavailable`. `AC10_TLSVerifyFull`: resource `strict` (no `tls_ca`) is refused; resource `skip` is served; `podman logs sag` holds exactly one warning for `skip` after one preflight run. `AC12_OperatorCollision`: `sag resource test citext` reports a `citext` operator name.
- ACs: AC9, AC10, AC12.
- Verify: `make test-integration PG=17 PKG=./internal/adapter/postgres/... RUN='TestRefuses|TestSkipVerify|TestApprovalAccepts|TestReportsCitext|TestReadOnlyModeCheck|TestLoopback|TestLocalhost|TestOneWarning'`. Expected: `ok`.
- Done when: each preflight check runs inside `RequestBegin` and ends with `ROLLBACK`.

## T2.4 Extension approval

- Goal: Implement the fingerprint, the approval record with HMAC, the file store, and `sag extension approve` and `revoke` per PRD section 9.2.
- Owns: `internal/approval/*`, `internal/cli/extension.go`, `e2e/ac11_test.go`.
- Reads: `internal/keys`, `internal/cli/tty.go`.
- Depends on: `T2.0`.
- Tests first: `approval/fingerprint_test.go`: `TestFingerprintCanonical`, `TestSortedLists`; `approval/record_test.go`: `TestMACValid`, `TestMACChangeRefused`, `TestFileMode0600`; `cli/extension_test.go`: `TestApproveNeedsTerminal`, `TestApproveWrongConfirmationStops`.
- e2e: `AC11_ExtensionApproval`:
  1. `sag resource test ext` exits non-zero and prints the approval hint.
  2. The test runs `podman exec -it sag sag extension approve ext` on a pseudo-terminal and types `ext`.
  3. `sag resource test ext` passes. Within 70 s, `list_resources` lists `ext`.
  4. A record copy with one changed `mac` byte makes `sag resource test ext` fail. The test restores the record.
  5. The fixture runs `CREATE EXTENSION file_fdw` in `sag_ext`.
  6. Within 2 re-check intervals (20 s), `sql_query` on `ext` returns `resource_unavailable`.
- ACs: AC11.
- Verify: `go test -count=1 ./internal/approval/... ./internal/cli/... -run 'TestFingerprint|TestSorted|TestMAC|TestFileMode|TestApprove'`. Expected: `ok` for both packages.
- Done when: a record with a changed field or a changed fingerprint is refused.

## T2.5 Preflight schedule

- Goal: Run preflight at start, every 60 s for a failed resource, and every `preflight.recheck_interval` for a served resource. Update the resource status in the inventory.
- Owns: `internal/preflight/*`, `internal/inventory/*`.
- Reads: `internal/core`, `internal/config`.
- Depends on: `T2.0`.
- Tests first (fake clock): `TestRetryFailedAfter60s`, `TestRecheckServedAtInterval`, `TestFailedRecheckMarksUnavailable`, `TestPassMarksServed`, `TestStatusVisibleInInventory`.
- ACs: contributes to AC11.
- Verify: `go test -count=1 ./internal/preflight/... ./internal/inventory/...`. Expected: `ok` for both packages.
- Done when: the schedule uses only the injected clock.

## T2.6 Pipeline and SQL tools

- Goal: Implement the first pipeline (authorize, `Execute`) and the MCP tools `sql_list_tables`, `sql_describe_table`, `sql_query`. Wire adapters, the schedule, and `Options.WrapDialect` in the daemon.
- Owns: `internal/pipeline/*`, `internal/mcpserver/sql_tools.go`, `internal/daemon/run.go`.
- Reads: `internal/sql`, `internal/inventory`, `internal/preflight`, `internal/daemon/options.go`.
- Depends on: `T2.0`. Verify after `T2.1`.
- Tests first: `pipeline/pipeline_test.go`: `TestNotFoundForForeignResource`, `TestUnavailableForFailedPreflight`; `mcpserver/sql_tools_test.go`: `TestToolsFromRegistry`, `TestErrorResultShape`; `daemon/run_test.go`: `TestWrapDialectNilInProduct`.
- ACs: contributes to AC5, AC6.
- Verify: `go test -count=1 ./internal/pipeline/... ./internal/mcpserver/... ./internal/daemon/...`. Expected: `ok` for each package.
- Done when: `sql_query` reaches `Resource.Execute` through MCP.

## T2.7 Conformance image and guard

- Goal: Build `localhost/sag-conformance:e2e` with the pass-through wrapper. Prove that the product binary has no pass-through code.
- Owns: `e2e/conformance/*`, `cmd/sag/guard_test.go`, `e2e/ac05_test.go`, `e2e/ac06_test.go`, `e2e/ac07_test.go`.
- Reads: `internal/daemon/options.go`, `internal/sql/dialect.go`.
- Depends on: `T2.0`. Verify after `T2.1`, `T2.2`, and `T2.6`.
- Tests first: `cmd/sag/guard_test.go`: `TestAC07_NoPassThroughDep` (`go list -deps ./cmd/sag`), `TestAC07_NoPassThroughSymbol` (`go build`, then `go tool nm`).
- e2e: `AC05_C2BlocksWrites` (on `sag-conformance`): `INSERT`, `UPDATE`, `DELETE`, `CREATE TABLE` return `upstream_error` with SQLSTATE `25006`. `AC06_SingleStatement` (on `sag-conformance`): `SELECT 1; DELETE FROM t` returns a server error, and the row count of `t` stays the same. `AC07_NoPassThrough` (no MCP call): `go list -deps ./cmd/sag` holds no `e2e/conformance/passthrough` package, and the binary copied from `localhost/sag:e2e` has no symbol of that package.
- ACs: AC5, AC6, AC7.
- Verify: `go test -count=1 ./cmd/sag/... && make image-conformance`. Expected: `ok` and a built image.
- Done when: only `e2e/conformance/cmd/sag-conformance` imports `passthrough`.

## T2.8 Integration and gate

- Goal: Prove AC5 to AC12 end to end with no regression of AC1 to AC4, and close the layer 2 gate.
- Owns: every layer-2 file after all other layer-2 tasks merge. `docs/reviews/layer-2.md`.
- Depends on: `T2.1` to `T2.7`.
- Steps: README section 7.
- ACs: AC5 to AC12.
- Verify: `make e2e-matrix LAYER=2 PGS="14 17 19"`. Expected: exit 0. AC1 to AC12 `PASS`. AC13 to AC35 `NOT_IMPLEMENTED`.
- Done when: README section 7 step 11 is complete.

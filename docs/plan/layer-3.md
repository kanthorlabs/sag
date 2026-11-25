# Layer 3: C1 validation for PostgreSQL

| Item | Value |
|---|---|
| Goal | The product image accepts read statements and rejects every statement outside the allowlist. SAG sends only deparsed text. |
| ACs | AC13, AC14, AC15, AC16, AC17, AC18, AC19 |
| PRD sections | 8.3, 8.4 (function and operator rules, row cap), 8.5, 8.6 |
| Gate | `T3.7`: `make e2e-matrix LAYER=3 PGS="14 17 19"`, review in `docs/reviews/layer-3.md` (README section 7) |
| Available parallelism | 6 (`T3.1` to `T3.6`) |
| Agents at once | 3 (README section 3.1). Start order: `T3.1`, `T3.2`, `T3.4`, `T3.5`, `T3.3`, `T3.6` |
| Wall-clock with pool of 3 | `T3.0` 1 h + parallel phase 5 h + `T3.7` 3 h = 9 h |
| Critical path | `T3.0` (1 h) -> `T3.1` (5 h) -> `T3.7` (3 h) = 9 h |

A task is done only when its verify command passes and the task is merged (README section 6).

## File ownership

All paths below are in `internal/adapter/postgres/` unless the path starts with `e2e/`.

| Task | Owns |
|---|---|
| `T3.0` | `go.mod`, `go.sum`, `tree.go`, `rejection.go`, `dialect.go`, all layer-3 stub files until their owner task takes them |
| `T3.1` | `allowlist.go`, `walk.go`, `e2e/ac14_test.go` |
| `T3.2` | `static.go`, `relations.go`, `e2e/ac13_test.go` |
| `T3.3` | `funcrules.go` |
| `T3.4` | `catalog.go`, `e2e/ac15_test.go` |
| `T3.5` | `render.go`, `rowcap.go`, `e2e/ac16_test.go`, `e2e/ac17_test.go` |
| `T3.6` | `e2e/ac18_test.go`, `e2e/ac19_test.go` |

## T3.0 Contract

- Goal: Freeze the tree type, the rejection type, and the composition of `ValidateStatic`, `Render`, and `PlanRowCap`.
- Owns: see the ownership table.
- Reads: PRD sections 8.3, 8.4, 8.6.
- Depends on: `T2.8`.
- Content:
  - `go.mod`: `google.golang.org/protobuf` becomes a direct requirement.
  - `tree.go`: `pgTree{Result *pg_query.ParseResult, Funcs, Operators, Relations []qualifiedName}`. `ValidateStatic` fills the name lists for `ValidateCatalog`.
  - `rejection.go`: `Rejection{Node, Field, Name, Rule}` with the audit `reason` form `<Node>.<field>` or `<rule>:<name>`.
  - `dialect.go`: `ValidateStatic` calls, in this order, `checkStatementShape`, `walkAllowlist`, `checkRelations`, `checkFunctionNames`. `Render` calls `deparseAndRevalidate`. `PlanRowCap` calls `planRowCap`.
  - Stubs for each function above and for `validateCatalog`.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: every later task of layer 3 can compile against the stubs.

## T3.1 Allowlist and walk

- Goal: Walk every populated field of the pg_query tree with `protoreflect`. Reject a node type or a field outside the allowlist.
- Owns: `allowlist.go`, `walk.go`, `e2e/ac14_test.go`.
- Reads: `tree.go`, `rejection.go`.
- Depends on: `T3.0`.
- Allowlist content:
  - Node types: `RawStmt`, `SelectStmt`, `ExplainStmt`, `DefElem` (under `ExplainStmt` only), `WithClause`, `CommonTableExpr`, `ResTarget`, `ColumnRef`, `A_Star`, `A_Const`, `String`, `Integer`, `Float`, `Boolean`, `A_Expr`, `BoolExpr`, `NullTest`, `BooleanTest`, `CaseExpr`, `CaseWhen`, `CoalesceExpr`, `MinMaxExpr`, `NullIfExpr`, `SQLValueFunction`, `FuncCall`, `TypeCast`, `TypeName`, `CollateClause`, `RangeVar`, `RangeSubselect`, `RangeFunction`, `JoinExpr`, `Alias`, `SubLink`, `SortBy`, `WindowDef`, `RowExpr`, `A_ArrayExpr`, `A_Indirection`, `A_Indices`, `GroupingSet`, `List`.
  - Fields: list each permitted field per type. Include the fields that the parser always sets, for example `location` and `RangeVar.relpersistence`.
  - Exclude `SelectStmt.into_clause` and `SelectStmt.locking_clause`.
- Tests first: `walk_test.go`: `TestAC14_FieldOutsideAllowlist` (hand-built tree with one extra field), `TestUnknownNodeRejects`, `TestVisitsSubqueriesCTEsSetOpsLateral`, `TestReadCorpusAccepted` (a corpus of 40 read statements); `allowlist_test.go`: `TestEveryAllowedTypeExists` (each name resolves in the pg_query descriptor).
- e2e: `AC14_FieldAllowlist`: `SELECT * FROM t FOR UPDATE` returns `query_rejected`, and the audit `reason` names `SelectStmt.locking_clause`.
- ACs: AC14; contributes to AC13.
- Verify: `go test -count=1 ./internal/adapter/postgres/... -run 'TestAC14|TestUnknownNode|TestVisits|TestReadCorpus|TestEveryAllowed'`. Expected: `ok`.
- Done when: the read corpus passes and each walk test passes.

## T3.2 Statement rules and credential catalogs

- Goal: Allow one statement only, of shape `SELECT`, `WITH` of `SELECT` parts, or `EXPLAIN` of an allowed `SELECT` with options `format`, `verbose`, `costs`. Reject writable CTEs, `INTO`, row locks, `ParamRef`. Reject the relations of PRD section 8.6 at any depth.
- Owns: `static.go`, `relations.go`, `e2e/ac13_test.go`.
- Reads: `tree.go`, `rejection.go`.
- Depends on: `T3.0`.
- Tests first: `static_test.go`: `TestAC13_StaticRejects` (24 statements of AC13, no connection), `TestExplainOptions`, `TestWritableCTE`; `relations_test.go`: `TestCredentialCatalogsAnyDepth`, `TestCredentialCatalogsAnySchema`.
- e2e: `AC13_StaticRejects`: each statement returns `query_rejected`. Each statement carries a unique number. The PostgreSQL log holds none of these numbers.
- ACs: AC13; contributes to AC15.
- Verify: `go test -count=1 ./internal/adapter/postgres/... -run 'TestAC13|TestExplainOptions|TestWritableCTE|TestCredentialCatalogs'`. Expected: `ok`.
- Done when: each of the 24 statements returns a `Rejection`.

## T3.3 Static function rules

- Goal: Implement rules 3, 5, and 6 of PRD section 8.4.
- Owns: `funcrules.go`.
- Reads: `tree.go`, `rejection.go`.
- Depends on: `T3.0`.
- Tests first: `funcrules_test.go`: `TestSchemaQualifiedOutsidePgCatalog`, `TestPrefixRule`, `TestPrefixAllowlist12Names`, `TestDenylistFamilies`, `TestCurrentSettingRejected`.
- ACs: contributes to AC15.
- Verify: `go test -count=1 ./internal/adapter/postgres/... -run 'TestSchemaQualified|TestPrefix|TestDenylist|TestCurrentSetting'`. Expected: `ok`.
- Done when: each named function and family of rule 6 rejects.

## T3.4 Catalog rules and volatility gate

- Goal: Implement rules 1, 2, and 4 of PRD section 8.4 through `pg_proc`, `pg_operator`, and `pg_namespace` inside the request transaction. Names go as bind values.
- Owns: `catalog.go`, `e2e/ac15_test.go`.
- Reads: `tree.go`, `rejection.go`, `internal/sql`.
- Depends on: `T3.0`.
- Volatility rule: reject a function name if any `pg_catalog` match has `provolatile = 'v'`, except `random`, `clock_timestamp`, `gen_random_uuid`, `timeofday`.
- Tests first: `catalog_integration_test.go`: `TestVolatilityGate` (`random()` passes, `setseed(0.5)` rejects), `TestFunctionOutsidePgCatalog`, `TestOperatorOutsidePgCatalog`, `TestBrinGinRejected`.
- e2e: `AC15_C1OnlyEffects`: each row of PRD section 8.5, each relation of section 8.6, and `current_setting('primary_conninfo')` return `query_rejected`.
- ACs: AC15.
- Verify: `make test-integration PG=17 PKG=./internal/adapter/postgres/... RUN='TestVolatilityGate|TestFunctionOutside|TestOperatorOutside|TestBrinGin'`. Expected: `ok`.
- Done when: the catalog rules run on PostgreSQL 14 and 19 in the integration tier.

## T3.5 Render and row cap

- Goal: Render with `pg_query.Deparse`, parse and validate the rendered text again, and build `RowCapPlan` per PRD section 8.4.
- Owns: `render.go`, `rowcap.go`, `e2e/ac16_test.go`, `e2e/ac17_test.go`.
- Reads: `tree.go`, `rejection.go`, `dialect.go`.
- Depends on: `T3.0`.
- Tests first: `render_test.go`: `TestDeparseIndependentOfSCS` (`'a\'` renders as `E'a\\'`), `TestRenderRevalidates`, `TestRenderNormalizesWhitespace`; `rowcap_test.go`: `TestOneDeclareRequired`, `TestFetchIsMaxRowsPlusOne`, `TestExplainNoCursor`.
- e2e: `AC16_LexerSettings` (phase 2): on PostgreSQL 14 to 18, with `ALTER ROLE postgres SET standard_conforming_strings = off`, the AC16 statement returns in less than 1 s, and `pg_locks` shows no new lock. On PostgreSQL 19, the `ALTER ROLE` fails, and the statement returns in less than 1 s. The test resets the role. `AC17_RenderedTextOnly`: `select   1` appears in the log as `SELECT 1`, and the log has no `select   1`.
- ACs: AC16, AC17.
- Verify: `go test -count=1 ./internal/adapter/postgres/... -run 'TestDeparse|TestRender|TestOneDeclare|TestFetchIs|TestExplainNoCursor'`. Expected: `ok`.
- Done when: the executor receives only rendered text.

## T3.6 C2 through a view, and integrity

- Goal: Prove that C2 blocks a write in code that the AST does not show, and that the full suite changes no data.
- Owns: `e2e/ac18_test.go`, `e2e/ac19_test.go`.
- Reads: `e2e/harness/api.go`.
- Depends on: `T3.0`.
- Tests first: the e2e subtests below. Each registers its fixture: function `f_insert` and view `v` in `sag_e2e` for AC18; the snapshot query for AC19.
- e2e: `AC18_ViewWriteBlocked`: `SELECT * FROM v` returns `upstream_error` with SQLSTATE `25006`, and the row count stays the same. `AC19_Integrity` (phase 2, last): row count, checksum, and schema object list of `sag_e2e` equal the snapshot from before phase 1.
- ACs: AC18, AC19.
- Verify: `go vet -tags e2e ./e2e/...`. Expected: exit 0.
- Done when: both subtests compile and register.

## T3.7 Integration and gate

- Goal: Prove AC13 to AC19 end to end with no regression of AC1 to AC12, and close the layer 3 gate.
- Owns: every layer-3 file after all other layer-3 tasks merge. `docs/reviews/layer-3.md`.
- Depends on: `T3.1` to `T3.6`.
- Steps: README section 7.
- ACs: AC13 to AC19.
- Verify: `make e2e-matrix LAYER=3 PGS="14 17 19"`. Expected: exit 0. AC1 to AC19 `PASS`. AC20 to AC35 `NOT_IMPLEMENTED`.
- Done when: README section 7 step 11 is complete.

# Layer 5: MCP server

| Item | Value |
|---|---|
| Goal | The MCP endpoint meets PRD sections 11.3, 11.4, 13.2, and 13.3. Claude Code uses SAG. |
| ACs | AC28, AC29, AC30, AC31 |
| PRD sections | 11.3, 11.4, 13.2, 13.3 |
| Gate | `T5.4`: `make e2e-matrix LAYER=5 PGS="14 17 19"`, plus AC31 on the macOS development machine, review in `docs/reviews/layer-5.md` (README section 7) |
| Available parallelism | 3 (`T5.1` to `T5.3`) |
| Agents at once | 3 (README section 3.1). Start order: `T5.2`, `T5.1`, `T5.3` |
| Wall-clock with pool of 3 | `T5.0` 0.5 h + parallel phase 2 h + `T5.4` 2 h = 4.5 h |
| Critical path | `T5.0` (0.5 h) -> `T5.2` (2 h) -> `T5.4` (2 h) = 4.5 h |

A task is done only when its verify command passes and the task is merged (README section 6).

## File ownership

| Task | Owns |
|---|---|
| `T5.0` | `internal/mcpserver/types.go`, `internal/endpoint/server.go` (middleware chain), `e2e/harness/api.go` (amendments), all layer-5 stub files until their owner task takes them |
| `T5.1` | `internal/endpoint/origin.go`, `e2e/ac28_test.go` |
| `T5.2` | `internal/mcpserver/schemas.go`, `errors.go`, `list_resources.go`, `sql_tools.go`, `e2e/ac29_test.go`, `e2e/ac30_test.go` |
| `T5.3` | `e2e/harness/claude.go`, `e2e/ac31_test.go` |

## T5.0 Contract

- Goal: Freeze the tool schema types, the error result type, and the middleware chain order.
- Owns: see the ownership table.
- Reads: PRD sections 11.4, 13.3.
- Depends on: `T4.5`.
- Content:
  - `mcpserver/types.go`: Go types for the input and output of the 4 tools of PRD section 13.3, and `ErrorResult{Code, Message, RequestID}`.
  - `endpoint/server.go`: chain order `origin` -> body and header limits -> JWT -> go-sdk bridge -> MCP handler.
  - `harness/api.go`: `ClaudeRunner` signature.
  - Stubs: `endpoint.originCheck`, `harness.RunClaude`.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: every later task of layer 5 can compile against the stubs.

## T5.1 Origin check

- Goal: Reject a request with an `Origin` header outside `server.allowed_origins` with `403`. Accept a request without `Origin`.
- Owns: `internal/endpoint/origin.go`, `e2e/ac28_test.go`.
- Reads: `internal/config`, `internal/endpoint/server.go`.
- Depends on: `T5.0`.
- Tests first: `origin_test.go`: `TestOriginOutsideListRejected`, `TestNoOriginPasses`, `TestEmptyListRejectsAnyOrigin`, `TestListedOriginPasses`.
- e2e: `AC28_EndpointAuthOrigin`: no JWT gives `401`; `Origin: https://evil.example` gives `403`.
- ACs: AC28.
- Verify: `go test -count=1 ./internal/endpoint/... -run 'TestOrigin|TestNoOrigin|TestEmptyList|TestListedOrigin'`. Expected: `ok`.
- Done when: the origin check runs before the JWT check.

## T5.2 Tool contract

- Goal: Give each tool the annotations, the description sentence, the input and output schemas, and the error result of PRD section 13.3.
- Owns: `internal/mcpserver/schemas.go`, `errors.go`, `list_resources.go`, `sql_tools.go`, `e2e/ac29_test.go`, `e2e/ac30_test.go`.
- Reads: `internal/mcpserver/types.go`, `internal/pipeline`, `internal/inventory`.
- Depends on: `T5.0`.
- Tests first: `schemas_test.go`: `TestAnnotations`, `TestDescriptionSentence`, `TestStructuredEqualsText`, `TestOutputMatchesSchema`; `errors_test.go`: `TestErrorCodesClosedSet`, `TestNotFoundForForeignResource`.
- e2e: `AC29_Authorization`: agent `ac29a` gets `not_found` for resource `only-b`, and `list_resources` for `ac29a` excludes `only-b`. `AC30_ToolShapes`: the go-sdk client calls the 4 tools, and each `structuredContent` is validated with jsonschema-go against the output schema.
- ACs: AC29, AC30.
- Verify: `go test -count=1 ./internal/mcpserver/...`. Expected: `ok`.
- Done when: each tool result has `structuredContent` and the same JSON as text.

## T5.3 Claude Code check

- Goal: Run `claude -p` against the e2e `sag` container and find the marker value of a fixture row.
- Owns: `e2e/harness/claude.go`, `e2e/ac31_test.go`.
- Reads: `e2e/harness/api.go`.
- Depends on: `T5.0`.
- Method: write a temporary MCP config file with mode 0600 and the JWT. Run `NODE_EXTRA_CA_CERTS=<test CA> claude -p "<prompt>" --mcp-config <file> --strict-mcp-config --allowedTools mcp__sag__list_resources,mcp__sag__sql_query`. Delete the file.
- Tests first: `claude_test.go`: `TestConfigFileMode0600`, `TestConfigFileRemoved`.
- e2e: `AC31_ClaudeCode`: the subtest waits until `list_resources` for `ac31` holds `ac31`. The output holds the marker value. The audit log holds exactly one `allow` record with operation `query` for `sub` `ac31`, and no record of another tool. If `claude` is absent or not logged in, the subtest fails with the message `prerequisite missing`.
- ACs: AC31.
- Verify: `go test -count=1 ./e2e/harness/... -run 'TestConfigFile'`. Expected: `ok`.
- Done when: the subtest passes on the macOS development machine.

## T5.4 Integration and gate

- Goal: Prove AC28 to AC31 end to end with no regression of AC1 to AC27, and close the layer 5 gate.
- Owns: every layer-5 file after all other layer-5 tasks merge. `docs/reviews/layer-5.md`.
- Depends on: `T5.1` to `T5.3`.
- Steps: README section 7. Run the matrix on the macOS development machine, so AC31 runs.
- ACs: AC28 to AC31.
- Verify: `make e2e-matrix LAYER=5 PGS="14 17 19"`. Expected: exit 0. AC1 to AC31 `PASS`. AC32 to AC35 `NOT_IMPLEMENTED`.
- Done when: README section 7 step 11 is complete.

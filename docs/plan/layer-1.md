# Layer 1: Skeleton, e2e harness, config, secrets, JWT

| Item | Value |
|---|---|
| Goal | The `sag` image starts in Podman with Podman secrets. A JWT-authenticated MCP call over TLS returns `list_resources` with one PostgreSQL resource that passed a minimal preflight. The e2e harness and the AC report work. |
| ACs | AC1, AC2, AC3, AC4 |
| PRD sections | 5, 6, 10.3, 11.1, 11.2, 12.1, 13.1, 13.4, 15.2 step 1 |
| Gate | `T1.10`: `make e2e-matrix LAYER=1 PGS="14 17 19"`, review in `docs/reviews/layer-1.md` (README section 7) |
| Available parallelism | 11 (`T1.1` to `T1.9`, `X.1`, `X.2`) |
| Agents at once | 3 (README section 3.1). Start order: `T1.6`, `T1.7`, `T1.3`, `T1.4`, `T1.9`, `T1.1`, `T1.2`, `T1.5`, `T1.8`, `X.2`, `X.1` |
| Wall-clock with pool of 3 | `T1.0` 1 h + parallel phase 6.5 h + `T1.10` 3.5 h = 11 h |
| Critical path | `T1.0` (1 h) -> `T1.6` (2.5 h) -> `T1.7` verify (1 h after `T1.6`) -> `T1.10` (3.5 h) = 8 h |

A task is done only when its verify command passes and the task is merged (README section 6).

Layer 1 brings a minimal MCP endpoint and the container forward from PRD steps 5 and 6. Each AC needs a full e2e test, and that test needs both.

## File ownership

| Task | Owns |
|---|---|
| `T1.0` | `go.mod`, `go.sum`, `Makefile`, `cmd/sag/main.go`, `internal/cli/cli.go`, `internal/core/*`, `e2e/acs.go`, `e2e/harness/api.go`, all stub files until their owner task takes them |
| `T1.1` | `internal/config/*`, `internal/cli/config_check.go`, `e2e/ac01_test.go` |
| `T1.2` | `internal/secret/*`, `internal/keys/*`, `e2e/ac02_test.go` |
| `T1.3` | `internal/token/*`, `internal/cli/jwt_generate.go`, `internal/cli/tty.go`, `e2e/ac03_test.go`, `e2e/ac04_test.go` |
| `T1.4` | `internal/endpoint/*`, `internal/mcpserver/*` |
| `T1.5` | `internal/adapter/postgres/adapter.go`, `connconfig.go`, `preflight.go`, `internal/inventory/*` |
| `T1.6` | `internal/testenv/*` |
| `T1.7` | `e2e/harness/*` except `api.go`, `e2e/e2e_test.go`, `e2e/harness_skeleton_test.go` |
| `T1.8` | `e2e/cmd/acreport/*` |
| `T1.9` | `internal/daemon/*`, `internal/cli/serve.go`, `internal/cli/version.go`, `deploy/Containerfile` |
| `X.1` | `.github/workflows/ci.yml` (jobs `unit` and `build-macos`) |
| `X.2` | `deploy/dev/sag.yaml`, `scripts/dev-up.sh`, `scripts/dev-down.sh` |

## T1.0 Contract

- Goal: Freeze the layout, interfaces, types, and stub files of layer 1, so that `T1.1` to `T1.9` run in parallel.
- Owns: see the ownership table.
- Reads: PRD sections 13.4, 14.2; `foundation.md`; `testing.md`.
- Depends on: `B.1`.
- Content:
  - `go.mod` with the 6 layer-1 modules of `foundation.md` section 2, added with `go get <module>@<version>`.
  - `Makefile` targets: `build`, `image`, `test`, `test-integration`, `e2e`, `e2e-matrix`, `contract-check`, `dev-up`, `dev-down`.
  - `internal/core`: all types of PRD section 14.2, `Trace`, error codes of PRD section 13.3, the registry (`Register`, `Lookup`, `Kinds`).
  - `internal/cli/cli.go`: dispatch of the 7 commands of PRD section 13.1 and the `IO` type (`In`, `Out`, `Err`, `IsTerminal`).
  - Exported signatures of `config`, `secret`, `keys`, `token`, `inventory`, `endpoint`, `mcpserver`, `daemon`, `testenv`. Each stub body returns `errors.ErrUnsupported`.
  - `e2e/acs.go` with the 35 rows of `testing.md` section 6.
  - `e2e/harness/api.go`: the `AC` registry type, `Register`, `Env`, `ResourceSpec`, `AgentSpec`.
- Tests first: `internal/core/registry_test.go`: `TestRegisterDuplicateKindPanics`, `TestLookupUnknownKind`.
- ACs: none.
- Verify: `make contract-check` (runs `go build ./...`, `go vet ./...`, `go vet -tags integration ./...`, `go vet -tags e2e ./e2e/...`, `go test -count=1 ./internal/core/...`). Expected: exit 0.
- Done when: every later task of layer 1 can compile against the stubs.

## T1.1 Config

- Goal: Load and validate `sag.yaml` per PRD section 13.4. Implement `sag config check`.
- Owns: `internal/config/*`, `internal/config/testdata/ac01-inline-dsn.yaml`, `ac01-unknown-key.yaml`, `ac01-max-rows.yaml`, `ac01-recheck-5s.yaml`, `ac01-valid.yaml`, `internal/cli/config_check.go`, `e2e/ac01_test.go`.
- Reads: `internal/core`, `internal/secret` signatures.
- Depends on: `T1.0`.
- Tests first: `config_test.go`: `TestAC01_ConfigCheckRejects` (4 fixtures exit non-zero), `TestAC01_ValidWithoutSecretsDir` (exit 0 when `/run/secrets` is absent), `TestDefaults` (`max_concurrent_per_agent` 20), `TestHardMaximums`, `TestLimitKeysUnderLimits`, `TestMaxConcurrentOnlyInDefaults` (rejected under `resources.<name>.limits`).
- e2e: `AC01_ConfigCheckRejects`: `podman run --rm localhost/sag:e2e config check --config /fixtures/<f>.yaml` for each of the 5 fixtures.
- ACs: AC1.
- Verify: `go test -count=1 ./internal/config/... ./internal/cli/... -run 'TestAC01|TestDefaults|TestHardMaximums|TestLimitKeys|TestMaxConcurrent'`. Expected: `ok` for both packages.
- Done when: the 5 fixtures give the PRD exit codes in the unit test.

## T1.2 Secrets and keys

- Goal: Load Podman secret files with the owner and mode rule. Decode the master key. Derive the JWT key and the approval key with HKDF.
- Owns: `internal/secret/*`, `internal/keys/*`, `e2e/ac02_test.go`.
- Reads: `internal/core`.
- Depends on: `T1.0`.
- Tests first: `secret/load_test.go`: `TestAC02_RefusesMode0444`, `TestAC02_RefusesShortMasterKey`, `TestRefusesWrongOwner`; `secret/credential_test.go`: `TestCredentialRedacted` (`String`, `GoString`, `MarshalJSON`, `LogValue`); `keys/derive_test.go`: `TestJWTKeyInfoLabel`, `TestApprovalKeyInfoLabel`, `TestVersionChangesKey`.
- e2e: `AC02_StartRefusesBadSecrets`: a container with a secret at `mode=0444` exits non-zero. A container with a 16-byte master key exits non-zero. `/healthz` never answers.
- ACs: AC2.
- Verify: `go test -count=1 ./internal/secret/... ./internal/keys/...`. Expected: `ok` for both packages.
- Done when: the daemon can call `secret.Load(dir, 65532, names)` and `keys.JWTKey(master, version)`.

## T1.3 Token and JWT command

- Goal: Issue and verify JWTs per PRD section 11.2. Implement `sag jwt generate` with the terminal check.
- Owns: `internal/token/*`, `internal/cli/jwt_generate.go`, `internal/cli/tty.go`, `e2e/ac03_test.go`, `e2e/ac04_test.go`.
- Reads: `internal/keys`, `internal/config` signatures.
- Depends on: `T1.0`.
- Tests first: `token/verify_test.go`: `TestAC03_Rejects` (9 cases, each asserts the cause), `TestVerifyOrder`; `token/issue_test.go`: `TestClosedClaims`, `TestExpEqualsIatPlusLifetime`; `cli/jwt_generate_test.go`: `TestAC04_RefusesNoTerminal`, `TestAC04_RefusesUnknownSub`, `TestPrintsBareJWT`.
- e2e: `AC03_JWTRejects`: each of the 9 tokens gets HTTP `401` with `WWW-Authenticate: Bearer`. The `auth.version` case uses `sag-v2`. `AC04_JWTGenerateRefuses`: `podman exec sag sag jwt generate ac04` without `-t` exits non-zero. On a pseudo-terminal, `podman exec -it sag sag jwt generate nobody` exits non-zero.
- ACs: AC3, AC4.
- Verify: `go test -count=1 ./internal/token/... ./internal/cli/... -run 'TestAC03|TestAC04|TestVerifyOrder|TestClosedClaims|TestExp|TestPrintsBareJWT'`. Expected: `ok` for both packages.
- Done when: `token.Verify` returns the PRD cause for each case.

## T1.4 Endpoint and MCP skeleton

- Goal: Serve `/healthz` and `/mcp` over TLS 1.3 with the JWT middleware. Serve the `list_resources` tool.
- Owns: `internal/endpoint/*`, `internal/mcpserver/*`.
- Reads: `internal/token`, `internal/inventory`, `internal/core` signatures.
- Depends on: `T1.0`.
- Tests first: `endpoint/auth_test.go`: `TestHeaderForm`, `Test401HasWWWAuthenticate`, `TestToolSeesSub` (go-sdk `req.Extra.TokenInfo.UserID`); `endpoint/server_test.go`: `TestTLS13Minimum`, `TestBodyLimit64KiB`, `TestHealthzNoToken`; `mcpserver/list_resources_test.go`: `TestListsOnlyVisible`.
- ACs: contributes to AC3.
- Verify: `go test -count=1 ./internal/endpoint/... ./internal/mcpserver/...`. Expected: `ok` for both packages.
- Done when: an in-process HTTPS test client gets `list_resources` with a valid token and `401` without one.

## T1.5 Minimal PostgreSQL adapter and inventory

- Goal: Register the `postgres` kind. Build the pgx config from the secret. Run a minimal preflight: connect, `BEGIN TRANSACTION READ ONLY`, `SELECT version()`, `ROLLBACK`. Hold resource status and tag authorization.
- Owns: `internal/adapter/postgres/adapter.go`, `connconfig.go`, `preflight.go`, `internal/inventory/*`.
- Reads: `internal/core`, `internal/secret`, `internal/config` signatures, `internal/testenv`.
- Depends on: `T1.0`. Verify after `T1.6`.
- Tests first: `connconfig_test.go`: `TestRejectsQueryParams`, `TestExecModeExec`, `TestNoFallbacks`, `TestLoopbackIPLiteralOnly`; `preflight_integration_test.go`: `TestPreflightMinimal`; `inventory_test.go`: `TestVisibleDefaultDeny`, `TestLookupNotFound`.
- ACs: none in layer 1. Base for AC9 and AC29.
- Verify: `make test-integration PG=17 PKG='./internal/adapter/postgres/... ./internal/inventory/...'`. Expected: `ok` for both packages.
- Done when: a resource that passes the minimal preflight is visible to its agents only.

## T1.6 Test environment

- Goal: Start and remove PostgreSQL containers, the test CA, and Podman secrets per `testing.md` section 2.
- Owns: `internal/testenv/*`.
- Reads: `testing.md`.
- Depends on: `T1.0`.
- Tests first: `testenv_integration_test.go`: `TestStartPostgresTLS` (`pg_stat_ssl.ssl` true), `TestSuperuser`, `TestLogStatementAll`, `TestTeardownRemovesSecrets`, `TestPG13Starts`.
- ACs: none. Base for every e2e AC.
- Verify: `make test-integration PG=17 PKG=./internal/testenv/...`. Expected: `ok`.
- Done when: `testenv.StartPostgres(ctx, major)` returns a running TLS server with a superuser DSN.

## T1.7 e2e harness and runner

- Goal: Start SAG containers, compose `sag.yaml` from the AC registry, mint JWTs, drive a pseudo-terminal, and run `TestE2E` with its two phases.
- Owns: `e2e/harness/*` except `api.go`, `e2e/e2e_test.go`, `e2e/harness_skeleton_test.go`.
- Reads: `e2e/harness/api.go`, `internal/testenv`, `internal/token`, `internal/keys`.
- Depends on: `T1.0`. Verify after `T1.6`.
- Tests first: `harness/compose_test.go`: `TestComposeConfigFromFragments`, `TestUniqueAgents`; `harness/pty_integration_test.go`: `TestPTYDrivesPodmanExec`; `e2e/harness_skeleton_test.go`: `Harness_Skeleton` (harness check, not an AC: `list_resources` returns `main` for agent `skeleton`).
- ACs: none. Base for every e2e AC.
- Verify: `make test-integration PG=17 PKG=./e2e/harness/...`. Expected: `ok`.
- Done when: a registered AC subtest runs in phase 1 or phase 2 against a composed config.

## T1.8 AC report

- Goal: Build `ac-report.json` and `ac-report.md` from `go test -json` events per `testing.md` section 4.
- Owns: `e2e/cmd/acreport/*`.
- Reads: `e2e/acs.go`.
- Depends on: `T1.0`.
- Tests first: `report_test.go`: `TestStatusRules`, `TestNotImplementedWhenAbsent`, `TestGateFailsOnEarlierLayer`, `TestMarkdownHas35Rows`, `TestMergeMajors`.
- ACs: none. Base for every gate.
- Verify: `go test -count=1 ./e2e/cmd/acreport/...`. Expected: `ok`.
- Done when: `acreport` exits 1 for a failing AC at or below the layer.

## T1.9 Daemon and image

- Goal: Wire `sag serve`: remove `PG*` variables, set `RLIMIT_CORE` to 0, load config and secrets, open resources, run the minimal preflight, serve. Build the image.
- Owns: `internal/daemon/*`, `internal/cli/serve.go`, `internal/cli/version.go`, `deploy/Containerfile`.
- Reads: all layer-1 signatures.
- Depends on: `T1.0`.
- Tests first: `daemon/env_test.go`: `TestRemovesPGVariables`; `daemon/rlimit_test.go`: `TestCoreLimitZero`; `daemon/run_test.go`: `TestRefusesBadMasterKey`, `TestRefusesSecretMode`.
- ACs: contributes to AC2.
- Verify: `make image && podman run --rm localhost/sag:e2e version`. Expected: exit 0 and one version line.
- Done when: the image builds with cgo, runs as UID 65532, and has `/var/lib/sag/audit` and `/var/lib/sag/approvals` with mode 0700.

## X.1 CI skeleton (track)

- Goal: Run `make test` on Linux and `go build ./...` on macOS arm64 for each push.
- Owns: `.github/workflows/ci.yml` (jobs `unit` and `build-macos`).
- Reads: `Makefile`.
- Depends on: `T1.0`.
- Tests first: none. The workflow lint is the check.
- ACs: none. `T6.3` adds the AC35 jobs.
- Verify: `podman run --rm -v "$PWD:/repo:ro" -w /repo docker.io/rhysd/actionlint:1.7.7`. Expected: exit 0 and no output.
- Done when: the workflow lints clean. Actions are pinned by commit SHA.

## X.2 Development stack (track)

- Goal: Start and stop the macOS development stack of `dev-setup.md`.
- Owns: `deploy/dev/sag.yaml`, `scripts/dev-up.sh`, `scripts/dev-down.sh`.
- Reads: `dev-setup.md`, `deploy/Containerfile`.
- Depends on: `T1.0`. Verify after `T1.9`.
- Tests first: none.
- ACs: none.
- Verify: `make dev-up && curl -fsS --cacert "$(mkcert -CAROOT)/rootCA.pem" https://localhost:8443/healthz && make dev-down`. Expected: exit 0.
- Done when: the stack starts from the Podman secrets of `dev-setup.md`.

## T1.10 Integration and gate

- Goal: Prove AC1 to AC4 end to end and close the layer 1 gate.
- Owns: every layer-1 file after all other layer-1 tasks merge. `docs/reviews/layer-1.md`.
- Depends on: `T1.1` to `T1.9`, `X.1`, `X.2`.
- Steps: README section 7.
- ACs: AC1, AC2, AC3, AC4.
- Verify: `make e2e-matrix LAYER=1 PGS="14 17 19"`. Expected: exit 0. `ac-report.md` shows AC1 to AC4 `PASS`, `Harness_Skeleton` passed, and AC5 to AC35 `NOT_IMPLEMENTED`.
- Done when: README section 7 step 11 is complete.

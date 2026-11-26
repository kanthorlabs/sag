# SAG devx implementation plan

| Field | Value |
|---|---|
| Owner | Ulrich |
| Author | Aelita |
| Date | 2026-11-26 |
| Source of truth | `docs/PRD-devx.md`, revision 6 (commit `715799c`) |
| Base | `docs/PRD.md` revision 16, tag `sag-layer-6`, v1 plan in `docs/plan/` |
| Scope | `sag bootstrap`, `sag service`, `sag credential`, `sag logs`, SecureStorage (Podman, AWS Secrets Manager), Tailscale serve, port checks, `sag agent setup`, release flow, removal of the development scripts. DX-AC1 to DX-AC53. |

`docs/PRD-devx.md` wins on any conflict with this plan. "PRD-devx N" names section N of that file.

## Files

| File | Content |
|---|---|
| `README.md` | Scope, rules, module layout, code facts, test environment, task graph, gates, AC map, estimate, risks |
| `layer-1.md` to `layer-11.md` | Goal, ACs, gate, file ownership, and tasks of each layer |

## 1. Rules

The rules of `docs/plan/README.md` sections 1 to 7 apply. The v1 files `foundation.md` and `testing.md` apply, except where this section states a difference.

| Item | v1 rule | devx rule |
|---|---|---|
| Task ID | `T<N>.<k>` | `D<N>.<k>`. `D<N>.0` is the contract task. The last task of a layer is the integration task. |
| Integration branch | `feat/v1-postgres` | `feat/devx` |
| Integration worktree | `.dev/integration` | `.dev/devx` |
| Task branch | `feat/v1-postgres--<task-id>` | `feat/devx--<task-id>`, created from `feat/devx` |
| Task worktree | `.dev/<task-id>` | `.dev/<task-id>`, for example `.dev/D4.2` |
| Claims, merge lock | `.dev/claims/<task-id>/`, `.dev/claims/merge.lock/` | Same paths |
| Layer tag | `sag-layer-<N>` | `sag-devx-<N>`. `sag-devx-0` marks the commit of this plan. |
| Review file | `docs/reviews/layer-<N>.md` | `docs/reviews/devx-layer-<N>.md` |
| Layer count | 6 | 11 |
| Gate proof | Local e2e matrix, CI only in layer 6 | CI run on each layer (section 6), dev-machine run from layer 10 |
| Verify of a task | Unit, integration, or e2e command | Unit or integration command only. The DX e2e tests need Ubuntu and run only in CI. |

Additional rules:

- Read `docs/PRD-devx.md`, this file, and the file of your layer before you claim a task.
- Read the PRD-devx sections that your task names. Do not change a behavior that the PRD states.
- Only Ulrich pushes. Each gate asks Ulrich to push `feat/devx`. Never push `main`.
- Never print, log, or commit a credential value, a JWT, or the Tailscale IP of the macOS development machine.
- A test reads the Tailscale IP at run time. Test output and the AC report write `<tailscale IP>` in its place.
- No test writes a private key to a file. The endpoint CA key exists only in memory (PRD-devx 5.7).
- Name no Go package, symbol, or exported identifier with the text `devxtest`. DX-AC33 searches the product symbol table for this text.
- A host command never calls `Load` of a SecureStorage backend (PRD-devx 11.1).

### 1.1 Bootstrap of the plan (D0)

Run these commands one time in `.dev/devx` after Ulrich approves this plan:

```sh
git tag sag-devx-0 feat/devx
mkdir -p ../claims ../local
```

## 2. Dev-machine-only criteria

Ulrich ruling Q24 and PRD-devx 17 define four groups that run only on the macOS development machine of Ulrich:

| Group | Criteria |
|---|---|
| macOS | DX-AC34, DX-AC35 |
| Podman install and machine | DX-AC38 |
| Real harnesses | DX-AC47, DX-AC48 |
| Tailscale | DX-AC52 |

- No GitHub Actions job runs these criteria.
- CI tests the logic of these features with harness stubs and a fake `tailscale` command. No CI job installs a harness.
- The AC report records the result of each `Dev machine` criterion: pass or fail, and the date.
- The AC report also records the versions of macOS, Podman, Claude Code, pi, and Tailscale.
- Ulrich starts the dev-machine group from a terminal outside a Claude Code session (PRD-devx 17.1, R24). An agent prepares the command and reads the report.
- The PRD-devx 18.1 manual check list needs a macOS arm64 machine without Podman. The development machine does not qualify. Section 10 risk 3 records this gap.

### 2.1 Shared-machine constraints

The development machine is shared with the work of Ulrich. Each devx task, test, and gate obeys these constraints:

- No criterion removes Podman.
- No criterion restarts the Podman machine `podman-machine-default`.
- No criterion starts a second Podman machine.
- The dev-machine group checks its preconditions before any change: `podman machine list` shows exactly one machine, `podman-machine-default`, and it runs. No container `sag` exists. Port 8443 is free. On a failed check, the group exits 1 and changes nothing.
- Run `make dev-down` before the group while `scripts/dev-down.sh` exists. It removes the container of the old development scripts.
- DX-AC38 stops before `installer`. It never runs `installer`, `podman machine init`, or `podman machine start`.
- The real-harness tests copy these files before the group and restore them after the PRD-devx 15 teardown: `~/.claude.json`, `~/.claude/settings.json`, `~/.pi/agent/mcp.json`, and the shell profile. The group also saves the output of `tailscale serve status --json`.
- After the restore, the group compares each file with its copy. For `~/.claude.json`, it compares only `mcpServers`. The serve status equals the saved output. No file of `~/.claude/backups` holds `mcpServers.sag`.
- Do not run the v1 e2e matrix and the dev-machine group at the same time.
- The local work of the 3 agents uses the same Podman machine. Run integration tests with unique container names only (`internal/testenv` rule).

## 3. Module layout

New and changed paths. One responsibility per package. The v1 layout of `docs/plan/foundation.md` section 3 stays valid.

| Path | Responsibility | Layer |
|---|---|---|
| `internal/deploy` | `ProductFlags`, `ContainerFlags`, log flags, `ImageDigest`, image reference, `podman run` arguments, Quadlet text, secret mounts from `sag.yaml` | 1, 8 |
| `deploy/embed.go` | Package `deployfiles`: `go:embed pins`. `go:embed` cannot reach a parent directory, so the embed lives in `deploy/`. | 3 |
| `deploy/pins` | Pin file of PRD-devx 3 | 3 |
| `internal/secret` | `SecureStorage`, `Metadata`, the three errors, reference parse (`secret:`, `aws-sm:`), the v1 file load | 1, 9 |
| `internal/secret/podman` | Podman backend | 1 |
| `internal/secret/awssm` | AWS Secrets Manager backend | 9 |
| `internal/podman` | Host Podman command runner: stored path, standard input for values, `inspect`, `ps`, `logs`, `exec` | 1 |
| `internal/hoststate` | `<cfg>` paths, lock, `answers.json`, `podman` path search | 2 |
| `internal/prompt` | `/dev/tty` open, `[y/N]` prompt, hidden line read, answer resolution (flag, stored answer, prompt, exit 3) | 2, 4 |
| `internal/stepline` | Step line writer `<status> <item> <detail>` | 2 |
| `internal/testhook` | Test hooks. `testhook_devxtest.go` (build tag `devxtest`) reads `SAG_TEST_*`. `testhook_product.go` (`!devxtest`) holds no-op hooks. | 2 |
| `internal/masterkey` | Creation of `sag-master-key` | 2 |
| `internal/endpointcert` | CA and server certificate, renewal sequence of PRD-devx 5.7 | 2 |
| `internal/image` | Image pull when absent; image selection of PRD-devx 4.3 | 2, 8 |
| `internal/service` | `service start` and `service stop`, `/healthz` wait, LaunchAgent, user Quadlet file | 2 |
| `internal/bootstrap` | Bootstrap plan and apply, steps 1 to 15 | 2, 3, 7 |
| `internal/pins` | Pin parse and selection | 3 |
| `internal/deps` | Dependency rule, macOS pkg download and check, apt install, Podman machine | 3 |
| `internal/tailscale` | Tailscale detection, serve status parse, serve rule, operator step | 3 |
| `internal/portcheck` | Holder of `127.0.0.1:8443` | 3 |
| `internal/credential` | `credential create`, `list`, `inspect`, `delete` flows | 4, 5 |
| `internal/agentsetup` | `agent setup` for Claude Code and pi, JSON write rule, assets | 6, 7 |
| `internal/cli` | One file per command. New: `mode.go`, `bootstrap.go`, `service.go`, `credential.go`, `credential_check.go`, `logs.go`, `audit_read.go`, `host_forms.go`, `agent_setup.go`, `agent_issue.go` | 2 to 7 |
| `e2e/devx` | DX e2e suite, build tag `devxe2e`: `acs.go` (53 rows), `devx_test.go` (`TestDevx`), one file `dxac<NN>_test.go` per criterion | 1 to 10 |
| `e2e/devx/dxh` | DX harness: host binary runner, pseudo-terminal driver, test `PATH` directory, teardown, dev-machine snapshot and restore | 1, 10 |
| `e2e/devx/cmd/fakeharness` | Stub `claude` and `pi`, selected by `argv[0]` | 6 |
| `e2e/devx/cmd/faketailscale` | Fake `tailscale` of PRD-devx 17.1 | 3 |
| `.github/workflows/release.yml` | Release workflow of PRD-devx 4.2 | 8 |

### 3.1 `internal/agentsetup` layout

```
internal/agentsetup/
  agentsetup.go             go:embed of assets/skill/SKILL.md and assets/pi/sag-guard.ts
  assets/skill/SKILL.md
  assets/pi/sag-guard.ts
  assets/pi/sag-guard.test.ts
```

- The `go:embed` directive names the two product files. The test file stays out of the binary.
- `sag-guard.ts` imports only types from `@earendil-works/pi-coding-agent`, with `import type`. Node runs the file without that package.
- `sag-guard.test.ts` uses only `node:test` and `node:assert`. It calls the default export with a test `pi` object and checks the result of the `tool_call` handler.
- CI runs `node --test "internal/agentsetup/assets/pi/*.test.ts"` with Node 24 and its default type strip.
- A Go unit test `TestDXAC49_SkillNamesToolsAndCodes` checks that the skill body holds each tool name and each error code of PRD 13.3. The test reads them from the Go constants.

## 4. Code facts at the base

These facts hold at commit `715799c`. The named layer changes each one.

| Fact | Layer |
|---|---|
| `internal/config/config.go` rejects an empty `resources` map (`must hold at least one resource`). | 1 |
| `e2e/harness/flags.go` holds `ProductFlags`, `ContainerFlags`, and `TestOnlyFlags` with `--log-driver k8s-file`. `e2e/ac01_test.go`, `e2e/ac32_test.go`, `e2e/harness/quadlet_test.go`, and `e2e/harness/flags_test.go` use them. | 1 |
| `deploy/sag.container` has `Image=localhost/sag:latest` and no log keys. | 1 |
| `internal/scrub.Scrubber` is an immutable value from `scrub.New`. PRD-devx 11.4 needs an add at `Load` time. | 9 |
| `go.mod` has no direct `golang.org/x/term`. `golang.org/x/sys` is only indirect. | 4 |
| `go.mod` has no AWS SDK module. | 9 |
| `internal/adapter/postgres/preflight.go`, `internal/cli/extension.go`, and `internal/cli/jwt_generate.go` hold the text `podman exec -it`. | 5 |
| `internal/cli/jwt_generate.go` checks only standard output for a terminal. | 5 |
| `internal/cli/extension.go` revoke behavior without an approval record differs from PRD-devx 16 row 9.2. The task verifies it. | 5 |
| `internal/cli/cli.go` has exit codes 0, 1, and 2 only. Code 3 is absent. | 2 |
| `.github/workflows/ci.yml` grants only `contents: read`. It has no devx job. | 1, 8 |
| `Makefile` has the `dev-up` and `dev-down` targets. `scripts/` holds `dev-up.sh` and `dev-down.sh`. | 11 |

## 5. Test environment

PRD-devx 17.1 is the source. This section adds the decisions of the plan.

### 5.1 DX e2e suite

- Package `e2e/devx`. Its test files have the build tag `devxe2e`. `make e2e` does not build them, so no v1 run touches the real `HOME`.
- `e2e/devx/acs.go` has no build tag, so `acreport` imports the table.
- One top-level test `TestDevx`. Subtests run in series, in the order of `e2e/devx/acs.go`. Name form: `TestDevx/<group>/DXAC<NN>_<Name>`.
- `TestDevx/<group>` runs the PRD-devx 15 teardown before and after the group.
- DX-AC1 to DX-AC4 run first, in this order.
- The suite runs the test build `bin/sag-devxtest` (`make build-devxtest`, tag `devxtest`). DX-AC33 and DX-AC30 also use the product build `bin/sag`.
- `SAG_TEST_IMAGE` names `localhost/sag:e2e`, built by `make image`.
- The harness stubs and the fake `tailscale` are Go programs. `dxh` builds them into a test `PATH` directory.
- DX-AC44 has no Go subtest. The `devx-unit` CI job runs the Node tests. `acreport` reads the step result.
- DX-AC49 is the Go unit test `TestDXAC49_SkillNamesToolsAndCodes`. The `devx-unit` job runs it with `go test -json`. `acreport` reads its events.

### 5.2 Makefile targets

| Target | Function |
|---|---|
| `build-devxtest` | `go build -tags devxtest -o bin/sag-devxtest ./cmd/sag` |
| `e2e-devx` | `make build build-devxtest image`, then `go test -tags devxe2e -json` of `./e2e/devx/...` for `DX_GROUP=main\|pins\|install`, then `acreport -suite devx -env ci` |
| `e2e-devx-mac` | Dev-machine group: precondition check, snapshot, `go test -tags devxe2e -run 'TestDevx/mac'`, teardown, restore check, `acreport -suite devx -env dev-machine` |

### 5.3 CI jobs

| Job | Runner | Content | Layer |
|---|---|---|---|
| `devx-e2e` | `ubuntu-24.04` | `sudo loginctl enable-linger "$USER"`, `XDG_RUNTIME_DIR=/run/user/$(id -u)`, `make e2e-devx DX_GROUP=main`. Upload `e2e/report/devx-main/`. | 1 |
| `devx-pins` | `ubuntu-24.04` | Test-pin criteria DX-AC6, DX-AC7, DX-AC8 (`DX_GROUP=pins`) | 3 |
| `devx-install` | `ubuntu-24.04` | DX-AC5 (`DX_GROUP=install`). Removes and installs Podman on the runner. | 3 |
| `devx-unit` | `ubuntu-24.04` | `actions/setup-node` with Node 24, `node --test`, `go test -json -run '^TestDXAC49_'` | 6 |
| `devx-report` | `ubuntu-24.04`, `needs` each devx job, `if: always()` | `go run ./e2e/cmd/acreport merge-devx` of each devx report. Upload artifact `devx-report`. | 1 |
| `image-main` | `ubuntu-24.04` and `ubuntu-24.04-arm` | On a push to `main` only: build and push `docker.io/kanthorlabs/sag:main` | 8 |

- Each devx job sets `CGO_ENABLED: "1"` and pins each action by commit SHA.
- `make e2e-devx` takes `LAYER=<N>`. Each integration task sets `N` in `ci.yml` to its layer.
- A job of a later layer starts as a skeleton in its layer contract.

### 5.4 AC report

- `e2e/devx/acs.go` holds 53 rows: `ID` (`DX-AC01` to `DX-AC53`), `Layer`, `Test`, `Run` (`ci` or `dev-machine`).
- `acreport -suite devx -env <env>` marks each row of the other environment `EXCLUDED`.
- `acreport -suite devx -env dev-machine` adds the block `env`: date, macOS version, Podman version, Claude Code version, pi version, Tailscale version. It never records an IP address.
- `acreport merge-devx -ci <file>... -dev <file>` gives the combined report. Gate rule: each row with `layer <= N` is `PASS`.
- From layer 10, the gate merges the CI report with the dev-machine report of the same commit.

## 6. Layer gate

The v1 gate (`docs/plan/README.md` section 7) applies with these steps in place of its steps 3 and 4:

1. In `.dev/devx`, run `make test` and `make e2e LAYER=6 PG=17`. Expected: exit 0. AC1 to AC34 `PASS` (v1 regression).
2. From layer 6, run `node --test "internal/agentsetup/assets/pi/*.test.ts"`. Expected: exit 0.
3. Ask Ulrich to push `feat/devx`. Wait for the CI run.
4. Download the artifacts `ac35` and `devx-report`. Expected: AC35 `PASS`. Each CI DX-AC with `layer <= N` is `PASS`.
5. From layer 10, ask Ulrich to run `make e2e-devx-mac` on the development machine from a terminal outside Claude Code. Merge its report with `acreport merge-devx`.
6. If a step fails, fix the cause in `.dev/devx` and run the step again. A fix after step 3 needs a new push.

- The review diff is `git diff sag-devx-<N-1>`. The review sub-agent gets the PRD-devx sections of the layer.
- The review file appends the final `devx-report` table and, from layer 10, the dev-machine `env` block.
- The gate closes when the tag `sag-devx-<N>` exists and every DX-AC with `layer <= N` is `PASS`.

## 7. Task graph

Notation of `docs/plan/README.md` section 3. Each layer has at most 3 parallel tasks after its contract task.

```
Bootstrap
  D0 -> D1.0

Layer 1  Base: deploy package, SecureStorage, DX harness (no DX-AC)
  D1.0 -> D1.1 D1.2 D1.3
  all  -> D1.4

Layer 2  Bootstrap and service on Ubuntu (DX-AC1 2 4 27 28 33 36)
  D1.4 -> D2.0 -> D2.1 D2.2 D2.3
  D2.3 (verify after D2.1, D2.2)
  all  -> D2.4

Layer 3  Dependencies, Tailscale serve, port check (DX-AC3 5 6 7 8 50 53)
  D2.4 -> D3.0 -> D3.1 D3.2 D3.3
  D3.3 (verify after D3.2)
  all  -> D3.4

Layer 4  Credential create, list, inspect (DX-AC9-17 29 37)
  D3.4 -> D4.0 -> D4.1 D4.2 D4.3
  D4.2 (verify after D4.1)
  D4.3 (verify after D4.2)
  all  -> D4.4

Layer 5  Credential delete, host forms, logs (DX-AC18-25)
  D4.4 -> D5.0 -> D5.1 D5.2 D5.3
  all  -> D5.4

Layer 6  Agent setup, local mode (DX-AC26 39-45 49)
  D5.4 -> D6.0 -> D6.1 D6.2 D6.3
  D6.1 (verify after D6.2, D6.3)
  all  -> D6.4

Layer 7  Agent setup, remote mode and bootstrap step 14 (DX-AC46 51)
  D6.4 -> D7.0 -> D7.1 D7.2
  all  -> D7.3

Layer 8  Image selection and release workflow (DX-AC30)
  D7.3 -> D8.0 -> D8.1 D8.2
  all  -> D8.3

Layer 9  AWS Secrets Manager backend (DX-AC31 32)
  D8.3 -> D9.0 -> D9.1 D9.2 D9.3
  D9.2 (verify after D9.1)
  D9.3 (verify after D9.1)
  all  -> D9.4

Layer 10 macOS development machine (DX-AC34 35 38 47 48 52)
  D9.4 -> D10.0 -> D10.1 D10.2 D10.3
  all  -> D10.4

Layer 11 Removal of the development scripts (no DX-AC)
  D10.4 -> D11.0 -> D11.1 D11.2
  all  -> D11.3
```

The layers follow PRD-devx 19. Layers 2 and 3 split build step 2. Layers 6 and 7 split build step 5. Layer N maps to build step: 1→1, 2→2, 3→2, 4→3, 5→4, 6→5, 7→5, 8→6, 9→7, 10→8, 11→9.

## 8. AC map

| AC | Layer | Task | Test | Run |
|---|---|---|---|---|
| DX-AC1 | 2 | `D2.3` | `DXAC01_BootstrapUbuntu` | CI |
| DX-AC2 | 2 | `D2.3` | `DXAC02_ContainerAndCert` | CI |
| DX-AC3 | 3 | `D3.3` | `DXAC03_BootstrapIdempotent` | CI |
| DX-AC4 | 2 | `D2.1` | `DXAC04_BootstrapNoTerminal` | CI |
| DX-AC5 | 3 | `D3.1` | `DXAC05_PodmanAptInstall` | CI |
| DX-AC6 | 3 | `D3.1` | `DXAC06_InstallApproval` | CI |
| DX-AC7 | 3 | `D3.1` | `DXAC07_ReleaseChecksum` | CI |
| DX-AC8 | 3 | `D3.1` | `DXAC08_VersionRule` | CI |
| DX-AC9 | 4 | `D4.2` | `DXAC09_CreateCanary` | CI |
| DX-AC10 | 4 | `D4.3` | `DXAC10_CanaryNoLeak` | CI |
| DX-AC11 | 4 | `D4.2` | `DXAC11_CreateRejectsForm` | CI |
| DX-AC12 | 4 | `D4.1` | `DXAC12_ConnectionCheck` | CI |
| DX-AC13 | 4 | `D4.1` | `DXAC13_LoopbackWarn` | CI |
| DX-AC14 | 4 | `D4.2` | `DXAC14_CreateStdin` | CI |
| DX-AC15 | 4 | `D4.2` | `DXAC15_CreateSecretExists` | CI |
| DX-AC16 | 4 | `D4.2` | `DXAC16_CreateRollback` | CI |
| DX-AC17 | 4 | `D4.3` | `DXAC17_ListInspect` | CI |
| DX-AC18 | 5 | `D5.1` | `DXAC18_DeleteNoTerminal` | CI |
| DX-AC19 | 5 | `D5.1` | `DXAC19_DeleteConfirm` | CI |
| DX-AC20 | 5 | `D5.1` | `DXAC20_Delete` | CI |
| DX-AC21 | 5 | `D5.1` | `DXAC21_DeleteStopped` | CI |
| DX-AC22 | 5 | `D5.1` | `DXAC22_DeleteResume` | CI |
| DX-AC23 | 5 | `D5.1` | `DXAC23_DeleteForeignSecret` | CI |
| DX-AC24 | 5 | `D5.2` | `DXAC24_HostJWTGenerate` | CI |
| DX-AC25 | 5 | `D5.3` | `DXAC25_Logs` | CI |
| DX-AC26 | 6 | `D6.3` | `DXAC26_DenyRules` | CI |
| DX-AC27 | 2 | `D2.2` | `DXAC27_Autostart` | CI |
| DX-AC28 | 2 | `D2.2` | `DXAC28_ServiceStopStart` | CI |
| DX-AC29 | 4 | `D4.3` | `DXAC29_CertRenewal` | CI |
| DX-AC30 | 8 | `D8.1` | `DXAC30_ImageSelection` | CI |
| DX-AC31 | 9 | `D9.3` | `DXAC31_AWSBackend` | CI |
| DX-AC32 | 9 | `D9.2` | `DXAC32_AWSUnavailable` | CI |
| DX-AC33 | 2 | `D2.1` | `DXAC33_ProductBinary` | CI |
| DX-AC34 | 10 | `D10.1` | `DXAC34_MacBootstrap` | Dev machine |
| DX-AC35 | 10 | `D10.1` | `DXAC35_MacCredential` | Dev machine |
| DX-AC36 | 2 | `D2.1` | `DXAC36_HostGuards` | CI |
| DX-AC37 | 4 | `D4.2` | `DXAC37_CreateTLSCA` | CI |
| DX-AC38 | 10 | `D10.1` | `DXAC38_MacPkgChecksum` | Dev machine |
| DX-AC39 | 6 | `D6.1` | `DXAC39_ClaudeCodeSetup` | CI |
| DX-AC40 | 6 | `D6.1` | `DXAC40_JWTNoLeak` | CI |
| DX-AC41 | 6 | `D6.3` | `DXAC41_PiSetup` | CI |
| DX-AC42 | 6 | `D6.3` | `DXAC42_CATrustConflict` | CI |
| DX-AC43 | 6 | `D6.3` | `DXAC43_AgentSetupRefusals` | CI |
| DX-AC44 | 6 | `D6.2` | `node --test` step of `devx-unit` | CI |
| DX-AC45 | 6 | `D6.1` | `DXAC45_AgentIssue` | CI |
| DX-AC46 | 7 | `D7.2` | `DXAC46_BootstrapAgentSetup` | CI |
| DX-AC47 | 10 | `D10.2` | `DXAC47_RealClaudeCode` | Dev machine |
| DX-AC48 | 10 | `D10.2` | `DXAC48_RealPi` | Dev machine |
| DX-AC49 | 6 | `D6.2` | `TestDXAC49_SkillNamesToolsAndCodes` (unit) | CI |
| DX-AC50 | 3 | `D3.2` | `DXAC50_TailscaleFake` | CI |
| DX-AC51 | 7 | `D7.1` | `DXAC51_RemoteMode` | CI |
| DX-AC52 | 10 | `D10.3` | `DXAC52_MacTailscale` | Dev machine |
| DX-AC53 | 3 | `D3.3` | `DXAC53_PortHolder` | CI |

DX-AC29 needs `credential list` (build step 3). Thus it closes in layer 4. Layer 2 builds the renewal sequence with unit tests.

## 9. Estimate

Aelita estimate in hours, not a commitment. Wall-clock is contract + longest parallel task + integration. The integration estimate holds one CI cycle.

| Layer | Contract | Parallel tasks (start order) | Parallel phase | Integration | Wall-clock |
|---|---|---|---|---|---|
| 1 | `D1.0` 1 | `D1.3` 4, `D1.1` 3, `D1.2` 2.5 | 4 | `D1.4` 3 | 8 |
| 2 | `D2.0` 1.5 | `D2.1` 4.5, `D2.2` 3.5, `D2.3` 3 | 4.5 | `D2.4` 3 | 9 |
| 3 | `D3.0` 1 | `D3.1` 4, `D3.2` 3.5, `D3.3` 2 | 4 | `D3.4` 3 | 8 |
| 4 | `D4.0` 1 | `D4.2` 5, `D4.3` 3, `D4.1` 2.5 | 5 | `D4.4` 3 | 9 |
| 5 | `D5.0` 1 | `D5.1` 4, `D5.2` 2, `D5.3` 1.5 | 4 | `D5.4` 3 | 8 |
| 6 | `D6.0` 1.5 | `D6.1` 5, `D6.3` 4.5, `D6.2` 3 | 5 | `D6.4` 3.5 | 10 |
| 7 | `D7.0` 0.5 | `D7.1` 2.5, `D7.2` 2.5 | 2.5 | `D7.3` 2.5 | 5.5 |
| 8 | `D8.0` 0.5 | `D8.2` 3, `D8.1` 2.5 | 3 | `D8.3` 2.5 | 6 |
| 9 | `D9.0` 1 | `D9.1` 3.5, `D9.2` 3.5, `D9.3` 3.5 | 3.5 | `D9.4` 3 | 7.5 |
| 10 | `D10.0` 2 | `D10.1` 3, `D10.2` 3, `D10.3` 2 | 3 | `D10.4` 4 | 9 |
| 11 | `D11.0` 0.5 | `D11.1` 1.5, `D11.2` 1.5 | 1.5 | `D11.3` 2 | 4 |
| Total | | | | | 84 |

| Item | Value |
|---|---|
| Wall-clock with the pool of 3 agents | about 84 h, plus the wait for 11 pushes by Ulrich |
| Agent-hours | about 137 h |
| Serial floor | about 44 h: 11 contract tasks and 11 integration tasks |
| CI run | about 45 min per push (v1 matrix and devx jobs in parallel) |

## 10. Risks

1. Only CI proves the Ubuntu criteria. A task cannot run the DX e2e suite on the macOS machine. Defects surface at the gate and cost a push cycle.
2. Each gate needs a push by Ulrich and a CI run. Layer 10 also needs Ulrich at a terminal outside Claude Code.
3. PRD-devx 18.1 needs a macOS machine without Podman. No such machine exists in the plan. Build step 8 lists the check list, but no gate can run it.
4. PRD-devx 8: the secret list of `podman container inspect` on Podman 4.9.3 is not confirmed. `D1.2` confirms it in CI. On a negative result, DX-AC20 and DX-AC2 use the Quadlet fallback of PRD-devx 8.
5. DX-AC5 removes Podman with `apt-get remove`. The task assumes that the rootless store and the image stay. `D3.1` proves it in CI.
6. The systemd user manager with linger on the GitHub runner is not proven. `D1.3` proves it with the skeleton job.
7. LocalStack image and access rules (PRD-devx 17.1, build step 7) are not confirmed. `D9.0` confirms them.
8. The stub formats follow one recorded line of Claude Code 2.1.332. A new harness version can change the format. The pins in PRD-devx 1 hold the versions.
9. DX-AC34 and DX-AC52 change the real Tailscale serve configuration of Ulrich during the group. The teardown and the saved serve status restore it.
10. The release workflow and the `image-main` job run for real only on a tag and on a push to `main`. Before that, only `actionlint` and DX-AC30 prove them. Docker Hub secrets must exist first.
11. The symbol check of DX-AC33 fails when a product symbol holds the text `devxtest`. Section 1 forbids the name.
12. 3 agents and the v1 regression run share one Podman machine. CPU and memory compete.

Cut order if time runs short. No cut removes a security control or its AC:

1. Skip the local v1 regression of gate step 1 in layers 3, 5, 7, and 11. These layers change no daemon code. CI keeps the full matrix.
2. Defer the `image-main` job. Ulrich pushes the `main` image by hand one time.

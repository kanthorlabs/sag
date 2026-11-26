# SAG devx implementation plan

| Field | Value |
|---|---|
| Owner | Ulrich |
| Author | Aelita |
| Revision | 3 |
| Date | 2026-11-26 |
| Source of truth | `docs/PRD-devx.md`, revision 8 (commit `8227e10`) |
| Base | `docs/PRD.md` revision 16, tag `sag-layer-6`, v1 plan in `docs/plan/` |
| Scope | `sag bootstrap`, `sag service`, `sag credential`, `sag logs`, SecureStorage (Podman, AWS Secrets Manager), Tailscale serve, port checks, `sag agent setup`, release flow, removal of the development scripts. DX-AC1 to DX-AC53. |

`docs/PRD-devx.md` wins on any conflict with this plan. "PRD-devx N" names section N of that file.

## Files

| File | Content |
|---|---|
| `README.md` | Rules, machines, module layout, code facts, test environment, gates, task graph, AC map, estimate, risks |
| `layer-1.md` to `layer-9.md` | Phase A layers: goal, ACs, gate, file ownership, and tasks |
| `layer-10.md` | Phase B layer: the Linux port. Blocked until Ulrich gives access to the Debian machine. |

## 1. Rules

The rules of `docs/plan/README.md` sections 1 to 7 apply. The v1 files `foundation.md` and `testing.md` apply, except where this section states a difference.

| Item | v1 rule | devx rule |
|---|---|---|
| Task ID | `T<N>.<k>` | `D<N>.<k>`. `D<N>.0` is the contract task. The last task of a layer is the integration task. Layer 0 has the tasks `D0.1` to `D0.3` and no gate. |
| Integration branch | `feat/v1-postgres` | `feat/devx` |
| Integration worktree | `.dev/integration` | `.dev/devx` |
| Task branch | `feat/v1-postgres--<task-id>` | `feat/devx--<task-id>`, created from `feat/devx` |
| Task worktree | `.dev/<task-id>` | `.dev/<task-id>`, for example `.dev/D4.2` |
| Claims, merge lock | `.dev/claims/<task-id>/`, `.dev/claims/merge.lock/` | Same paths |
| DX run lock | None | `.dev/claims/devx-e2e.lock/`. `make e2e-devx` holds it during a run. The file `owner` in the lock directory holds the PID and the worktree of the run. |
| Layer tag | `sag-layer-<N>` | `sag-devx-<N>`. `sag-devx-0` marks the commit of this plan. |
| Review file | `docs/reviews/layer-<N>.md` | `docs/reviews/devx-layer-<N>.md` |
| Layer count | 6 | 10. Phase A: layers 1 to 9. Phase B: layer 10. |
| Gate proof | Local e2e matrix | Phase A: local run on the macOS development machine. Phase B: run on the Debian machine. No CI job is a gate. |
| Verify of a task | Unit, integration, or e2e command | Unit command, and `make e2e-devx` for each DX criterion of the task |
| Push | First push in `T6.4` | No gate needs a push. Only Ulrich pushes, when he wants a CI result. |

Additional rules:

- Read `docs/PRD-devx.md`, this file, and the file of your layer before you claim a task.
- Read the PRD-devx sections that your task names. Do not change a behavior that the PRD states.
- Never push. Never touch `main`.
- Never print, log, or commit a credential value, a JWT, or the Tailscale IP of the macOS development machine.
- A test reads the Tailscale IP at run time. Test output and the AC report write `<tailscale IP>` in its place.
- No test writes a private key to a file. The endpoint CA key exists only in memory (PRD-devx 5.7).
- Name no Go package, symbol, or exported identifier with the text `devxtest`. DX-AC33 searches the product symbol table for this text.
- A host command never calls `Load` of a SecureStorage backend (PRD-devx 11.1).
- A task can own a file of a closed layer when the ownership table of its layer lists the file.
- In phase A, `bootstrap` and `service` accept only `darwin/arm64`. On another platform, they exit 1 with `unsupported platform <os>/<arch>`. Layer 10 adds Linux.
- An OS-specific file has the suffix `_darwin.go` or `_linux.go`. In phase A, each `_linux.go` file is a stub that returns the `unsupported platform` error.
- A task that owns an OS-specific file runs `GOOS=linux CGO_ENABLED=0 go vet` on its packages before the merge.

### 1.1 Layer 0

Layer 0 has no contract task and no gate. `D1.0` starts after `D0.1`, `D0.2`, and `D0.3`.

#### D0.1 Plan bootstrap

Run these commands one time in `.dev/devx` after Ulrich approves this plan:

```sh
git tag sag-devx-0 feat/devx
mkdir -p ../claims ../local
```

#### D0.2 PRD.md changes

- Goal: Apply each row of PRD-devx 16 to `docs/PRD.md` in one documents commit on `feat/devx`.
- Owns: `docs/PRD.md`.
- Method: Copy the text of each row into the named PRD section. Keep each phrase of the table below. Add the revision line `revision 17`. Show the diff to Ulrich. Commit only after Ulrich approves it.
- Verify: run `grep -nF '<phrase>' docs/PRD.md` for each row of the table. Expected: each command exits 0.

| PRD section | Phrase |
|---|---|
| 2 G1 | `SecureStorage backend` |
| 4 Personas | `devx host commands` |
| 5 | `golang.org/x/term` |
| 6 | `tailscale serve --tcp` |
| 6.1 | `no file holds the CA key` |
| 7.1 | `in the account of the operator` |
| 7.3 | `T8` |
| 7.4 TB2, TB4 | `AWS backend as a secret source` |
| 7.6 | `R11 to R28` |
| 9.1 | `to approve, run: sag extension approve <name>` |
| 9.2 | `exits 0 when no approval record exists` |
| 10.2 | `at the time that` |
| 10.5 | `remote_addr` |
| 11.1 | `sag bootstrap` |
| 11.2 | `sag agent issue` |
| 12.1 | `sag credential create` |
| 13.1 | `sag agent setup` |
| 13.2 | `never passes the JWT as a command argument` |
| 13.4 | `aws-sm:` |
| 13.5 | `No devx command creates these secrets` |
| 15.2 | `build order` |

#### D0.3 Harness form record (Ulrich)

- Goal: Record the connected `sag` line of `claude mcp list` 2.1.332 (PRD-devx 19 step 5). Stop the development stack before the first DX run.
- Owns: `e2e/devx/testdata/harness-forms.txt`.
- Run by: Ulrich, on the macOS development machine.
- Steps:
  1. Start SAG with `make dev-up`.
  2. Create a temporary directory `<tmp>` with mode 0700.
  3. Write `<tmp>/.claude.json` by hand with `mcpServers.sag` of PRD-devx 12.4 and a JWT from `podman exec -it sag-dev sag jwt generate claude-code-dev`.
  4. Run `CLAUDE_CONFIG_DIR=<tmp> NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" claude mcp list`.
  5. Copy the line that starts with `sag: ` to `e2e/devx/testdata/harness-forms.txt`. Remove `<tmp>`.
  6. Run `make dev-down`. The containers `sag-dev` and `sag-dev-pg` then do not exist, and port 8443 is free.
- Verify: `test "$(grep -c '^sag: ' e2e/devx/testdata/harness-forms.txt)" = 1 && ! grep -q 'eyJ' e2e/devx/testdata/harness-forms.txt`. Expected: exit 0.
- Done when: the coordinator commits the file on `feat/devx`.

## 2. Machines

### 2.1 Phase A: macOS development machine

The machine is shared with the work of Ulrich. Each devx task, test, and gate obeys these constraints:

- No task or test removes Podman, restarts `podman-machine-default`, or starts a second Podman machine.
- No code path of a test calls `podman machine init`, `start`, `stop`, or `rm`, or a Podman uninstall.
- DX-AC38 stops before `installer`. It never runs `installer`.
- No test runs `tailscale funnel`. Only DX-AC52 runs the real `tailscale`.
- `make e2e-devx` takes the DX run lock. One DX run uses the machine at a time.
- The gate runs the v1 e2e matrix and the DX suites in series.
- The local work of the 3 agents uses the same Podman machine. Run integration tests with unique container names only (`internal/testenv` rule).

`dxh` checks these preconditions before each DX run. On a failed check, the run exits 1, names the check, and changes nothing:

- `podman machine list` shows exactly one machine, `podman-machine-default`, and it runs.
- `/var` in `podman-machine-default` has at least 10 GiB free (ruling Q36). `dxh` reads it with `podman machine ssh podman-machine-default df -Pk /var`. Below the limit, the run writes `fail podman machine free space <n> GiB; need 10`.
- No container `sag`, `sag-dev`, or `sag-dev-pg` exists. Port 8443 is free.
- No volume `sag-state` and no image `docker.io/kanthorlabs/sag` exist.
- No secret `sag-master-key`, `sag-tls-cert`, `sag-tls-key`, or `sag-aws-credentials` exists. No secret has a `sag.resource` label.
- `<cfg>`, `~/Library/LaunchAgents/net.kanthorlabs.sag.plist`, and `~/Library/Logs/sag/` do not exist.

Run recovery:

- `dxh` writes the marker `.dev/local/devx-run-active` at the start of a run. It removes the marker after the last teardown.
- When a precondition fails and the marker exists, `dxh` runs the teardown one time and checks again.
- When a precondition fails and no marker exists, the run exits 1 and changes nothing.
- `make e2e-devx` removes a lock whose owner PID does not run.

Teardown:

- `dxh` runs the PRD-devx 15 teardown for macOS before and after each group.
- In place of step 1, `dxh` runs `podman secret rm` for each secret with a `sag.resource` label. It never calls `sag credential delete`.
- Before each group except `image`, `dxh` tags `localhost/sag:devx-e2e` as `docker.io/kanthorlabs/sag:main`. The LaunchAgent process has no `SAG_TEST_IMAGE`, so it finds this tag and pulls nothing. Teardown step 7 removes the tag.
- No test calls `launchctl setenv`. No test adds a test key to the LaunchAgent.

Podman store (ruling Q36):

- Each container, volume, secret, network, and image that `dxh` or a test creates has the label `sag.devx-e2e=1`. Test containers run with `--rm`.
- The product objects of PRD-devx 15 have no such label. The preconditions prove that they are absent before the run. The run thus created each of them.
- The teardown removes each labelled container, volume, secret, and network, then the product objects of PRD-devx 15. It keeps the image `localhost/sag:devx-e2e`.
- `dxh` records each image that it pulls by its exact name in `.dev/local/devx-pulled.txt`. An agent removes a pulled image only by a name of this record.
- No task, test, or agent runs `podman system prune`, `podman image prune`, `podman volume prune`, `podman system reset`, or `podman rmi -a`.
- DX-AC7 and DX-AC38 remove their download directory on the host at the end of the criterion.
- Each gate records `podman system df` before and after its run (section 6.1).

Child environment:

- `dxh` builds the environment of each child process from an allow list: `HOME`, the test `PATH`, `TMPDIR`, `USER`, `LANG`, and each `SAG_TEST_*` variable.
- No other variable passes. This removes `CLAUDE_CONFIG_DIR`, `PI_CODING_AGENT_DIR`, `NODE_EXTRA_CA_CERTS`, `XDG_CONFIG_HOME`, `ZDOTDIR`, `SHELL`, and each `CLAUDE*` variable.
- A criterion test sets a variable only when its criterion states it, for example `SHELL=/bin/bash` for DX-AC41.
- In the suites `Main`, `Pins`, and `Mac`, `dxh` sets `CLAUDE_CONFIG_DIR` and `PI_CODING_AGENT_DIR` to new test directories for each group.
- The `User` suite sets neither variable (ruling Q35).

Test `PATH`:

- The test `PATH` holds the test program directory and `/usr/bin:/bin:/usr/sbin:/sbin`.
- `sag` finds `podman` through the search order of PRD-devx 5.3.
- `dxh` checks that `claude`, `pi`, and `tailscale` resolve only to a stub, a fake, or nothing.
- Exception: a criterion that needs a real program gets one new directory with a symbolic link to each real program that it needs. `dxh` adds only that directory to the test `PATH`.
- DX-AC52 gets a link to `tailscale`. The `User` suite gets links to `claude`, `pi`, and `node`.

Who runs what:

| Suite | Run by | Terminal |
|---|---|---|
| `Main`, `Pins`, `Mac` | An agent | Any |
| `User` | Ulrich | Outside each Claude Code session |

- The `User` suite refuses to start while a `claude` process runs. Ulrich exits each Claude Code session first. The coordinator resumes after the run.
- The `User` suite uses the default Claude Code profile: `~/.claude` and `~/.claude.json` (ruling Q35).
- When `CLAUDE_CONFIG_DIR` is set, the `User` suite exits 1 with `unset CLAUDE_CONFIG_DIR; this suite uses the default profile` and changes nothing.
- When `PI_CODING_AGENT_DIR` is set, the `User` suite exits 1 with `unset PI_CODING_AGENT_DIR; this suite uses the default profile` and changes nothing.
- Precondition: `env -u CLAUDE_CONFIG_DIR claude -p "ok"` works. Ulrich checks it before the run.
- No suite, task, or agent changes `~/.ccp`.
- Before the `User` suite, `dxh` copies `~/.claude.json`, `~/.claude/settings.json`, `~/.pi/agent/mcp.json`, and the shell profile to `.dev/local/devx-user/<UTC time>/`, mode 0700. A file that is absent at the copy is recorded as absent.
- After the PRD-devx 15 teardown with steps 9 and 10, `dxh` compares only the SAG entries of PRD-devx 17.1 with the copy. It restores no file.
- A file recorded as absent passes the compare when it is absent or holds no SAG entry.
- On a difference, the run exits 1, keeps the copy directory, and prints its path. Ulrich restores by hand.
- After a successful compare, `dxh` removes the copy directory. It can hold a token of Ulrich.
- DX-AC41 changes `~/.bash_profile`. The test copies it, removes the two lines of PRD-devx 12.5 at the end, and compares.
- DX-AC52 saves `tailscale serve status --json` before its run. After the teardown, the status equals the saved output.

### 2.2 Phase B: Debian machine

- Layer 10 is blocked until Ulrich gives access to the Debian machine.
- At the start of phase B, Ulrich rules the Debian version and the Linux platform scope (PRD-devx 1.4).
- A PRD-devx revision records the ruling before `D10.0` starts.
- The constraints of section 2.1 apply to the Debian machine until Ulrich states other rules.
- The test user needs linger and `sudo -n` (PRD-devx 17.1). DX-AC5 removes and installs Podman on the machine.

## 3. Module layout

New and changed paths. One responsibility per package. The v1 layout of `docs/plan/foundation.md` section 3 stays valid.

| Path | Responsibility | Layer |
|---|---|---|
| `internal/deploy` | `ProductFlags`, `ContainerFlags`, log flags, `ImageDigest`, image reference, `podman run` arguments, Quadlet text. `mounts.go`: secret mounts from `sag.yaml`. | 1, 8 |
| `deploy/embed.go` | Package `deployfiles`: `go:embed pins`. `go:embed` cannot reach a parent directory, so the embed lives in `deploy/`. | 3 |
| `deploy/pins` | Pin file of PRD-devx 3 | 3 |
| `deploy/Containerfile` | Copies `deploy/embed.go` and `deploy/pins` into the build stage | 3 |
| `internal/secret` | `SecureStorage`, `Metadata`, the three errors, reference parse (`secret:`, `aws-sm:`), the v1 file load | 1, 8 |
| `internal/secret/podman` | Podman backend | 1 |
| `internal/secret/awssm` | AWS Secrets Manager backend | 8 |
| `internal/podman` | Host Podman command runner: stored path, standard input for values, `inspect`, `ps`, `logs`, `exec` | 1 |
| `internal/hoststate` | `<cfg>` paths, lock, `answers.json`, `podman` path search | 2 |
| `internal/prompt` | `/dev/tty` open, `[y/N]` prompt, hidden line read, answer resolution (flag, stored answer, prompt, exit 3) | 2, 4 |
| `internal/stepline` | Step line writer `<status> <item> <detail>` | 2 |
| `internal/testhook` | Test hooks. `testhook_devxtest.go` (build tag `devxtest`) reads `SAG_TEST_*`. `testhook_product.go` (`!devxtest`) holds no-op hooks. | 2 |
| `internal/masterkey` | Creation of `sag-master-key` | 2 |
| `internal/endpointcert` | CA and server certificate, renewal sequence of PRD-devx 5.7 | 2 |
| `internal/image` | Image pull when absent; image selection of PRD-devx 4.3 | 2, 7 |
| `internal/service` | `service start` and `service stop`, `/healthz` wait. macOS: `podman run`, LaunchAgent. Linux: user Quadlet file, `systemctl --user`. | 2, 10 |
| `internal/bootstrap` | Bootstrap plan and apply, steps 1 to 15 | 2, 3, 6 |
| `internal/pins` | Pin parse and selection | 3 |
| `internal/deps` | Dependency rule. macOS: pkg download and check, Podman machine. Linux: apt install. | 3, 10 |
| `internal/tailscale` | Tailscale detection, serve status parse, serve rule. Linux: operator step. | 3, 10 |
| `internal/portcheck` | Holder of `127.0.0.1:8443`. macOS: `lsof`. Linux: `ss`. | 3, 10 |
| `internal/credential` | `credential create`, `list`, `inspect`, `delete` flows, backend selection | 4, 5, 8 |
| `internal/agentsetup` | `agent setup` for Claude Code and pi, JSON write rule, assets, remote mode | 6 |
| `internal/cli` | One file per command. New: `mode.go`, `bootstrap.go`, `service.go`, `credential.go`, `credential_check.go`, `logs.go`, `audit_read.go`, `host_forms.go`, `agent_setup.go`, `agent_issue.go`. `D2.0` creates each of them. | 2 to 6 |
| `e2e/devx` | DX e2e suites, build tag `devxe2e`: `acs.go` (53 rows), one file `suite_<suite>_test.go` per suite, one file `dxac<NN>_test.go` per criterion | 1 to 10 |
| `e2e/devx/dxh` | DX harness: host binary runner, pseudo-terminal driver, child environment, test `PATH`, preconditions, teardown, group `Ensure` helpers, `User` suite copy and compare | 1, 2, 4, 6, 8, 9, 10 |
| `e2e/devx/cmd/fakeharness` | Stub `claude` and `pi`, selected by `argv[0]` | 6 |
| `e2e/devx/cmd/faketailscale` | Fake `tailscale` of PRD-devx 17.1 | 3 |
| `e2e/devx/cmd/fakeinstall` | Test install command of the test pins. It records its argument and runs no `sudo`. | 3 |
| `.github/workflows/release.yml` | Release workflow of PRD-devx 4.2 | 7 |

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
- `make e2e-devx-unit` and CI run `node --test "internal/agentsetup/assets/pi/*.test.ts"` with Node 24 and its default type strip.
- A Go unit test `TestDXAC49_SkillNamesToolsAndCodes` checks that the skill body holds each tool name and each error code of PRD 13.3. The test reads them from the Go constants.

## 4. Code facts at the base

These facts hold at commit `715799c`. The named layer changes each one.

| Fact | Layer |
|---|---|
| `docs/PRD.md` is revision 16 and has none of the PRD-devx 16 rows. | 0 |
| `internal/config/config.go` rejects an empty `resources` map (`must hold at least one resource`). | 1 |
| `e2e/harness/flags.go` holds `ProductFlags`, `ContainerFlags`, and `TestOnlyFlags` with `--log-driver k8s-file`. `e2e/ac01_test.go`, `e2e/ac32_test.go`, `e2e/harness/quadlet_test.go`, and `e2e/harness/flags_test.go` use them. | 1 |
| `deploy/sag.container` has `Image=localhost/sag:latest` and no log keys. | 1 |
| `internal/cli/cli.go` has exit codes 0, 1, and 2 only. Code 3 is absent. | 2 |
| Existing tests in `internal/cli/*_test.go` call `cli.Run` with `serve`, `jwt generate`, and `extension approve`. | 2 |
| `deploy/Containerfile` copies only `go.mod`, `go.sum`, `cmd`, and `internal`. `internal/daemon/containerfile_test.go` checks the file. | 3 |
| `go.mod` has no direct `golang.org/x/term`. `golang.org/x/sys` is only indirect. | 4 |
| `internal/adapter/postgres/preflight.go`, `internal/cli/extension.go`, and `internal/cli/jwt_generate.go` hold the text `podman exec -it`. `e2e/ac11_test.go` expects that hint text. | 5 |
| `internal/cli/jwt_generate.go` checks only standard output for a terminal. | 5 |
| `internal/cli/extension.go` revoke behavior without an approval record differs from PRD-devx 16 row 9.2. The task verifies it. | 5 |
| `internal/scrub.Scrubber` is an immutable value from `scrub.New`. PRD-devx 11.4 needs an add at `Load` time. | 8 |
| `go.mod` has no AWS SDK module. | 8 |
| `.github/workflows/ci.yml` has the jobs `unit`, `build-macos`, `e2e`, and `ac35`. Each job grants only `contents: read`. | 1, 7 |
| `Makefile` has the `dev-up` and `dev-down` targets. `scripts/` holds `dev-up.sh` and `dev-down.sh`. The container `sag-dev` publishes `127.0.0.1:8443`. | 0, 9 |

## 5. Test environment

PRD-devx 17.1 is the source. This section adds the decisions of the plan.

### 5.1 DX e2e suites and groups

Terms:

- Suite: one top-level Go test `TestDevx<Suite>`. `SUITE=<Suite>` of `make e2e-devx` selects it.
- Group: one subtest `TestDevx<Suite>/<group>`. The PRD-devx 15 teardown runs before and after each group.
- Criterion test: one subtest `TestDevx<Suite>/<group>/DXAC<NN>_<Name>`.
- Group setup: one function per group. It runs after the teardown before the group.

Group setup rule:

- The group setup creates the state that the criteria of the group need, for example a bootstrapped host or the canary resource.
- Each criterion test calls the `dxh` `Ensure` helper of each state that it needs. An `Ensure` helper creates the state only when it is absent.
- A criterion test never needs the effect of another criterion subtest. The exception is an order that PRD-devx 17 states inside one criterion.
- Each criterion that `RUN` selects thus passes alone.
- The contract task of each layer adds the groups of the layer to the suite file and owns the suite file in that layer.

| `Ensure` helper | File | State | Owner |
|---|---|---|---|
| `EnsureBootstrapped` | `dxh/ensure_bootstrap.go` | `bootstrap --autostart yes --agent-setup claude-code=no --agent-setup pi=no` completed | `D2.3` |
| `EnsureCanary` | `dxh/ensure_canary.go` | Bootstrapped host, PostgreSQL container, and the canary resource of DX-AC9 | `D4.2` |
| `EnsureAgentSetup` | `dxh/ensure_agent.go` | Bootstrapped host, and agent setup of the stubs `claude` and `pi` as DX-AC39 and DX-AC41 | `D6.3` |
| `EnsureLocalStack` | `dxh/localstack.go` | LocalStack container pinned by digest and `storage.aws` in `sag.yaml` | `D8.3` |

| Suite | Phase | Run by | Groups in order, with their criteria in order |
|---|---|---|---|
| `Main` | A, B | Agent | `base`: DX-AC1, DX-AC2, DX-AC3. `noterm`: DX-AC4. `guards`: DX-AC33, DX-AC36. `service`: DX-AC27, DX-AC28. `tailscale`: DX-AC50. `port`: DX-AC53. `credential`: DX-AC9, DX-AC10, DX-AC11 to DX-AC17, DX-AC37. `cert`: DX-AC29. `delete`: DX-AC18 to DX-AC23. `hostforms`: DX-AC24, DX-AC25. `agent`: DX-AC39, DX-AC41, DX-AC40, DX-AC26, DX-AC42, DX-AC43, DX-AC45, DX-AC51. `bootagent`: DX-AC46. `image`: DX-AC30. `aws`: DX-AC31, DX-AC32. |
| `Pins` | A, B | Agent | `pins`: DX-AC6, DX-AC7, DX-AC8 |
| `Mac` | A | Agent | `macboot`: DX-AC34. `maccred`: DX-AC35. `macpkg`: DX-AC38. `mactailscale`: DX-AC52. |
| `User` | A | Ulrich | `harness`: the default path case of DX-AC39 (`DXAC39_DefaultPath`), DX-AC47, DX-AC48 |
| `Install` | B | Agent | `install`: DX-AC5 |

- Package `e2e/devx`. Its test files have the build tag `devxe2e`. `make e2e` does not build them, so no v1 run touches the real `HOME`.
- `e2e/devx/acs.go` has no build tag, so `acreport` imports the table.
- A layer adds its groups. A group of a later layer is absent.
- The suites run the test build `bin/sag-devxtest`. DX-AC33 also uses the product build `bin/sag`. DX-AC30 uses two test builds (`D7.1`).
- `SAG_TEST_IMAGE` names `localhost/sag:devx-e2e`. No v1 target builds this tag, so the DX run lock protects it.
- `dxh` builds the harness stubs, the fake `tailscale`, and `fakeinstall` into the test program directory.
- DX-AC44 has no Go subtest. `make e2e-devx-unit` runs the Node tests and writes a TAP file.
- DX-AC49 is the Go unit test `TestDXAC49_SkillNamesToolsAndCodes`. `make e2e-devx-unit` runs it with `go test -json`.

### 5.2 Makefile targets

| Target | Function | Layer |
|---|---|---|
| `build-devxtest` | `go build -tags devxtest -o bin/sag-devxtest ./cmd/sag` | 1 |
| `e2e-devx` | Variables `SUITE` (default `Main`), `RUN` (criterion pattern, default all), `LAYER`. Wait for the DX run lock with `mkdir`, and write the file `owner`. Remove a lock whose owner PID does not run. Then `make build build-devxtest`, and build `localhost/sag:devx-e2e` from `deploy/Containerfile` with the label `sag.devx-e2e=1`. Run `go test -tags devxe2e -json -count=1 -timeout 90m -run '^TestDevx$(SUITE)$$/.*/$(RUN)' ./e2e/devx/`. Write `e2e/report/devx-$(SUITE)/events.json` and the commit hash. Release the lock on each exit path. Run `acreport -suite devx -layer $(LAYER)`. | 1 |
| `e2e-devx-unit` | `node --test --test-reporter=tap "internal/agentsetup/assets/pi/*.test.ts"` to `e2e/report/devx-unit/node.tap`, and `go test -json -count=1 -run '^TestDXAC49_' ./internal/agentsetup/` to `e2e/report/devx-unit/events.json`. Then `acreport -suite devx -layer $(LAYER)`. | 6 |

- Example: `make e2e-devx SUITE=Main RUN=DXAC04_ LAYER=2` runs DX-AC4 only.
- `PHASE` (default `A`) passes the phase to `acreport`.

### 5.3 CI jobs

No CI job is a gate (PRD-devx 17). A CI result is information for Ulrich.

| Job | Runner | Content | Layer |
|---|---|---|---|
| `unit`, `build-macos`, `e2e`, `ac35` | v1 | Unchanged | - |
| `devx-unit` | `ubuntu-24.04` | `go vet -tags devxtest ./...`, `go vet -tags devxe2e ./e2e/devx/...`, `go test -count=1 -tags devxtest ./internal/testhook/...`. From layer 6: `actions/setup-node` with Node 24, then `node --test "internal/agentsetup/assets/pi/*.test.ts"`. | 1, 6 |
| `image-main` | `ubuntu-24.04` and `ubuntu-24.04-arm` | On a push to `main` only. `needs: [unit, e2e, ac35, devx-unit]`. Build and push `docker.io/kanthorlabs/sag:main`. | 7 |

- The `unit` job runs `make test`. It thus runs the unit tests of the stubs, the fake `tailscale`, and the fake runners.
- Each job keeps `contents: read`. `image-main` reads only the secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`.
- Each devx job sets `CGO_ENABLED: "1"` and pins each action by commit SHA.

### 5.4 AC report

- `e2e/devx/acs.go` holds 53 rows: `ID` (`DX-AC01` to `DX-AC53`), `Run` (`A`, `AB`, or `B`), `RunBy` (`agent`, `ulrich`, or both), and `Tests`.
- Each element of `Tests` has a name and a layer. DX-AC39 has `DXAC39_ClaudeCodeSetup` (layer 6) and `DXAC39_DefaultPath` (layer 9).
- `acreport -suite devx -layer <N> -phase <A|B>` reads each `e2e/report/devx-*/events.json` and `e2e/report/devx-unit/node.tap`.
- A row is `PASS` when each of its tests with layer `<= N` passes. A row without the phase in `Run` is `OTHER_PHASE`.
- Gate rule: each row of the phase with a test of layer `<= N` is `PASS`, and each events file has the same commit hash.
- Layer 9 exception: the `User` events file can have an earlier commit. The diff from that commit changes only `scripts/`, the `dev-up` and `dev-down` targets of `Makefile`, and documents. Else Ulrich runs the `User` suite again.
- The report adds the block `env`: date, OS version, Podman version, and, for the `User` and `Mac` suites, the Claude Code, pi, and Tailscale versions. It never records an IP address.
- The report writes `e2e/report/devx/ac-report.md` and `ac-report.json`.

## 6. Layer gate

### 6.1 Phase A gate

The v1 gate (`docs/plan/README.md` section 7) applies on `feat/devx` with these steps in place of its steps 3 and 4:

1. In `.dev/devx`, run `podman system df` and record the output. Run `make test` and `make e2e-matrix LAYER=6 PGS="14 17 19"`. Expected: exit 0 (v1 regression).
2. From layer 2, run `make e2e-devx SUITE=Main LAYER=<N>`. From layer 3, also run `make e2e-devx SUITE=Pins LAYER=<N>`. From layer 6, also run `make e2e-devx-unit LAYER=<N>`.
3. In layer 9, also run the steps of `D9.6`: `make e2e-devx SUITE=Mac LAYER=9` and the `User` run of Ulrich.
4. Read `e2e/report/devx/ac-report.md`. Expected: the gate rule of section 5.4 holds. Run `podman system df` and record the output.
5. If a step fails, fix the cause in `.dev/devx` and run the step again.

- The review diff is `git diff sag-devx-<N-1> > ../local/devx-layer-<N>.diff`.
- The review sub-agent uses `model: fable` and `effort: high`. It gets the diff, the PRD-devx sections of the layer, the AC report, and the layer file. It counts as one agent of the pool.
- After the review fixes, run steps 1 to 4 again. This second run is part of the gate.
- The review file appends the final `ac-report.md` table, the `env` block, and the two `podman system df` outputs of the second run.
- A growth of the reclaimable size from the previous gate to this gate is a defect of the layer.
- The gate closes when the tag `sag-devx-<N>` exists and the gate rule holds.

### 6.2 Phase B gate

The phase A gate applies on the Debian machine, with these differences:

- Step 1 runs `make test` and `make e2e LAYER=6 PG=17`.
- Step 2 runs the suites `Main`, `Pins`, and `Install` with `PHASE=B`, and `make e2e-devx-unit LAYER=10 PHASE=B`.
- Step 3 does not apply.

## 7. Task graph

Notation of `docs/plan/README.md` section 3. "U" marks a step that Ulrich runs.

```
Layer 0  Plan bootstrap, PRD.md changes, harness form record
  D0.1 D0.2 D0.3(U)

Phase A, macOS development machine

Layer 1  Base: deploy package, SecureStorage, DX harness (no DX-AC)
  D0.1 D0.2 D0.3 -> D1.0 -> D1.1 D1.2 D1.3
  all  -> D1.4

Layer 2  Bootstrap and service on macOS (DX-AC1 2 4 27 28 33 36)
  D1.4 -> D2.0 -> D2.1 D2.2 D2.3 D2.4
  D2.2 (verify after D2.1)
  D2.3 (verify after D2.2, D2.4)
  all  -> D2.5

Layer 3  Dependencies, Tailscale serve, port check (DX-AC3 6 7 8 50 53)
  D2.5 -> D3.0 -> D3.1 D3.2 D3.3
  D3.3 (verify after D3.1, D3.2)
  all  -> D3.4

Layer 4  Credential create, list, inspect (DX-AC9-17 29 37)
  D3.4 -> D4.0 -> D4.1 D4.2 D4.3
  D4.2 (verify after D4.1)
  D4.3 (verify after D4.2)
  all  -> D4.4

Layer 5  Credential delete, host forms, logs (DX-AC18-25)
  D4.4 -> D5.0 -> D5.1 D5.2 D5.3
  D5.1 (verify after D5.2)
  all  -> D5.4

Layer 6  Agent setup and bootstrap step 14 (DX-AC26 39-46 49 51)
  D5.4 -> D6.0 -> D6.1 D6.2 D6.3 D6.4 D6.5
  D6.1 (verify after D6.2)
  D6.3 (verify after D6.1, D6.2)
  D6.4 (verify after D6.1)
  D6.5 (verify after D6.1, D6.3)
  all  -> D6.6

Layer 7  Image selection and release workflow (DX-AC30)
  D6.6 -> D7.0 -> D7.1 D7.2
  all  -> D7.3

Layer 8  AWS Secrets Manager backend (DX-AC31 32)
  D7.3 -> D8.0 -> D8.1 D8.2 D8.3
  D8.2 (verify after D8.1, D8.3)
  D8.3 (verify after D8.1)
  all  -> D8.4

Layer 9  macOS criteria, real harnesses, script removal (DX-AC34 35 38 47 48 52, DX-AC39 default path)
  D8.4 -> D9.0 -> D9.1 D9.2 D9.3 D9.4 D9.5
  D9.3 (verify after D9.1)
  all  -> D9.6(U)

Phase B, Debian machine (blocked on access)

Layer 10 Linux port (DX-AC5, phase B clauses, phase B run of each A, B criterion)
  D9.6 + access + ruling -> D10.0 -> D10.1 D10.2 D10.3
  D10.2 (verify after D10.1)
  D10.3 (verify after D10.1, D10.2)
  all  -> D10.4
```

Layer N maps to the PRD-devx 19 step: 1→1, 2→2, 3→2, 4→3, 5→4, 6→5, 7→6, 8→7, 9→8 and 9, 10→10. Layers 2 and 3 split step 2. Layer 9 closes steps 8 and 9.

## 8. AC map

Each DX-AC has exactly one task. Run by: "A" is an agent, "U" is Ulrich.

| AC | Phase | Layer | Task | Test | Suite | Run by |
|---|---|---|---|---|---|---|
| DX-AC1 | A, B | 2 | `D2.3` | `DXAC01_Bootstrap` | `Main` | A |
| DX-AC2 | A, B | 2 | `D2.3` | `DXAC02_ContainerAndCert` | `Main` | A |
| DX-AC3 | A, B | 3 | `D3.3` | `DXAC03_BootstrapIdempotent` | `Main` | A |
| DX-AC4 | A, B | 2 | `D2.2` | `DXAC04_BootstrapNoTerminal` | `Main` | A |
| DX-AC5 | B | 10 | `D10.1` | `DXAC05_PodmanAptInstall` | `Install` | A |
| DX-AC6 | A, B | 3 | `D3.1` | `DXAC06_InstallApproval` | `Pins` | A |
| DX-AC7 | A, B | 3 | `D3.1` | `DXAC07_ReleaseChecksum` | `Pins` | A |
| DX-AC8 | A, B | 3 | `D3.1` | `DXAC08_VersionRule` | `Pins` | A |
| DX-AC9 | A, B | 4 | `D4.2` | `DXAC09_CreateCanary` | `Main` | A |
| DX-AC10 | A, B | 4 | `D4.3` | `DXAC10_CanaryNoLeak` | `Main` | A |
| DX-AC11 | A, B | 4 | `D4.2` | `DXAC11_CreateRejectsForm` | `Main` | A |
| DX-AC12 | A, B | 4 | `D4.1` | `DXAC12_ConnectionCheck` | `Main` | A |
| DX-AC13 | A, B | 4 | `D4.1` | `DXAC13_LoopbackWarn` | `Main` | A |
| DX-AC14 | A, B | 4 | `D4.2` | `DXAC14_CreateStdin` | `Main` | A |
| DX-AC15 | A, B | 4 | `D4.2` | `DXAC15_CreateSecretExists` | `Main` | A |
| DX-AC16 | A, B | 4 | `D4.2` | `DXAC16_CreateRollback` | `Main` | A |
| DX-AC17 | A, B | 4 | `D4.3` | `DXAC17_ListInspect` | `Main` | A |
| DX-AC18 | A, B | 5 | `D5.1` | `DXAC18_DeleteNoTerminal` | `Main` | A |
| DX-AC19 | A, B | 5 | `D5.1` | `DXAC19_DeleteConfirm` | `Main` | A |
| DX-AC20 | A, B | 5 | `D5.1` | `DXAC20_Delete` | `Main` | A |
| DX-AC21 | A, B | 5 | `D5.1` | `DXAC21_DeleteStopped` | `Main` | A |
| DX-AC22 | A, B | 5 | `D5.1` | `DXAC22_DeleteResume` | `Main` | A |
| DX-AC23 | A, B | 5 | `D5.1` | `DXAC23_DeleteForeignSecret` | `Main` | A |
| DX-AC24 | A, B | 5 | `D5.2` | `DXAC24_HostJWTGenerate` | `Main` | A |
| DX-AC25 | A, B | 5 | `D5.3` | `DXAC25_Logs` | `Main` | A |
| DX-AC26 | A, B | 6 | `D6.3` | `DXAC26_DenyRules` | `Main` | A |
| DX-AC27 | A, B | 2 | `D2.3` | `DXAC27_Autostart` | `Main` | A |
| DX-AC28 | A, B | 2 | `D2.3` | `DXAC28_ServiceStopStart` | `Main` | A |
| DX-AC29 | A, B | 4 | `D4.3` | `DXAC29_CertRenewal` | `Main` | A |
| DX-AC30 | A, B | 7 | `D7.1` | `DXAC30_ImageSelection` | `Main` | A |
| DX-AC31 | A, B | 8 | `D8.3` | `DXAC31_AWSBackend` | `Main` | A |
| DX-AC32 | A, B | 8 | `D8.2` | `DXAC32_AWSUnavailable` | `Main` | A |
| DX-AC33 | A, B | 2 | `D2.1` | `DXAC33_ProductBinary` | `Main` | A |
| DX-AC34 | A | 9 | `D9.2` | `DXAC34_MacBootstrap` | `Mac` | A |
| DX-AC35 | A | 9 | `D9.2` | `DXAC35_MacCredential` | `Mac` | A |
| DX-AC36 | A, B | 2 | `D2.1` | `DXAC36_HostGuards` | `Main` | A |
| DX-AC37 | A, B | 4 | `D4.2` | `DXAC37_CreateTLSCA` | `Main` | A |
| DX-AC38 | A | 9 | `D9.2` | `DXAC38_MacPkgChecksum` | `Mac` | A |
| DX-AC39 | A, B | 6, 9 | `D6.1` | `DXAC39_ClaudeCodeSetup` (layer 6), `DXAC39_DefaultPath` (layer 9) | `Main`, `User` | A, U |
| DX-AC40 | A, B | 6 | `D6.3` | `DXAC40_JWTNoLeak` | `Main` | A |
| DX-AC41 | A, B | 6 | `D6.3` | `DXAC41_PiSetup` | `Main` | A |
| DX-AC42 | A, B | 6 | `D6.3` | `DXAC42_CATrustConflict` | `Main` | A |
| DX-AC43 | A, B | 6 | `D6.3` | `DXAC43_AgentSetupRefusals` | `Main` | A |
| DX-AC44 | A, B | 6 | `D6.2` | `node --test`, TAP file of `e2e-devx-unit` | - | A |
| DX-AC45 | A, B | 6 | `D6.1` | `DXAC45_AgentIssue` | `Main` | A |
| DX-AC46 | A, B | 6 | `D6.5` | `DXAC46_BootstrapAgentSetup` | `Main` | A |
| DX-AC47 | A | 9 | `D9.3` | `DXAC47_RealClaudeCode` | `User` | U |
| DX-AC48 | A | 9 | `D9.3` | `DXAC48_RealPi` | `User` | U |
| DX-AC49 | A, B | 6 | `D6.2` | `TestDXAC49_SkillNamesToolsAndCodes` (unit) | - | A |
| DX-AC50 | A, B | 3 | `D3.2` | `DXAC50_TailscaleFake` | `Main` | A |
| DX-AC51 | A, B | 6 | `D6.4` | `DXAC51_RemoteMode` | `Main` | A |
| DX-AC52 | A | 9 | `D9.4` | `DXAC52_MacTailscale` | `Mac` | A |
| DX-AC53 | A, B | 3 | `D3.3` | `DXAC53_PortHolder` | `Main` | A |

- `D6.1` writes `DXAC39_DefaultPath` in the `User` suite file. Ulrich runs it in layer 9.
- DX-AC29 closes in layer 4 (PRD-devx 19 step 3, ruling Q31). Layer 2 tests the renewal sequence with unit tests and a fake clock.

### 8.1 Phase B clauses

Layer 10 adds the phase B clauses to the test files of phase A. The AC map keeps the phase A task as the owner of the criterion.

| AC | Phase B clause | Task |
|---|---|---|
| DX-AC1 | `systemctl --user list-dependencies default.target` holds `sag.service` | `D10.2` |
| DX-AC2 | The checks for the container of the user Quadlet unit | `D10.2` |
| DX-AC8 | `apt` test item with an older candidate | `D10.1` |
| DX-AC10 | The Quadlet file and the user journal | `D10.2` |
| DX-AC27 | The Quadlet `[Install]` section and `list-dependencies` | `D10.2` |
| DX-AC36 | Each host command as UID 0 exits 1 | `D10.3` |
| DX-AC40 | The user journal | `D10.2` |
| DX-AC41 | `~/.bashrc` | `D10.3` |
| DX-AC50 | The Quadlet `PublishPort=` and the operator step | `D10.3` |
| DX-AC53 | The process name from `ss` | `D10.3` |

Each other criterion with the run `A, B` runs again without change in the `D10.4` gate.

## 9. Estimate

Aelita estimate in hours, not a commitment. Wall-clock is contract + parallel phase + integration. The integration estimate holds two gate runs (before and after the review fixes), one review cycle, and the fixes.

| Layer | Contract | Parallel tasks (start order) | Parallel phase | Gate run | Integration | Wall-clock |
|---|---|---|---|---|---|---|
| 0 | - | `D0.2` 1.5, `D0.3` 0.5 (Ulrich) | 1.5 | - | - | 1.5 |
| 1 | `D1.0` 1 | `D1.3` 4, `D1.1` 3, `D1.2` 2.5 | 4 | 1 | `D1.4` 4 | 9 |
| 2 | `D2.0` 1.5 | `D2.3` 4, `D2.2` 2.5, `D2.1` 2, `D2.4` 2.5 | 5 | 1.25 | `D2.5` 4.5 | 11 |
| 3 | `D3.0` 1 | `D3.1` 3.5, `D3.2` 3, `D3.3` 2.5 | 4 | 1.5 | `D3.4` 5 | 10 |
| 4 | `D4.0` 1 | `D4.2` 5, `D4.3` 3, `D4.1` 2.5 | 5.5 | 1.75 | `D4.4` 5.5 | 12 |
| 5 | `D5.0` 1 | `D5.1` 4, `D5.2` 2.5, `D5.3` 1.5 | 4 | 2 | `D5.4` 6 | 11 |
| 6 | `D6.0` 1.5 | `D6.1` 5, `D6.3` 4, `D6.2` 3, `D6.5` 2.5, `D6.4` 2.5 | 8 | 2.25 | `D6.6` 6.5 | 16 |
| 7 | `D7.0` 0.5 | `D7.2` 3, `D7.1` 3 | 3 | 2.5 | `D7.3` 6.5 | 10 |
| 8 | `D8.0` 1.5 | `D8.1` 3.5, `D8.2` 3.5, `D8.3` 3.5 | 4 | 2.75 | `D8.4` 7 | 12.5 |
| 9 | `D9.0` 1 | `D9.1` 3, `D9.2` 3, `D9.3` 3, `D9.4` 2, `D9.5` 1.5 | 5 | 3.25 | `D9.6` 8.5 | 14.5 |
| Phase A | | | | | | 107.5 |
| 10 | `D10.0` 2 | `D10.2` 4, `D10.1` 3.5, `D10.3` 3 | 5 | 2 | `D10.4` 8 | 15 |
| Total | | | | | | 122.5 |

| Item | Value |
|---|---|
| Gate run | v1 matrix (about 45 min) and the DX suites of the layer, in series |
| Wall-clock with the pool of 3 agents | about 108 h for phase A, about 15 h for phase B |
| Agent-hours | about 180 h |
| Serial floor | about 75 h: 10 contract tasks, 10 integration tasks with two gate runs each, and layer 0 |
| Pushes | None needed by a gate |
| Ulrich time | `D0.2` approval, `D0.3` (about 20 min), the `User` run of `D9.6` (about 30 min), the phase B access and ruling, each review approval |

## 10. Risks

1. Phase A proves no Linux path. Linux defects show only in layer 10. Its estimate has the least evidence.
2. Phase B waits for the Debian machine and a ruling. Debian 12 ships Podman 4.3.1, which is older than the Linux pin 4.9.3. The ruling can change the Linux pin, PRD-devx 1 and 3, and DX-AC5.
3. 3 agents, the DX suites, and the v1 matrix share `podman-machine-default` (4 CPU, 8 GiB). The DX run lock makes the DX runs wait for each other. The free-space precondition of section 2.1 stops a DX run below 10 GiB. Ulrich frees other space himself.
4. The DX suites change real user state on the machine: `<cfg>`, the LaunchAgent, the container `sag`, port 8443, and `~/.bash_profile`. A crashed run can leave state. The next run removes it through the marker of section 2.1. Without the marker, the precondition check stops and names it.
5. DX-AC10 scans the real `$HOME` of Ulrich for a private key. The scan reads only files that changed after the start of the group. A file older than the group cannot hold a key of that group. The scan has a limit of 5 min and prints no file content.
6. DX-AC52 changes the real Tailscale serve configuration of Ulrich during its run. The teardown and the saved serve status check it.
7. The `User` suite detects a Claude Code process by the command name `claude`. A Claude Code process with another command name is not detected. `D9.1` also matches a `node` process whose arguments hold `claude`.
8. The stub formats follow one recorded line of Claude Code 2.1.332 (`D0.3`). A new harness version can change the format. The pins in PRD-devx 1 hold the versions.
9. `host.containers.internal` on the Podman machine reaches a published port of the macOS host only through `gvproxy`. `D4.0` confirms it for DX-AC9 before the tasks start.
10. DX-AC30 proves the digest pull only from a local registry through `SAG_TEST_REGISTRY` (ruling Q34). The first real release proves the pull from `docker.io`.
11. DX-AC32 needs an AWS endpoint that drops packets. `D8.0` confirms a method on the machine, for example `podman pause` of the LocalStack container. No method changes `podman-machine-default`.
12. The UID 0 clause of DX-AC36 needs `sudo -n`. The development machine does not give it, so the clause runs only in phase B. Phase A has the unit test `TestUID0Refused`.
13. Probe evidence (plan review, Ubuntu 24.04 container, arm64, Podman 4.9.3): `podman container inspect` lists the secrets, and `apt-get remove podman` keeps the rootless store. `D1.2` confirms the secret list on the development machine. `D10.0` confirms both on the Debian machine.
14. The release workflow and the `image-main` job run for real only on a tag and on a push to `main`. Before that, only `actionlint` and DX-AC30 prove them. Docker Hub secrets must exist first.
15. The symbol check of DX-AC33 fails when a product symbol holds the text `devxtest`. Section 1 forbids the name.
16. Devx v1 does not run the PRD-devx 18.1 check list (ruling Q32). R20 stays. `docs/reviews/devx-layer-9.md` records "18.1 not run; no macOS machine without Podman".

Cut order if time runs short. No cut removes a security control or its AC:

1. Skip the v1 regression of gate step 1 in layers 3, 5, 7, and 9. These layers change no daemon code.
2. Defer the `image-main` job. Ulrich pushes the `main` image by hand one time.

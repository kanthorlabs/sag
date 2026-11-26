# Layer 9: macOS criteria, real harnesses, removal of the development scripts

| Item | Value |
|---|---|
| Phase | A (last layer) |
| Goal | The run-`A` criteria prove the macOS install check, the macOS start at login, the real harness formats, and the real `tailscale serve`. The devx commands replace the development scripts. The machine state of Ulrich is the same after the runs. |
| ACs | DX-AC34, DX-AC35, DX-AC38, DX-AC47, DX-AC48, DX-AC52, and the default path case of DX-AC39 |
| PRD-devx sections | 5.3 (macOS), 5.5 (macOS), 5.8, 6 (macOS), 14, 15, 17.1 (phase A), 18 R20, R24, R28, 19 steps 8 and 9 |
| Gate | `D9.6`: README section 6.1 with step 3. Final gate of phase A. |
| Ulrich runs | The `User` suite in `D9.6`, from a terminal outside each Claude Code session |
| Agents at once | 3. Start order: `D9.1`, `D9.2`, `D9.3`, then `D9.4`, `D9.5` |
| Wall-clock | `D9.0` 1 h + parallel phase 5 h + `D9.6` 8.5 h = 14.5 h |

The constraints of README section 2.1 apply to each task of this layer.

## File ownership

| Task | Owns |
|---|---|
| `D9.0` | `e2e/devx/suite_mac_test.go`, stubs of `e2e/devx/dxh/userrun.go` and `e2e/devx/dxh/versions.go` |
| `D9.1` | `e2e/devx/dxh/userrun.go`, `e2e/devx/dxh/userrun_test.go`, `e2e/devx/dxh/versions.go`, `e2e/devx/dxh/versions_test.go`, `e2e/devx/suite_user_test.go` |
| `D9.2` | `e2e/devx/dxac34_test.go`, `dxac35_test.go`, `dxac38_test.go` |
| `D9.3` | `e2e/devx/dxac47_test.go`, `dxac48_test.go`, `e2e/devx/dxh/realharness.go`, `e2e/devx/dxh/realharness_test.go` |
| `D9.4` | `e2e/devx/dxac52_test.go`, `e2e/devx/dxh/tailscale.go`, `e2e/devx/dxh/tailscale_test.go` |
| `D9.5` | `scripts/dev-up.sh`, `scripts/dev-down.sh`, `Makefile` (targets `dev-up` and `dev-down`), `docs/plan/dev-setup.md` |
| `D9.6` | Every layer-9 file after `D9.1` to `D9.5` merge. `docs/reviews/devx-layer-9.md`. |

## D9.0 Contract

- Goal: Freeze the `User` run guard API, the version record API, and the `Mac` suite.
- Owns: see the ownership table.
- Reads: README section 2.1; PRD-devx 15, 17.1.
- Depends on: `D8.4`.
- Content:
  - `userrun.go`: `CheckNoClaude(ctx) error`, `Snapshot(dir) (Snapshot, error)`, `CompareSAGEntries(Snapshot) error`.
  - `versions.go`: `Record(ctx) (Env, error)` with the OS, Podman, Claude Code, pi, and Tailscale versions.
  - `suite_mac_test.go`: `TestDevxMac` with the groups `macboot`, `maccred`, `macpkg`, `mactailscale`.
  - The removal list of PRD-devx 14: `scripts/dev-up.sh`, `scripts/dev-down.sh`, and the `dev-up` and `dev-down` targets of `Makefile`. `deploy/dev/sag.yaml` stays.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check && grep -rn 'dev-up\|dev-down\|sag-dev-pg' --include='*.go' --include='*.yml' --include=Makefile .`. Expected: exit 0, and the search finds only the `Makefile` targets.
- Done when: `D9.1` to `D9.5` compile against the stubs.

## D9.1 `User` run guard

- Goal: Stop the `User` suite on an unsafe state. Copy and compare the harness files of Ulrich without a restore.
- Owns: see the ownership table.
- Reads: README section 2.1; PRD-devx 15, 17.1; R24.
- Depends on: `D9.0`.
- Method:
  - `CheckNoClaude`: when a process with the command name `claude` runs, or a `node` process whose arguments hold `claude`, exit 1 and change nothing.
  - Profile check (ruling Q35): when `CLAUDE_CONFIG_DIR` is set, exit 1 with `unset CLAUDE_CONFIG_DIR; this suite uses the default profile` and change nothing. When `PI_CODING_AGENT_DIR` is set, exit 1 with `unset PI_CODING_AGENT_DIR; this suite uses the default profile`.
  - The `User` suite uses `~/.claude` and `~/.claude.json`. No code path reads or writes `~/.ccp`.
  - The test `PATH` of the `User` suite holds one new directory with links to the real `claude`, `pi`, and `node` (README section 2.1).
  - `Snapshot`: copy `~/.claude.json`, `~/.claude/settings.json`, `~/.pi/agent/mcp.json`, and the shell profile to `.dev/local/devx-user/<UTC time>/`, mode 0700. Record a file that is absent as absent.
  - `userrun.go` runs PRD-devx 15 steps 9 and 10 on the real harness files after the teardown steps of `dxh`.
  - After the teardown, `CompareSAGEntries` compares only the SAG entries of PRD-devx 17.1 with the copy. A file recorded as absent passes when it is absent or holds no SAG entry. It restores no file.
  - On a difference, exit 1, keep the copy directory, and print its path. After a successful compare, remove the copy directory.
  - The `User` suite calls `CheckNoClaude`, the profile check, the preconditions, and `Snapshot` before the group `harness`.
  - The runner masks the Tailscale IP in each output line with `<tailscale IP>`.
- Tests first: `userrun_test.go` with fake commands and a test `HOME`: `TestRefusesClaudeProcess`, `TestRefusesNodeClaudeProcess`, `TestRefusesClaudeConfigDir`, `TestRefusesPiCodingAgentDir`, `TestTeardownHarnessSteps`, `TestAbsentFileSnapshot`, `TestCompareSAGEntriesOnly`, `TestDifferenceKeepsCopy`, `TestNoRestore`, `TestCopyMode0700`, `TestMasksTailscaleIP`; `versions_test.go`: `TestEnvHasNoIP`.
- ACs: none.
- Verify: `go test -count=1 ./e2e/devx/dxh/... && go vet -tags devxe2e ./e2e/devx/...`. Expected: `ok`, then exit 0.
- Done when: no code path of the guard writes a harness file of Ulrich.

## D9.2 macOS bootstrap, credential, and pkg check

- Goal: Prove bootstrap with the installed Podman, the LaunchAgent, the credential commands, and the pkg checksum on macOS.
- Owns: see the ownership table.
- Reads: PRD-devx 3, 5.3, 5.5, 6, 7 to 9, 17.1.
- Depends on: `D9.0`.
- Method:
  - DX-AC34: `bootstrap --autostart yes --agent-setup claude-code=no --agent-setup pi=no` with the installed Podman and without `tailscale` in the test `PATH`. Check `launchctl print`. After `sag service stop`, `launchctl kickstart -k` gives `/healthz` 200 within 30 s.
  - DX-AC35: a PostgreSQL container on a free host port. `credential create`, `list`, and `delete` as DX-AC9, DX-AC17, and DX-AC20.
  - DX-AC38: `SAG_TEST_ABSENT=podman` and `SAG_TEST_STOP_BEFORE_INSTALL`. The test downloads the pinned pkg and never reaches `installer`.
- Tests first: none. The criteria are e2e only.
- e2e: `DXAC34_MacBootstrap`, `DXAC35_MacCredential`, `DXAC38_MacPkgChecksum` in the suite `Mac`.
- ACs: DX-AC34, DX-AC35, DX-AC38.
- Verify: `make e2e-devx SUITE=Mac RUN='DXAC3[458]_' LAYER=9`. Expected: exit 0.
- Done when: no test runs `installer`, `podman machine init`, or `podman machine start`.

## D9.3 Real harnesses

- Goal: Write the tests of the real Claude Code 2.1.332 and pi 1.6.1 formats, the deny rules, and the pi guard.
- Owns: see the ownership table.
- Reads: PRD-devx 12, 17 rows DX-AC47 and DX-AC48, 17.1, R24, R25.
- Depends on: `D9.0`. Verify after `D9.1`.
- Method:
  - Check the harness versions first. Another version fails the criterion with `prerequisite missing`.
  - DX-AC47: `claude mcp list` from a shell without `NODE_EXTRA_CA_CERTS`. `claude -p --output-format json --allowedTools "Bash(sag:*)"` for a prompt that asks for `sag credential delete x`. Expect a match in `permission_denials`. Resource `x` does not exist.
  - DX-AC48: `pi mcp list --json` from a new login shell. `pi --mode json` for a prompt that asks for `podman secret rm x`. Expect a tool result with a reason that starts with `sag-guard:`.
  - The test prints no JWT and no harness configuration content.
  - An agent does not run the `User` suite. Ulrich runs it in `D9.6`.
- Tests first: `realharness_test.go`: `TestVersionGate`, `TestOutputHasNoJWT`.
- e2e: `DXAC47_RealClaudeCode`, `DXAC48_RealPi` in the suite `User`.
- ACs: DX-AC47, DX-AC48.
- Verify: `go test -count=1 ./e2e/devx/dxh/... -run 'VersionGate|OutputHasNoJWT' && go vet -tags devxe2e ./e2e/devx/...`. Expected: `ok`, then exit 0.
- Done when: each test changes only the files of the `D9.1` copy list.

## D9.4 Real Tailscale serve

- Goal: Prove the real `tailscale serve --tcp` forward on macOS and its removal.
- Owns: see the ownership table.
- Reads: PRD-devx 5.8, 15, 17 row DX-AC52, 17.1.
- Depends on: `D9.0`.
- Method:
  - Precondition: Tailscale up and no TCP 8443 entry in `tailscale serve status --json`. Else the criterion fails with `prerequisite missing` and changes nothing.
  - Save the serve status before the run. For this criterion only, the test `PATH` holds one new directory with a link to the real `tailscale` (README section 2.1).
  - `curl --cacert <cfg>/ca.pem https://<tailscale IP>:8443/healthz` returns 200. `lsof -nP -iTCP:8443 -sTCP:LISTEN` shows no `*:8443`.
  - After `tailscale serve --tcp 8443 off`, the status has no TCP 8443 entry and the `curl` fails. The status equals the saved status.
  - Never run `tailscale funnel`. Read the address at run time. Write it to no file, log line, or report.
- Tests first: `tailscale_test.go` in `dxh`: `TestNoAddressInOutput`, `TestStatusCompare`.
- e2e: `DXAC52_MacTailscale` in the suite `Mac`.
- ACs: DX-AC52.
- Verify: `go test -count=1 ./e2e/devx/dxh/... -run 'NoAddressInOutput|StatusCompare' && make e2e-devx SUITE=Mac RUN=DXAC52_ LAYER=9`. Expected: `ok`, then exit 0.
- Done when: the serve status after the run equals the saved status.

## D9.5 Script removal and development setup

- Goal: Remove the development scripts and their targets. Rewrite the development setup (PRD-devx 14).
- Owns: see the ownership table.
- Reads: PRD-devx 4.4, 14.
- Depends on: `D9.0`.
- Method: remove the items of the removal list. `docs/plan/dev-setup.md` holds the steps of PRD-devx 4.4, one `sag credential create`, and one `sag agent setup`. It names no JWT in a command argument and no mkcert step.
- Tests first: none. The task changes no Go code.
- ACs: none.
- Verify: `make test && ! make -n dev-up`. Expected: exit 0.
- Done when: `make dev-up` gives `No rule to make target`.

## D9.6 Integration and final phase A gate

- Goal: Run the `Mac` suite and the `User` suite, and close phase A.
- Owns: see the ownership table.
- Depends on: `D9.1` to `D9.5`.
- Steps:
  1. Run README section 6.1 steps 1 and 2.
  2. Run `make e2e-devx SUITE=Mac LAYER=9`.
  3. Tell Ulrich the command and its preconditions. Ulrich exits each Claude Code session and opens a new terminal. He runs `unset CLAUDE_CONFIG_DIR PI_CODING_AGENT_DIR` and checks that `env -u CLAUDE_CONFIG_DIR claude -p "ok"` works. Then he runs `make e2e-devx SUITE=User LAYER=9` in `.dev/devx`.
  4. After Ulrich resumes the coordinator, read `e2e/report/devx-User/events.json` and the report. Check that they hold no IP address, no JWT, and no credential value.
  5. Run the review, the fixes, and the second gate run of README section 6.1. Ulrich runs the `User` suite again only when the exception of README section 5.4 does not hold.
  6. Record in `docs/reviews/devx-layer-9.md`: "18.1 not run; no macOS machine without Podman" (ruling Q32). Append the `env` block.
- ACs: DX-AC34, DX-AC35, DX-AC38, DX-AC47, DX-AC48, DX-AC52, and the default path case of DX-AC39.
- Verify: `e2e/report/devx/ac-report.md` after steps 1 to 5. Expected: each DX-AC with the run `A` or `A, B` is `PASS`. The `User` compare passes.
- Done when: the tag `sag-devx-9` exists. Phase A is complete. The merge of `feat/devx` into `main` is a decision of Ulrich.

# Layer 6: Agent setup and bootstrap step 14

| Item | Value |
|---|---|
| Phase | A |
| Goal | `sag agent setup claude-code` and `sag agent setup pi` register SAG in the local and remote modes, install the skill, the CA trust, and the command control, and verify the connection. Bootstrap sets up each approved harness. |
| ACs | DX-AC26, DX-AC39 (without its default path case), DX-AC40 to DX-AC46, DX-AC49, DX-AC51 (phase A forms) |
| PRD-devx sections | 2.3, 2.4, 5.2 steps 3, 14, 15, 5.4 (agent setup answers), 5.8 (second machine steps), 12, 17.1 (harness stubs, Node) |
| Gate | `D6.6`: README section 6.1 with `make e2e-devx-unit` |
| Agents at once | 3. Start order: `D6.1`, `D6.3`, `D6.2`, then `D6.5`, `D6.4` |
| Wall-clock | `D6.0` 1.5 h + parallel phase 8 h + `D6.6` 6.5 h = 16 h |

## File ownership

| Task | Owns |
|---|---|
| `D6.0` | `internal/agentsetup/agentsetup.go`, `types.go`, `paths.go`, stubs of the files below, `internal/bootstrap/steps.go` (calls of step 14 and step 15), `Makefile` (`e2e-devx-unit`), `.github/workflows/ci.yml` (Node step of `devx-unit`), `e2e/devx/suite_user_test.go`, `e2e/devx/suite_main_test.go`, stub of `e2e/devx/dxh/ensure_agent.go` |
| `D6.1` | `internal/agentsetup/jsonwrite.go`, `register.go`, `jwt.go`, `catrust.go`, `plan.go`, `verify.go`, their tests, `internal/cli/agent_issue.go`, `internal/cli/agent_setup.go`, their tests, `e2e/devx/dxac39_test.go`, `e2e/devx/user_dxac39_test.go`, `e2e/devx/dxac45_test.go` |
| `D6.2` | `internal/agentsetup/assets/*`, `internal/agentsetup/skill_test.go`, `e2e/devx/cmd/fakeharness/*` |
| `D6.3` | `internal/agentsetup/control.go`, `profile.go`, their tests, `e2e/devx/dxh/ensure_agent.go`, `e2e/devx/dxh/ensure_agent_test.go`, `e2e/devx/dxac26_test.go`, `dxac40_test.go`, `dxac41_test.go`, `dxac42_test.go`, `dxac43_test.go` |
| `D6.4` | `internal/agentsetup/remote.go`, `remote_test.go`, `e2e/devx/dxac51_test.go` |
| `D6.5` | `internal/bootstrap/agents.go`, `internal/bootstrap/next.go`, their tests, `internal/cli/bootstrap.go`, `internal/cli/bootstrap_test.go`, `e2e/devx/dxac46_test.go` |
| `D6.6` | Every layer-6 file after `D6.1` to `D6.5` merge. `docs/reviews/devx-layer-6.md`. |

## D6.0 Contract

- Goal: Freeze the agent setup API, the paths, the answer keys, the remote mode call points, and the bootstrap call of agent setup.
- Owns: see the ownership table.
- Reads: PRD-devx 5.2, 12; README section 3.1; `e2e/devx/testdata/harness-forms.txt` (`D0.3`).
- Depends on: `D5.4`.
- Content:
  - `agentsetup.go`: the `go:embed` of `assets/skill/SKILL.md` and `assets/pi/sag-guard.ts` (README section 3.1).
  - `types.go`: `Harness` (`claude-code`, `pi`), `Mode` (local, remote), `Options` (`sub`, `url`, answers), `Run(ctx, Options) error`, `PlanOnly(ctx, Options) (Plan, error)` for bootstrap step 3.
  - `paths.go`: the PRD-devx 12.2 tables and the `CLAUDE_CONFIG_DIR`, `PI_CODING_AGENT_DIR`, and `ZDOTDIR` rules.
  - Remote call points: `plan.go` calls `planRemote`, and `register.go` calls `remoteJWT`. Both live in `remote.go` with `ParseURL(url) (Mode, error)` and `ReadJWT(tty, sub) (secret.Credential, error)`.
  - Bootstrap step 14 calls `agentsetup.Run` with the plan answers under the bootstrap lock. Step 15 calls `next.Print`.
  - `Makefile`: target `e2e-devx-unit` of README section 5.2. `ci.yml`: Node 24 and `node --test` in `devx-unit`.
  - `suite_user_test.go`: `TestDevxUser` with the group `harness`. It refuses to run until `D9.1` adds the guard.
  - `suite_main_test.go`: add the groups `agent` and `bootagent` to `TestDevxMain`, each with its group setup (README section 5.1). `dxh/ensure_agent.go`: `EnsureAgentSetup(t)` stub.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check && podman run --rm -v "$PWD:/repo:ro" -w /repo docker.io/rhysd/actionlint:1.7.9`. Expected: exit 0.
- Done when: `D6.1` to `D6.5` compile against the stubs.

## D6.1 Registration, JSON write rule, JWT issue, CA trust, plan, verification

- Goal: Write `mcpServers.sag` with a fresh or kept JWT. Apply the CA trust. Check the plan conflicts. Verify the connection. Add the container command `sag agent issue`.
- Owns: see the ownership table.
- Reads: PRD-devx 12.1 to 12.5, 12.8; PRD 11.2.
- Depends on: `D6.0`. Verify after `D6.2`.
- Method:
  - JSON write rule: decode only the path of the named key. Keep each other value as `json.RawMessage`. `SetEscapeHTML(false)`. Temporary file and rename in the directory of the link target. Keep the mode. The MCP file ends with mode 0600.
  - Claude Code entry `{"type": "http", "url", "headers"}`. pi entry `{"url", "headers", "exposure": "direct"}`. Never a `command` entry. Never `claude mcp add` or `pi mcp add`.
  - Kept entry: same `url`, `sub` equal, and no 401 from one `initialize` request. Else issue a new JWT.
  - Local issue: `podman exec sag sag agent issue <sub>` without `-t`. Read the JWT from the pipe.
  - `sag agent issue` exits 3 when standard output is a terminal. It writes no log line. It issues also for an unlisted `sub`.
  - Plan conflicts of PRD-devx 12.3. Each gives one `fail` line and exit 1 before any change.
  - CA trust of PRD-devx 12.5: `env.NODE_EXTRA_CA_CERTS` in `settings.json` with the JSON write rule. Another present value gives the bundle command and exit 1.
  - Verify: run `claude mcp list` or `pi mcp list --json` in a new empty temporary directory, timeout 60 s, environment without a JWT. Claude Code child without `NODE_EXTRA_CA_CERTS`. pi child with `NODE_EXTRA_CA_CERTS=<cfg>/ca.pem`.
  - `--deny-rules`, `--shell-profile`, `--guard`, `--url`, and `--sub` flags in `internal/cli/agent_setup.go`. The control and profile stubs of `D6.0` do nothing until `D6.3` merges.
  - `user_dxac39_test.go`: the default path case of DX-AC39 in the `User` suite. Ulrich runs it in layer 9.
- Tests first: `jsonwrite_test.go`: `TestKeepsBigIntegerText`, `TestKeepsUnicodeEscape`, `TestNoHTMLEscape`, `TestSymlinkTargetWritten`, `TestModeKept`, `TestNewFileMode0600`; `register_test.go`: `TestNoCommandEntry`, `TestKeptEntryByteEqual`, `TestOtherSubNewJWT`, `TestUnauthorizedNewJWT`, `TestJWTNotInArgsOrEnv`; `plan_test.go`: one test per conflict; `catrust_test.go`: `TestAcceptsSameCert`, `TestBundleCommand`; `verify_test.go`: `TestClaudeChildEnv`, `TestPiChildEnv`, `TestNotConnectedExit1`; `agent_issue_test.go`: `TestTerminalStdoutExit3`, `TestUnlistedSubIssued`.
- e2e: `DXAC39_ClaudeCodeSetup`, `DXAC45_AgentIssue`.
- ACs: DX-AC39 (without its default path case), DX-AC45.
- Verify: `go test -count=1 ./internal/agentsetup/... ./internal/cli/... && go vet -tags devxe2e ./e2e/devx/... && make e2e-devx SUITE=Main RUN='DXAC39_|DXAC45_' LAYER=6`. Expected: `ok`, then exit 0.
- Done when: no output, log, argument, or environment of a child holds a JWT.

## D6.2 Skill, pi guard, harness stubs

- Goal: Write the skill and `sag-guard.ts` with their tests. Build the harness stubs.
- Owns: see the ownership table.
- Reads: PRD-devx 12.6, 12.7, 17.1; PRD 13.3; README section 3.1; `e2e/devx/testdata/harness-forms.txt`.
- Depends on: `D6.0`.
- Method:
  - `SKILL.md` head: frontmatter `name: sag`, then the marker `<!-- managed by sag agent setup -->`.
  - `SKILL.md` body: the four tools as `mcp__sag__<tool>`, the read-only rules, the error codes, the human command with `--agent <sub>`, the never-run rule.
  - `sag-guard.ts`: line 1 `// managed by sag agent setup`. Block on `sag credential delete` or `podman secret rm` anywhere, and on the regular expression of PRD-devx 12.7. Return `{ block: true, reason: "sag-guard: <matched text> blocked" }`.
  - `sag-guard.test.ts`: the block and pass cases of DX-AC44.
  - `fakeharness`: `claude mcp list` prints the `sag` line in the recorded form of `harness-forms.txt`. `pi mcp list --json` prints the `sag` state that the test sets. The stub records its arguments, environment, and working directory.
- Tests first: `sag-guard.test.ts` (Node); `skill_test.go`: `TestDXAC49_SkillNamesToolsAndCodes`, `TestSkillMarker`; `fakeharness` tests: `TestRecordsArgsEnvDir`, `TestClaudeLineMatchesRecord`.
- ACs: DX-AC44, DX-AC49.
- Verify: `go test -count=1 ./internal/agentsetup/... ./e2e/devx/cmd/fakeharness/... && make e2e-devx-unit LAYER=6`. Expected: `ok`, then exit 0.
- Done when: the binary embeds the skill and the guard, and not the test file.

## D6.3 Command control and shell profile

- Goal: Apply the deny rules, the pi guard file, and the shell profile line with their answers.
- Owns: see the ownership table.
- Reads: PRD-devx 12.2, 12.3, 12.5, 12.7.
- Depends on: `D6.0`. Verify after `D6.1`, `D6.2`.
- Method:
  - Answers `--deny-rules`, `--shell-profile`, `--guard`, keyed in `answers.json` by the absolute path of the approved file.
  - Claude Code: the three deny rules in `settings.json` with the JSON write rule. No duplicate.
  - pi: two lines in the shell profile of PRD-devx 12.2. `sag-guard.ts` only over a file with the marker.
  - DX-AC41 on macOS with `SHELL=/bin/bash` writes `~/.bash_profile`. The test copies the file, removes the two lines at the end, and compares (README section 2.1).
  - DX-AC40 searches the service log `~/Library/Logs/sag/service.log` in phase A. `DXAC40_JWTNoLeak` calls `EnsureAgentSetup`.
  - `EnsureAgentSetup` calls `EnsureBootstrapped`. When `mcpServers.sag` is absent in the test directories, it runs the agent setup of DX-AC39 and DX-AC41 with the stubs.
- Tests first: `control_test.go`: `TestDenyRulesNoDuplicate`, `TestDenyRulesNoKeepsFile`, `TestGuardMarkerOnly`; `profile_test.go`: `TestProfileByShell`, `TestProfileLineOnce`, `TestFishRefused`; `dxh/ensure_agent_test.go` with a fake runner: `TestEnsureAgentSetupSkipsWhenPresent`.
- e2e: `DXAC26_DenyRules`, `DXAC40_JWTNoLeak`, `DXAC41_PiSetup`, `DXAC42_CATrustConflict`, `DXAC43_AgentSetupRefusals`.
- ACs: DX-AC26, DX-AC40, DX-AC41, DX-AC42, DX-AC43.
- Verify: `go test -count=1 ./internal/agentsetup/... ./e2e/devx/dxh/... && make e2e-devx SUITE=Main RUN='DXAC26_|DXAC4[0-3]_' LAYER=6`. Expected: `ok`, then exit 0.
- Done when: an exit in the plan changes no file.

## D6.4 Remote mode

- Goal: Register SAG at a remote URL with a JWT that the operator types.
- Owns: see the ownership table.
- Reads: PRD-devx 5.8, 12.1, 12.3, 12.4.
- Depends on: `D6.0`. Verify after `D6.1`.
- Method:
  - `--url` needs `https`, a host, port 8443, and path `/mcp`. Another form gives exit 2.
  - No Podman call and no container in the remote mode. `<cfg>/ca.pem` absent is a plan conflict.
  - Read the JWT on `/dev/tty` with echo off. Decode `sub` without a signature check. Another `sub` gives exit 1.
  - The kept-entry check is part of the plan. A needed JWT without a terminal gives exit 3 before any change.
  - An entry with the header value `${SAG_JWT}` is another form.
- Tests first: `remote_test.go`: `TestURLForms`, `TestNoPodmanCall`, `TestSubMismatchExit1`, `TestNoTerminalExit3NoChange`, `TestVariableHeaderIsOtherForm`.
- e2e: `DXAC51_RemoteMode`.
- ACs: DX-AC51.
- Verify: `go test -count=1 ./internal/agentsetup/... -run 'URLForms|NoPodman|SubMismatch|NoTerminal|VariableHeader' && make e2e-devx SUITE=Main RUN=DXAC51_ LAYER=6`. Expected: `ok`, then exit 0.
- Done when: the typed JWT goes only to the MCP configuration file.

## D6.5 Bootstrap step 14 and step 15

- Goal: Ask the agent setup answer per detected harness, run agent setup in the bootstrap process, and print the next command.
- Owns: see the ownership table.
- Reads: PRD-devx 5.2, 5.4, 12.3, 12.5.
- Depends on: `D6.0`. Verify after `D6.1`, `D6.3`.
- Method:
  - Detect a harness by `claude` or `pi` in `PATH`. Key `agent_setup` by the harness and its absolute config directory.
  - Flags `--agent-setup`, `--deny-rules`, `--shell-profile`, `--guard` in `internal/cli/bootstrap.go`.
  - The bootstrap plan omits the conflict "container `sag` does not run".
  - A `NODE_EXTRA_CA_CERTS` conflict in step 14 changes no file of that harness. Bootstrap completes the other steps and exits 1.
  - Step 15 prints the endpoint and `sag credential create <name> --kind postgres` with one `--agent <sub>` per completed harness. It prints `sag agent setup <harness>` for each detected harness without setup.
  - With Tailscale, step 15 prints the second machine steps with the address at run time.
- Tests first: `agents_test.go`: `TestAskPerDetectedHarness`, `TestNoTerminalNamesAgentSetup`, `TestCATrustConflictExit1AfterSteps`; `next_test.go`: `TestNextCommandAgents`, `TestNoCompletedHarness`.
- e2e: `DXAC46_BootstrapAgentSetup`.
- ACs: DX-AC46.
- Verify: `go test -count=1 ./internal/bootstrap/... ./internal/cli/... -run 'Bootstrap|AskPer|NoTerminalNames|CATrust|NextCommand|NoCompleted' && make e2e-devx SUITE=Main RUN=DXAC46_ LAYER=6`. Expected: `ok`, then exit 0.
- Done when: step 14 runs under the lock of step 4.

## D6.6 Integration and gate

- Goal: Prove the layer 6 criteria on the development machine and close the layer 6 gate.
- Owns: see the ownership table.
- Depends on: `D6.1` to `D6.5`.
- Steps: README section 6.1.
- ACs: DX-AC26, DX-AC39 (without its default path case), DX-AC40 to DX-AC46, DX-AC49, DX-AC51.
- Verify: `make test && make e2e-matrix LAYER=6 PGS="14 17 19" && make e2e-devx SUITE=Main LAYER=6 && make e2e-devx SUITE=Pins LAYER=6 && make e2e-devx-unit LAYER=6`. Expected: exit 0. The devx report shows each phase A DX-AC of layers 2 to 6 `PASS`.
- Done when: the tag `sag-devx-6` exists.

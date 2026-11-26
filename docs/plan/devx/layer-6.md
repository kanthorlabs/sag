# Layer 6: Agent setup, local mode

| Item | Value |
|---|---|
| Goal | `sag agent setup claude-code` and `sag agent setup pi` register SAG in the local mode, install the skill, the CA trust, and the command control, and verify the connection. |
| ACs | DX-AC26, DX-AC39, DX-AC40, DX-AC41, DX-AC42, DX-AC43, DX-AC44, DX-AC45, DX-AC49 |
| PRD-devx sections | 2.3, 2.4, 12 (local mode), 17.1 (harness stubs, Node) |
| Gate | `D6.4`: README section 6, with gate step 2 (Node tests). The `devx-unit` job passes. |
| Agents at once | 3. Start order: `D6.1`, `D6.3`, `D6.2` |
| Wall-clock | `D6.0` 1.5 h + parallel phase 5 h + `D6.4` 3.5 h = 10 h |

## File ownership

| Task | Owns |
|---|---|
| `D6.0` | `internal/agentsetup/agentsetup.go`, `internal/agentsetup/types.go`, `internal/agentsetup/paths.go`, stubs of the files below, `internal/cli/agent_setup.go` and `internal/cli/agent_issue.go` stubs, `e2e/devx/testdata/harness-forms.txt`, `.github/workflows/ci.yml` (`devx-unit` skeleton) |
| `D6.1` | `internal/agentsetup/jsonwrite.go`, `register.go`, `jwt.go`, their tests, `internal/cli/agent_issue.go`, `internal/cli/agent_setup.go`, `e2e/devx/dxac39_test.go`, `dxac40_test.go`, `dxac45_test.go` |
| `D6.2` | `internal/agentsetup/assets/*`, `internal/agentsetup/skill_test.go`, `e2e/devx/cmd/fakeharness/*` |
| `D6.3` | `internal/agentsetup/catrust.go`, `control.go`, `verify.go`, `plan.go`, their tests, `e2e/devx/dxac26_test.go`, `dxac41_test.go`, `dxac42_test.go`, `dxac43_test.go` |
| `D6.4` | Every layer-6 file after `D6.1` to `D6.3` merge. `docs/reviews/devx-layer-6.md`. |

## D6.0 Contract

- Goal: Freeze the agent setup API, the paths, the answer keys, and the recorded harness form.
- Owns: see the ownership table.
- Reads: PRD-devx 12; README section 3.1.
- Depends on: `D5.4`.
- Content:
  - `agentsetup.go`: the `go:embed` of `assets/skill/SKILL.md` and `assets/pi/sag-guard.ts` (README section 3.1).
  - `types.go`: `Harness` (`claude-code`, `pi`), `Mode` (local, remote), `Options` (`sub`, `url`, answers), `Run(ctx, Options) error`, `PlanOnly(ctx, Options) (Plan, error)` for bootstrap step 3.
  - `paths.go`: the PRD-devx 12.2 table, the shell profile table, and the `CLAUDE_CONFIG_DIR` and `PI_CODING_AGENT_DIR` rules.
  - Record the harness form (PRD-devx 19 step 5). Ulrich runs these steps on the macOS machine, from a terminal outside Claude Code:
    1. Start SAG with `make dev-up`.
    2. Create a temporary directory `<tmp>` with mode 0700.
    3. Write `<tmp>/.claude.json` by hand with `mcpServers.sag` of PRD-devx 12.4 and a JWT from `podman exec -it sag-dev sag jwt generate claude-code-dev`.
    4. Run `CLAUDE_CONFIG_DIR=<tmp> NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" claude mcp list`.
    5. Copy the line that starts with `sag: ` to `e2e/devx/testdata/harness-forms.txt`. Remove `<tmp>`. Run `make dev-down`.
  - The recorded line holds no JWT. The contract checks this before the commit.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: `D6.1` to `D6.3` compile against the stubs, and `harness-forms.txt` holds one `sag: ` line.

## D6.1 Registration, JSON write rule, JWT issue

- Goal: Write `mcpServers.sag` with a fresh or kept JWT, and add the container command `sag agent issue`.
- Owns: see the ownership table.
- Reads: PRD-devx 12.1, 12.4; PRD 11.2.
- Depends on: `D6.0`. Verify after `D6.2`, `D6.3`.
- Method:
  - JSON write rule: decode only the path of the named key. Keep each other value as `json.RawMessage`. `SetEscapeHTML(false)`. Temporary file and rename in the directory of the link target. Keep the mode. The MCP file ends with mode 0600.
  - Claude Code entry `{"type": "http", "url", "headers"}`. pi entry `{"url", "headers", "exposure": "direct"}`. Never a `command` entry. Never `claude mcp add` or `pi mcp add`.
  - Kept entry: same `url`, `sub` equal, and no 401 from one `initialize` request. Else issue a new JWT.
  - Local issue: `podman exec sag sag agent issue <sub>` without `-t`. Read the JWT from the pipe.
  - `sag agent issue` exits 3 when standard output is a terminal. It writes no log line. It issues also for an unlisted `sub`.
- Tests first: `jsonwrite_test.go`: `TestKeepsBigIntegerText`, `TestKeepsUnicodeEscape`, `TestNoHTMLEscape`, `TestSymlinkTargetWritten`, `TestModeKept`, `TestNewFileMode0600`; `register_test.go`: `TestNoCommandEntry`, `TestKeptEntryByteEqual`, `TestOtherSubNewJWT`, `TestUnauthorizedNewJWT`, `TestJWTNotInArgsOrEnv`; `agent_issue_test.go`: `TestTerminalStdoutExit3`, `TestUnlistedSubIssued`.
- e2e: `DXAC39_ClaudeCodeSetup`, `DXAC40_JWTNoLeak`, `DXAC45_AgentIssue`.
- ACs: DX-AC39, DX-AC40, DX-AC45.
- Verify: `go test -count=1 ./internal/agentsetup/... ./internal/cli/... -run 'Keeps|NoHTML|Symlink|Mode|NoCommand|Kept|OtherSub|Unauthorized|JWTNot|TerminalStdout|Unlisted'`. Expected: `ok`.
- Done when: no output, log, argument, or environment of a child holds a JWT.

## D6.2 Skill, pi guard, harness stubs

- Goal: Write the skill and `sag-guard.ts` with their tests. Build the harness stubs for CI.
- Owns: see the ownership table.
- Reads: PRD-devx 12.6, 12.7, 17.1; PRD 13.3; README section 3.1.
- Depends on: `D6.0`.
- Method:
  - `SKILL.md` head: frontmatter `name: sag`, then the marker `<!-- managed by sag agent setup -->`.
  - `SKILL.md` body: the four tools as `mcp__sag__<tool>`, the read-only rules, the error codes, the human command with `--agent <sub>`, the never-run rule.
  - `sag-guard.ts`: line 1 `// managed by sag agent setup`. Block on `sag credential delete` or `podman secret rm` anywhere, and on the regular expression of PRD-devx 12.7. Return `{ block: true, reason: "sag-guard: <matched text> blocked" }`.
  - `sag-guard.test.ts`: the block and pass cases of DX-AC44.
  - `fakeharness`: `claude mcp list` and `pi mcp list --json` print the `sag` state that the test sets, in the recorded form. The stub records its arguments, environment, and working directory.
- Tests first: `sag-guard.test.ts` (Node); `skill_test.go`: `TestDXAC49_SkillNamesToolsAndCodes`, `TestSkillMarker`; `fakeharness` tests: `TestRecordsArgsEnvDir`.
- ACs: DX-AC44, DX-AC49.
- Verify: `node --test "internal/agentsetup/assets/pi/*.test.ts" && go test -count=1 ./internal/agentsetup/... ./e2e/devx/cmd/fakeharness/...`. Expected: exit 0, then `ok`.
- Done when: the binary embeds the skill and the guard, and not the test file.

## D6.3 CA trust, command control, plan, verification

- Goal: Apply the CA trust and the command control with their answers. Check the plan conflicts. Verify the connection.
- Owns: see the ownership table.
- Reads: PRD-devx 12.1 to 12.3, 12.5, 12.7, 12.8.
- Depends on: `D6.0`.
- Method:
  - Answers `--deny-rules`, `--shell-profile`, `--guard`, keyed in `answers.json` by the absolute path of the approved file.
  - Plan conflicts of PRD-devx 12.3. Each gives one `fail` line and exit 1 before any change.
  - Claude Code: `env.NODE_EXTRA_CA_CERTS` and the three deny rules in `settings.json` with the JSON write rule.
  - pi: two lines in the shell profile. `sag-guard.ts` only over a file with the marker.
  - Present value rule of PRD-devx 12.5. Another value gives the bundle command and exit 1.
  - Verify: run `claude mcp list` or `pi mcp list --json` in a new empty temporary directory, timeout 60 s, environment without a JWT. Claude Code child without `NODE_EXTRA_CA_CERTS`. pi child with `NODE_EXTRA_CA_CERTS=<cfg>/ca.pem`.
- Tests first: `plan_test.go`: one test per conflict; `catrust_test.go`: `TestAcceptsSameCert`, `TestBundleCommand`, `TestProfileByShell`; `control_test.go`: `TestDenyRulesNoDuplicate`, `TestGuardMarkerOnly`; `verify_test.go`: `TestClaudeChildEnv`, `TestPiChildEnv`, `TestNotConnectedExit1`.
- e2e: `DXAC26_DenyRules`, `DXAC41_PiSetup`, `DXAC42_CATrustConflict`, `DXAC43_AgentSetupRefusals`.
- ACs: DX-AC26, DX-AC41, DX-AC42, DX-AC43.
- Verify: `go test -count=1 ./internal/agentsetup/...`. Expected: `ok`.
- Done when: an exit in the plan changes no file.

## D6.4 Integration and gate

- Goal: Prove the layer 6 criteria in CI and close the layer 6 gate.
- Owns: see the ownership table.
- Depends on: `D6.1` to `D6.3`.
- Steps: README section 6. Set the devx gate layer of `ci.yml` to 6. A test that changes `~/.bashrc` restores it.
- ACs: DX-AC26, DX-AC39 to DX-AC45, DX-AC49.
- Verify: CI run. Expected: `ac35` `PASS`. `devx-report` shows each DX-AC of layers 2 to 6 `PASS`.
- Done when: the tag `sag-devx-6` exists.

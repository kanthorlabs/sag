# Layer 7: Agent setup, remote mode and bootstrap step 14

| Item | Value |
|---|---|
| Goal | An agent on a second tailnet machine registers SAG with a typed JWT. Bootstrap sets up each approved harness. |
| ACs | DX-AC46, DX-AC51 |
| PRD-devx sections | 5.2 steps 3, 14, 15, 5.4 (agent setup answers), 5.8 (second machine steps), 12.1, 12.4 (remote mode) |
| Gate | `D7.3`: README section 6 |
| Agents at once | 2. Start order: `D7.1`, `D7.2` |
| Wall-clock | `D7.0` 0.5 h + parallel phase 2.5 h + `D7.3` 2.5 h = 5.5 h |

## File ownership

| Task | Owns |
|---|---|
| `D7.0` | `internal/agentsetup/remote.go` stub, `internal/bootstrap/steps.go` (calls of step 14 and step 15) |
| `D7.1` | `internal/agentsetup/remote.go`, `remote_test.go`, `e2e/devx/dxac51_test.go` |
| `D7.2` | `internal/bootstrap/agents.go`, `internal/bootstrap/next.go`, their tests, `internal/cli/bootstrap.go` (flags `--agent-setup`, `--deny-rules`, `--shell-profile`, `--guard`), `e2e/devx/dxac46_test.go` |
| `D7.3` | Every layer-7 file after `D7.1` and `D7.2` merge. `docs/reviews/devx-layer-7.md`. |

## D7.0 Contract

- Goal: Freeze the remote mode API and the bootstrap call of agent setup.
- Owns: see the ownership table.
- Reads: PRD-devx 5.2, 12.1, 12.4.
- Depends on: `D6.4`.
- Content: `agentsetup.ParseURL(url) (Mode, error)`, `agentsetup.ReadJWT(tty, sub) (secret.Credential, error)`, bootstrap step 14 calls `agentsetup.Run` with the plan answers under the bootstrap lock.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: `D7.1` and `D7.2` compile against the stubs.

## D7.1 Remote mode

- Goal: Register SAG at a remote URL with a JWT that the operator types.
- Owns: see the ownership table.
- Reads: PRD-devx 5.8, 12.1, 12.3, 12.4.
- Depends on: `D7.0`.
- Method:
  - `--url` needs `https`, a host, port 8443, and path `/mcp`. Another form gives exit 2.
  - No Podman call and no container in the remote mode. `<cfg>/ca.pem` absent is a plan conflict.
  - Read the JWT on `/dev/tty` with echo off. Decode `sub` without a signature check. Another `sub` gives exit 1.
  - The kept-entry check is part of the plan. A needed JWT without a terminal gives exit 3 before any change.
  - An entry with the header value `${SAG_JWT}` is another form.
- Tests first: `remote_test.go`: `TestURLForms`, `TestNoPodmanCall`, `TestSubMismatchExit1`, `TestNoTerminalExit3NoChange`, `TestVariableHeaderIsOtherForm`.
- e2e: `DXAC51_RemoteMode`.
- ACs: DX-AC51.
- Verify: `go test -count=1 ./internal/agentsetup/... -run 'URLForms|NoPodman|SubMismatch|NoTerminal|VariableHeader'`. Expected: `ok`.
- Done when: the typed JWT goes only to the MCP configuration file.

## D7.2 Bootstrap step 14 and step 15

- Goal: Ask the agent setup answer per detected harness, run agent setup in the bootstrap process, and print the next command.
- Owns: see the ownership table.
- Reads: PRD-devx 5.2, 5.4, 12.3, 12.5.
- Depends on: `D7.0`.
- Method:
  - Detect a harness by `claude` or `pi` in `PATH`. Key `agent_setup` by the harness and its absolute config directory.
  - The bootstrap plan omits the conflict "container `sag` does not run".
  - A `NODE_EXTRA_CA_CERTS` conflict in step 14 changes no file of that harness. Bootstrap completes the other steps and exits 1.
  - Step 15 prints the endpoint and `sag credential create <name> --kind postgres` with one `--agent <sub>` per completed harness. It prints `sag agent setup <harness>` for each detected harness without setup.
  - With Tailscale, step 15 prints the second machine steps with `<tailscale IP>` replaced by the address at run time.
- Tests first: `agents_test.go`: `TestAskPerDetectedHarness`, `TestNoTerminalNamesAgentSetup`, `TestCATrustConflictExit1AfterSteps`; `next_test.go`: `TestNextCommandAgents`, `TestNoCompletedHarness`.
- e2e: `DXAC46_BootstrapAgentSetup`.
- ACs: DX-AC46.
- Verify: `go test -count=1 ./internal/bootstrap/...`. Expected: `ok`.
- Done when: step 14 runs under the lock of step 4.

## D7.3 Integration and gate

- Goal: Prove the layer 7 criteria in CI and close the layer 7 gate.
- Owns: see the ownership table.
- Depends on: `D7.1`, `D7.2`.
- Steps: README section 6. Set the devx gate layer of `ci.yml` to 7.
- ACs: DX-AC46, DX-AC51.
- Verify: CI run. Expected: `ac35` `PASS`. `devx-report` shows each DX-AC of layers 2 to 7 `PASS`.
- Done when: the tag `sag-devx-7` exists.

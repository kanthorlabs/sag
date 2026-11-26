# Layer 10: macOS development machine

| Item | Value |
|---|---|
| Goal | The dev-machine criteria prove the macOS paths, the real harness formats, and the real `tailscale serve`. The machine state of Ulrich is the same after the group. |
| ACs | DX-AC34, DX-AC35, DX-AC38, DX-AC47, DX-AC48, DX-AC52 (all `Dev machine`) |
| PRD-devx sections | 5.3 (macOS), 5.5 (macOS), 5.8, 6 (macOS), 15, 17.1 (macOS development machine), 18 R20, R24, R28 |
| Gate | `D10.4`: README section 6 with gate step 5 |
| Agents at once | 3. Start order: `D10.1`, `D10.2`, `D10.3` |
| Wall-clock | `D10.0` 2 h + parallel phase 3 h + `D10.4` 4 h = 9 h |

The shared-machine constraints of README section 2.1 apply to each task of this layer. An agent writes and unit-tests the group. Only Ulrich runs it on the machine.

## File ownership

| Task | Owns |
|---|---|
| `D10.0` | `e2e/devx/dxh/devmachine.go` (precondition check, snapshot, restore check, version record), `e2e/devx/dxh/devmachine_test.go`, `Makefile` (`e2e-devx-mac` body) |
| `D10.1` | `e2e/devx/dxac34_test.go`, `dxac35_test.go`, `dxac38_test.go` |
| `D10.2` | `e2e/devx/dxac47_test.go`, `dxac48_test.go`, `e2e/devx/dxh/realharness.go` |
| `D10.3` | `e2e/devx/dxac52_test.go`, `e2e/devx/dxh/tailscale.go` |
| `D10.4` | Every layer-10 file after `D10.1` to `D10.3` merge. `docs/reviews/devx-layer-10.md`. |

## D10.0 Contract and group runner

- Goal: Freeze the dev-machine group runner. It refuses to start on an unsafe machine state and restores the files of Ulrich.
- Owns: see the ownership table.
- Reads: README section 2; PRD-devx 15, 17.1.
- Depends on: `D9.4`.
- Content:
  - Precondition check before any change: exactly one Podman machine, `podman-machine-default`, in state running. No container `sag`. Port 8443 free. On a failure, exit 1 with the failed check.
  - Snapshot to a directory with mode 0700 under `.dev/local/devx-mac/`: `~/.claude.json`, `~/.claude/settings.json`, `~/.pi/agent/mcp.json`, the shell profile, and `tailscale serve status --json`.
  - After the group: PRD-devx 15 teardown, then restore each file, then the compare of README section 2.1.
  - Version record for the `env` block: `sw_vers -productVersion`, `podman --version`, `claude --version`, `pi --version`, `tailscale version`.
  - The runner masks the Tailscale IP in each output line with `<tailscale IP>`.
  - Remove the snapshot directory after a successful compare. It can hold a JWT of Ulrich.
- Tests first: `devmachine_test.go` with fake commands: `TestRefusesSecondMachine`, `TestRefusesStoppedMachine`, `TestRefusesContainerSag`, `TestRefusesPortHeld`, `TestCompareMcpServersOnly`, `TestMasksTailscaleIP`, `TestNoPodmanMachineCommandRuns`.
- ACs: none.
- Verify: `go test -count=1 ./e2e/devx/dxh/...`. Expected: `ok`.
- Done when: no code path of the runner calls `podman machine init`, `start`, `stop`, `rm`, or a Podman uninstall.

## D10.1 macOS bootstrap, credential, and pkg check

- Goal: Prove bootstrap, the LaunchAgent, the credential commands, and the pkg checksum on macOS.
- Owns: see the ownership table.
- Reads: PRD-devx 3, 5.3, 5.5, 6, 7 to 9, 17.1.
- Depends on: `D10.0`.
- Method:
  - DX-AC34: `bootstrap --autostart yes --agent-setup claude-code=no --agent-setup pi=no` with the installed Podman. Check `launchctl print`. After `sag service stop`, `launchctl kickstart -k` gives `/healthz` 200 within 30 s.
  - DX-AC35: a PostgreSQL container on a free host port. `credential create`, `list`, and `delete` as DX-AC9, DX-AC17, and DX-AC20.
  - DX-AC38: `SAG_TEST_ABSENT=podman` and `SAG_TEST_STOP_BEFORE_INSTALL`. The test never reaches `installer`.
- Tests first: none beyond `D10.0`. The criteria are e2e only.
- e2e: `DXAC34_MacBootstrap`, `DXAC35_MacCredential`, `DXAC38_MacPkgChecksum` in group `mac`.
- ACs: DX-AC34, DX-AC35, DX-AC38.
- Verify: `go vet -tags devxe2e ./e2e/devx/...`. Expected: exit 0.
- Done when: each test runs only inside the `D10.0` runner.

## D10.2 Real harnesses

- Goal: Prove the real Claude Code 2.1.332 and pi 1.6.1 formats, the deny rules, and the pi guard.
- Owns: see the ownership table.
- Reads: PRD-devx 12, 17 rows DX-AC47 and DX-AC48, R24, R25.
- Depends on: `D10.0`.
- Method:
  - Check the harness versions first. Another version fails the criterion with `prerequisite missing`.
  - DX-AC47: `claude mcp list` from a shell without `NODE_EXTRA_CA_CERTS`. `claude -p --output-format json --allowedTools "Bash(sag:*)"` for a prompt that asks for `sag credential delete x`. Expect a match in `permission_denials`. Resource `x` does not exist.
  - DX-AC48: `pi mcp list --json` from a new login shell. `pi --mode json` for a prompt that asks for `podman secret rm x`. Expect a tool result with a reason that starts with `sag-guard:`.
  - The test prints no JWT and no harness configuration content.
- Tests first: `realharness_test.go`: `TestVersionGate`, `TestOutputHasNoJWT`.
- e2e: `DXAC47_RealClaudeCode`, `DXAC48_RealPi` in group `mac`.
- ACs: DX-AC47, DX-AC48.
- Verify: `go test -count=1 ./e2e/devx/dxh/... -run 'VersionGate|OutputHasNoJWT' && go vet -tags devxe2e ./e2e/devx/...`. Expected: `ok`, then exit 0.
- Done when: each test changes only the files of the `D10.0` snapshot list.

## D10.3 Real Tailscale serve

- Goal: Prove the real `tailscale serve --tcp` forward on macOS and its removal.
- Owns: see the ownership table.
- Reads: PRD-devx 5.8, 15, 17 row DX-AC52.
- Depends on: `D10.0`.
- Method:
  - Precondition: Tailscale up and no TCP 8443 entry in `tailscale serve status --json`. Else the criterion fails with `prerequisite missing` and changes nothing.
  - `curl --cacert <cfg>/ca.pem https://<tailscale IP>:8443/healthz` returns 200. `lsof -nP -iTCP:8443 -sTCP:LISTEN` shows no `*:8443`.
  - After `tailscale serve --tcp 8443 off`, the status has no TCP 8443 entry and the `curl` fails.
  - Read the address at run time. Write it to no file, log line, or report.
- Tests first: `tailscale_test.go` in `dxh`: `TestNoAddressInOutput`.
- e2e: `DXAC52_MacTailscale` in group `mac`.
- ACs: DX-AC52.
- Verify: `go test -count=1 ./e2e/devx/dxh/... -run TestNoAddressInOutput`. Expected: `ok`.
- Done when: the serve status after the group equals the saved status.

## D10.4 Integration and gate

- Goal: Run the dev-machine group, merge its report with CI, and close the layer 10 gate.
- Owns: see the ownership table.
- Depends on: `D10.1` to `D10.3`.
- Steps:
  1. Run README section 6 steps 1 to 4. Set the devx gate layer of `ci.yml` to 10.
  2. Ask Ulrich to run `make dev-down`, then `make e2e-devx-mac` from a terminal outside Claude Code.
  3. Read `e2e/report/devx-mac/ac-report.json`. Check that it holds no IP address, no JWT, and no credential value.
  4. Run `acreport merge-devx` with the CI report and the dev-machine report.
  5. Run the review and the tag steps of README section 6. Append the `env` block to the review file.
- ACs: DX-AC34, DX-AC35, DX-AC38, DX-AC47, DX-AC48, DX-AC52.
- Verify: the merged report. Expected: each DX-AC of layers 2 to 10 `PASS`. The restore check of the group passes.
- Done when: the tag `sag-devx-10` exists.

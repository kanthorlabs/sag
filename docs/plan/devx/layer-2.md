# Layer 2: Bootstrap and service on Ubuntu

| Item | Value |
|---|---|
| Goal | `sag bootstrap` on an Ubuntu host with Podman creates the SAG state and starts SAG. `sag service start` and `stop` work. |
| ACs | DX-AC1, DX-AC2, DX-AC4, DX-AC27, DX-AC28, DX-AC33, DX-AC36 |
| PRD-devx sections | 1.1, 1.3, 2.1, 4.3 (pull when absent), 5.1, 5.2 steps 1 to 12 and 15, 5.4, 5.5, 5.6, 5.7, 6 (without the port check), 17.1 (`devxtest`) |
| Gate | `D2.4`: README section 6 |
| Agents at once | 3. Start order: `D2.1`, `D2.2`, `D2.3` |
| Wall-clock | `D2.0` 1.5 h + parallel phase 4.5 h + `D2.4` 3 h = 9 h |

These bootstrap steps stay stubs in this layer: the dependency rule (step 6), Tailscale (steps 3 and 13), the port check (step 3), and agent setup (step 14). Each stub writes no line.

## File ownership

| Task | Owns |
|---|---|
| `D2.0` | `internal/cli/cli.go` (dispatch table, exit code 3, host and container command sets), `internal/hoststate/types.go`, `internal/stepline/stepline.go`, `internal/testhook/*.go` stubs, `internal/bootstrap/steps.go`, stubs of `internal/service`, `internal/endpointcert`, `internal/masterkey`, `internal/image` |
| `D2.1` | `internal/cli/mode.go`, `internal/cli/bootstrap.go`, `internal/hoststate/*`, `internal/prompt/*`, `internal/testhook/*`, `internal/bootstrap/*` except `steps.go`, `e2e/devx/dxac04_test.go`, `e2e/devx/dxac33_test.go`, `e2e/devx/dxac36_test.go` |
| `D2.2` | `internal/service/*`, `internal/cli/service.go`, `e2e/devx/dxac27_test.go`, `e2e/devx/dxac28_test.go` |
| `D2.3` | `internal/endpointcert/*`, `internal/masterkey/*`, `internal/image/*`, `e2e/devx/dxac01_test.go`, `e2e/devx/dxac02_test.go` |
| `D2.4` | Every layer-2 file after `D2.1` to `D2.3` merge. `docs/reviews/devx-layer-2.md`. |

## D2.0 Contract

- Goal: Freeze the host command frame, the state types, the step order, and the test hook names.
- Owns: see the ownership table.
- Reads: PRD-devx 1, 2.1, 5, 6, 17.1.
- Depends on: `D1.4`.
- Content:
  - `cli.go`: `ExitRefusal = 3`. The dispatch table lists each command of PRD-devx 1.1. Each host command not built yet exits 1 with `not implemented`.
  - `hoststate/types.go`: `Paths` (`<cfg>`, `sag.yaml`, `ca.pem`, `answers.json`, `lock`), `Answers` with the JSON keys `autostart`, `agent_setup`, `deny_rules`, `shell_profile`, `guard`, `podman_path`, `tailscale_ip`.
  - `stepline.go`: `Writer.Line(status, item, detail)` with the five statuses of PRD-devx 1.3.
  - `testhook`: names for `SAG_TEST_PINS`, `SAG_TEST_IMAGE`, `SAG_TEST_CERT_DAYS`, `SAG_TEST_ABSENT`, `SAG_TEST_STOP_BEFORE_INSTALL`, and the failure points of DX-AC16, DX-AC22, DX-AC29, DX-AC37.
  - `bootstrap/steps.go`: the ordered list of the 15 steps with one function value per step.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check && go vet -tags devxtest ./...`. Expected: exit 0.
- Done when: `D2.1` to `D2.3` compile against the stubs.

## D2.1 Host frame, state, and bootstrap flow

- Goal: Select host or container behavior. Refuse UID 0. Hold the lock and the answers. Run the bootstrap plan and apply steps.
- Owns: see the ownership table.
- Reads: PRD-devx 1.1, 1.3, 2.1, 5.1 to 5.5, 17.1.
- Depends on: `D2.0`.
- Method:
  - Container behavior when `/run/.containerenv` exists. Internal commands and `sag serve` on the host exit 2.
  - `<cfg>` is `$XDG_CONFIG_HOME/sag` or `~/.config/sag`, mode 0700. On a host without `<cfg>`, an exit in step 3 leaves no `<cfg>`.
  - The lock is a `flock` on `<cfg>/lock`. A held lock gives exit 1. A command that holds the lock passes it to the start steps.
  - `podman` search order: `PATH`, `/opt/podman/bin/podman`, `/opt/homebrew/bin/podman`. Store the path in `answers.json`.
  - Answer resolution: flag, then stored answer, then prompt on `/dev/tty`. No terminal and no flag gives exit 3 and names the flag.
  - Plan in step 3, then lock, then plan again in step 5.
  - `testhook_devxtest.go` reads `SAG_TEST_*`. `testhook_product.go` ignores each variable.
- Tests first: `hoststate`: `TestCfgDirXDG`, `TestCfgMode0700`, `TestLockHeldExit1`, `TestAnswersMode0600`, `TestPodmanSearchOrder`; `prompt`: `TestFlagBeatsStored`, `TestNoTerminalNamesFlag`, `TestOnlyYOrYesApproves`; `bootstrap`: `TestExitInPlanLeavesNoCfg`, `TestPlanTwice`; `cli`: `TestUID0Refused`, `TestInternalCommandOnHostExit2`.
- e2e: `DXAC04_BootstrapNoTerminal`, `DXAC33_ProductBinary`, `DXAC36_HostGuards`. DX-AC36 runs each host command of PRD-devx 1.1 with `sudo`.
- ACs: DX-AC4, DX-AC33, DX-AC36.
- Verify: `go test -count=1 ./internal/cli/... ./internal/hoststate/... ./internal/prompt/... ./internal/bootstrap/... ./internal/testhook/...` and `go test -tags devxtest -count=1 ./internal/testhook/...`. Expected: `ok`.
- Done when: no prompt goes to standard output.

## D2.2 Service start and stop, start at login

- Goal: Start and stop the container `sag` on Ubuntu and macOS. Apply the start at login.
- Owns: see the ownership table.
- Reads: PRD-devx 5.5, 6, 13; `internal/deploy`.
- Depends on: `D2.0`.
- Method:
  - Ubuntu: write `~/.config/containers/systemd/sag.container` from `deploy.Quadlet`. Run `systemctl --user daemon-reload`, then `systemctl --user restart sag`.
  - macOS: start the Podman machine only when it is stopped. Run `podman run -d --replace --name sag --restart=on-failure` with `deploy.RunArgs`.
  - Wait up to 30 s for `GET /healthz` with `<cfg>/ca.pem`. On a timeout, print the last 50 lines of the daemon log and exit 1.
  - LaunchAgent `net.kanthorlabs.sag` with `RunAtLoad` and `ProgramArguments` `<absolute path of sag> service start`. Output to `~/Library/Logs/sag/service.log`.
  - Without the domain `gui/<uid>`, write the file and the `warn autostart loads at the next GUI login` line.
- Tests first: `service_test.go` with a fake runner: `TestUbuntuStartOrder`, `TestMacStartsStoppedMachineOnly`, `TestHealthzTimeoutTailsLog`, `TestLaunchAgentPlist`, `TestBootoutNotLoadedIsNotFailure`, `TestQuadletAutostartNo`.
- e2e: `DXAC27_Autostart`, `DXAC28_ServiceStopStart`.
- ACs: DX-AC27, DX-AC28.
- Verify: `go test -count=1 ./internal/service/... ./internal/cli/...`. Expected: `ok`.
- Done when: `GOOS=darwin go build ./...` passes.

## D2.3 Master key, certificate, image pull

- Goal: Create `sag-master-key`, the endpoint certificate secrets, and `<cfg>/ca.pem`. Pull the image when it is absent.
- Owns: see the ownership table.
- Reads: PRD-devx 4.3, 5.6, 5.7, 13.
- Depends on: `D2.0`.
- Method:
  - Master key: 32 bytes from `crypto/rand`, base64, to the standard input of `podman secret create sag-master-key -`.
  - CA without the name constraints extension, 397 days, key only in memory. One server certificate with `localhost`, `127.0.0.1`, `::1`, and an optional Tailscale address.
  - The five-step sequence and the four triggers of PRD-devx 5.7. `SAG_TEST_CERT_DAYS` sets the validity in the test build.
  - The image is `SAG_TEST_IMAGE` in the test build, else the reference of PRD-devx 4.3. Pull only when absent.
  - DX-AC2 counts the secrets of `podman container inspect sag`. If `D1.4` recorded a negative 4.9.3 result, count the `Secret=` lines of the Quadlet file (PRD-devx 8).
- Tests first: `endpointcert`: `TestNoNameConstraints`, `TestServerNames`, `TestTailscaleNameAdded`, `TestSequenceOrder`, `TestTriggers`, `TestKeyNeverInArgsOrFile`; `masterkey`: `TestValueOnStdinOnly`, `TestNeverReplaced`; `image`: `TestPullOnlyWhenAbsent`.
- e2e: `DXAC01_BootstrapUbuntu`, `DXAC02_ContainerAndCert`.
- ACs: DX-AC1, DX-AC2.
- Verify: `go test -count=1 ./internal/endpointcert/... ./internal/masterkey/... ./internal/image/...`. Expected: `ok`.
- Done when: no file outside the secret store holds a private key.

## D2.4 Integration and gate

- Goal: Prove the layer 2 criteria in CI and close the layer 2 gate.
- Owns: see the ownership table.
- Depends on: `D2.1` to `D2.3`.
- Steps: README section 6. Set the devx gate layer of `ci.yml` to 2.
- ACs: DX-AC1, DX-AC2, DX-AC4, DX-AC27, DX-AC28, DX-AC33, DX-AC36.
- Verify: CI run. Expected: `ac35` `PASS`. `devx-report` shows each DX-AC of layer 2 `PASS`.
- Done when: the tag `sag-devx-2` exists.

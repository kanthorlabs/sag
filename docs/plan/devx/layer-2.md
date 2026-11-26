# Layer 2: Bootstrap and service on macOS

| Item | Value |
|---|---|
| Phase | A |
| Goal | `sag bootstrap` on the macOS development machine with Podman creates the SAG state, writes the LaunchAgent, and starts SAG. `sag service start` and `stop` work. |
| ACs | DX-AC1, DX-AC2, DX-AC4, DX-AC27, DX-AC28, DX-AC33, DX-AC36 (phase A forms) |
| PRD-devx sections | 1.1, 1.3, 2.1, 4.3 (pull when absent), 5.1, 5.2 steps 1 to 12 and 15, 5.4, 5.5 (macOS), 5.6, 5.7, 6 (macOS, without the port check), 17.1 (`devxtest`) |
| Gate | `D2.5`: README section 6.1 |
| Agents at once | 3. Start order: `D2.3`, `D2.2`, `D2.1`, then `D2.4` |
| Wall-clock | `D2.0` 1.5 h + parallel phase 5 h + `D2.5` 4.5 h = 11 h |

These bootstrap steps stay stubs in this layer: the dependency rule (step 6), Tailscale (steps 3 and 13), the port check (step 3), and agent setup (step 14). Each stub writes no line. Each `_linux.go` file returns the `unsupported platform` error.

## File ownership

| Task | Owns |
|---|---|
| `D2.0` | `internal/cli/cli.go` (dispatch table, exit code 3, host and container command sets, mode field of the command frame), each new `internal/cli` command file of README section 3 as a stub, `internal/hoststate/types.go`, `internal/stepline/stepline.go`, `internal/testhook/*.go` stubs, `internal/bootstrap/steps.go`, stubs of `internal/service`, `internal/endpointcert`, `internal/masterkey`, `internal/image` |
| `D2.1` | `internal/cli/mode.go`, the existing `internal/cli/*_test.go` files, `internal/hoststate/*`, `internal/prompt/*`, `internal/testhook/*`, `e2e/devx/dxac33_test.go`, `e2e/devx/dxac36_test.go` |
| `D2.2` | `internal/bootstrap/*` except `steps.go`, `internal/cli/bootstrap.go`, `internal/cli/bootstrap_test.go`, `e2e/devx/dxac04_test.go` |
| `D2.3` | `internal/service/*`, `internal/cli/service.go`, `internal/cli/service_test.go`, `e2e/devx/dxac01_test.go`, `e2e/devx/dxac02_test.go`, `e2e/devx/dxac27_test.go`, `e2e/devx/dxac28_test.go` |
| `D2.4` | `internal/endpointcert/*`, `internal/masterkey/*`, `internal/image/*` |
| `D2.5` | Every layer-2 file after `D2.1` to `D2.4` merge. `docs/reviews/devx-layer-2.md`. |

## D2.0 Contract

- Goal: Freeze the host command frame, the state types, the step order, and the test hook names.
- Owns: see the ownership table.
- Reads: PRD-devx 1, 2.1, 5, 6, 17.1.
- Depends on: `D1.4`, `D0.3`.
- Content:
  - `cli.go`: `ExitRefusal = 3`. The command frame has a mode field (host or container). A test sets it directly. The dispatch table lists each command of PRD-devx 1.1.
  - Each new command file of README section 3 holds its entry function. A command not built yet exits 1 with `not implemented`. Later contract tasks own these files and do not change `cli.go`.
  - `hoststate/types.go`: `Paths` (`<cfg>`, `sag.yaml`, `ca.pem`, `answers.json`, `lock`), `Answers` with the JSON keys `autostart`, `agent_setup`, `deny_rules`, `shell_profile`, `guard`, `podman_path`, `tailscale_ip`.
  - `stepline.go`: `Writer.Line(status, item, detail)` with the five statuses of PRD-devx 1.3.
  - `testhook`: each hook name of PRD-devx 17.1: `SAG_TEST_PINS`, `SAG_TEST_IMAGE`, `SAG_TEST_CERT_DAYS`, `SAG_TEST_ABSENT`, `SAG_TEST_STOP_BEFORE_INSTALL`, and the failure points of DX-AC16, DX-AC22, DX-AC29, DX-AC37. No later task adds a hook name.
  - `bootstrap/steps.go`: the ordered list of the 15 steps with one function value per step.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: `D2.1` to `D2.4` compile against the stubs.

## D2.1 Host frame and state

- Goal: Select host or container behavior. Refuse UID 0. Hold the lock and the answers. Resolve the answers.
- Owns: see the ownership table.
- Reads: PRD-devx 1.1, 1.3, 2.1, 5.3 (search order), 5.4, 17.1.
- Depends on: `D2.0`.
- Method:
  - Container behavior when `/run/.containerenv` exists. Internal commands and `sag serve` on the host exit 2.
  - Update the existing `internal/cli` tests to set the mode field of the command frame to container.
  - `<cfg>` is `$XDG_CONFIG_HOME/sag` or `~/.config/sag`, mode 0700.
  - The lock is a `flock` on `<cfg>/lock`. A held lock gives exit 1. A command that holds the lock passes it to the start steps.
  - `podman` search order: `PATH`, `/opt/podman/bin/podman`, `/opt/homebrew/bin/podman`. Store the path in `answers.json`.
  - Answer resolution: flag, then stored answer, then prompt on `/dev/tty`. No terminal and no flag gives exit 3 and names the flag.
  - `testhook_devxtest.go` reads `SAG_TEST_*`. `testhook_product.go` ignores each variable.
  - DX-AC36 runs the clauses without UID 0 in phase A (PRD-devx 17). Layer 10 adds the UID 0 clause.
- Tests first: `hoststate`: `TestCfgDirXDG`, `TestCfgMode0700`, `TestLockHeldExit1`, `TestAnswersMode0600`, `TestPodmanSearchOrder`; `prompt`: `TestFlagBeatsStored`, `TestNoTerminalNamesFlag`, `TestOnlyYOrYesApproves`; `cli`: `TestUID0Refused`, `TestInternalCommandOnHostExit2`.
- e2e: `DXAC33_ProductBinary`, `DXAC36_HostGuards`.
- ACs: DX-AC33, DX-AC36.
- Verify: `go test -count=1 ./internal/cli/... ./internal/hoststate/... ./internal/prompt/... ./internal/testhook/... && go test -tags devxtest -count=1 ./internal/testhook/... && make e2e-devx SUITE=Main RUN='DXAC3[36]_' LAYER=2`. Expected: `ok`, then exit 0.
- Done when: no prompt goes to standard output.

## D2.2 Bootstrap flow

- Goal: Run the bootstrap plan and apply steps in the order of PRD-devx 5.2.
- Owns: see the ownership table.
- Reads: PRD-devx 1.1, 5.1 to 5.4.
- Depends on: `D2.0`. Verify after `D2.1`.
- Method:
  - Step 1 refuses UID 0, a container, and each platform other than `darwin/arm64` (README section 1). The list of accepted platforms lives in `internal/bootstrap/platform.go`.
  - Plan in step 3, then the lock in step 4, then plan again in step 5.
  - On a host without `<cfg>`, an exit in step 3 leaves no `<cfg>`.
  - `--autostart yes|no` flag. Bootstrap writes the answer to `answers.json`.
  - Step 15 prints the endpoint and the next command without agents.
- Tests first: `bootstrap`: `TestExitInPlanLeavesNoCfg`, `TestPlanTwice`, `TestStepOrder`, `TestUnsupportedPlatformExit1`; `cli`: `TestBootstrapFlags`.
- e2e: `DXAC04_BootstrapNoTerminal`.
- ACs: DX-AC4.
- Verify: `go test -count=1 ./internal/bootstrap/... ./internal/cli/... -run 'Bootstrap|ExitInPlan|PlanTwice|StepOrder|UnsupportedPlatform' && make e2e-devx SUITE=Main RUN=DXAC04_ LAYER=2`. Expected: `ok`, then exit 0.
- Done when: an exit in the plan changes no file and no secret.

## D2.3 Service start and stop on macOS, start at login

- Goal: Start and stop the container `sag` on macOS. Apply the LaunchAgent. Prove the bootstrap criteria of the layer.
- Owns: see the ownership table.
- Reads: PRD-devx 5.5, 6, 13; `internal/deploy`.
- Depends on: `D2.0`. Verify after `D2.2`, `D2.4`.
- Method:
  - Start the Podman machine only when it is stopped. On the development machine, it runs, so no machine command runs.
  - Run `podman run -d --replace --name sag --restart=on-failure` with `deploy.RunArgs`.
  - Wait up to 30 s for `GET /healthz` with `<cfg>/ca.pem`. On a timeout, print the last 50 lines of the daemon log and exit 1.
  - LaunchAgent `net.kanthorlabs.sag` with `RunAtLoad` and `ProgramArguments` `<absolute path of sag> service start`. Output to `~/Library/Logs/sag/service.log`.
  - `launchctl bootout`, then `launchctl bootstrap gui/<uid> <plist>`. A `bootout` of an agent that is not loaded is not a failure.
  - Without the domain `gui/<uid>`, write the file and the `warn autostart loads at the next GUI login` line.
  - `service_linux.go` stays a stub until layer 10.
- Tests first: `service_test.go` with a fake runner: `TestMacStartsStoppedMachineOnly`, `TestHealthzTimeoutTailsLog`, `TestLaunchAgentPlist`, `TestBootoutNotLoadedIsNotFailure`, `TestNoGUIDomainWarns`, `TestLinuxUnsupported`.
- e2e: `DXAC01_Bootstrap`, `DXAC02_ContainerAndCert`, `DXAC27_Autostart`, `DXAC28_ServiceStopStart`.
- ACs: DX-AC1, DX-AC2, DX-AC27, DX-AC28.
- Verify: `go test -count=1 ./internal/service/... ./internal/cli/... -run 'Service|Mac|Healthz|LaunchAgent|Bootout|GUIDomain|LinuxUnsupported' && GOOS=linux CGO_ENABLED=0 go vet ./internal/service/... && make e2e-devx SUITE=Main RUN='DXAC0[12]_|DXAC2[78]_' LAYER=2`. Expected: `ok`, then exit 0.
- Done when: the teardown removes the LaunchAgent and the precondition check of the next run passes.

## D2.4 Master key, certificate, image pull

- Goal: Create `sag-master-key`, the endpoint certificate secrets, and `<cfg>/ca.pem`. Pull the image when it is absent.
- Owns: see the ownership table.
- Reads: PRD-devx 4.3, 5.6, 5.7, 13.
- Depends on: `D2.0`.
- Method:
  - Master key: 32 bytes from `crypto/rand`, base64, to the standard input of `podman secret create sag-master-key -`.
  - CA without the name constraints extension, 397 days, key only in memory. One server certificate with `localhost`, `127.0.0.1`, `::1`, and an optional Tailscale address.
  - The five-step sequence and the four triggers of PRD-devx 5.7. A fake clock tests the 30-day trigger (ruling Q31). `SAG_TEST_CERT_DAYS` sets the validity in the test build.
  - The image is `SAG_TEST_IMAGE` in the test build, else the reference of PRD-devx 4.3. Pull only when absent.
- Tests first: `endpointcert`: `TestNoNameConstraints`, `TestServerNames`, `TestTailscaleNameAdded`, `TestSequenceOrder`, `TestTriggers`, `TestRenewalWithFakeClock`, `TestResumeAfterStep3`, `TestKeyNeverInArgsOrFile`; `masterkey`: `TestValueOnStdinOnly`, `TestNeverReplaced`; `image`: `TestPullOnlyWhenAbsent`.
- ACs: none. `D2.3` proves DX-AC1 and DX-AC2 with this code.
- Verify: `go test -count=1 ./internal/endpointcert/... ./internal/masterkey/... ./internal/image/...`. Expected: `ok`.
- Done when: no file outside the secret store holds a private key.

## D2.5 Integration and gate

- Goal: Prove the layer 2 criteria on the development machine and close the layer 2 gate.
- Owns: see the ownership table.
- Depends on: `D2.1` to `D2.4`.
- Steps: README section 6.1.
- ACs: DX-AC1, DX-AC2, DX-AC4, DX-AC27, DX-AC28, DX-AC33, DX-AC36.
- Verify: `make test && make e2e-matrix LAYER=6 PGS="14 17 19" && make e2e-devx SUITE=Main LAYER=2`. Expected: exit 0. The devx report shows each phase A DX-AC of layer 2 `PASS`.
- Done when: the tag `sag-devx-2` exists.

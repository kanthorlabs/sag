# Layer 10: Linux port

| Item | Value |
|---|---|
| Phase | B |
| Goal | `sag bootstrap` and `sag service` work on the ruled Linux platform. Each criterion with the run `A, B` or `B` passes on the Debian machine in its phase B form. |
| ACs | DX-AC5. The phase B clauses of README section 8.1. The phase B run of each criterion with the run `A, B`. |
| PRD-devx sections | 1, 1.4, 3, 5.3 (Linux install), 5.4 (Linux answers), 5.5 (Linux), 5.8 (operator step), 6 (Linux), 8 (secret list), 15 (Linux steps), 17.1 (phase B), 19 step 10 |
| Gate | `D10.4`: README section 6.2 |
| Blocked by | Access to the Debian machine. The ruling of the Debian version and the Linux platform scope. The PRD-devx revision that records the ruling. |
| Agents at once | 3. Start order: `D10.2`, `D10.1`, `D10.3` |
| Wall-clock | `D10.0` 2 h + parallel phase 5 h + `D10.4` 8 h = 15 h |

Agents edit code in their worktrees on the macOS development machine. They run each verify command that names the Debian machine through the access that Ulrich gives.

## File ownership

| Task | Owns |
|---|---|
| `D10.0` | `deploy/pins` (Linux line), `internal/bootstrap/platform.go` (list of accepted platforms only), the signatures of each `_linux.go` file below, `e2e/devx/suite_install_test.go`, `e2e/devx/dxh/precondition.go`, `e2e/devx/dxh/precondition_test.go`, `docs/reviews/devx-layer-10.md` (machine record) |
| `D10.1` | `internal/bootstrap/platform.go` (after `D10.0`), `internal/bootstrap/platform_test.go`, `internal/deps/*_linux.go`, `internal/deps/apt_test.go`, `e2e/devx/dxac05_test.go`, `e2e/devx/dxac08_test.go`, `e2e/devx/testdata/pins/*` |
| `D10.2` | `internal/service/*_linux.go`, `internal/service/linux_test.go`, `e2e/devx/dxh/journal.go`, `e2e/devx/dxh/journal_test.go`, `e2e/devx/dxh/teardown.go`, `e2e/devx/dxac01_test.go`, `dxac02_test.go`, `dxac10_test.go`, `dxac27_test.go`, `dxac40_test.go` |
| `D10.3` | `internal/portcheck/*_linux.go`, `internal/portcheck/ss_test.go`, `internal/tailscale/*_linux.go`, `internal/tailscale/operator_test.go`, `e2e/devx/dxac36_test.go`, `dxac41_test.go`, `dxac50_test.go`, `dxac53_test.go` |
| `D10.4` | Every layer-10 file after `D10.1` to `D10.3` merge. Each file that a Linux defect fix needs; the review file lists it. `docs/reviews/devx-layer-10.md`. |

## D10.0 Contract and machine record

- Goal: Apply the ruling of Ulrich. Freeze the Linux call points. Record the Debian machine facts.
- Owns: see the ownership table.
- Reads: PRD-devx 1, 1.4, 3, 8, 17.1, 19 step 10; README section 2.2.
- Depends on: `D9.6`, access to the Debian machine, and the PRD-devx revision of the ruling.
- Content:
  - `deploy/pins`: the Linux line of the PRD-devx revision of the ruling.
  - `platform.go`: add the ruled Linux platforms to the list of accepted platforms. `D10.1` adds the distribution check.
  - Signatures of the Linux files: apt install, user Quadlet service, `ss` parse, operator step, user journal read.
  - `suite_install_test.go`: `TestDevxInstall` with the group `install`.
  - Linux form of the `dxh` preconditions: no Podman machine check, and no check of the LaunchAgent or `~/Library/Logs/sag/`. Check that the file `~/.config/containers/systemd/sag.container` and the unit `sag.service` do not exist.
  - Linux free-space check: the file system of the Podman store (`podman info --format '{{.Store.GraphRoot}}'`) has at least 10 GiB free. Below the limit, the run writes `fail podman store free space <n> GiB; need 10`.
  - Machine record in `docs/reviews/devx-layer-10.md`: OS version, Podman version from apt, linger state, `sudo -n` result for the test user.
  - Confirm that `podman container inspect` lists the secrets of a container (PRD-devx 8). Confirm that `apt-get install -y podman` installs `uidmap`. Record both results.
- Tests first: `dxh/precondition_test.go` with a fake runner: `TestLinuxPreconditions`, `TestLinuxRefusesLowDisk`. The other content has no logic.
- ACs: none.
- Verify: `make contract-check && GOOS=linux CGO_ENABLED=0 go vet ./... && go test -count=1 ./e2e/devx/dxh/... -run Linux`. Expected: exit 0.
- Done when: `D10.1` to `D10.3` compile against the signatures, and the machine record exists.

## D10.1 Linux install

- Goal: Accept the ruled Linux platform in bootstrap step 1. Apply the dependency rule of PRD-devx 5.3 with `apt`.
- Owns: see the ownership table.
- Reads: PRD-devx 3, 5.3, 5.4, 17.1.
- Depends on: `D10.0`.
- Method:
  - Step 1 reads `ID` of `/etc/os-release` and accepts only the ruled distribution. Each other distribution gets `unsupported platform <os>/<arch>`.
  - Parse `apt-cache policy podman`. Remove the epoch and the Debian suffix. Compare with the pin in the plan.
  - Run `sudo apt-get install -y podman`. Check `newuidmap`. Without a terminal, call `sudo -n`. A refusal by `sudo` gives exit 3.
  - The install prompt of PRD-devx 5.4 names the apt candidate and the pin.
  - Test pins: the `release` item for `linux/*` and the apt package `hello`. Add the `apt` clause to DX-AC8.
  - DX-AC5: `apt-get remove podman` keeps the rootless store. Prove that the image stays after the install.
- Tests first: `platform_test.go`: `TestRuledLinuxAccepted`, `TestOtherDistributionExit1`; `apt_test.go`: `TestAptVersionParse` (`4.9.3+ds1-1ubuntu0.1` gives `4.9.3`), `TestEpochRemoved`, `TestNoCandidateFailsInPlan`, `TestOlderCandidateFailsInPlan`, `TestNewuidmapMissingExit1`, `TestSudoNonInteractive`.
- e2e: `DXAC05_PodmanAptInstall` in the suite `Install`. The `apt` clause of `DXAC08_VersionRule`.
- ACs: DX-AC5. Phase B clause of DX-AC8.
- Verify: `GOOS=linux CGO_ENABLED=0 go vet ./internal/deps/... ./internal/bootstrap/...`, then on the Debian machine `go test -count=1 ./internal/deps/... ./internal/bootstrap/... && make e2e-devx SUITE=Install LAYER=10 PHASE=B && make e2e-devx SUITE=Pins LAYER=10 PHASE=B`. Expected: exit 0, then `ok`, then exit 0.
- Done when: bootstrap never changes an installed version on Linux.

## D10.2 Linux service and start at login

- Goal: Start and stop SAG through the user Quadlet file and the systemd user manager.
- Owns: see the ownership table.
- Reads: PRD-devx 5.5, 6, 9.3 step 7, 13, 15.
- Depends on: `D10.0`. Verify after `D10.1`, because bootstrap step 1 must accept Linux.
- Method:
  - Write `~/.config/containers/systemd/sag.container` from `deploy.Quadlet`. Run `systemctl --user daemon-reload`, then `systemctl --user restart sag`. Stop with `systemctl --user stop sag`.
  - `--autostart yes` adds `[Install] WantedBy=default.target`. `--autostart no` has no `[Install]` section. Bootstrap does not enable linger.
  - `dxh/teardown.go` adds the Linux steps of PRD-devx 15: remove the Quadlet file, then `systemctl --user daemon-reload`.
  - `dxh/journal.go` reads the user journal for DX-AC10 and DX-AC40.
  - Add the phase B clauses of DX-AC1, DX-AC2, DX-AC10, DX-AC27, and DX-AC40.
- Tests first: `linux_test.go` with a fake runner: `TestLinuxStartOrder`, `TestQuadletAutostartNo`, `TestQuadletAutostartYes`, `TestStopUsesSystemctl`; `journal_test.go`: `TestJournalUserScope`.
- e2e: the phase B clauses of `DXAC01_Bootstrap`, `DXAC02_ContainerAndCert`, `DXAC10_CanaryNoLeak`, `DXAC27_Autostart`, `DXAC40_JWTNoLeak`.
- ACs: phase B clauses of DX-AC1, DX-AC2, DX-AC10, DX-AC27, DX-AC40.
- Verify: `GOOS=linux CGO_ENABLED=0 go vet ./internal/service/... ./e2e/devx/dxh/...`, then on the Debian machine `go test -count=1 ./internal/service/... ./e2e/devx/dxh/... && make e2e-devx SUITE=Main RUN='DXAC0[12]_|DXAC10_|DXAC2[78]_|DXAC40_' LAYER=10 PHASE=B`. Expected: exit 0, then `ok`, then exit 0.
- Done when: the teardown leaves no Quadlet file and no unit `sag.service`.

## D10.3 Linux port check, operator step, UID 0, bash profile

- Goal: Name a port holder with `ss`. Add the Tailscale operator step. Prove the other phase B clauses.
- Owns: see the ownership table.
- Reads: PRD-devx 5.4, 5.8 (operator step), 6, 12.2, 17.
- Depends on: `D10.0`. Verify after `D10.1`, `D10.2`.
- Method:
  - `ss -ltnp` names the process on TCP port 8443. The `podman ps` check comes first.
  - Operator condition: `OperatorUser` of `tailscale debug prefs` is the current user. Else, with a terminal, ask the prompt of PRD-devx 5.4. With approval, step 13 runs `sudo tailscale set --operator=<user>` before the serve rule. Without a terminal or approval, write `skip tailscale operator not set; loopback only`.
  - The fake `tailscale` reports the operator. No test runs the real `tailscale` on Linux.
  - DX-AC36: each host command as UID 0 exits 1, through `sudo -n`.
  - DX-AC41: `SHELL=/bin/bash` writes `~/.bashrc`. The test copies the file, removes the two lines at the end, and compares.
  - Add the phase B clauses of DX-AC36, DX-AC41, DX-AC50, and DX-AC53.
- Tests first: `ss_test.go`: `TestSSParse`, `TestSSRootlessport`; `operator_test.go`: `TestOperatorPrompt`, `TestOperatorNoTerminalSkips`, `TestOperatorSetBeforeServe`.
- e2e: the phase B clauses of `DXAC36_HostGuards`, `DXAC41_PiSetup`, `DXAC50_TailscaleFake`, `DXAC53_PortHolder`.
- ACs: phase B clauses of DX-AC36, DX-AC41, DX-AC50, DX-AC53.
- Verify: `GOOS=linux CGO_ENABLED=0 go vet ./internal/portcheck/... ./internal/tailscale/...`, then on the Debian machine `go test -count=1 ./internal/portcheck/... ./internal/tailscale/... && make e2e-devx SUITE=Main RUN='DXAC36_|DXAC41_|DXAC5[03]_' LAYER=10 PHASE=B`. Expected: exit 0, then `ok`, then exit 0.
- Done when: a port conflict on Linux changes no file, secret, or container.

## D10.4 Integration and phase B gate

- Goal: Prove each phase B criterion on the Debian machine and close devx v1.
- Owns: see the ownership table.
- Depends on: `D10.1` to `D10.3`.
- Steps: README section 6.2. Fix each Linux defect of a criterion with the run `A, B` in this task. After a fix, run the phase A gate commands of the affected suites on the macOS development machine again.
- ACs: DX-AC5, and each criterion with the run `A, B` in its phase B form.
- Verify: on the Debian machine, `make test && make e2e LAYER=6 PG=17 && make e2e-devx SUITE=Main LAYER=10 PHASE=B && make e2e-devx SUITE=Pins LAYER=10 PHASE=B && make e2e-devx SUITE=Install LAYER=10 PHASE=B && make e2e-devx-unit LAYER=10 PHASE=B`. Expected: exit 0. The devx report shows each DX-AC with the run `A, B` or `B` `PASS`.
- Done when: the tag `sag-devx-10` exists. The merge of `feat/devx` into `main` is a decision of Ulrich.

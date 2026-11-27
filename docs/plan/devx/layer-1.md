# Layer 1: Base for devx

| Item | Value |
|---|---|
| Phase | A |
| Goal | The v1 product runs on `internal/deploy` and the Podman SecureStorage backend. The DX harness runs an empty `Main` suite on the macOS development machine. |
| ACs | None of DX. v1 AC32 and AC33 pass with `internal/deploy` (PRD-devx 19 step 1). |
| PRD-devx sections | 8 (secret list check), 11.1, 11.2 Podman backend, 13, 16 row 13.4, 17.1 |
| Gate | `D1.4`: README section 6.1 |
| Agents at once | 3. Start order: `D1.3`, `D1.1`, `D1.2` |
| Wall-clock | `D1.0` 1 h + parallel phase 4 h + `D1.4` 4 h = 9 h |

## File ownership

| Task | Owns |
|---|---|
| `D1.0` | `internal/deploy/*.go` stubs, `internal/secret/storage.go`, `internal/secret/podman/*.go` stubs, `internal/podman/*.go` stubs, `e2e/devx/acs.go`, all layer-1 stub files until their owner task takes them |
| `D1.1` | `internal/deploy/*` except `mounts.go` and `mounts_test.go`, `e2e/harness/flags.go`, `e2e/harness/flags_test.go`, `e2e/harness/quadlet_test.go`, `e2e/ac01_test.go`, `e2e/ac32_test.go`, `deploy/sag.container` |
| `D1.2` | `internal/secret/podman/*`, `internal/podman/*`, `internal/secret/storage_test.go`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/daemon/resources.go`, `internal/deploy/mounts.go`, `internal/deploy/mounts_test.go` |
| `D1.3` | `e2e/devx/suite_main_test.go`, `e2e/devx/dxh/*`, `e2e/cmd/acreport/*`, `Makefile`, `.github/workflows/ci.yml` |
| `D1.4` | Every layer-1 file after `D1.1` to `D1.3` merge. `docs/reviews/devx-layer-1.md`. |

## D1.0 Contract

- Goal: Freeze the `internal/deploy` API, the SecureStorage interface, the Podman runner API, and the DX AC table.
- Owns: see the ownership table.
- Reads: PRD-devx 11.1, 13, 17; `e2e/harness/flags.go`; `e2e/acs.go`; README section 5.4.
- Depends on: `D0.1`, `D0.2`, `D0.3`.
- Content:
  - `internal/deploy`: `ProductFlags`, `ContainerFlags`, `LogFlags`, `ImageDigest` (empty `var`), `RunArgs(cfg, opts) []string`, `Quadlet(cfg, opts) string`, `SecretMounts(cfg) []string` in `mounts.go`. `opts` holds the image, the config source path, the autostart value, and the restart policy.
  - `internal/secret/storage.go`: `Metadata`, `ErrExists`, `ErrNotFound`, `ErrUnavailable`, `SecureStorage`, exactly as PRD-devx 11.1.
  - `internal/podman`: `Runner{Path string}`, `Run(ctx, stdin io.Reader, args ...string)`, `SecretCreate`, `SecretInspect`, `SecretList`, `SecretRemove`, `ContainerInspect`.
  - `e2e/devx/acs.go`: the 53 rows of README section 8 with `ID`, `Run`, `RunBy`, and `Tests` (name and layer).
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: `D1.1` to `D1.3` compile against the stubs.

## D1.1 Deploy package

- Goal: Move the container flags to `internal/deploy`. Add the log flags. Build the `podman run` arguments and the Quadlet text from one source.
- Owns: see the ownership table.
- Reads: PRD-devx 13; PRD 6, 13.5; `deploy/dev/sag.yaml`.
- Depends on: `D1.0`.
- Method:
  - `harness.ContainerFlags` refers to `deploy.ContainerFlags`. `TestOnlyFlags` loses `--log-driver k8s-file`.
  - `ContainerFlags` adds `--log-driver k8s-file --log-opt max-size=10mb`. The Quadlet keys are `LogDriver=k8s-file` and `PodmanArgs=--log-opt=max-size=10mb`.
  - The publish is `127.0.0.1:8443:8443` only.
  - Update `deploy/sag.container` to the Quadlet output for `deploy/dev/sag.yaml` and the server-install options.
- Tests first: `internal/deploy/quadlet_test.go`: `TestQuadletGolden` (compares with `deploy/sag.container`), `TestQuadletLogKeys`, `TestQuadletAutostartInstallSection`; `run_test.go`: `TestRunArgsLogFlags`, `TestRunArgsPublishLoopbackOnly`.
- e2e: none of DX. The v1 subtests `AC32_ContainerInspect` and `AC33_SecretMounts` run on the new flags.
- ACs: v1 AC32, AC33.
- Verify: `go test -count=1 ./internal/deploy/... ./e2e/harness/... && make e2e LAYER=6 PG=17 RUN='AC3[23]_'`. Expected: `ok`, then exit 0.
- Done when: no e2e harness file defines a container flag outside `internal/deploy`.

## D1.2 Podman SecureStorage backend

- Goal: Implement the Podman backend of PRD-devx 11.2. Accept an empty `resources` map. Build the secret mounts for `secret:` references. Confirm the secret list of `podman container inspect` on the development machine.
- Owns: see the ownership table.
- Reads: PRD-devx 8, 11, 13; PRD 6, 13.4; `internal/secret/load.go`.
- Depends on: `D1.0`.
- Method:
  - `Put` writes the value to the standard input of `podman secret create --label <k>=<v> <name> -`.
  - `Metadata` uses `podman secret inspect <name>` without `--showsecret`.
  - `List` runs `podman secret ls`, then `Metadata` of each name, then a label filter in SAG.
  - `Load` is the v1 file load with the PRD 6 owner and mode rules.
  - `config.Load` accepts an absent or empty `resources` key. AC1 keeps its five fixtures.
  - `SecretMounts`: each `secret:` reference of `sag.yaml` and `sag-master-key`. Layer 8 adds the `storage.aws.credentials` reference and the `aws-sm:` skip.
- Tests first: `internal/secret/podman/podman_test.go` with a fake runner: `TestPutValueOnStdinOnly`, `TestPutExistsReturnsErrExists`, `TestDeleteAbsentIsNil`, `TestMetadataNoShowsecret`, `TestListFiltersLabels`, `TestErrorsHoldNoValue`; `config_test.go`: `TestEmptyResourcesAccepted`, `TestAbsentResourcesAccepted`; `mounts_test.go`: `TestSecretMountsSecretRefs`, `TestSecretMountsMasterKey`.
- Integration: `internal/podman/inspect_integration_test.go`: `TestContainerInspectListsSecrets`.
- ACs: none.
- Verify: `go test -count=1 ./internal/secret/... ./internal/config/... ./internal/daemon/... ./internal/deploy/...` and `make test-integration PG=17 PKG=./internal/podman/... RUN=TestContainerInspectListsSecrets`. Expected: `ok`.
- Done when: `D1.4` records the result of `TestContainerInspectListsSecrets` on the development machine.

## D1.3 DX harness, Makefile, CI unit job

- Goal: Run an empty `Main` suite on the macOS development machine under the DX run lock. Produce the devx AC report. Add the `devx-unit` CI job.
- Owns: see the ownership table.
- Reads: PRD-devx 15, 17.1; README sections 2.1, 5; `e2e/cmd/acreport/*`; `e2e/harness/tty.go`.
- Depends on: `D1.0`.
- Method:
  - `dxh` runs the host binary with the child environment and the test `PATH` of README section 2.1. It drives a pseudo-terminal with `creack/pty`.
  - Child environment: the allow list of README section 2.1. In the suites `Main`, `Pins`, and `Mac`, new test directories for `CLAUDE_CONFIG_DIR` and `PI_CODING_AGENT_DIR` for each group.
  - `dxh` builds Go programs into the test program directory. It runs the teardown of README section 2.1: PRD-devx 15 steps 2 to 8 for macOS, `podman secret rm` in place of step 1, and the labelled objects.
  - `dxh` checks the preconditions of README section 2.1 before the first group, the free space first. A failed check exits 1 and changes nothing.
  - Run recovery of README section 2.1: the marker `.dev/local/devx-run-active`, and one teardown on a failed precondition with the marker.
  - `make e2e-devx` writes the lock file `owner`. It removes a lock whose owner PID does not run, with `kill -0 <PID>`.
  - Each container, volume, secret, network, and image that `dxh` creates has the label `sag.devx-e2e=1`. `dxh` records each pulled image in `.dev/local/devx-pulled.txt`.
  - Group setup: before each group except `image`, tag `localhost/sag:devx-e2e` as `docker.io/kanthorlabs/sag:main`.
  - `TestDevxMain` runs the group `harness` with the subtest `DX_Harness_Skeleton`. It checks the preconditions and rootless `podman info` through `podman-machine-default`.
  - `DX_Harness_Skeleton` also proves the host effects of an agent session: `launchctl print gui/<uid>` exits 0. A file write and removal in `~/Library/LaunchAgents` and in `~/.config` succeed. A listener on `127.0.0.1:8443` opens and closes. One pseudo-terminal round trip succeeds. The free-space check passes.
  - `acreport`: `-suite devx`, `-layer`, `-phase`, the `env` block, and the gate rule of README section 5.4.
  - `Makefile`: `build-devxtest` and `e2e-devx` (README section 5.2). `contract-check` adds `go vet -tags devxtest ./...` and `go vet -tags devxe2e ./e2e/devx/...`.
  - `ci.yml`: job `devx-unit` (README section 5.3).
- Tests first: `e2e/cmd/acreport/report_test.go`: `TestDevxOtherPhase`, `TestDevxEnvBlockHasNoIP`, `TestDevxGateRule`, `TestDevxCommitMismatchFails`, `TestDevxTestLayers`; `dxh/teardown_test.go` with a fake runner: `TestTeardownOrder`, `TestTeardownRemovesOnlyLabeled`, `TestTeardownNeverCallsCredentialDelete`, `TestTeardownKeepsDevxImage`; `dxh/precondition_test.go`: `TestRefusesSecondMachine`, `TestRefusesStoppedMachine`, `TestRefusesLowDisk`, `TestRefusesContainerSag`, `TestRefusesDevContainers`, `TestRefusesPortHeld`, `TestRefusesCfgOrLaunchAgent`, `TestRefusesSAGSecretsOrVolume`, `TestMarkerRunsTeardownOnce`, `TestNoMarkerChangesNothing`, `TestNoPodmanMachineCommandRuns`; `dxh/env_test.go`: `TestChildEnvAllowList`, `TestHarnessDirsAreTestDirs`; `dxh/path_test.go`: `TestPathHasNoHarnessOrTailscale`, `TestRealProgramLinkDir`; `dxh/group_test.go`: `TestGroupSetupTagsMainImage`.
- ACs: none.
- Verify: `go test -count=1 ./e2e/cmd/acreport/... ./e2e/devx/dxh/... && make contract-check && make e2e-devx SUITE=Main LAYER=1 && podman run --rm -v "$PWD:/repo:ro" -w /repo docker.io/rhysd/actionlint:1.7.9`. Expected: `ok`, then exit 0 for each command.
- Done when: `e2e/report/devx/ac-report.md` shows `DX_Harness_Skeleton` `PASS` and the AC table rows equal README section 8.

## D1.4 Integration and gate

- Goal: Prove v1 AC1 to AC34 on the new base and close the layer 1 gate.
- Owns: see the ownership table.
- Depends on: `D1.1` to `D1.3`.
- Steps: README section 6.1. Record the result of `TestContainerInspectListsSecrets` on the development machine in `docs/reviews/devx-layer-1.md` (PRD-devx 19 step 1).
- ACs: v1 AC32, AC33. No DX-AC.
- Verify: `make test && make e2e-matrix LAYER=6 PGS="14 17 19" && make e2e-devx SUITE=Main LAYER=1`. Expected: exit 0. The devx report shows `DX_Harness_Skeleton` `PASS` and each DX-AC `NOT_IMPLEMENTED`.
- Done when: the tag `sag-devx-1` exists.

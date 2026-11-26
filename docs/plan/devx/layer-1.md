# Layer 1: Base for devx

| Item | Value |
|---|---|
| Goal | The v1 product runs on `internal/deploy` and the Podman SecureStorage backend. The DX e2e suite runs green in CI. |
| ACs | None of DX. v1 AC32 and AC33 pass with `internal/deploy` (PRD-devx 19 step 1). |
| PRD-devx sections | 8 (Podman 4.9.3 check), 11.1, 11.2 Podman backend, 13, 16 row 13.4, 17.1 |
| Gate | `D1.4`: README section 6. The `devx-e2e` skeleton job and `devx-report` pass in CI. |
| Agents at once | 3. Start order: `D1.3`, `D1.1`, `D1.2` |
| Wall-clock | `D1.0` 1 h + parallel phase 4 h + `D1.4` 3 h = 8 h |

## File ownership

| Task | Owns |
|---|---|
| `D1.0` | `internal/deploy/*.go` stubs, `internal/secret/storage.go`, `internal/secret/podman/*.go` stubs, `internal/podman/*.go` stubs, `e2e/devx/acs.go`, all layer-1 stub files until their owner task takes them |
| `D1.1` | `internal/deploy/*`, `e2e/harness/flags.go`, `e2e/harness/flags_test.go`, `e2e/harness/quadlet_test.go`, `e2e/ac01_test.go`, `e2e/ac32_test.go`, `deploy/sag.container` |
| `D1.2` | `internal/secret/podman/*`, `internal/podman/*`, `internal/secret/storage_test.go`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/daemon/resources.go` |
| `D1.3` | `e2e/devx/devx_test.go`, `e2e/devx/dxh/*`, `e2e/cmd/acreport/*`, `Makefile`, `.github/workflows/ci.yml` |
| `D1.4` | Every layer-1 file after `D1.1` to `D1.3` merge. `docs/reviews/devx-layer-1.md`. |

## D1.0 Contract

- Goal: Freeze the `internal/deploy` API, the SecureStorage interface, the Podman runner API, and the DX AC table.
- Owns: see the ownership table.
- Reads: PRD-devx 11.1, 13, 17; `e2e/harness/flags.go`; `e2e/acs.go`.
- Depends on: `D0`.
- Content:
  - `internal/deploy`: `ProductFlags`, `ContainerFlags`, `LogFlags`, `ImageDigest` (empty `var`), `RunArgs(cfg, opts) []string`, `Quadlet(cfg, opts) string`, `SecretMounts(cfg) []string`. `opts` holds the image, the config source path, the autostart value, and the restart policy.
  - `internal/secret/storage.go`: `Metadata`, `ErrExists`, `ErrNotFound`, `ErrUnavailable`, `SecureStorage`, exactly as PRD-devx 11.1.
  - `internal/podman`: `Runner{Path string}`, `Run(ctx, stdin io.Reader, args ...string)`, `SecretCreate`, `SecretInspect`, `SecretList`, `SecretRemove`, `ContainerInspect`.
  - `e2e/devx/acs.go`: the 53 rows of README section 8 with `ID`, `Layer`, `Test`, `Run`.
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
  - Secret mounts: each `secret:` reference of `sag.yaml`, the `storage.aws.credentials` reference, and `sag-master-key`. An `aws-sm:` reference gets no mount.
  - Update `deploy/sag.container` to the Quadlet output for `deploy/dev/sag.yaml` and the server-install options.
- Tests first: `internal/deploy/quadlet_test.go`: `TestQuadletGolden` (compares with `deploy/sag.container`), `TestQuadletLogKeys`, `TestQuadletAutostartInstallSection`; `run_test.go`: `TestRunArgsLogFlags`, `TestRunArgsPublishLoopbackOnly`, `TestSecretMountsSkipAWS`.
- e2e: none of DX. The v1 subtests `AC32_ContainerInspect` and `AC33_SecretMounts` run on the new flags.
- ACs: v1 AC32, AC33.
- Verify: `go test -count=1 ./internal/deploy/... ./e2e/harness/...`. Expected: `ok`.
- Done when: no e2e harness file defines a container flag outside `internal/deploy`.

## D1.2 Podman SecureStorage backend

- Goal: Implement the Podman backend of PRD-devx 11.2. Accept an empty `resources` map. Confirm the secret list of `podman container inspect` on Podman 4.9.3.
- Owns: see the ownership table.
- Reads: PRD-devx 8, 11; PRD 6, 13.4; `internal/secret/load.go`.
- Depends on: `D1.0`.
- Method:
  - `Put` writes the value to the standard input of `podman secret create --label <k>=<v> <name> -`.
  - `Metadata` uses `podman secret inspect <name>` without `--showsecret`.
  - `List` runs `podman secret ls`, then `Metadata` of each name, then a label filter in SAG.
  - `Load` is the v1 file load with the PRD 6 owner and mode rules.
  - `config.Load` accepts an absent or empty `resources` key. AC1 keeps its five fixtures.
  - The daemon loads `secret:` references through the backend `Load`.
- Tests first: `internal/secret/podman/podman_test.go` with a fake runner: `TestPutValueOnStdinOnly`, `TestPutExistsReturnsErrExists`, `TestDeleteAbsentIsNil`, `TestMetadataNoShowsecret`, `TestListFiltersLabels`, `TestErrorsHoldNoValue`; `config_test.go`: `TestEmptyResourcesAccepted`, `TestAbsentResourcesAccepted`.
- Integration: `internal/podman/inspect_integration_test.go`: `TestContainerInspectListsSecrets`. The CI `e2e` job runs it on Podman 4.9.3 through `make test-integration`.
- ACs: none.
- Verify: `go test -count=1 ./internal/secret/... ./internal/config/... ./internal/daemon/...` and `make test-integration PG=17 PKG=./internal/podman/... RUN=TestContainerInspectListsSecrets`. Expected: `ok`.
- Done when: the 4.9.3 result is in the CI log of the `D1.4` push. `D1.4` records it.

## D1.3 DX e2e harness and CI skeleton

- Goal: Run an empty DX suite in CI on the real `HOME` with the systemd user manager. Produce the devx AC report.
- Owns: see the ownership table.
- Reads: PRD-devx 15, 17.1; README section 5; `e2e/cmd/acreport/*`; `e2e/harness/tty.go`.
- Depends on: `D1.0`.
- Method:
  - `dxh` runs the host binary with a given environment and `PATH`. It drives a pseudo-terminal with `creack/pty`.
  - `dxh` builds Go programs into a test `PATH` directory, reads the user journal, and runs the PRD-devx 15 teardown steps 1 to 8.
  - `TestDevx` runs the subtest `DX_Harness_Skeleton`. It checks `systemctl --user is-system-running` and rootless `podman info`.
  - `acreport`: `-suite devx`, `-env ci|dev-machine`, the `env` block, and `merge-devx` (README section 5.4).
  - `Makefile`: `build-devxtest`, `e2e-devx`, `e2e-devx-mac` (README section 5.2).
  - `ci.yml`: jobs `devx-e2e` and `devx-report` (README section 5.3).
- Tests first: `e2e/cmd/acreport/report_test.go`: `TestDevxExcludesOtherEnv`, `TestDevxEnvBlockHasNoIP`, `TestMergeDevxGate`; `dxh/teardown_test.go` with a fake runner: `TestTeardownOrder`.
- ACs: none.
- Verify: `go test -count=1 ./e2e/cmd/acreport/... ./e2e/devx/dxh/... && go vet -tags devxe2e ./e2e/devx/... && podman run --rm -v "$PWD:/repo:ro" -w /repo docker.io/rhysd/actionlint:1.7.9`. Expected: `ok`, then exit 0.
- Done when: `actionlint` passes and the AC table rows equal README section 8.

## D1.4 Integration and gate

- Goal: Prove v1 AC1 to AC35 on the new base and close the layer 1 gate.
- Owns: see the ownership table.
- Depends on: `D1.1` to `D1.3`.
- Steps: README section 6. Run `make e2e-matrix LAYER=6 PGS="14 17 19"` in place of gate step 1. Record the result of `TestContainerInspectListsSecrets` on Podman 4.9.3 in `docs/reviews/devx-layer-1.md`.
- ACs: v1 AC32, AC33. No DX-AC.
- Verify: CI run. Expected: `ac35` `PASS`. `devx-report` shows `DX_Harness_Skeleton` `PASS` and each DX-AC `NOT_IMPLEMENTED`.
- Done when: the tag `sag-devx-1` exists.

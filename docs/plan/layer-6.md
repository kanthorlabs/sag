# Layer 6: Container and platforms

| Item | Value |
|---|---|
| Goal | The Quadlet deployment matches the tested container. CI proves the PostgreSQL range 14 to 19 and the macOS build. |
| ACs | AC32, AC33, AC34, AC35 |
| PRD sections | 6, 13.5, 15.2 step 6 |
| Gate | `T6.4`: local `make e2e-matrix LAYER=6 PGS="14 17 19"`, then the CI run, review in `docs/reviews/layer-6.md` (README section 7) |
| Available parallelism | 3 (`T6.1` to `T6.3`) |
| Agents at once | 3 (README section 3.1). Start order: `T6.3`, `T6.1`, `T6.2` |
| Wall-clock with pool of 3 | `T6.0` 0.5 h + parallel phase 2 h + `T6.4` 3 h = 5.5 h |
| Critical path | `T6.0` (0.5 h) -> `T6.3` (2 h) -> `T6.4` (3 h, with the CI run) = 5.5 h |

A task is done only when its verify command passes and the task is merged (README section 6).

## File ownership

| Task | Owns |
|---|---|
| `T6.0` | `e2e/harness/flags.go`, `e2e/harness/api.go` (amendments), all layer-6 stub files until their owner task takes them |
| `T6.1` | `deploy/sag.container`, `e2e/harness/quadlet_test.go`, `e2e/ac32_test.go`, `e2e/ac34_test.go` |
| `T6.2` | `e2e/harness/busybox.go`, `e2e/ac33_test.go` |
| `T6.3` | `.github/workflows/ci.yml`, `e2e/cmd/acreport/merge.go` |

## T6.0 Contract

- Goal: Freeze the SAG container flag list as one exported value. Freeze the input format of `acreport merge`.
- Owns: see the ownership table.
- Reads: PRD section 6; `testing.md` sections 3 and 5.
- Depends on: `T5.4`.
- Content:
  - `flags.go`: `ContainerFlags` with the values of `testing.md` section 3. The harness uses only this value.
  - `harness/api.go`: `BusyboxMount` signature.
  - Stub: `acreport merge` reads the `ac-report.json` file of each matrix job and the `build-macos` result.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: every later task of layer 6 can compile against the stubs.

## T6.1 Quadlet and container checks

- Goal: Write the Quadlet file with the container constraints of PRD section 6. Prove that it equals the harness flags.
- Owns: `deploy/sag.container`, `e2e/harness/quadlet_test.go`, `e2e/ac32_test.go`, `e2e/ac34_test.go`.
- Reads: `e2e/harness/flags.go`.
- Depends on: `T6.0`.
- Tests first: `quadlet_test.go`: `TestQuadletMatchesHarnessFlags`, `TestPublishPortLoopbackOrTailnet`, `TestOneSecretLinePerSecret`.
- e2e: `AC32_ContainerInspect`: `podman inspect` with `jq` shows `ReadonlyRootfs` true, user `65532`, all capabilities dropped, `no-new-privileges`, and host IP `127.0.0.1` or an address in `100.64.0.0/10`. `AC34_JWTGenerateNoTTY`: `podman exec sag sag jwt generate a` exits non-zero.
- ACs: AC32, AC34.
- Verify: `go test -count=1 ./e2e/harness/... -run 'TestQuadlet|TestPublishPort|TestOneSecretLine'`. Expected: `ok`.
- Done when: the Quadlet keys and the harness flags are equal.

## T6.2 Secret mount check

- Goal: Prove the owner and mode of each mounted secret inside the container.
- Owns: `e2e/harness/busybox.go`, `e2e/ac33_test.go`.
- Reads: `e2e/harness/api.go`.
- Depends on: `T6.0`.
- Method: copy `/bin/busybox` out of `busybox:1.37.0-musl` with `podman create` and `podman cp`. Mount it read-only at `/e2e/busybox` in the e2e `sag` container only.
- Tests first: `busybox_integration_test.go`: `TestBusyboxExtracted`, `TestMountReadOnly`.
- e2e: `AC33_SecretMounts`: `podman exec sag /e2e/busybox sh -c '/e2e/busybox stat -c "%n %u %a %F" /run/secrets/*'` shows `65532 400` for each regular file. The busybox shell has no `stat` in `PATH`, so the command calls the applet through `/e2e/busybox`. The count of regular files equals the count of secrets in `podman container inspect`. A directory that Podman adds from `mounts.conf`, for example `rhsm`, is outside SAG control and outside the check.
- ACs: AC33.
- Verify: `make test-integration PG=17 PKG=./e2e/harness/... RUN='TestBusybox|TestMountReadOnly'`. Expected: `ok`.
- Done when: the production image and the Quadlet file stay without busybox.

## T6.3 CI matrix

- Goal: Add the `e2e` matrix job and the `ac35` job of `testing.md` section 5.
- Owns: `.github/workflows/ci.yml`, `e2e/cmd/acreport/merge.go`.
- Reads: `Makefile`, `e2e/cmd/acreport/report.go`.
- Depends on: `T6.0`.
- Tests first: `merge_test.go`: `TestAC35NeedsAllMajors`, `TestAC35NeedsMacOSBuild`, `TestAC31ExcludedInCI`.
- ACs: AC35.
- Verify: `go test -count=1 ./e2e/cmd/acreport/... && podman run --rm -v "$PWD:/repo:ro" -w /repo docker.io/rhysd/actionlint:1.7.7`. Expected: `ok`, then exit 0 with no output.
- Done when: the workflow lints clean and the merge rule passes its tests.

## T6.4 Integration, gate, and CI run

- Goal: Prove AC32 to AC35 with no regression of AC1 to AC31, and close the layer 6 gate.
- Owns: every layer-6 file after all other layer-6 tasks merge. `docs/reviews/layer-6.md`.
- Depends on: `T6.1` to `T6.3`.
- Steps:
  1. Run README section 7 steps 1 to 4 on the macOS development machine.
  2. Ask Ulrich to push `feat/v1-postgres`. Wait for the CI run.
  3. Download the `ac35` job report. Use it as the gate report.
  4. Run README section 7 steps 5 to 12. A fix in step 8 needs a new push by Ulrich.
- ACs: AC32 to AC35.
- Verify: `make e2e-matrix LAYER=6 PGS="14 17 19"`, then the CI `ac35` job. Expected: local exit 0 with AC1 to AC34 `PASS`; the CI merged report shows AC35 `PASS`.
- Done when: README section 7 step 11 is complete. v1 is ready for release.

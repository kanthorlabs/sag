# Layer 8: Image selection and release workflow

| Item | Value |
|---|---|
| Goal | A release binary pulls the image by its embedded digest. A tag `v*` publishes attested binaries and the release image. A push to `main` publishes the dev image. |
| ACs | DX-AC30 |
| PRD-devx sections | 4.1, 4.2, 4.3, 4.4 |
| Gate | `D8.3`: README section 6. `actionlint` passes on both workflows. |
| Agents at once | 2. Start order: `D8.2`, `D8.1` |
| Wall-clock | `D8.0` 0.5 h + parallel phase 3 h + `D8.3` 2.5 h = 6 h |

## File ownership

| Task | Owns |
|---|---|
| `D8.0` | `internal/image/select.go` stub, `.github/workflows/release.yml` skeleton, `.github/workflows/ci.yml` (`image-main` skeleton) |
| `D8.1` | `internal/image/*`, `internal/cli/version.go`, `internal/cli/version_test.go`, `e2e/devx/dxac30_test.go`, `e2e/devx/testdata/registries.conf` |
| `D8.2` | `.github/workflows/release.yml`, `.github/workflows/ci.yml` (job `image-main` only), `deploy/Containerfile` (multi-architecture build arguments only) |
| `D8.3` | Every layer-8 file after `D8.1` and `D8.2` merge. `docs/reviews/devx-layer-8.md`. |

## D8.0 Contract

- Goal: Freeze the image reference function and the workflow job names.
- Owns: see the ownership table.
- Reads: PRD-devx 4.
- Depends on: `D7.3`.
- Content:
  - `image.Reference() (ref string, devBuild bool)`: `docker.io/kanthorlabs/sag@<deploy.ImageDigest>`, else `docker.io/kanthorlabs/sag:main`.
  - `release.yml` jobs: `image`, `binaries` (matrix of three targets), `release`.
  - `ci.yml` job `image-main` with `if: github.event_name == 'push' && github.ref == 'refs/heads/main'`.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: `D8.1` and `D8.2` compile and lint against the skeletons.

## D8.1 Image selection

- Goal: Select the image by digest or by the `main` tag, and pull it only when it is absent.
- Owns: see the ownership table.
- Reads: PRD-devx 4.3, 17.1.
- Depends on: `D8.0`.
- Method:
  - A dev build writes `warn image dev build; pulls the main tag` only when it pulls.
  - `sag version` prints the version and the image reference.
  - `SAG_TEST_IMAGE` keeps priority in the test build only.
  - DX-AC30 runs a local registry container pinned by digest. A `registries.conf` mirror maps `docker.io/kanthorlabs/sag` to the local registry. The product binary thus pulls through the mirror.
  - The test builds one binary with `-X github.com/kanthorlabs/sag/internal/deploy.ImageDigest=<test digest>` and one without.
- Tests first: `select_test.go`: `TestDigestReference`, `TestDevBuildMainTag`, `TestWarnOnlyOnPull`; `version_test.go`: `TestPrintsImageReference`.
- e2e: `DXAC30_ImageSelection`.
- ACs: DX-AC30.
- Verify: `go test -count=1 ./internal/image/... ./internal/cli/... -run 'Digest|DevBuild|WarnOnly|ImageReference'`. Expected: `ok`.
- Done when: the product build ignores `SAG_TEST_IMAGE`.

## D8.2 Release workflow and dev image

- Goal: Write the release workflow of PRD-devx 4.2 and the dev image job of PRD-devx 4.1.
- Owns: see the ownership table.
- Reads: PRD-devx 4.1, 4.2, 4.4; `docs/plan/testing.md` section 5.
- Depends on: `D8.0`.
- Method:
  - `release.yml` on tag `v*`, permissions `contents: write`, `id-token: write`, `attestations: write`.
  - Step order of PRD-devx 4.2. Build the image per architecture on native runners `ubuntu-24.04` and `ubuntu-24.04-arm`. Join them in one manifest list. Record the digest of the manifest list.
  - Build `sag_darwin_arm64` on `macos-26`, `sag_linux_amd64` on `ubuntu-24.04`, `sag_linux_arm64` on `ubuntu-24.04-arm`, with cgo and the two `-X` flags.
  - Write `SHA256SUMS`. Attest each binary with `actions/attest-build-provenance`. Publish the GitHub release.
  - `image-main` builds the same manifest list with the tag `main`. Registry login reads `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`.
  - Pin each action by commit SHA. Do not grant a write permission to `ci.yml` outside `image-main`.
- Tests first: none. The workflow has no unit test.
- ACs: none.
- Verify: `podman run --rm -v "$PWD:/repo:ro" -w /repo docker.io/rhysd/actionlint:1.7.9`. Expected: exit 0 with no output.
- Done when: the release job runs only after the image job and the binary jobs succeed.

## D8.3 Integration and gate

- Goal: Prove DX-AC30 in CI and close the layer 8 gate.
- Owns: see the ownership table.
- Depends on: `D8.1`, `D8.2`.
- Steps: README section 6. Set the devx gate layer of `ci.yml` to 8. Ask Ulrich to confirm that the GitHub secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` exist.
- ACs: DX-AC30.
- Verify: CI run. Expected: `ac35` `PASS`. `devx-report` shows each DX-AC of layers 2 to 8 `PASS`. `image-main` is skipped on `feat/devx`.
- Done when: the tag `sag-devx-8` exists.

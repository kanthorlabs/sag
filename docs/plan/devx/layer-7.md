# Layer 7: Image selection and release workflow

| Item | Value |
|---|---|
| Phase | A |
| Goal | A release binary pulls the image by its embedded digest. A tag `v*` publishes attested binaries and the release image. A push to `main` publishes the dev image. |
| ACs | DX-AC30 (phase A form) |
| PRD-devx sections | 4.1, 4.2, 4.3, 4.4 |
| Gate | `D7.3`: README section 6.1. `actionlint` passes on both workflows. |
| Blocked by | Open point O1 (README section 10 risk 10): the registry redirect of DX-AC30 on the development machine |
| Agents at once | 2. Start order: `D7.2`, `D7.1` |
| Wall-clock | `D7.0` 0.5 h + parallel phase 3 h + `D7.3` 6.5 h = 10 h |

## File ownership

| Task | Owns |
|---|---|
| `D7.0` | `internal/image/select.go` stub, `.github/workflows/release.yml` skeleton, `.github/workflows/ci.yml` (`image-main` skeleton) |
| `D7.1` | `internal/image/*`, `internal/cli/version.go`, `internal/cli/version_test.go`, `e2e/devx/dxac30_test.go`, `e2e/devx/testdata/registry/*` |
| `D7.2` | `.github/workflows/release.yml`, `.github/workflows/ci.yml` (job `image-main` only), `deploy/Containerfile` (multi-architecture build arguments only) |
| `D7.3` | Every layer-7 file after `D7.1` and `D7.2` merge. `docs/reviews/devx-layer-7.md`. |

## D7.0 Contract

- Goal: Freeze the image reference function, the workflow job names, and the registry redirect of DX-AC30.
- Owns: see the ownership table.
- Reads: PRD-devx 4, 17.1.
- Depends on: `D6.6` and the ruling of O1.
- Content:
  - `image.Reference() (ref string, devBuild bool)`: `docker.io/kanthorlabs/sag@<deploy.ImageDigest>`, else `docker.io/kanthorlabs/sag:main`.
  - `release.yml` jobs: `image`, `binaries` (matrix of three targets), `release`.
  - `ci.yml` job `image-main` with `if: github.event_name == 'push' && github.ref == 'refs/heads/main'` and `needs: [unit, e2e, ac35, devx-unit]`.
  - Record the redirect method of the O1 ruling in `docs/reviews/devx-layer-7.md`. The method changes no setting of `podman-machine-default`.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check && podman run --rm -v "$PWD:/repo:ro" -w /repo docker.io/rhysd/actionlint:1.7.9`. Expected: exit 0.
- Done when: `D7.1` and `D7.2` compile and lint against the skeletons.

## D7.1 Image selection

- Goal: Select the image by digest or by the `main` tag, and pull it only when it is absent.
- Owns: see the ownership table.
- Reads: PRD-devx 4.3, 17.1.
- Depends on: `D7.0`.
- Method:
  - A dev build writes `warn image dev build; pulls the main tag` only when it pulls.
  - `sag version` prints the version and the image reference.
  - `SAG_TEST_IMAGE` keeps priority in the test build only.
  - DX-AC30 runs a local registry container pinned by digest and uses the redirect method of `D7.0`.
  - The test builds one binary with `-X github.com/kanthorlabs/sag/internal/deploy.ImageDigest=<test digest>` and one without.
- Tests first: `select_test.go`: `TestDigestReference`, `TestDevBuildMainTag`, `TestWarnOnlyOnPull`; `version_test.go`: `TestPrintsImageReference`.
- e2e: `DXAC30_ImageSelection`.
- ACs: DX-AC30.
- Verify: `go test -count=1 ./internal/image/... ./internal/cli/... -run 'Digest|DevBuild|WarnOnly|ImageReference' && make e2e-devx SUITE=Main RUN=DXAC30_ LAYER=7`. Expected: `ok`, then exit 0.
- Done when: the product build ignores `SAG_TEST_IMAGE`, and the DX-AC30 run leaves no registry setting on `podman-machine-default`.

## D7.2 Release workflow and dev image

- Goal: Write the release workflow of PRD-devx 4.2 and the dev image job of PRD-devx 4.1.
- Owns: see the ownership table.
- Reads: PRD-devx 4.1, 4.2, 4.4; `docs/plan/testing.md` section 5.
- Depends on: `D7.0`.
- Method:
  - `release.yml` on tag `v*`, permissions `contents: write`, `id-token: write`, `attestations: write`.
  - Step order of PRD-devx 4.2. Build the image per architecture on native runners `ubuntu-24.04` and `ubuntu-24.04-arm`. Join them in one manifest list. Record the digest of the manifest list.
  - Build `sag_darwin_arm64` on `macos-26`, `sag_linux_amd64` on `ubuntu-24.04`, `sag_linux_arm64` on `ubuntu-24.04-arm`, with cgo and the two `-X` flags.
  - Write `SHA256SUMS`. Attest each binary with `actions/attest-build-provenance`. Publish the GitHub release.
  - `image-main` builds the same manifest list with the tag `main`. Registry login reads `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`.
  - Pin each action by commit SHA. Each job of `ci.yml` keeps `contents: read`.
- Tests first: none. The workflow has no unit test.
- ACs: none.
- Verify: `podman run --rm -v "$PWD:/repo:ro" -w /repo docker.io/rhysd/actionlint:1.7.9 && make image`. Expected: exit 0 with no `actionlint` output.
- Done when: the release job runs only after the image job and the binary jobs succeed.

## D7.3 Integration and gate

- Goal: Prove DX-AC30 on the development machine and close the layer 7 gate.
- Owns: see the ownership table.
- Depends on: `D7.1`, `D7.2`.
- Steps: README section 6.1. Ask Ulrich to confirm that the GitHub secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` exist.
- ACs: DX-AC30.
- Verify: `make test && make e2e-matrix LAYER=6 PGS="14 17 19" && make e2e-devx SUITE=Main LAYER=7 && make e2e-devx SUITE=Pins LAYER=7 && make e2e-devx-unit LAYER=7`. Expected: exit 0. The devx report shows each phase A DX-AC of layers 2 to 7 `PASS`.
- Done when: the tag `sag-devx-7` exists.

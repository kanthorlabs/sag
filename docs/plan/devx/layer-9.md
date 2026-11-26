# Layer 9: AWS Secrets Manager backend

| Item | Value |
|---|---|
| Goal | A resource credential lives in AWS Secrets Manager. SAG starts and serves without AWS, and an AWS resource comes back when AWS returns. |
| ACs | DX-AC31, DX-AC32 |
| PRD-devx sections | 7.4 rule 6, 7.5, 8, 9, 11.1 to 11.4, 16 rows 5 and 10.2 |
| Gate | `D9.4`: README section 6, with `make e2e-matrix LAYER=6 PGS="14 17 19"` in place of gate step 1 |
| Agents at once | 3. Start order: `D9.1`, `D9.2`, `D9.3` |
| Wall-clock | `D9.0` 1 h + parallel phase 3.5 h + `D9.4` 3 h = 7.5 h |

## File ownership

| Task | Owns |
|---|---|
| `D9.0` | `go.mod`, `go.sum` (AWS SDK modules), `internal/config/storage.go`, `internal/secret/ref.go`, `internal/secret/awssm/*.go` stubs, `internal/scrub/scrub.go` (`Add` signature), `e2e/devx/dxh/localstack.go` (image digest constant) |
| `D9.1` | `internal/secret/awssm/*`, `internal/secret/ref_test.go`, `internal/config/storage_test.go` |
| `D9.2` | `internal/daemon/resources.go`, `internal/daemon/aws_open.go`, their tests, `internal/scrub/*`, `internal/preflight/scheduler.go`, `e2e/devx/dxac32_test.go` |
| `D9.3` | `internal/credential/backend.go`, `internal/credential/*_test.go` (AWS cases), `e2e/devx/dxh/localstack.go`, `e2e/devx/dxac31_test.go`, `.github/workflows/ci.yml` (LocalStack setup in `devx-e2e`) |
| `D9.4` | Every layer-9 file after `D9.1` to `D9.3` merge. `docs/reviews/devx-layer-9.md`. |

## D9.0 Contract

- Goal: Freeze the AWS backend API, the `storage.aws` schema, the reference parse, and the scrubber add.
- Owns: see the ownership table.
- Reads: PRD-devx 11, 16 row 5, 17.1.
- Depends on: `D8.3`.
- Content:
  - `go.mod`: add `github.com/aws/aws-sdk-go-v2/config` and `github.com/aws/aws-sdk-go-v2/service/secretsmanager`.
  - `config/storage.go`: `storage.aws.region`, `storage.aws.credentials` (`secret:` only). An `aws-sm:` reference needs both.
  - `secret/ref.go`: `ParseRef` for `secret:<name>` and `aws-sm:<name>` with `^[A-Za-z0-9/_+=.@-]{1,512}$`. `server.tls_cert`, `server.tls_key`, `storage.aws.credentials`, and the master key accept only `secret:`.
  - `scrub.Scrubber.Add(secret.Credential)`, safe for concurrent use.
  - Confirm the LocalStack image digest and its access rules (PRD-devx 19 step 7). Record them in `dxh/localstack.go`.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: `D9.1` to `D9.3` compile against the stubs.

## D9.1 AWS backend

- Goal: Implement `SecureStorage` on AWS Secrets Manager.
- Owns: see the ownership table.
- Reads: PRD-devx 11.1 to 11.3.
- Depends on: `D9.0`.
- Method:
  - `Put`: `CreateSecret` with `SecretString` and the labels as the only tags. `Load`: `GetSecretValue`. `Delete`: `DeleteSecret` with `ForceDeleteWithoutRecovery`. `Metadata`: `DescribeSecret`. `List`: `ListSecrets` with a `tag-key` filter, then a label filter in SAG.
  - Each call has a timeout of 10 s. Map errors to `ErrExists`, `ErrNotFound`, `ErrUnavailable`.
  - Daemon configuration: `config.LoadDefaultConfig` with `WithRegion` and `WithSharedCredentialsFiles(["/run/secrets/<name>"])`. Host configuration: the default chain of the operator.
- Tests first: `awssm_test.go` with a fake client: `TestPutTagsExactlyLabels`, `TestPutExists`, `TestDeleteForce`, `TestDeleteAbsentNil`, `TestListTagFilter`, `TestTimeout10s`, `TestErrorsHoldNoValue`; `ref_test.go`: `TestAWSNameRegexp`, `TestSecretOnlyFields`.
- ACs: none.
- Verify: `go test -count=1 ./internal/secret/... ./internal/config/...`. Expected: `ok`.
- Done when: no AWS key goes to `sag.yaml`.

## D9.2 Daemon availability without AWS

- Goal: Start the listener without AWS. Open each `aws-sm:` resource in the background, with the 60 s retry.
- Owns: see the ownership table.
- Reads: PRD-devx 11.4; PRD 9.1, 10.2.
- Depends on: `D9.0`. Verify after `D9.1`.
- Method:
  - The daemon reads each `secret:` reference at start. It adds each resource with an `aws-sm:` reference as unopened.
  - A background open starts at once and calls `Load`. The open retry of PRD 9.1 calls `Load` again every 60 s.
  - A request to an unopened resource returns `resource_unavailable`.
  - The scrubber adds each credential when `Load` returns it. At start it adds the value of the `storage.aws.credentials` file.
  - `sag config check` skips the owner and mode check for each `aws-sm:` reference.
- Tests first: `aws_open_test.go`: `TestListenerBeforeAWS`, `TestUnavailableUntilLoad`, `TestRetryBringsBack`, `TestScrubAddOnLoad`, `TestCredentialsFileScrubbed`; `scrub_test.go`: `TestAddConcurrent`.
- e2e: `DXAC32_AWSUnavailable`. The test drops packets to the LocalStack endpoint.
- ACs: DX-AC32.
- Verify: `go test -count=1 ./internal/daemon/... ./internal/scrub/... ./internal/preflight/...`. Expected: `ok`.
- Done when: a `secret:` resource is served while AWS is unreachable.

## D9.3 Host commands on the AWS backend

- Goal: Run `credential create --backend aws-sm`, `list`, and `delete` on the AWS backend.
- Owns: see the ownership table.
- Reads: PRD-devx 7.4, 7.5, 8, 9, 17.1.
- Depends on: `D9.0`. Verify after `D9.1`.
- Method:
  - `--backend aws-sm` needs `storage.aws` in `sag.yaml`. Secret names `sag/<name>/conn` and `sag/<name>/ca`.
  - `delete` collects owned secrets in each configured backend. An unreachable backend gives exit 1 and no change.
  - LocalStack runs as a container pinned by digest. The test sets `AWS_ENDPOINT_URL_SECRETS_MANAGER` in the container environment.
- Tests first: `backend_test.go`: `TestBackendNeedsStorageAWS`, `TestAWSSecretNames`, `TestDeleteUnreachableNoChange`, `TestListMixedBackends`.
- e2e: `DXAC31_AWSBackend`.
- ACs: DX-AC31.
- Verify: `go test -count=1 ./internal/credential/...`. Expected: `ok`.
- Done when: the tags of each created AWS secret are exactly the three labels.

## D9.4 Integration and gate

- Goal: Prove DX-AC31 and DX-AC32 in CI with no v1 regression, and close the layer 9 gate.
- Owns: see the ownership table.
- Depends on: `D9.1` to `D9.3`.
- Steps: README section 6. Run `go mod tidy` and check that only the AWS SDK modules and their requirements change. Set the devx gate layer of `ci.yml` to 9.
- ACs: DX-AC31, DX-AC32.
- Verify: local `make e2e-matrix LAYER=6 PGS="14 17 19"`, then the CI run. Expected: local exit 0. CI `ac35` `PASS`. `devx-report` shows each DX-AC of layers 2 to 9 `PASS`.
- Done when: the tag `sag-devx-9` exists.

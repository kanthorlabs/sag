# Layer 8: AWS Secrets Manager backend

| Item | Value |
|---|---|
| Phase | A |
| Goal | A resource credential lives in AWS Secrets Manager. SAG starts and serves without AWS, and an AWS resource comes back when AWS returns. |
| ACs | DX-AC31, DX-AC32 (phase A forms) |
| PRD-devx sections | 7.4 rule 6, 7.5, 8, 9, 11.1 to 11.4, 13 (secret mounts), 16 rows 5 and 10.2, 17.1 (LocalStack) |
| Gate | `D8.4`: README section 6.1 |
| Agents at once | 3. Start order: `D8.1`, `D8.2`, `D8.3` |
| Wall-clock | `D8.0` 1.5 h + parallel phase 4 h + `D8.4` 7 h = 12.5 h |

## File ownership

| Task | Owns |
|---|---|
| `D8.0` | `go.mod`, `go.sum` (AWS SDK modules), `e2e/devx/suite_main_test.go`, stubs of `internal/config/storage.go`, `internal/secret/ref.go`, `internal/secret/awssm/*.go`, `internal/scrub/scrub.go` (`Add` signature), `internal/credential/backend.go`, `e2e/devx/dxh/localstack.go` (image digest constant, `EnsureLocalStack` stub) |
| `D8.1` | `internal/secret/awssm/*`, `internal/secret/ref.go`, `internal/secret/ref_test.go`, `internal/config/storage.go`, `internal/config/storage_test.go`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/config/schema.go` |
| `D8.2` | `internal/daemon/resources.go`, `internal/daemon/aws_open.go`, `internal/daemon/run.go`, their tests, `internal/scrub/*`, `internal/preflight/scheduler.go`, `internal/preflight/scheduler_test.go`, `internal/cli/config_check.go`, `internal/cli/config_check_test.go`, `e2e/devx/dxac32_test.go` |
| `D8.3` | `internal/credential/backend.go`, `internal/credential/create.go`, `internal/credential/delete.go`, `internal/credential/list.go`, `internal/credential/*_test.go`, `internal/cli/credential.go`, `internal/cli/credential_test.go`, `internal/deploy/mounts.go`, `internal/deploy/mounts_test.go`, `e2e/devx/dxh/localstack.go`, `e2e/devx/dxh/localstack_test.go`, `e2e/devx/dxac31_test.go` |
| `D8.4` | Every layer-8 file after `D8.1` to `D8.3` merge. `docs/reviews/devx-layer-8.md`. |

## D8.0 Contract

- Goal: Freeze the AWS backend API, the `storage.aws` schema, the reference parse, and the scrubber add. Confirm LocalStack and the packet drop method on the development machine.
- Owns: see the ownership table.
- Reads: PRD-devx 11, 16 row 5, 17.1.
- Depends on: `D7.3`.
- Content:
  - `go.mod`: add `github.com/aws/aws-sdk-go-v2/config` and `github.com/aws/aws-sdk-go-v2/service/secretsmanager`.
  - Signatures only: `config.StorageAWS` (`Region`, `Credentials`), `secret.ParseRef(string) (Ref, error)`, `Ref` (`Backend`, `Name`), `awssm.New(cfg) SecureStorage`, `scrub.Scrubber.Add(secret.Credential)`, `credential.BackendFor(cfg, name) (SecureStorage, error)`.
  - `suite_main_test.go`: add the group `aws` to `TestDevxMain` with its group setup (README section 5.1). `dxh/localstack.go`: `EnsureLocalStack(t)` stub.
  - Confirm the LocalStack image digest and its access rules (PRD-devx 19 step 7). Write the digest constant to `dxh/localstack.go`.
  - Confirm a method that makes the AWS endpoint drop packets on the development machine, for example `podman pause` of the LocalStack container. The method changes no setting of `podman-machine-default`. Record it in `docs/reviews/devx-layer-8.md`.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: `D8.1` to `D8.3` compile against the stubs, and both results are recorded.

## D8.1 AWS backend, reference parse, `storage.aws`

- Goal: Implement `SecureStorage` on AWS Secrets Manager. Parse the references. Validate `storage.aws`.
- Owns: see the ownership table.
- Reads: PRD-devx 11.1 to 11.3, 16 row 13.4.
- Depends on: `D8.0`.
- Method:
  - `ParseRef` accepts `secret:<name>` and `aws-sm:<name>` with `^[A-Za-z0-9/_+=.@-]{1,512}$` for the AWS name.
  - `server.tls_cert`, `server.tls_key`, `storage.aws.credentials`, and the master key accept only `secret:`.
  - `storage.aws.region` and `storage.aws.credentials`. An `aws-sm:` reference needs both.
  - `Put`: `CreateSecret` with `SecretString` and the labels as the only tags. `Load`: `GetSecretValue`. `Delete`: `DeleteSecret` with `ForceDeleteWithoutRecovery`. `Metadata`: `DescribeSecret`. `List`: `ListSecrets` with a `tag-key` filter, then a label filter in SAG.
  - Each call has a timeout of 10 s. Map errors to `ErrExists`, `ErrNotFound`, `ErrUnavailable`.
  - Daemon configuration: `config.LoadDefaultConfig` with `WithRegion` and `WithSharedCredentialsFiles(["/run/secrets/<name>"])`. Host configuration: the default chain of the operator.
- Tests first: `ref_test.go`: `TestAWSNameRegexp`, `TestSecretOnlyFields`; `storage_test.go`: `TestAWSRefNeedsStorage`, `TestCredentialsSecretOnly`; `awssm_test.go` with a fake client: `TestPutTagsExactlyLabels`, `TestPutExists`, `TestDeleteForce`, `TestDeleteAbsentNil`, `TestListTagFilter`, `TestTimeout10s`, `TestErrorsHoldNoValue`.
- ACs: none.
- Verify: `go test -count=1 ./internal/secret/... ./internal/config/...`. Expected: `ok`.
- Done when: no AWS key goes to `sag.yaml`.

## D8.2 Daemon availability without AWS

- Goal: Start the listener without AWS. Open each `aws-sm:` resource in the background, with the 60 s retry.
- Owns: see the ownership table.
- Reads: PRD-devx 11.4; PRD 9.1, 10.2.
- Depends on: `D8.0`. Verify after `D8.1`, `D8.3`, because DX-AC32 calls `EnsureLocalStack`.
- Method:
  - The daemon reads each `secret:` reference at start. It adds each resource with an `aws-sm:` reference as unopened.
  - A background open starts at once and calls `Load`. The open retry of PRD 9.1 calls `Load` again every 60 s.
  - A request to an unopened resource returns `resource_unavailable`.
  - The scrubber adds each credential when `Load` returns it. At start it adds the value of the `storage.aws.credentials` file.
  - `sag config check` skips the owner and mode check for each `aws-sm:` reference.
  - DX-AC32 uses the packet drop method of `D8.0`. `DXAC32_AWSUnavailable` calls `EnsureLocalStack`.
- Tests first: `aws_open_test.go`: `TestListenerBeforeAWS`, `TestUnavailableUntilLoad`, `TestRetryBringsBack`, `TestScrubAddOnLoad`, `TestCredentialsFileScrubbed`; `scrub_test.go`: `TestAddConcurrent`; `config_check_test.go`: `TestAWSRefSkipsFileCheck`.
- e2e: `DXAC32_AWSUnavailable`.
- ACs: DX-AC32.
- Verify: `go test -count=1 ./internal/daemon/... ./internal/scrub/... ./internal/preflight/... ./internal/cli/... && make e2e-devx SUITE=Main RUN=DXAC32_ LAYER=8`. Expected: `ok`, then exit 0.
- Done when: a `secret:` resource is served while AWS is unreachable.

## D8.3 Host commands and mounts on the AWS backend

- Goal: Run `credential create --backend aws-sm`, `list`, and `delete` on the AWS backend. Mount the `storage.aws.credentials` secret.
- Owns: see the ownership table.
- Reads: PRD-devx 7.4, 7.5, 8, 9, 13, 17.1.
- Depends on: `D8.0`. Verify after `D8.1`.
- Method:
  - `--backend aws-sm` needs `storage.aws` in `sag.yaml`. Secret names `sag/<name>/conn` and `sag/<name>/ca`.
  - `delete` collects owned secrets in each configured backend. An unreachable backend gives exit 1 and no change.
  - `SecretMounts` adds the `storage.aws.credentials` reference. An `aws-sm:` reference gets no mount.
  - LocalStack runs as a container pinned by digest, with the label `sag.devx-e2e=1`. The test sets `AWS_ENDPOINT_URL_SECRETS_MANAGER` in the container environment.
  - `EnsureLocalStack` calls `EnsureBootstrapped`. It starts LocalStack and adds `storage.aws` to `sag.yaml` when they are absent. `DXAC31_AWSBackend` calls it.
- Tests first: `backend_test.go`: `TestBackendNeedsStorageAWS`, `TestAWSSecretNames`, `TestDeleteUnreachableNoChange`, `TestListMixedBackends`; `localstack_test.go` with a fake runner: `TestEnsureLocalStackSkipsWhenPresent`; `mounts_test.go`: `TestSecretMountsSkipAWS`, `TestSecretMountsAWSCredentials`.
- e2e: `DXAC31_AWSBackend`.
- ACs: DX-AC31.
- Verify: `go test -count=1 ./internal/credential/... ./internal/deploy/... ./internal/cli/... ./e2e/devx/dxh/... && make e2e-devx SUITE=Main RUN=DXAC31_ LAYER=8`. Expected: `ok`, then exit 0.
- Done when: the tags of each created AWS secret are exactly the three labels.

## D8.4 Integration and gate

- Goal: Prove DX-AC31 and DX-AC32 with no v1 regression, and close the layer 8 gate.
- Owns: see the ownership table.
- Depends on: `D8.1` to `D8.3`.
- Steps: README section 6.1. Run `go mod tidy` and check that only the AWS SDK modules and their requirements change.
- ACs: DX-AC31, DX-AC32.
- Verify: `make test && make e2e-matrix LAYER=6 PGS="14 17 19" && make e2e-devx SUITE=Main LAYER=8 && make e2e-devx SUITE=Pins LAYER=8 && make e2e-devx-unit LAYER=8`. Expected: exit 0. The devx report shows each phase A DX-AC of layers 2 to 8 `PASS`.
- Done when: the tag `sag-devx-8` exists.

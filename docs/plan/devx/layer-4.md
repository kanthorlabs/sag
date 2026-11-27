# Layer 4: Credential create, list, inspect

| Item | Value |
|---|---|
| Phase | A |
| Goal | An operator registers a PostgreSQL resource with `sag credential create` and reads its metadata. No credential value leaves the backend and the container. |
| ACs | DX-AC9 to DX-AC17, DX-AC29, DX-AC37 (phase A forms) |
| PRD-devx sections | 2.3, 5.7 (renewal check through `list`), 7, 8, 11.1, 17.1 (argument check) |
| Gate | `D4.4`: README section 6.1 |
| Agents at once | 3. Start order: `D4.2`, `D4.3`, `D4.1` |
| Wall-clock | `D4.0` 1 h + parallel phase 5.5 h + `D4.4` 5.5 h = 12 h |

## File ownership

| Task | Owns |
|---|---|
| `D4.0` | `go.mod`, `go.sum` (`golang.org/x/term`), `internal/core/credential.go`, `internal/prompt/hidden.go` stub, stubs of `internal/credential/create.go`, `list.go`, `e2e/devx/suite_main_test.go`, stub of `e2e/devx/dxh/ensure_canary.go` |
| `D4.1` | `internal/adapter/postgres/credential.go`, `internal/adapter/postgres/credential_test.go`, `internal/adapter/postgres/credential_integration_test.go`, `internal/cli/credential_check.go`, `internal/cli/credential_check_test.go`, `e2e/devx/dxac12_test.go`, `e2e/devx/dxac13_test.go` |
| `D4.2` | `internal/credential/create.go`, `internal/credential/input.go`, `internal/credential/yaml.go`, their tests, `internal/prompt/hidden.go`, `internal/prompt/hidden_test.go`, `internal/cli/credential.go`, `internal/cli/credential_test.go`, `e2e/devx/dxac09_test.go`, `dxac11_test.go`, `dxac14_test.go`, `dxac15_test.go`, `dxac16_test.go`, `dxac37_test.go`, `e2e/devx/dxh/ensure_canary.go`, `e2e/devx/dxh/ensure_canary_test.go` |
| `D4.3` | `internal/credential/list.go`, `internal/credential/list_test.go`, `e2e/devx/dxac10_test.go`, `dxac17_test.go`, `dxac29_test.go`, `e2e/devx/dxh/leak.go`, `e2e/devx/dxh/leak_test.go` |
| `D4.4` | Every layer-4 file after `D4.1` to `D4.3` merge. `docs/reviews/devx-layer-4.md`. |

## D4.0 Contract

- Goal: Freeze the kind interface, the hidden read, and the credential command API. Confirm the DSN host of DX-AC9 on the development machine.
- Owns: see the ownership table.
- Reads: PRD-devx 7, 8; PRD 16 row 5.
- Depends on: `D3.4`.
- Content:
  - `core/credential.go`: `CredentialKind` and `CredentialNotes` exactly as PRD-devx 7.2. `CreatableKinds()` from the registry.
  - `go.mod`: add `golang.org/x/term` as a direct module.
  - `prompt.ReadHidden(tty, label) (secret.Credential, error)`.
  - `credential.Create(ctx, opts) error`, `credential.List(ctx, opts) ([]Entry, error)`, `credential.Inspect(ctx, name, opts) (Entry, error)`. `Entry` holds exactly the fields of PRD-devx 8.
  - `suite_main_test.go`: add the groups `credential` and `cert` to `TestDevxMain`, each with its group setup (README section 5.1). `dxh/ensure_canary.go`: `EnsureCanary(t)` stub.
  - Confirm that a container on `podman-machine-default` reaches a PostgreSQL container through `host.containers.internal` and its published port. Record the result in `docs/reviews/devx-layer-4.md`.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: `D4.1` to `D4.3` compile against the stubs, and the DSN host result is recorded.

## D4.1 Kind `postgres` and connection check

- Goal: Implement `CredentialKind` for `postgres` and the container command `sag credential check`.
- Owns: see the ownership table.
- Reads: PRD-devx 7.2, 7.4; PRD 9.1; `internal/adapter/postgres/connconfig.go`.
- Depends on: `D4.0`.
- Method:
  - `ValidateCredential` calls `parseDSN`. It sets `LoopbackHost` for `localhost`, `127.0.0.0/8`, and `::1`.
  - `CheckCredential` opens one connection with `tls` and `tls_ca`, then runs the `Engine` and `Version` checks.
  - `sag credential check --kind <kind> --tls <mode>` reads `{"value": ..., "tls_ca": ...}` from standard input. It prints one fixed error on failure.
- Tests first: `credential_test.go`: `TestValidateMatchesParseDSN`, `TestLoopbackHosts`, `TestErrorsFixedText`; `credential_check_test.go`: `TestReadsStdinJSON`, `TestFixedErrorExit1`.
- Integration: `TestCheckCredentialAgainstPG` with `internal/testenv`.
- e2e: `DXAC12_ConnectionCheck`, `DXAC13_LoopbackWarn`.
- ACs: DX-AC12, DX-AC13.
- Verify: `go test -count=1 ./internal/adapter/postgres/... ./internal/cli/... -run 'Credential|ReadsStdin|FixedError|ErrorsFixed|Validate|Loopback' && make test-integration PG=17 PKG=./internal/adapter/postgres/... RUN=TestCheckCredentialAgainstPG && make e2e-devx SUITE=Main RUN='DXAC1[23]_' LAYER=4`. Expected: `ok`, then exit 0.
- Done when: no error text holds a part of the value.

## D4.2 `credential create`

- Goal: Validate and store one credential, add its resource, and restart SAG, with rollback on failure.
- Owns: see the ownership table.
- Reads: PRD-devx 7, 11.1, 17.1.
- Depends on: `D4.0`. Verify after `D4.1`.
- Method:
  - Value input of PRD-devx 7.3: hidden read on `/dev/tty`, or `--stdin` with the 64 KiB, NUL, and line break limits.
  - The validation rules in the PRD-devx 7.4 order. No secret exists after a failure.
  - Effects of PRD-devx 7.5 under the lock. Edit `sag.yaml` with a temporary file and a rename. Validate with the `config check` rules.
  - Rollback restores `sag.yaml`, deletes only the secrets of this call, runs `sag service start`, and exits 1.
  - Test hooks: fail step 3 after step 1 (DX-AC16), fail step 3 after step 2 (DX-AC37).
  - `EnsureCanary` calls `EnsureBootstrapped`. It starts the PostgreSQL container and creates the canary resource as DX-AC9 when the resource is absent.
- Tests first: `create_test.go` with fake backend and runner: `TestValidationOrder`, `TestNameRegexp`, `TestResourceExists`, `TestSecretExists`, `TestStdinLimits`, `TestStdinTerminalRefused`, `TestPipedWithoutStdinExit3`, `TestRollbackDeletesOwnSecretsOnly`, `TestKeepsOtherYAML`, `TestArgsHoldNoValue`; `dxh/ensure_canary_test.go` with a fake runner: `TestEnsureCanarySkipsWhenPresent`.
- e2e: `DXAC09_CreateCanary`, `DXAC11_CreateRejectsForm`, `DXAC14_CreateStdin`, `DXAC15_CreateSecretExists`, `DXAC16_CreateRollback`, `DXAC37_CreateTLSCA`.
- ACs: DX-AC9, DX-AC11, DX-AC14, DX-AC15, DX-AC16, DX-AC37.
- Verify: `go test -count=1 ./internal/credential/... ./internal/prompt/... ./internal/cli/... ./e2e/devx/dxh/... && make e2e-devx SUITE=Main RUN='DXAC09_|DXAC1[1456]_|DXAC37_' LAYER=4`. Expected: `ok`, then exit 0.
- Done when: the value goes only to standard input of `podman secret create` and `podman exec -i`.

## D4.3 `credential list`, `inspect`, leak search

- Goal: Show resource and credential metadata. Prove that no canary part and no CA key leaks. Prove the certificate renewal through `list` and `bootstrap`.
- Owns: see the ownership table.
- Reads: PRD-devx 2.3, 5.7, 8, 17.
- Depends on: `D4.0`. Verify after `D4.2`.
- Method:
  - Fields of PRD-devx 8 only. `exists` and `created_at` come from `Metadata`, `mounted` from `podman container inspect sag`.
  - Never call `Load`. No non-test file holds `--showsecret`.
  - Less than 30 days on `<cfg>/ca.pem` gives the expiry `warn` line on standard error.
  - An unreachable backend gives a `warn` line and exit 0.
  - `dxh/leak.go` searches the sources of DX-AC10 for given strings. In phase A, the service log is `~/Library/Logs/sag/service.log`, and the start at login file is the LaunchAgent.
  - The private key scan of DX-AC10 walks `$HOME` and the temporary directory. It does not follow a symbolic link. It reads only PEM blocks and prints no file content.
  - The scan reads only regular files with a change time at or after the start of the group and a size of at most 1 MiB. It does not leave the file system of `$HOME`.
  - The scan skips a directory that gives a permission error. One output line gives the count of skipped directories.
  - The scan has a time limit of 5 min. It fails on the limit.
  - `DXAC10_CanaryNoLeak` and `DXAC17_ListInspect` call `EnsureCanary`.
- Tests first: `list_test.go`: `TestJSONFieldSetExact`, `TestNoLoadCall`, `TestUnreachableBackendWarns`, `TestExpiryWarn`, `TestInspectUnknownExit1`; `leak_test.go`: `TestFindsGivenString`, `TestKeyScanPrintsNoContent`, `TestKeyScanSkipsOldFiles`, `TestKeyScanCountsDenied`.
- e2e: `DXAC10_CanaryNoLeak`, `DXAC17_ListInspect`, `DXAC29_CertRenewal`.
- ACs: DX-AC10, DX-AC17, DX-AC29.
- Verify: `go test -count=1 ./internal/credential/... ./e2e/devx/dxh/... && make e2e-devx SUITE=Main RUN='DXAC1[07]_|DXAC29_' LAYER=4`. Expected: `ok`, then exit 0.
- Done when: the JSON object of list and inspect has exactly the field set of PRD-devx 8.

## D4.4 Integration and gate

- Goal: Prove the layer 4 criteria on the development machine and close the layer 4 gate.
- Owns: see the ownership table.
- Depends on: `D4.1` to `D4.3`.
- Steps: README section 6.1. Run `go mod tidy` and check that only `golang.org/x/term` and its requirements change.
- ACs: DX-AC9 to DX-AC17, DX-AC29, DX-AC37.
- Verify: `make test && make e2e-matrix LAYER=6 PGS="14 17 19" && make e2e-devx SUITE=Main LAYER=4 && make e2e-devx SUITE=Pins LAYER=4`. Expected: exit 0. The devx report shows each phase A DX-AC of layers 2 to 4 `PASS`.
- Done when: the tag `sag-devx-4` exists.

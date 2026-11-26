# Layer 5: Credential delete, host forms, logs

| Item | Value |
|---|---|
| Goal | An operator removes a resource only with a typed confirmation on a terminal. The host forms and the log commands work. |
| ACs | DX-AC18 to DX-AC25 |
| PRD-devx sections | 1.2, 9, 10, 16 rows 9.1, 9.2, 11.2 |
| Gate | `D5.4`: README section 6 |
| Agents at once | 3. Start order: `D5.1`, `D5.2`, `D5.3` |
| Wall-clock | `D5.0` 1 h + parallel phase 4 h + `D5.4` 3 h = 8 h |

## File ownership

| Task | Owns |
|---|---|
| `D5.0` | `internal/podman/exec.go` (`ExecTTY`, `Exec`), stubs of `internal/credential/delete.go`, `internal/cli/host_forms.go`, `internal/cli/logs.go`, `internal/cli/audit_read.go`, `internal/testhook` name for DX-AC22 |
| `D5.1` | `internal/credential/delete.go`, `internal/credential/delete_test.go`, `internal/credential/oneshot.go`, `e2e/devx/dxac18_test.go` to `dxac23_test.go` |
| `D5.2` | `internal/cli/host_forms.go`, `internal/cli/jwt_generate.go`, `internal/cli/extension.go`, `internal/cli/resource_test_cmd_test.go`, `internal/adapter/postgres/preflight.go`, `preflight_test.go`, `preflight_integration_test.go`, the tests of the named `internal/cli` files, `e2e/devx/dxac24_test.go` |
| `D5.3` | `internal/cli/logs.go`, `internal/cli/audit_read.go`, their tests, `e2e/devx/dxac25_test.go` |
| `D5.4` | Every layer-5 file after `D5.1` to `D5.3` merge. `docs/reviews/devx-layer-5.md`. |

## D5.0 Contract

- Goal: Freeze the terminal pass-through of `podman exec`, the delete API, and the log command API.
- Owns: see the ownership table.
- Reads: PRD-devx 1.2, 9, 10.
- Depends on: `D4.4`.
- Content:
  - `podman.ExecTTY(ctx, args)` gives the terminal of the process to `podman exec -it`. `podman.Exec(ctx, stdin, args)` runs without `-t`.
  - `credential.Delete(ctx, name, tty) error`.
  - Stub commands `logs daemon`, `logs audit`, `audit read`, and the four host forms.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: `D5.1` to `D5.3` compile against the stubs.

## D5.1 `credential delete`

- Goal: Remove one resource and its owned secrets after the typed name on `/dev/tty`.
- Owns: see the ownership table.
- Reads: PRD-devx 2.3, 9.
- Depends on: `D5.0`.
- Method:
  - No `--yes`, no `--stdin`, no `--json`. Each gives exit 2.
  - No `/dev/tty` gives exit 3 and no change.
  - Ownership by the label `sag.resource=<name>`, also for secrets that no entry references.
  - The nine steps of PRD-devx 9.3 in order. Revoke with `podman exec` when the container runs. Else run a one-shot container with the same image, state volume, and `sag-master-key`.
  - Idempotent: a second call completes the remaining steps. Test hook: fail step 8 (DX-AC22).
- Tests first: `delete_test.go`: `TestFlagsExit2`, `TestNoTTYExit3NoChange`, `TestWrongNameExit3`, `TestUnreachableBackendExit1`, `TestOwnsUnreferencedLabeled`, `TestSkipForeignSecret`, `TestStoppedContainerOneShotRevoke`, `TestSecondDeleteCompletes`.
- e2e: `DXAC18_DeleteNoTerminal`, `DXAC19_DeleteConfirm`, `DXAC20_Delete`, `DXAC21_DeleteStopped`, `DXAC22_DeleteResume`, `DXAC23_DeleteForeignSecret`.
- ACs: DX-AC18 to DX-AC23.
- Verify: `go test -count=1 ./internal/credential/...`. Expected: `ok`.
- Done when: `delete` never removes a secret without the `sag.resource` label.

## D5.2 Host forms and PRD text changes

- Goal: Run `jwt generate`, `extension approve`, `extension revoke`, and `resource test` from the host. Apply the PRD-devx 16 text and behavior rows.
- Owns: see the ownership table.
- Reads: PRD-devx 1.2, 16 rows 9.1, 9.2, 11.2.
- Depends on: `D5.0`.
- Method:
  - Host `jwt generate` and `extension approve` exit 3 unless standard input and standard output are terminals. No `--json`: exit 2.
  - Container `jwt generate` checks standard input and standard output.
  - Hint text of PRD 9.1: `to approve, run: sag extension approve <name>`. Update the error texts that hold `podman exec -it`.
  - `extension revoke` exits 0 without an approval record, after it writes the tombstone.
- Tests first: `host_forms_test.go`: `TestHostFormArgs`, `TestRedirectExit3`, `TestJSONExit2`; `jwt_generate_test.go`: `TestStdinNotTerminalRefused`; `preflight_test.go`: `TestHintText`; `extension_test.go`: `TestRevokeNoRecordExit0`.
- e2e: `DXAC24_HostJWTGenerate`.
- ACs: DX-AC24.
- Verify: `go test -count=1 ./internal/cli/... ./internal/adapter/postgres/...`. Expected: `ok`.
- Done when: no product source holds the text `podman exec -it` in a user message.

## D5.3 `sag logs` and `sag audit read`

- Goal: Show the daemon log and the audit log from the host without a terminal.
- Owns: see the ownership table.
- Reads: PRD-devx 10; PRD 10.5.
- Depends on: `D5.0`.
- Method: `sag logs daemon` runs `podman logs sag` with `--follow`, `--since`, and `--tail`. `sag logs audit` runs `podman exec sag sag audit read --date <date>`. `audit read` prints the file lines without change. The default date is the current UTC date.
- Tests first: `logs_test.go`: `TestDaemonFlagsPassed`, `TestAuditDefaultUTCDate`; `audit_read_test.go`: `TestPrintsLinesUnchanged`, `TestDateFormat`.
- e2e: `DXAC25_Logs`.
- ACs: DX-AC25.
- Verify: `go test -count=1 ./internal/cli/... -run 'Logs|Audit|Daemon|PrintsLines|DateFormat'`. Expected: `ok`.
- Done when: both commands exit 0 in a process without a controlling terminal.

## D5.4 Integration and gate

- Goal: Prove the layer 5 criteria in CI and close the layer 5 gate.
- Owns: see the ownership table.
- Depends on: `D5.1` to `D5.3`.
- Steps: README section 6. Set the devx gate layer of `ci.yml` to 5.
- ACs: DX-AC18 to DX-AC25.
- Verify: CI run. Expected: `ac35` `PASS`. `devx-report` shows each DX-AC of layers 2 to 5 `PASS`.
- Done when: the tag `sag-devx-5` exists.

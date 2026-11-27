# Layer 6 gate: devx

Phase A, macOS development machine. Gate commit `f2a12b0`. Tag commit `a2c53c1`.

## Tasks

| Task | Merge |
|---|---|
| `D6.0` contract | `06a873a` |
| `D6.2` skill, pi guard, harness stubs, TAP report | `e6b35a0` |
| `D6.1` | `f8ff208` |
| `D6.3` | `d89aeb5` |
| `D6.5` | `775d4dc` |
| `D6.4` | `d471096` |
| `D6.6a` integration fixes | `686b667` |
| `D6.6b` volume leak fix | `d75fd49` |
| `D6.6e` DX test review fixes | `b6155a8` |
| `D6.6c` agent setup review fixes | `8ed30e7` |
| `D6.6d` bootstrap review fixes | `52e1504` |
| `D6.6f` layer 6 e2e fixes | `6956b6e` |
| `D6.6g` deferred answer save, DXAC40 audit source | `f2a12b0` |
| `D6.6h` DXAC40 check that cannot run | `a2c53c1` |

## Findings fixed at integration

- B78 - status:FIXED - action:YES - DXAC42 failed after the bundle step - fix:use the suite `HOME` and `<cfg>`; assert the full PRD-devx 12.5 line - why:criterion text
- B79 - status:FIXED - action:YES - DXAC45 step 9 got exit 3 - fix:pass `--deny-rules` - why:no terminal in the suite
- B80 - status:FIXED - action:YES - vacuous passes in DXAC41 and DXAC42 - fix:fail when the bundle command or the stub record is absent - why:a check that cannot fail does not count
- S47 - status:FIXED - action:YES - skipped DX subtests could pass the parent - fix:`t.Fatal` in place of `t.Skip` - why:acreport counts a skip as a failure
- B81 - status:FIXED - action:YES - labelled PostgreSQL test containers leaked anonymous volumes - fix:`podman rm --volumes` in teardown and canary cleanup - why:the leak filled the Podman machine
- S48 - status:FIXED - action:YES - gate Main exits 2 before Pins runs on the same commit - fix:the gate script prints each fail and skip event and the final report lines - why:exit 2 alone hides the cause

## Independent review 1 (Fable), verdict REWORK

The review covered `sag-devx-5..d75fd49`. Tasks `D6.6c`, `D6.6d` and `D6.6e` fixed the items. Aelita verified each item and checked the key tests against mutants.

- B82 to B104 - status:FIXED - action:YES - answers and prompts of PRD-devx 12.3, verify of a failed `sag`, DX-AC43 conflict causes, remote mode without Podman, remote JWT after the lock, bootstrap step 3 plan, `NODE_EXTRA_CA_CERTS` sources, two `mcpServers.sag` conflicts, `--shell-profile no` with an unsupported shell, flags of the other harness, MCP file mode 0600, JSON write rule, bundle `<source>`, step 15 on a CA trust conflict, DX-AC39/40/41/45/26/44/49 checks that cannot fail or are weak, Tailscale steps when Tailscale is down, container refusal - fix:see `.dev/local/devx-layer-6-review.md` - why:PRD-devx 12 and 17
- B105 - status:FIXED - action:YES - CA trust line printed `fail fail ca-trust` - fix:`CATrustConflictError.Error()` has no `fail` prefix - why:PRD-devx 12.5 line
- S49 to S65 - status:FIXED - action:YES - bundle override, dead code, JWT at each local run, `--url` parts, missing step lines, verify `WaitDelay`, unknown `--agent-setup` harness, conflict by error text, unit tests that cannot fail, missing plan conflict tests, DX-AC41 `HOME`, DX-AC45 cleanup, DX-AC51 file compare, `EnsureAgentSetup` state, `ReadJWT` echo, relative harness directories, conflict printed twice
- S66 - status:OPEN - action:NO - `DXAC39_DefaultPath` skips in the User suite - fix:none in layer 6 - why:layer 9 replaces the skip

## DX runs after the review fixes

- B106 - status:FIXED - action:YES - pi did not ask the shell profile answer without a known shell - fix:the answer is needed; the prompt names the shell (decision D6-e) - why:DX-AC43
- B107 - status:FIXED - action:YES - second pi run printed `skip guard` - fix:store each flag answer; reuse the stored guard answer (decision D6-f) - why:DX-AC41, PRD-devx 12.3
- B108 - status:FIXED - action:YES - remote mode without a terminal exited 1 - fix:`ErrNoTerminal` gives exit 3; the terminal check runs before the lock - why:DX-AC51
- B109 - status:FIXED - action:YES - DXAC40 read only stdout of `podman logs sag` - fix:combined output - why:DX-AC40
- B110 - status:FIXED - action:YES - DXAC41 bash case compared the `<cfg>` of the suite `HOME` - fix:the `<cfg>` of the test `HOME` - why:DX-AC41
- B111 - status:FIXED - action:YES - `answers.json` written on an exit in the plan - fix:write the answers after the plan passes - why:DX-AC43, DX-AC51
- B112 - status:FIXED - action:YES - DXAC40 ran the host binary path in the container - fix:copy the audit directory with `podman cp` - why:DX-AC40

## Independent review 2, verdict one blocker

The review covered `d75fd49..f2a12b0`.

- B113 - status:FIXED - action:YES - DXAC40 positive control could not run - fix:remove the dead skip and loop; the leak search covers each source - why:a check that cannot fail does not count
- S67 - status:FIXED - action:NO - DXAC40 positive control compared each JWT with each MCP file - fix:none - why:B113 removes the loop

## Final gate on `f2a12b0`

```
gofmt -l internal e2e cmd                    → empty
go vet ./... and -tags devxe2e               → exit 0
go test -tags devxtest ./internal/...        → exit 0
make contract-check                          → exit 0
make test                                    → exit 0
make e2e-devx-unit LAYER=6                   → exit 0
make e2e-devx SUITE=Main LAYER=6             → two runs, 133 pass events each, no fail, no skip
make e2e-devx SUITE=Pins LAYER=6             → exit 0, report over Main and Pins at f2a12b0
make e2e-matrix LAYER=6 PGS="14 17 19"       → exit 0
```

The two Main runs exit 2 only because the report of the earlier Pins run named `686b667`. The Pins run then wrote the report over both suites at `f2a12b0`.

```
PASS DX_Harness_Skeleton  PASS DXAC01  PASS DXAC02  PASS DXAC03  PASS DXAC04
PASS DXAC06  PASS DXAC07  PASS DXAC08  PASS DXAC09  PASS DXAC10
PASS DXAC11  PASS DXAC12  PASS DXAC13  PASS DXAC14  PASS DXAC15
PASS DXAC16  PASS DXAC17  PASS DXAC18  PASS DXAC19  PASS DXAC20
PASS DXAC21  PASS DXAC22  PASS DXAC23  PASS DXAC24  PASS DXAC25
PASS DXAC26  PASS DXAC27  PASS DXAC28  PASS DXAC29  PASS DXAC33
PASS DXAC36  PASS DXAC37  PASS DXAC39  PASS DXAC40  PASS DXAC41
PASS DXAC42  PASS DXAC43  PASS DXAC44  PASS DXAC45  PASS DXAC46
PASS DXAC49  PASS DXAC50  PASS DXAC51  PASS DXAC53
```

`D6.6h` changes only `e2e/devx/dxac40_test.go`. On `a2c53c1`, the layer 6 set `DXAC(26|39|40|41|42|43|45|46|51)_` passed two times, with no fail and no skip.

After the runs, `~/.config/sag`, the LaunchAgent and each container with label `sag.devx-e2e=1` are absent. No DX run added a Podman volume.

## Disk

```
after: /var 13865020 KiB free; 14 local volumes before and after
```

## CI after the tag

- B114 - status:FIXED - action:YES - `TestB86KeptRemoteEntryPlanPasses` needed `claude` in the `PATH` of the machine - CI of pull request 1 at `b1489a4` failed in `unit` and each `e2e` job; `ac35` then found no report - fix:the test puts a stub `claude` in its own `PATH`; all unit tests pass with a `PATH` without `claude` and `pi` - why:a unit test must not depend on the machine

## Open items

- S45 - status:OPEN - action:YES - preflight rechecks of all resources start at the same moment - fix:spread the first timer - why:load peak at start

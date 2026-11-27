# Layer 5 gate: devx

Phase A, macOS development machine. Gate commit `022e356`.

## Tasks

| Task | Merge |
|---|---|
| `D5.0` contract | `30ca441` |
| `D5.3` `logs daemon`, `logs audit`, `audit read` | `d9b1e2a` |
| `D5.2` host forms and PRD-devx 16 text rows | `6cab782` |
| `D5.1` `credential delete` | `ed12099` |
| `D5.5b` terminal check, host form and `logs` review fixes | `d632516` |
| `D5.5a` `delete` review fixes | `0711c6b` |
| `D5.5c` DX criterion review fixes | `022e356` |
| CI test fixes from `feat/sag-v1` | `67d8c62` |

## Findings fixed at integration

- B46 - status:FIXED - action:YES - host `extension approve` without terminals gave exit 1 - fix:exit 3 - why:PRD-devx 1.2
- B47 - status:FIXED - action:YES - DXAC24 failure messages could print a JWT - fix:check for a token first; print only the byte count of the redirect file - why:secret rule
- B55 - status:FIXED - action:YES - DXAC25 edited `sag.yaml` by text search and failed after the `delete` group - fix:create the resource with `credential create --stdin` - why:a criterion must not depend on the format of an earlier group
- B56 - status:FIXED - action:YES - v1 dblink hint test kept the old hint - fix:expect the PRD-devx 16 row 9.1 hint - why:`D5.2` changed the hint
- B57 - status:FIXED - action:YES - v1 AC04 and AC34 expected `not a terminal` - fix:expect `must be terminals` - why:`D5.2` changed the message

## Independent review (Fable), verdict REWORK

The review covered `425c824..ed12099`. Tasks `D5.5a`, `D5.5b` and `D5.5c` fixed the items. Aelita verified each item.

- B48 - status:FIXED - action:YES - `/dev/null` passed the terminal check - fix:`term.IsTerminal` (decision D5-c); unit and DXAC24 cases for `</dev/null` and `>/dev/null` - why:PRD-devx 1.2
- B49 - status:FIXED - action:YES - DXAC20 lacked the mount clause - fix:read the secrets of `podman container inspect sag` before and after - why:criterion text
- B50 - status:FIXED - action:YES - DXAC25 accepted any `request_id` - fix:take the `request_id` of a SQL tool call; exactly one audit line holds it - why:criterion text
- B51 - status:FIXED - action:YES - `delete --yes=false` passed - fix:reject each named flag when present - why:PRD-devx 9.1
- B52 - status:FIXED - action:YES - a second `delete` did not complete steps 7 and 9 - fix:resume record `<cfg>/delete-pending/<name>.json` (decision D5-b) - why:PRD-devx 9.3
- B53 - status:FIXED - action:YES - `logs audit` gave exit 1 for an invalid date - fix:check the date on the host - why:PRD-devx 1.3
- B54 - status:FIXED - action:YES - no argument test for `delete` - fix:full argument lists of each child process, running and stopped; no canary and no JWT - why:PRD-devx 17.1
- S30 to S40, S42 - status:FIXED - action:YES - one-shot argument list, no-change and step order proofs, exact `skip` line, DXAC21 tombstone and stopped container, DXAC19 argument order, no output in `Expect` failures, DXAC24 status 200, `--json` on `extension approve`, full `logs audit` arguments, `credentialDelete` exit codes, exported state volume name, labelled secret of another entry not owned
- S41 - status:OPEN - action:NO - `Runner.Exec` and `tolerateNoApproval` have no product effect - fix:none - why:outside the layer 5 scope
- S43 - status:OPEN - action:NO - a resumed `delete` prints empty `kind` and `agents` - fix:none - why:the PRD fixes no step 3 text for a resume
- S44 - status:OPEN - action:NO - DXAC25 DSN values come from the clock - fix:none - why:the target is unreachable and the leak search does not use them

## Final gate on `022e356`

```
gofmt -l internal e2e cmd                    → empty
make contract-check                          → exit 0
make test                                    → exit 0
make e2e-devx SUITE=Main LAYER=5             → two runs, no fail, no skip
make e2e-devx SUITE=Pins LAYER=5             → exit 0, report over Main and Pins at 022e356
make e2e-matrix LAYER=6 PGS="14 17 19"       → exit 0
```

The two Main runs exit 2 only because the report of the earlier Pins run named `ed12099`. The Pins run then wrote the report over both suites at `022e356`.

```
PASS DX_Harness_Skeleton  PASS DXAC01  PASS DXAC02  PASS DXAC03  PASS DXAC04
PASS DXAC06  PASS DXAC07  PASS DXAC08  PASS DXAC09  PASS DXAC10
PASS DXAC11  PASS DXAC12  PASS DXAC13  PASS DXAC14  PASS DXAC15
PASS DXAC16  PASS DXAC17  PASS DXAC18  PASS DXAC19  PASS DXAC20
PASS DXAC21  PASS DXAC22  PASS DXAC23  PASS DXAC24  PASS DXAC25
PASS DXAC27  PASS DXAC28  PASS DXAC29  PASS DXAC33  PASS DXAC36
PASS DXAC37  PASS DXAC50  PASS DXAC53
```

After the runs, `~/.config/sag`, the LaunchAgent and each container with label `sag.devx-e2e=1` are absent.

## Disk

```
after: /var 12027144 KiB free
```

## Open items

- B58 - status:OPEN - action:YES - v1 e2e stalls on the Linux CI runner - In some jobs, sag sends no traffic to PostgreSQL for about 70 s after start; requests time out and preflights time out. The Mac gate passes. - fix:root cause under analysis - why:pull request 1 CI is red

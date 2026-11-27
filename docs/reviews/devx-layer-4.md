# Layer 4 gate: devx

Phase A, macOS development machine. Gate commit `01d4e8a`.

## Tasks

| Task | Merge |
|---|---|
| `D4.0` contract | `ec6f755` |
| `D4.1` kind `postgres` and `credential check` | `47f8bd8` |
| `D4.3` `list`, `inspect`, leak search | `5f4dc6a` |
| `D4.2` `credential create` | `9d652a9`, `7843c00` |
| `D4.5a` product review fixes | `d9e421e` |
| `D4.5b` create-side DX test fixes | `13c716f` |
| `D4.5c` list-side DX test fixes | `aec81ae` |

## DSN host on the development machine

Image used: `docker.io/library/postgres:17.12-bookworm` (present locally, no pull).

Commands and exit codes:

```
# Start the server container
podman run --detach --name dx40-pg --label sag.devx-e2e=1 \
  --env POSTGRES_PASSWORD=<throwaway> \
  --publish 127.0.0.1::5432 \
  docker.io/library/postgres:17.12-bookworm
# exit 0

podman port dx40-pg 5432/tcp
# 127.0.0.1:44681

# Wait for readiness via local socket
podman exec dx40-pg pg_isready -h 127.0.0.1 -p 5432 -U postgres
# 127.0.0.1:5432 - accepting connections; exit 0

# From client container: pg_isready via host.containers.internal
podman run --rm --name dx40-client --label sag.devx-e2e=1 \
  --env PGPASSWORD=<throwaway> \
  docker.io/library/postgres:17.12-bookworm \
  pg_isready -h host.containers.internal -p 44681 -U postgres
# host.containers.internal:44681 - accepting connections; exit 0

# From client container: psql select 1 via host.containers.internal
podman run --rm --name dx40-client --label sag.devx-e2e=1 \
  --env PGPASSWORD=<throwaway> \
  docker.io/library/postgres:17.12-bookworm \
  psql -h host.containers.internal -p 44681 -U postgres -c "select 1"
# returns one row with value 1; exit 0

# Remove server container
podman rm --force --volumes --ignore dx40-pg
# exit 0

# Verify both containers absent (exit 1 = not found)
podman container exists dx40-pg   # exit 1
podman container exists dx40-client  # exit 1
```

Outcome: a container on `podman-machine-default` reaches a PostgreSQL container through `host.containers.internal` and the published host port. `pg_isready` and `psql` both succeed. DX-AC9 can proceed.

## Findings fixed at integration

- B30 - status:FIXED - action:YES - DX-AC12 accepted each non-zero exit - fix:require exit 1 - why:criterion text
- B31 - status:FIXED - action:YES - `D4.2` files not committed - the merged commit did not build; the agent gates ran on its working tree - fix:commit `input.go`, `yaml.go` and two tests on the task branch - why:the gate tests committed code
- B32 - status:FIXED - action:YES - six files not formatted - fix:`gofmt -w` - why:gate rule
- B33 - status:FIXED - action:YES - `dxh` runner test used `syscall.Getsid` on Linux - fix:file name `runner_darwin_test.go` - why:phase B build
- B34 - status:FIXED - action:YES - stale file view after a rename - after a rename replaces `sag.yaml`, the Podman machine shows no file for about 6 s, and `podman run` fails with `statfs` - fix:`service start` waits until `podman machine ssh -- test -f <cfg>/sag.yaml` passes, at most 15 s, with unit tests (decision D4-a) - why:each `credential create` and each rollback failed
- B35 - status:FIXED - action:YES - expiry warn never written - the CLI did not pass `ca.pem` to `list` - fix:pass `<cfg>/ca.pem` to `list` only - why:PRD-devx 8, DX-AC29
- B36 - status:FIXED - action:YES - canary helper left its objects after a failure and used an unpinned image - fix:remove own leftovers first; `postgres:17.12-bookworm` - why:one failure broke each later criterion; README 6.1
- B37 - status:FIXED - action:YES - canary user and database were `postgres` - fix:random `cu_<hex>` and `cd_<hex>` - why:the leak search matched legal text
- B38 - status:FIXED - action:YES - TLS PostgreSQL of DX-AC37 never started - fix:key secret `uid=999,gid=999,mode=0600` - why:PostgreSQL refuses a key that others can read
- B39 - status:FIXED - action:YES - DX-AC10 called `sag audit read`, a command of a later layer - fix:`podman cp sag:/var/lib/sag/audit` and search each file - why:criterion text names the audit file
- B40 - status:FIXED - action:YES - LaunchAgent wait too short - the `RunAtLoad` run of `service start` can take 15 s plus `podman run` plus 30 s - fix:both idle helpers wait 90 s and report the `launchctl` state on timeout - why:the helpers wait for a legitimate product process

## Independent review (Fable), verdict REWORK

The review covered `sag-devx-3..366805f`. It found no secret leak in the product: the value goes only to standard input of `podman secret create` and `podman exec -i`. Tasks `D4.5a`, `D4.5b` and `D4.5c` fixed the items. Aelita verified each item and rewrote the argument test of B16.

- B1 - status:FIXED - action:YES - expiry warn never written - fix:see B35; `inspect` writes no expiry line - why:PRD-devx 8 names `list` only
- B2 - status:FIXED - action:YES - text output lacked the section 8 fields - fix:one labelled line per field - why:DX-AC17
- B3 - status:FIXED - action:YES - `--tls` default `disable` - fix:default `verify-full` - why:config default of PRD
- B4 - status:FIXED - action:YES - empty `--tls-ca` file accepted - fix:exit 1 when no certificate parses - why:PRD-devx 7.4 rule 5
- B5 - status:FIXED - action:YES - prompt refused when standard input is not a terminal - fix:open `/dev/tty`; exit 3 only when the open fails - why:PRD-devx 7.3
- B6 - status:FIXED - action:YES - canary database `postgres` - fix:see B37 - why:false failure
- B7 - status:FIXED - action:YES - leak failure printed the canary part - fix:report part names and source names only - why:secret rule
- B8 - status:FIXED - action:YES - leak sources empty or absent - fix:combined `podman logs`; a source error fails; the create output and the pseudo-terminal transcript are sources - why:DX-AC10
- B9 to B15 - status:FIXED - action:YES - DX-AC9, DX-AC11, DX-AC14, DX-AC15, DX-AC16, DX-AC17, DX-AC37 lacked clauses - fix:one assertion per clause; DX-AC9 types the DSN on a pseudo-terminal - why:criterion texts
- B16 - status:FIXED - action:YES - argument test could not fail - fix:`TestCreateChildProcessesHoldNoValue` runs `credentialCreate` against a recording `podman` script; a mutation that adds the value to an argument gives 4 failures - why:PRD-devx 17.1
- B17 - status:FIXED - action:YES - exit 1 assertions passed on a lock error - fix:wait for the LaunchAgent run; assert each fixed message - why:false pass
- B18 - status:FIXED - action:YES - unpinned image - fix:see B36 - why:README 6.1
- S1 to S11 - status:FIXED - action:YES - not-found by `errors.Is`, rollback warn lines, no lock for `list` and `inspect`, one line terminator, fixed check text, validation order, `Load` test with metadata, key scan rule tests, temporary directory symlink, `sag.yaml` content test, exact expiry line
- S12 - status:OPEN - action:NO - `--tls-ca` read error gives exit 2 - fix:none - why:PRD-devx 1.3 allows exit 2 for usage

## Final gate on `01d4e8a`

```
gofmt -l .                                   → empty
make contract-check                          → exit 0
make test                                    → exit 0
go test -tags devxtest ./internal/...        → exit 0
make e2e-matrix LAYER=6 PGS="14 17 19"       → exit 0 (on aec81ae; later commits change only e2e/devx)
make e2e-devx SUITE=Pins LAYER=4             → no fail, no skip
make e2e-devx SUITE=Main LAYER=4             → two runs, exit 0, report over Main and Pins at 01d4e8a
```

```
PASS DX_Harness_Skeleton  PASS DXAC01  PASS DXAC02  PASS DXAC03  PASS DXAC04
PASS DXAC06  PASS DXAC07  PASS DXAC08  PASS DXAC09  PASS DXAC10
PASS DXAC11  PASS DXAC12  PASS DXAC13  PASS DXAC14  PASS DXAC15
PASS DXAC16  PASS DXAC17  PASS DXAC27  PASS DXAC28  PASS DXAC29
PASS DXAC33  PASS DXAC36  PASS DXAC37  PASS DXAC50  PASS DXAC53
```

After the runs, `~/.config/sag`, the LaunchAgent, each container with label `sag.devx-e2e=1`, and each product and canary secret are absent.

## Disk

```
before: /var 15391616 KiB free
after:  /var 15215140 KiB free
```

## Open items

- S1 - status:OPEN - action:YES - two LaunchAgent idle helpers - `waitLaunchAgentIdle` in `e2e/devx/dxac01_test.go` and `dxh.WaitLaunchAgentIdle` do the same work - fix:keep `dxh.WaitLaunchAgentIdle` with the state report and remove the other in layer 5 - why:one owner per helper

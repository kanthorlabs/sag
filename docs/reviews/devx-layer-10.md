# Layer 10 gate: devx

Phase B on the Debian machine `homelab`, phase A on the macOS development machine. Gate commit `6b2ae17`.

## Machine record

| Item | Value | Verification command |
|---|---|---|
| OS | Debian GNU/Linux 13 (trixie) amd64 | `cat /etc/os-release` |
| Kernel | 6.12.41+deb13-amd64 | `uname -r` |
| `/usr/bin/podman` | 5.4.2+ds1-2+b2 (the DX-AC5 runs reinstalled the apt candidate; before: `+b1`) | `dpkg-query -W podman` |
| `/usr/local/bin/podman` | 6.1.3 of the teammate, first in the login PATH; unchanged; the gate PATH excludes `/usr/local/bin` | `/usr/local/bin/podman --version` |
| Linger | yes | `loginctl show-user cantholabs --property=Linger` |
| systemd | 257 (257.13-1~deb13u1) | `systemctl --version` |
| `sudo -n` | works through `/etc/sudoers.d/sag-devx` (temporary) | `sudo -n true` |
| iproute2 `ss` | 6.15.0 | `ss --version` |
| `uidmap` | Recommends of `podman`; installs with `apt-get install -y podman` | `apt-cache depends podman` |
| `podman container inspect` secrets | lists the secrets of a container (PRD-devx 8) | probe on homelab |
| Go | 1.27.4 in `/usr/local/go` | `go version` |
| Node | 24.15.0 in `.dev/tools/node`, gate PATH only | `node --version` |
| Gate PATH | `/usr/local/go/bin:<repo>/.dev/tools/node/bin:/usr/bin:/bin` | gate script |

## Tasks

| Task | Merge |
|---|---|
| `D10.0` Linux contract, preconditions, machine record | `3662a1a` |
| `D10.1` apt install, Debian check, DX-AC5 | `20f2a49` |
| `D10.2` Quadlet service and autostart | `13cada5` |
| `D10.3` port check with `ss`, operator step, UID 0 guard, bash profile | `751c3d1` |
| `D10.4` phase B gate fixes, review fixes | `c3c9653`, `78493c0`, `5c69098`, `6cbed54`, `bc8885b`, `5a2b516`, `479af1c`, `0afaf7e`, `6b2ae17` |

## Decisions

- D10-a: the gate commit in this file marks the layer. Only `vX.Y.Z` tags exist.
- D10-b: Linux support is Debian 13, amd64 and arm64 (PRD-devx revision 9).
- D10-c: the Linux pin is `podman 5.4.2 linux/* apt podman -`.
- D10-d: phase B runs on `homelab` with a temporary `sudo -n` rule. Tests use `/usr/bin/podman` only. Tests remove only objects with the label `sag.devx-e2e=1` or the exact product names.
- D10-e: Node 24.15.0 for DX-AC44 lives in `.dev/tools/node` on homelab. Only the gate PATH holds it.
- D10-f: on Linux the test databases and LocalStack publish on the route source IPv4 address. `host.containers.internal` reaches that address under pasta. Tests never print it.
- D10-g: the homelab gate runs `make e2e LAYER=6 PG=17 AC_SKIP=AC31`. v1 AC31 needs the logged-in `claude` of the machine owner. The phase A gate keeps AC31.

## Findings during the tasks

- S106 - status:FIXED - action:YES - D10.0 verify line cannot pass - `GOOS=linux CGO_ENABLED=0 go vet ./...` fails on each machine, because `internal/adapter/postgres` needs cgo (`pg_query`) - fix:vet the six Linux packages without cgo on macOS, and `go vet ./...` with cgo on homelab - why:a verify command that cannot pass gives no signal
- S107 - status:FIXED - action:YES - the D10.0 secret-list probe pulled `alpine:3` into the store of the teammate - the image count went from 50 to 51 - fix:removed `docker.io/library/alpine:3` (one tag on its ID); count back to 50; later probes use an image that exists in the store - why:homelab must stay as it was (D10-d)
- B149 - status:FIXED - action:YES - D10.0 Linux unit check passed for an existing inactive unit - `systemctl --user is-active` fails for a loaded but stopped `sag.service`, so the check passed - fix:require `LoadState` `not-found`; `TestLinuxRefusesExistingUnit` fails on the old check - why:README 2.2 says the unit must not exist
- B150 - status:FIXED - action:YES - Main and Pins suites ran the macOS preconditions on Linux - `SuiteEntry` always called the Podman machine check; `QuadletExists` was unset in Main and Pins - fix:`SuiteEntry` picks the Linux preconditions on Linux; each suite sets `QuadletExists`; nil fails closed; the Install suite uses `SuiteEntry` - why:the phase B gate runs Main and Pins on Debian
- B151 - status:FIXED - action:YES - D10.2 Linux teardown fails on a clean machine - `systemctl --user stop sag` exits 5 when the unit is absent; each group pre-teardown would fail - fix:treat an absent unit as success; unit test for the clean case - why:the gate runs on a clean machine
- B152 - status:FIXED - action:YES - D10.2 `Start` takes autostart from the old Quadlet file, not from `answers.json` - fix:read the stored autostart answer - why:PRD-devx 6
- S108 - status:FIXED - action:YES - D10.2 mutant of `TestStopUsesSystemctl` changed the assertion - fix:mutate the product code - why:a mutant must break the code
- B153 - status:FIXED - action:YES - D10.3 operator step runs `tailscale set` without `sudo` - fix:`sudo <tailscale> set --operator=<user>` with the terminal; argv test - why:PRD-devx 5.4, 5.8
- B154 - status:FIXED - action:YES - D10.3 DX-AC36 phase B tests only `bootstrap` - fix:each host command of PRD-devx 1.1 through `sudo -n`; fix the product guard when a command does not refuse - why:DX-AC36 says each host command
- S109 - status:FIXED - action:YES - D10.3 user name from `$USER` - fix:`os/user.Current()` - why:the process account decides
- B155 - status:FIXED - action:YES - D10.1 DX-AC5 checks `/usr/local/bin/podman` 6.1.3 through the run PATH, and the bootstrap child has no test PATH - fix:`/usr/bin/podman` by absolute path, child `PATH=TestPATH`, dpkg version before and after, `newuidmap` in the test PATH - why:the test must prove the apt package
- B156 - status:FIXED - action:YES - D10.1 DX-AC5 does not prove that the image stays - fix:same image ID before the remove and after bootstrap - why:layer 10 plan D10.1
- S110 - status:FIXED - action:YES - D10.1 DX-AC5 cleanup only logs a failed reinstall - fix:reinstall only when `/usr/bin/podman` is absent; `t.Errorf` on failure - why:the teammate machine must keep its Podman
- S111 - status:FIXED - action:YES - D10.3 old DX-AC36 phase B text check was unreachable - the test expected `refused to run as UID 0` from the bootstrap step; `commandGuard` in `internal/cli/mode.go` refuses first with `sag: host commands refuse root` - fix:the test runs each of the 14 host-only commands through `sudo -n` and expects exit 1 with the guard text - why:DX-AC36 says each host command
- B157 - status:FIXED - action:YES - Linux test children could not reach the systemd user manager - the README 2.1 allow list removed `XDG_RUNTIME_DIR`; bootstrap in DX-AC5 failed at `systemctl --user daemon-reload` - fix:pass `XDG_RUNTIME_DIR` on Linux only (PRD-devx 17.1 setup); README updated; `TestChildEnvXDGRuntimeDirOnLinuxOnly` fails with the Linux clause off - why:PRD-devx 17.1 needs the user manager
- B158 - status:FIXED - action:YES - DX-AC5 expected the old package build after the reinstall - apt installs the candidate (`5.4.2+ds1-2+b2`), not the removed build (`+b1`) - fix:expect the apt candidate read before the remove - why:PRD-devx 5.3 installs the apt candidate
- S112 - status:OPEN - action:NO - homelab Podman package moved from `5.4.2+ds1-2+b1` to `+b2` - the first DX-AC5 run reinstalled the candidate, a Debian rebuild of the same upstream version - fix:none; tell the teammate - why:a downgrade needs the old package file and gives no benefit
- S113 - status:OPEN - action:YES - the homelab image count went from 50 to 53 after the first test image build - the list after the run is in homelab `.dev/local/images-after-install1.txt` - fix:after the gate, remove only `localhost/sag:devx-e2e`; report the other new images to the teammate - why:D10-d allows only labeled or product names
- B159 - status:FIXED - action:YES - `sag service start` and `sag service stop` refused Linux - `internal/service/platform.go` accepted only `darwin/arm64`; gate run 1 failed DX-AC3, DX-AC24, DX-AC25, DX-AC27, DX-AC28, DX-AC36, DX-AC53 with `unsupported platform linux/amd64` - fix:accept `linux/amd64` and `linux/arm64` like bootstrap; platform test without a build tag - why:PRD section 6 runs `sag service start` on Debian

## Findings from the phase B gate runs and the review

The review covered `53a2b98..479af1c`.

- B160 - status:FIXED - action:YES - a `release` test item failed on Linux - `DefaultDownload` and `DefaultSHA256` were stubs on Linux; DX-AC7 phase B needs the download and the checksum check - fix:move both functions to the shared file `internal/deps/download.go` - why:DX-AC7 has the run `A, B`
- B161 - status:FIXED - action:YES - DX-AC6 and DX-AC7 wrote `darwin/arm64` test pins - bootstrap on Linux wrote `no pins for platform linux/amd64` - fix:build the test pin platform from `runtime.GOOS` and `runtime.GOARCH` - why:the tests did not exercise the phase B form
- B162 - status:FIXED - action:YES - DX-AC8 apt clause gave the child no PATH - bootstrap failed at `apt-cache` lookup with an empty stdout; the test also did not prove "no install" - fix:add the test PATH, expect `fail podman candidate <v>, pin 999.0.0`, compare the dpkg version before and after - why:PRD DX-AC8 demands exit 1 and no install for an older candidate
- B163 - status:FIXED - action:YES - DX-AC30 and the harness skeleton used macOS commands on Linux - `podman machine ssh` and `launchctl` do not exist on Debian - fix:on Linux read the local and user registries files; check `systemctl --user show-environment` and a write in `~/.config/containers/systemd` - why:both run in phase B
- B164 - status:FIXED - action:YES - the SAG container on Debian could not reach the test databases and LocalStack - they published on `127.0.0.1`; under pasta `host.containers.internal` reaches the host address, not loopback (probe on homelab: host address accepts, loopback no response); DX-AC9 canary create failed with `cannot connect` and cascaded to DX-AC9 to DX-AC23, DX-AC31, DX-AC37 - fix:`dxh.PublishAddr` gives the route source IPv4 on Linux and `127.0.0.1` on macOS; the canary, AWS test, DX-AC37 databases and LocalStack publish on it - why:PRD step 10 of `credential create` says that on Linux the database listens on a non-loopback address
- B165 - status:FIXED - action:YES - the homelab gate used `/usr/local/bin/podman` 6.1.3 - the non-interactive PATH starts with `/usr/local/bin` - fix:the gate script sets `PATH=/usr/local/go/bin:<node>/bin:/usr/bin:/bin` - why:HANDOFF 7.1 allows only `/usr/bin/podman`
- B166 - status:FIXED - action:YES - gate run 1 ran in `screen` with a controlling terminal - agentsetup unit tests and DX-AC50 waited on `/dev/tty` until the timeout - fix:agentsetup tests default to a no-terminal asker; the gate runs with `setsid nohup` and stdin `/dev/null` - why:a gate result must not depend on the terminal of the runner
- B167 - status:FIXED - action:YES - DX-AC44 phase B had no Node - homelab has no `node`; `acreport` failed DX-AC44 - fix:install Node 24.15.0 (SHA-256 checked against nodejs.org) in homelab `.dev/tools/node`, put it on the gate PATH only - why:DX-AC44 has the run `A, B` with Node 24
- S114 - status:FIXED - action:YES - v1 AC31 failed on homelab with `claude is not in PATH` - AC31 needs the logged-in `claude` of the machine owner; on homelab that is the teammate account - fix:homelab gate runs `make e2e LAYER=6 PG=17 AC_SKIP=AC31` (decision D10-g); phase A keeps AC31 - why:HANDOFF 7.1 forbids use of the teammate `~/.claude`
- B168 - status:FIXED - action:YES - repeated `sag service start` on Debian hit the systemd start limit - journal: `Start request repeated too quickly`, `start-limit-hit`; DX-AC22 failed at `systemctl --user restart sag` and DX-AC23, DX-AC41 cascaded - fix:Linux `start` runs `systemctl --user reset-failed sag` after `daemon-reload` and before `restart`; unit test checks the order - why:an explicit start must not fail because of earlier starts
- B169 - status:FIXED - action:YES - DX-AC30 pinned the `registry:3.0.0` arm64 manifest digest - on amd64 the registry container did not start and the push failed with exit 125 - fix:pin the index digest `sha256:6c5666b8…457e` - why:the test image must run on each phase B architecture
- B170 - status:FIXED - action:YES - DX-AC50 success cases on Debian got `skip tailscale operator not set` - the fake `tailscale debug prefs` gave an empty `OperatorUser` - fix:on Linux the test writes the current user as `OperatorUser`; the `operator-not-set` case still writes another user - why:PRD 5.8 condition 3 holds in the success cases
- B171 - status:FIXED - action:YES - DX-AC53 on Debian expected the container `sag` after `sag service stop` - the Quadlet unit removes the container at stop - fix:on Linux compare the container presence and the Quadlet file hash and mtime before and after - why:the AC text demands no file and no container change
- B172 - status:FIXED - action:YES - the harness skeleton checked free space through `podman machine ssh` on Debian - fix:use `CheckLinuxFreeSpace` on Linux - why:phase B has no Podman machine
- B173 - status:FIXED - action:YES - DX-AC41 bash case on Debian saw no container `sag` - the test home linked only `~/.local/share/containers/podman` (the macOS machine data); rootless Podman on Linux keeps its store in `~/.local/share/containers/storage` - fix:on Linux link the whole `~/.local/share/containers`, pass `XDG_RUNTIME_DIR`, and assert that Podman from the test home sees the container `sag` - why:the D6-a check passed with an empty store, so it could not catch the fault
- S115 - status:FIXED - action:YES - the homelab gate summary showed the AC report of the filtered unit run - that report said `Gate layer 10 phase B: PASS` while DX-AC41 failed in Main - fix:the gate script copies the report after the Pins run to `ac-report-full.md` and adds its gate, harness, `NOT_IMPLEMENTED` and `FAIL` lines to the summary - why:the gate decision needs the report over all suites
- B174 - status:FIXED - action:YES - the Debian operator step turned a prompt error into a skip - `EnsureOperator` returned `ErrOperatorSkip` for an `ask` error, and bootstrap continued as if the operator refused - fix:return the `ask` error; unit test `TestOperatorAskErrorReturned`; product mutant killed on homelab - why:a terminal fault must not look like a refusal
- S116 - status:OPEN - action:NO - apt absence uses the Podman path - `makeAptItem` treats an empty `podmanPath` as an absent package - fix:none - why:Podman is the only apt pin (PRD section 4); a second apt item is not in scope
- S117 - status:OPEN - action:NO - LocalStack publishes on `127.0.0.1` and on the route address on Linux - fix:none - why:the test process reaches LocalStack through `localhost:4566`; the container reaches it through the route address; both bindings are needed
- S118 - status:OPEN - action:NO - review asked about the new `registry:3.0.0` digest - fix:none - why:`sha256:6c5666b8…` is the index digest (Docker Hub `docker-content-digest`); the old `sha256:832ef183…` was the arm64 manifest (B169)
- B175 - status:FIXED - action:YES - the phase B report required the DX-AC39 default path case - PRD DX-AC39: "In phase A, this default path case runs with the real-harness criteria"; layer 8 of the PRD plan lists it as macOS; on homelab the User suite would use the teammate `~/.claude` - fix:`ACTest.Run` marks `DXAC39_DefaultPath` as phase A; the report gives `OTHER_PHASE` for it in phase B; unit test `TestDevxTestPhaseA`; mutant killed - why:the report must follow the PRD phase of each clause

## Phase B gate on `6b2ae17` (homelab)

The gate ran with `setsid nohup`, stdin `/dev/null`, and no controlling terminal.

```
make e2e-devx SUITE=Install LAYER=10 PHASE=B  → 3 pass events, no fail, no skip
make test                                     → exit 0
make e2e LAYER=6 PG=17 AC_SKIP=AC31           → exit 0
make e2e-devx SUITE=Main LAYER=10 PHASE=B     → 142 pass events, no fail, no skip
make e2e-devx SUITE=Pins LAYER=10 PHASE=B     → 6 pass events, no fail, no skip
make e2e-devx-unit LAYER=10 PHASE=B           → exit 0
acreport -suite devx -layer 10 -phase B (all four suites at 6b2ae17) → exit 0
```

The suite runs exit 2 only because the report of an earlier suite named an older commit. Gate layer 10 phase B: PASS. Harness skeleton: PASS.

```
PASS DX-AC01 to DX-AC33, DX-AC36, DX-AC37, DX-AC39 to DX-AC46, DX-AC49 to DX-AC51, DX-AC53
PASS DX-AC05 (run B)
OTHER_PHASE DX-AC34, DX-AC35, DX-AC38, DX-AC47, DX-AC48, DX-AC52
```

## Phase A gate on `6b2ae17` (macOS)

```
gofmt -l internal e2e cmd                    → empty
go vet ./... and -tags devxe2e               → exit 0
go test -tags devxtest ./internal/...        → exit 0
make contract-check                          → exit 0
make test                                    → exit 0
make e2e-devx-unit LAYER=10                  → exit 0
make e2e-devx SUITE=Mac LAYER=10             → 11 pass events, no fail, no skip (second run, see S120)
make e2e-devx SUITE=User LAYER=10            → 11 pass events, no fail, no skip
make e2e-devx SUITE=Main LAYER=10            → two runs, 140 pass events each, no fail, no skip
make e2e-devx SUITE=Pins LAYER=10            → 6 pass events, no fail, no skip
make e2e-matrix LAYER=6 PGS="14 17 19"       → exit 0
acreport -suite devx -layer 10 -phase A (all five suites at 6b2ae17) → exit 0
```

The `tailscale serve status` hash was the same before and after the Mac suite. Gate layer 10 phase A: PASS. Harness skeleton: PASS.

```
PASS DX-AC01 to DX-AC04, DX-AC06 to DX-AC53
OTHER_PHASE DX-AC05
```

## Homelab state after the gate

- No container, volume, secret, Quadlet file or `<cfg>` of SAG remains.
- New images from the test runs, not labeled, reported to the teammate: `localhost/sag:devx-e2e`, `localhost/sag:e2e`, `localhost/sag-conformance:e2e`, `docker.io/library/postgres:17.12-bookworm`, `docker.io/library/postgres:13-bookworm`, `docker.io/library/busybox:1.37.0-musl`, `docker.io/library/registry` (two untagged IDs), `docker.io/localstack/localstack` (untagged).
- The teammate removes `/etc/sudoers.d/sag-devx`.

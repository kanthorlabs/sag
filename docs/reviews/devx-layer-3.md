# Layer 3 gate: devx

Phase A, macOS development machine. Gate commit `21be67d`.

## Tasks

| Task | Merge |
|---|---|
| `D3.0` contract | `6d903ff` |
| `D3.2` Tailscale serve rule | `ece22e2` |
| `D3.3` port check and idempotence | `96945fc` |
| `D3.1` pins and dependency rule | `dc79b7f` |
| `D3.5a` product review fixes | `a6f647d` |
| `D3.5b` DX test review fixes | `c57ce0f` |

## Findings fixed at integration

- B25 - status:FIXED - action:YES - changed Tailscale address did not renew the certificate - the plan copied the detected address into the stored answers, so the certificate step saw no change - fix:the plan keeps the stored `tailscale_ip`; the certificate step receives the detected address - why:PRD-devx 5.7 renewal trigger
- B26 - status:FIXED - action:YES - DX-AC3 lock race - the `RunAtLoad` run of the LaunchAgent held the lock while DX-AC3 ran `service stop` - fix:DX-AC3 waits until `launchctl print` shows `state = not running`, at most 30 s - why:the pass depended on timing
- B27 - status:FIXED - action:YES - conformance image build failed - `internal/bootstrap` imports the embedded pins package, and `e2e/conformance/Containerfile` did not copy it - fix:copy `deploy/embed.go deploy/pins` - why:the v1 regression runs on this image
- B28 - status:FIXED - action:YES - release pin without checksum - `ValidateItem` accepted `-` for a `release` pin - fix:`release` requires 64 hex, `apt` requires `-` - why:PRD-devx 5.3 pin table
- B29 - status:FIXED - action:YES - DX-AC4 required an empty stdout - the plan writes `ok podman <version>` before the exit 3 - fix:DX-AC4 makes no stdout assertion - why:PRD-devx 5.2 step 3 and table 5.3 require the line; DX-AC4 sets no stdout rule

## Independent review (Fable), verdict CLOSE AFTER FIXES

The review covered `sag-devx-2..8a3ec4a`. It found no secret leak and no drift from decisions D2-a to D2-i. Task `D3.5a` fixed the product items, and task `D3.5b` fixed the DX test items. Aelita verified each item in the code.

- B1 - status:FIXED - action:YES - port check stops bootstrap without a usable Podman - fix:an empty Podman path or a failed `podman ps` means no container holder; the check continues with `lsof` - why:an approved install could never run
- B2 - status:FIXED - action:YES - DX-AC3 weak clauses - fix:the JWT from before the run authenticates at `/mcp`; the second bootstrap runs with `Setsid`; exactly one `skip tailscale command absent; loopback only` line - why:criterion text
- B3 - status:FIXED - action:YES - DX-AC50 weak clauses - fix:certificate names, fingerprint kept, no name for `0.0.0.0` and `192.168.1.10`, one port binding `127.0.0.1:8443`, no file or secret change on a conflict, `--autostart no` - why:criterion text
- B4 - status:FIXED - action:YES - DX-AC8 cannot see `<cfg>` - fix:own group after a teardown; `<cfg>` absent; full fail line; Podman version unchanged - why:criterion text
- B5 - status:FIXED - action:YES - DX-AC7 does not prove one install of the verified file - fix:the fake installer appends path and SHA-256; one line, path under `$TMPDIR/sag-deps-*`, pinned hash; `checksum mismatch` on the wrong hash - why:criterion text
- B6 - status:FIXED - action:YES - DX-AC53 skips its process cases - fix:retry the listen for 10 s, then fail - why:a skip hides a required case
- S1 to S10 - status:FIXED - action:YES - `ok podman <version>` line, pre-approval warn text, `sudo` check before the installer, version suffixes, second version read after the install, broken `podman --version`, strict pin parsing, holder image label, stale failure text, unit tests for the Tailscale and port plan wiring
- S11 - status:OPEN - action:NO - DX-AC53 compares only `<cfg>` and the container - fix:none - why:both exits occur before `podman run`
- S12 - status:OPEN - action:NO - machine rule reads the first machine - fix:none - why:PRD-devx 17.1 fixes one machine on the development machine

## Final gate on `21be67d`

```
gofmt -l .                                   → empty
make contract-check                          → exit 0
make test                                    → exit 0
go test -tags devxtest ./internal/...        → exit 0
make e2e-matrix LAYER=6 PGS="14 17 19"       → exit 0, gate passed (on c57ce0f; 21be67d changes one DX test)
make e2e-devx SUITE=Main LAYER=3             → two runs, no fail, no skip
make e2e-devx SUITE=Pins LAYER=3             → no fail, no skip
acreport -suite devx -layer 3 (Main + Pins)  → exit 0, devx gate layer 3 passed
```

```
PASS DX_Harness_Skeleton
PASS DXAC01_Bootstrap
PASS DXAC02_ContainerAndCert
PASS DXAC03_BootstrapIdempotent
PASS DXAC04_BootstrapNoTerminal
PASS DXAC06_InstallApproval
PASS DXAC07_ReleaseChecksum
PASS DXAC08_VersionRule
PASS DXAC27_Autostart
PASS DXAC28_ServiceStopStart
PASS DXAC33_ProductBinary
PASS DXAC36_HostGuards
PASS DXAC50_TailscaleFake
PASS DXAC53_PortHolder
```

After the runs, `~/.config/sag`, the LaunchAgent, and each container and secret with label `sag.devx-e2e=1` are absent.

## Disk

```
before: Images 80 9.476GB; /var 16513372 KiB free
after:  Images 80 9.476GB; /var 16511036 KiB free
```

# Layer 2 gate: devx

Phase A, macOS development machine. Gate commit `e60223e`.

## Tasks

| Task | Merge |
|---|---|
| `D2.0` contract | `1713b6f` |
| `D2.2` bootstrap flow | `5f2a8e9` |
| `D2.1` host frame and state | `546d736` |
| `D2.4` master key, certificate, image pull | merge before `6dfef40` |
| `D2.3` service start and stop on macOS | merge before `844c776` |

## Findings fixed at integration

- B16 - status:FIXED - action:YES - DX-AC4 ran the wrong binary - the test ran `bin/sag-devxtest` of the main repository - fix:`dxh.TestBinary()` returns the binary of the current worktree - why:the e2e result must test the branch under test
- B17 - status:FIXED - action:YES - DX-AC36 lock clause passed falsely - the clause accepted any exit 1 of a stub - fix:the clause runs `bootstrap`, `service start` and `service stop` with the lock held and requires exit 1 and the lock message - why:a test must fail when the lock code is absent
- B18 - status:FIXED - action:YES - `RUN` filters with `|` selected nothing - Go splits `-run` at a top-level `|` - fix:`make e2e-devx` and `make e2e` wrap `$(RUN)` in parentheses - why:the verify commands of the plan use this form
- B19 - status:FIXED - action:YES - Podman failure read as an absent secret - `masterkey` and `endpointcert` treated each non-zero exit as absent - fix:`podman.SecretAbsent` matches only the Podman texts `no secret with name` and `no such secret` - why:only a proven absence may create a secret
- B20 - status:FIXED - action:YES - second writer of `answers.json` - `endpointcert` had its own JSON code - fix:`endpointcert` uses `hoststate.LoadAnswers` and `hoststate.SaveAnswers` - why:one owner of the file format and an atomic write with mode 0600
- B21 - status:FIXED - action:YES - the product ran with the full environment - `runSAG` and `EnsureBootstrapped` used `NewRunner()` - fix:`dxh.NewProductRunner(t)` applies the allow list of README 2.1 - why:README 2.1 requires the allow list for each process under test
- B22 - status:FIXED - action:YES - the test image was not tagged - the suite passed no group setup, so the LaunchAgent process found no image - fix:each group runs `dxh.GroupSetupImage` - why:DX-AC27 must prove the real `launchd` start
- B23 - status:FIXED - action:YES - two copies of the image selection - `internal/cli/service.go` had its own `imageRef` - fix:`service` uses `image.Reference()` - why:one owner of the PRD-devx 4.3 rule

## Gate on `e60223e`

```
gofmt -l .                                   → empty
make contract-check                          → exit 0
make test                                    → exit 0
make e2e-matrix LAYER=6 PGS="14 17 19"       → exit 0, gate passed
make e2e-devx SUITE=Main LAYER=2             → exit 0, devx gate layer 2 passed
```

DX criteria of layer 2:

```
PASS DX_Harness_Skeleton
PASS DXAC01_Bootstrap
PASS DXAC02_ContainerAndCert
PASS DXAC04_BootstrapNoTerminal
PASS DXAC33_ProductBinary
PASS DXAC36_HostGuards
PASS DXAC27_Autostart
PASS DXAC28_ServiceStopStart
```

After the run, `~/.config/sag` and `~/Library/LaunchAgents/net.kanthorlabs.sag.plist` are absent.

## Disk

```
before: Images 78 9.473GB; Containers 5; Local Volumes 6 712.4MB; /var 16529472 KiB free
after:  Images 78 9.474GB; Containers 5; Local Volumes 6 712.4MB; /var 16525264 KiB free
```

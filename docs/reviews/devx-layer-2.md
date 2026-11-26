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

## Independent review (Fable), verdict CLOSE AFTER FIXES

The review covered `sag-devx-1..e60223e`. It found no secret leak, and decisions D2-a to D2-g hold in the code. Task `D2.5a` fixed the product items, and task `D2.5b` fixed the DX test and harness items. Aelita verified each item in the code.

- B1 - status:FIXED - action:YES - DX-AC33 symbol check cannot fail - fix:compare the build settings of `go version -m` for `bin/sag` and `bin/sag-devxtest` - why:false pass
- B2 - status:FIXED - action:YES - no proof that the product ignores `SAG_TEST_*` - fix:`bin/sag bootstrap` runs with false `SAG_TEST_IMAGE` and `SAG_TEST_REGISTRY`, and the container image does not use them; the unit test sets each variable - why:clause 1 of DX-AC33
- B3 - status:FIXED - action:YES - DX-AC4 has no 30 s bound - fix:30 s deadline - why:criterion text
- B4 - status:FIXED - action:YES - DX-AC4 does not check secrets - fix:no product secret exists after the run - why:criterion text
- B5 - status:FIXED - action:YES - DX-AC2 secret count is a tautology - fix:3 secrets with exact names - why:false pass
- B6 - status:FIXED - action:YES - DX-AC27 weak clauses - fix:order yes, no, yes; parse `RunAtLoad`; check the stop before `kickstart` - why:false pass
- B7 - status:FIXED - action:YES - extra arguments ignored - fix:exit 2 in `bootstrap`, `service start`, `service stop` - why:PRD-devx 1.3
- B8 - status:FIXED - action:YES - prompt without ` [y/N] ` - fix:`Start SAG at login? [y/N] ` with a test on the written text - why:PRD-devx 5.4
- B9 - status:FIXED - action:YES - run lock accepts any live owner - fix:`make e2e-devx` writes a random token to the lock and passes it as `DEVX_RUN_TOKEN`; `CheckRunLock` requires the same token - why:one DX run at a time
- B10 - status:FIXED - action:YES - `XDG_CONFIG_HOME` mismatch - fix:the suite refuses to start while `XDG_CONFIG_HOME` is set; the harness uses `~/.config/sag` - why:false pass and wrong teardown target
- B11 - status:FIXED - action:YES - modes depend on the umask - fix:`chmod` 0644 for `ca.pem` and `sag.yaml`, 0700 for an existing `<cfg>`, with tests - why:PRD-devx 2.1
- S1 to S15 - status:FIXED - action:YES - weak and missing tests, step lines, healthz timeout, pull on exit 1 only, platform check in `service`, image removal prefix guard, teardown failure as a test error, quoted lock path, DX-AC27 step times
- S16 - status:OPEN - action:NO - the LaunchAgent does not pass `XDG_CONFIG_HOME` - fix:none - why:a PRD decision comes first
- S17 - status:FIXED - action:YES - stub step list - fix:`TestStepOrder` holds the expected order; the stub list is removed - why:orphan code
- B24 - status:FIXED - action:YES - the Podman machine sees a deleted `<cfg>` for some seconds - with libkrun, a bootstrap within about 5 s of the teardown failed with `statfs .../sag.yaml: no such file or directory` - fix:the teardown polls `podman machine ssh -- test -e <cfg>` until the path is absent, at most 15 s; this replaces a fixed 6 s sleep of `D2.5b` - why:wait on the real condition
- S18 - status:OPEN - action:NO - the same delay can reach a user who deletes `<cfg>` and runs bootstrap again within seconds - fix:none - why:rare, a second run succeeds, the PRD does not state it

## Final gate on `736e41f`

```
gofmt -l .                                   → empty
make contract-check                          → exit 0
make test                                    → exit 0
make e2e-matrix LAYER=6 PGS="14 17 19"       → exit 0, gate passed
make e2e-devx SUITE=Main LAYER=2             → exit 0
make e2e LAYER=6 PG=17 RUN='AC3[23]_'        → exit 0
go test -tags devxe2e -run '^TestDevxMain$' ./e2e/devx/ → refused: no run token in environment
```

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

Disk: before `/var` 16525548 KiB free, after 16523520 KiB free; Images 78, 9.474GB before and after. `~/.config/sag` and the LaunchAgent are absent after the run.

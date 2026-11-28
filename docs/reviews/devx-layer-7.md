# Layer 7 gate: devx

Phase A, macOS development machine. Gate commit `6cd1505`.

## D7.0 registry check

- Registry image: `docker.io/library/registry:3.0.0@sha256:832ef18332f0dde1edff9b45d796231c68f9b9108cf91acb5e6fac450b8fbdcb`.
- The container ran with `--rm`, the label `sag.devx-e2e=1`, and `-p 127.0.0.1:<free port>:5000`.
- `podman push --tls-verify=false localhost/sag:devx-e2e localhost:<port>/kanthorlabs/sag:main` exits 0.
- `podman push --digestfile` writes no digest with the remote Podman client on macOS. The digest comes from the header `Docker-Content-Digest` of `HEAD /v2/kanthorlabs/sag/manifests/main`.
- `podman pull --tls-verify=false localhost:<port>/kanthorlabs/sag@<digest>` exits 0. The same pull with TLS verification fails.
- After the check, no registry container and no pulled reference exist. No setting of `podman-machine-default` changed.

## Tasks

| Task | Merge |
|---|---|
| `D7.0` contract | `317fc50` |
| `D7.2` release workflow, dev image job, shared image workflow | `d7e4832` |
| `D7.1` image selection, registry hook, `sag version` reference | `9ec6068` |
| `D7.3` review fixes | `6cd1505` |
| S45 preflight timer spread (layer 5 item) | `99666bc` |

## Decisions

- D7-a: the first preflight recheck, retry and open retry of resource `i` of `n` start at `(i+1)/n` of the interval.
- D7-c: `image.yml` builds each architecture on its native runner and joins one manifest list; `release.yml` and `image-main` call it.
- D7-d: only bootstrap pulls the image; the macOS `podman run` has `--pull=never`.
- D7-e: `sag version` prints the image reference on the host only.

## Findings fixed at integration

- B115 - status:FIXED - action:YES - DXAC30 did not prove the pull - fix:absent check before each run, exact lines, cleanup checks - why:a check that cannot fail does not count
- B116 - status:FIXED - action:YES - DXAC30 registry start lost its error and raced for the port - fix:capture stderr; try a new port up to three times - why:gate run 1 failed with exit 126
- B117 - status:FIXED - action:YES - gate script ran the v1 matrix with `LAYER=7` - fix:`LAYER=6` - why:README 6.1 step 1

## Independent review (Fable), verdict no blocker

The review covered `sag-devx-6..9ec6068`. Task `D7.3` fixed the items.

- S68 to S76, S78 - status:FIXED - action:YES - exact leftover-reference check, cleanup order, `install image` lines, registry hook tests in CI, product test with the hook variables set, literal `sag version` tests, `--pull=never`, version in the container, `image-main` concurrency, D7-a order text
- S77 - status:OPEN - action:NO - the registry configuration check reads system paths only - fix:none - why:the product writes no configuration
- S79 - status:OPEN - action:NO - reclaimable image size grew by the pinned registry image of DX-AC30 - fix:none - why:a pinned test image, not a leak

## GitHub secrets

`DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` exist in the repository (set 2026-11-27). The first push to `main` proves the login and the push of `docker.io/kanthorlabs/sag:main`.

## Final gate on `6cd1505`

```
gofmt -l internal e2e cmd                    → empty
go vet ./... and -tags devxe2e               → exit 0
go test -tags devxtest ./internal/...        → exit 0
make contract-check                          → exit 0
make test                                    → exit 0
make e2e-devx-unit LAYER=7                   → exit 0
make e2e-devx SUITE=Main LAYER=7             → two runs, 137 pass events each, no fail, no skip
make e2e-devx SUITE=Pins LAYER=7             → exit 0, report over Main and Pins at 6cd1505
make e2e-matrix LAYER=6 PGS="14 17 19"       → exit 0
actionlint 1.7.9                             → exit 0, no output
make image                                   → exit 0
```

The two Main runs exit 2 only because the report of the earlier Pins run named `9ec6068`. The Pins run then wrote the report over both suites at `6cd1505`.

```
PASS DX_Harness_Skeleton  PASS DXAC01  PASS DXAC02  PASS DXAC03  PASS DXAC04
PASS DXAC06  PASS DXAC07  PASS DXAC08  PASS DXAC09  PASS DXAC10
PASS DXAC11  PASS DXAC12  PASS DXAC13  PASS DXAC14  PASS DXAC15
PASS DXAC16  PASS DXAC17  PASS DXAC18  PASS DXAC19  PASS DXAC20
PASS DXAC21  PASS DXAC22  PASS DXAC23  PASS DXAC24  PASS DXAC25
PASS DXAC26  PASS DXAC27  PASS DXAC28  PASS DXAC29  PASS DXAC30
PASS DXAC33  PASS DXAC36  PASS DXAC37  PASS DXAC39  PASS DXAC40
PASS DXAC41  PASS DXAC42  PASS DXAC43  PASS DXAC44  PASS DXAC45
PASS DXAC46  PASS DXAC49  PASS DXAC50  PASS DXAC51  PASS DXAC53
```

DXAC30 alone also passed two times on `6cd1505`. After the runs, `~/.config/sag`, the LaunchAgent, each container with label `sag.devx-e2e=1`, the registry container and each pulled registry reference are absent. No DX run added a Podman volume.

## Disk

```
podman system df before and after the gate: equal
Images 93, reclaimable 10.09GB; Local Volumes 14, reclaimable 667.6MB
/var free before the gate: 13808636 KiB
```

## Open items

- S77, S79 - status:OPEN - action:NO - see above.

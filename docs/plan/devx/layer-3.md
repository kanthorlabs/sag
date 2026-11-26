# Layer 3: Dependencies, Tailscale serve, port check

| Item | Value |
|---|---|
| Phase | A |
| Goal | Bootstrap checks the pinned Podman, downloads and checks a `release` item, applies the Tailscale serve rule, and refuses a foreign holder of port 8443. A second run is idempotent. |
| ACs | DX-AC3, DX-AC6, DX-AC7, DX-AC8, DX-AC50, DX-AC53 (phase A forms) |
| PRD-devx sections | 3, 5.2 steps 3, 6, 13, 5.3 (macOS), 5.4 (install answer), 5.8 (without the operator step), 6 (port check with `lsof`), 17.1 (test pins, fake `tailscale`) |
| Gate | `D3.4`: README section 6.1 with the `Pins` suite |
| Agents at once | 3. Start order: `D3.1`, `D3.2`, `D3.3` |
| Wall-clock | `D3.0` 1 h + parallel phase 4 h + `D3.4` 5 h = 10 h |

## File ownership

| Task | Owns |
|---|---|
| `D3.0` | `deploy/pins`, `deploy/embed.go`, `deploy/Containerfile`, `internal/daemon/containerfile_test.go`, `internal/bootstrap/steps.go` (calls to the new stubs), stubs of `internal/pins`, `internal/deps`, `internal/tailscale`, `internal/portcheck`, `internal/bootstrap/deps.go`, `internal/bootstrap/tailscale.go`, `internal/bootstrap/port.go`, `internal/service/port.go`, `e2e/devx/suite_pins_test.go`, `e2e/devx/suite_main_test.go` |
| `D3.1` | `internal/pins/*`, `internal/deps/*`, `internal/bootstrap/deps.go`, `internal/cli/bootstrap.go`, `internal/cli/bootstrap_test.go`, `e2e/devx/cmd/fakeinstall/*`, `e2e/devx/dxac06_test.go`, `dxac07_test.go`, `dxac08_test.go`, `e2e/devx/testdata/pins/*` |
| `D3.2` | `internal/tailscale/*`, `internal/bootstrap/tailscale.go`, `e2e/devx/cmd/faketailscale/*`, `e2e/devx/dxac50_test.go` |
| `D3.3` | `internal/portcheck/*`, `internal/bootstrap/port.go`, `internal/service/port.go`, `e2e/devx/dxac03_test.go`, `e2e/devx/dxac53_test.go` |
| `D3.4` | Every layer-3 file after `D3.1` to `D3.3` merge. `docs/reviews/devx-layer-3.md`. |

## D3.0 Contract

- Goal: Freeze the pin file, the dependency, Tailscale, and port check APIs, and their bootstrap call points.
- Owns: see the ownership table.
- Reads: PRD-devx 3, 5.2, 5.8, 6.
- Depends on: `D2.5`.
- Content:
  - `deploy/pins`: the two lines of PRD-devx 3. Read the SHA-256 of `podman-installer-macos-arm64.pkg` from the v5.5.1 release page.
  - `deploy/embed.go`: package `deployfiles`, `//go:embed pins`, `var Pins []byte`.
  - `deploy/Containerfile`: add `COPY deploy/embed.go deploy/pins ./deploy/` to the build stage. Update `internal/daemon/containerfile_test.go` when it checks the copy list.
  - `deps.Plan(ctx, pins) (Plan, error)`, `deps.Apply(ctx, Plan) error`.
  - `tailscale.Detect(ctx) (State, error)`, `tailscale.Apply(ctx, State) error`. `State` holds the address, the skip reason, and the serve rule state.
  - `portcheck.Holder(ctx) (Holder, error)`. `Holder` is none, container `sag`, another container, or a process.
  - Stub files of the bootstrap and service call points: `bootstrap/deps.go`, `bootstrap/tailscale.go`, `bootstrap/port.go`, `service/port.go`.
  - `e2e/devx/suite_pins_test.go`: `TestDevxPins` with the group `pins`.
  - `e2e/devx/suite_main_test.go`: add the groups `tailscale` and `port` to `TestDevxMain`, each with its group setup (README section 5.1). `DXAC03_BootstrapIdempotent` calls `EnsureBootstrapped`.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check && make image && go test -count=1 ./internal/daemon/... -run Containerfile`. Expected: exit 0, then `ok`.
- Done when: `D3.1` to `D3.3` compile against the stubs, and `make image` builds.

## D3.1 Pins and dependency rule on macOS

- Goal: Apply the dependency rule of PRD-devx 5.3 on macOS. Download and check a `release` item.
- Owns: see the ownership table.
- Reads: PRD-devx 3, 5.3, 5.4, 17.1.
- Depends on: `D3.0`.
- Method:
  - Download a `release` item to a new directory with mode 0700. Compute the SHA-256. On a difference, delete the file and exit 1.
  - macOS install: `sudo installer -pkg <file> -target /`. Then search `podman` again. Without a terminal, call `sudo -n`. A refusal by `sudo` gives exit 3.
  - Run `podman machine init` only when no machine exists. Run `podman machine start` only when the machine is stopped. On the development machine, neither runs.
  - `--auto-approve <name>@<version>` in `internal/cli/bootstrap.go`. Another version gives the `ignored` `warn` line.
  - `SAG_TEST_PINS` replaces the pins. `SAG_TEST_ABSENT` marks an item absent. `SAG_TEST_STOP_BEFORE_INSTALL` stops before the install command and prints the verified path.
  - Test pins: a `release` item for `darwin/arm64` from a local HTTP server. Its install command in the test build is `fakeinstall`, which records its argument. No test runs `sudo` or `installer`.
  - The installed test item is a test program that prints its version.
  - `deps_linux.go` stays a stub until layer 10.
- Tests first: `pins_test.go`: `TestParseLine`, `TestSelectPlatform`, `TestLinuxWildcard`; `deps_test.go`: `TestNewerWarns`, `TestOlderFailsInPlan`, `TestPreApprovalOtherVersionIgnored`, `TestChecksumMismatchDeletes`, `TestInstallRunsOnVerifiedPath`, `TestSudoNonInteractive`, `TestNoMachineCommandWhenRunning`, `TestNoSecondMachine`.
- e2e: `DXAC06_InstallApproval`, `DXAC07_ReleaseChecksum`, `DXAC08_VersionRule` in the suite `Pins`.
- ACs: DX-AC6, DX-AC7, DX-AC8.
- Verify: `go test -count=1 ./internal/pins/... ./internal/deps/... ./internal/bootstrap/... && GOOS=linux CGO_ENABLED=0 go vet ./internal/deps/... && make e2e-devx SUITE=Pins LAYER=3`. Expected: `ok`, then exit 0.
- Done when: bootstrap never changes an installed version.

## D3.2 Tailscale serve rule

- Goal: Detect Tailscale, add its address to the certificate, and apply the serve rule of PRD-devx 5.8.
- Owns: see the ownership table.
- Reads: PRD-devx 5.4, 5.7, 5.8, 17.1.
- Depends on: `D3.0`.
- Method:
  - Conditions: command in `PATH`, and `tailscale ip -4` gives an address in `100.64.0.0/10`. Layer 10 adds the Linux operator condition.
  - Skip reasons: `command absent`, `down`, `address invalid`. One line `skip tailscale <reason>; loopback only`.
  - Plan: read `tailscale serve status --json`. Another `TCP."8443"` target or a true `AllowFunnel` key for `:8443` gives exit 1 before any change.
  - Step 13: run `tailscale serve --bg --tcp 8443 tcp://127.0.0.1:8443` only when `TCP."8443"` is absent. Never run `tailscale funnel`.
  - The fake `tailscale` answers from test files and records its arguments. A `serve` call stores the rule.
  - `operator_linux.go` stays a stub until layer 10.
- Tests first: `tailscale_test.go`: `TestAddressRange`, `TestSkipReasons`, `TestServeRuleMatch`, `TestOtherTargetConflict`, `TestFunnelConflict`, `TestNoServeWhenRulePresent`, `TestNeverFunnel`.
- e2e: `DXAC50_TailscaleFake`.
- ACs: DX-AC50.
- Verify: `go test -count=1 ./internal/tailscale/... ./e2e/devx/cmd/faketailscale/... && GOOS=linux CGO_ENABLED=0 go vet ./internal/tailscale/... && make e2e-devx SUITE=Main RUN=DXAC50_ LAYER=3`. Expected: `ok`, then exit 0.
- Done when: SAG publishes `127.0.0.1:8443` only in each case.

## D3.3 Port check and idempotence

- Goal: Name the holder of `127.0.0.1:8443` before a change. Prove that a second bootstrap is idempotent.
- Owns: see the ownership table.
- Reads: PRD-devx 5.2, 6.
- Depends on: `D3.0`. Verify after `D3.1`, `D3.2`.
- Method:
  - First `podman ps`: a container that publishes 8443 is the holder.
  - Else `lsof -nP -iTCP:8443 -sTCP:LISTEN`. `gvproxy` is the holder for each Podman publish, so the `podman ps` check comes first.
  - Bootstrap step 3 and `service start` call the check. The container `sag` gives `ok port-8443`.
  - DX-AC3 adds a resource entry with a test secret to `<cfg>/sag.yaml` and issues the JWT with `podman exec -it sag sag jwt generate` on a pseudo-terminal.
  - `portcheck_linux.go` stays a stub until layer 10.
- Tests first: `portcheck_test.go`: `TestContainerSagIsOk`, `TestOtherContainerFails`, `TestProcessFails`, `TestFreePortContinues`, `TestLsofParse`, `TestGvproxyNotAHolderWhenPodmanPublishes`.
- e2e: `DXAC53_PortHolder`, `DXAC03_BootstrapIdempotent`.
- ACs: DX-AC3, DX-AC53.
- Verify: `go test -count=1 ./internal/portcheck/... ./internal/service/... && GOOS=linux CGO_ENABLED=0 go vet ./internal/portcheck/... && make e2e-devx SUITE=Main RUN='DXAC03_|DXAC53_' LAYER=3`. Expected: `ok`, then exit 0.
- Done when: a port conflict changes no file, secret, or container.

## D3.4 Integration and gate

- Goal: Prove the layer 3 criteria on the development machine and close the layer 3 gate.
- Owns: see the ownership table.
- Depends on: `D3.1` to `D3.3`.
- Steps: README section 6.1.
- ACs: DX-AC3, DX-AC6, DX-AC7, DX-AC8, DX-AC50, DX-AC53.
- Verify: `make test && make e2e-matrix LAYER=6 PGS="14 17 19" && make e2e-devx SUITE=Main LAYER=3 && make e2e-devx SUITE=Pins LAYER=3`. Expected: exit 0. The devx report shows each phase A DX-AC of layers 2 and 3 `PASS`.
- Done when: the tag `sag-devx-3` exists.

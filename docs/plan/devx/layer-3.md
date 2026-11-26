# Layer 3: Dependencies, Tailscale serve, port check

| Item | Value |
|---|---|
| Goal | Bootstrap installs or checks the pinned Podman, applies the Tailscale serve rule, and refuses a foreign holder of port 8443. A second run is idempotent. |
| ACs | DX-AC3, DX-AC5, DX-AC6, DX-AC7, DX-AC8, DX-AC50, DX-AC53 |
| PRD-devx sections | 3, 5.2 steps 3, 6, 13, 5.3, 5.4 (install and operator answers), 5.8, 6 (port check), 17.1 (test pins, fake `tailscale`) |
| Gate | `D3.4`: README section 6. The jobs `devx-pins` and `devx-install` pass. |
| Agents at once | 3. Start order: `D3.1`, `D3.2`, `D3.3` |
| Wall-clock | `D3.0` 1 h + parallel phase 4 h + `D3.4` 3 h = 8 h |

## File ownership

| Task | Owns |
|---|---|
| `D3.0` | `deploy/pins`, `deploy/embed.go`, `internal/bootstrap/steps.go` (calls to the new stubs), stubs of `internal/pins`, `internal/deps`, `internal/tailscale`, `internal/portcheck`, `.github/workflows/ci.yml` (skeletons of `devx-pins` and `devx-install`) |
| `D3.1` | `internal/pins/*`, `internal/deps/*`, `e2e/devx/dxac05_test.go` to `dxac08_test.go`, `e2e/devx/testdata/pins/*` |
| `D3.2` | `internal/tailscale/*`, `e2e/devx/cmd/faketailscale/*`, `e2e/devx/dxac50_test.go` |
| `D3.3` | `internal/portcheck/*`, `e2e/devx/dxac03_test.go`, `e2e/devx/dxac53_test.go` |
| `D3.4` | Every layer-3 file after `D3.1` to `D3.3` merge. `docs/reviews/devx-layer-3.md`. |

## D3.0 Contract

- Goal: Freeze the pin file, the dependency, Tailscale, and port check APIs, and their bootstrap call points.
- Owns: see the ownership table.
- Reads: PRD-devx 3, 5.2, 5.8, 6.
- Depends on: `D2.4`.
- Content:
  - `deploy/pins`: the two lines of PRD-devx 3. Read the SHA-256 of `podman-installer-macos-arm64.pkg` from the v5.5.1 release page.
  - `deploy/embed.go`: package `deployfiles`, `//go:embed pins`, `var Pins []byte`.
  - `deps.Plan(ctx, pins) (Plan, error)`, `deps.Apply(ctx, Plan) error`.
  - `tailscale.Detect(ctx) (State, error)`, `tailscale.Apply(ctx, State) error`. `State` holds the address, the skip reason, and the serve rule state.
  - `portcheck.Holder(ctx) (Holder, error)`. `Holder` is none, container `sag`, another container, or a process.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `make contract-check`. Expected: exit 0.
- Done when: `D3.1` to `D3.3` compile against the stubs.

## D3.1 Pins and dependency rule

- Goal: Apply the dependency rule of PRD-devx 5.3 on macOS and Ubuntu.
- Owns: see the ownership table.
- Reads: PRD-devx 3, 5.3, 5.4, 17.1.
- Depends on: `D3.0`.
- Method:
  - Download a `release` item to a new directory with mode 0700. Compute the SHA-256. On a difference, delete the file and exit 1.
  - macOS install: `sudo installer -pkg <file> -target /`. Then search `podman` again.
  - Ubuntu: parse `apt-cache policy podman`. Remove the epoch and the Debian suffix. Compare with the pin in the plan. Run `sudo apt-get install -y podman`. Check `newuidmap`.
  - Without a terminal, call `sudo -n`. A refusal by `sudo` gives exit 3.
  - macOS: run `podman machine init` only when no machine exists. Run `podman machine start` only when the machine is stopped.
  - `SAG_TEST_PINS` replaces the pins. `SAG_TEST_ABSENT` marks an item absent. `SAG_TEST_STOP_BEFORE_INSTALL` stops before the install command and prints the verified path.
  - Test pins: a `release` item from a local HTTP server and the apt package `hello`.
  - DX-AC5: `apt-get remove podman` keeps the rootless store. Prove that the image stays after the reinstall.
- Tests first: `pins_test.go`: `TestParseLine`, `TestSelectPlatform`, `TestLinuxWildcard`; `deps_test.go`: `TestAptVersionParse` (`4.9.3+ds1-1ubuntu0.1` gives `4.9.3`), `TestNewerWarns`, `TestOlderFailsInPlan`, `TestPreApprovalOtherVersionIgnored`, `TestChecksumMismatchDeletes`, `TestInstallRunsOnVerifiedPath`, `TestSudoNonInteractive`, `TestNoSecondMachine`.
- e2e: `DXAC05_PodmanAptInstall` (group `install`), `DXAC06_InstallApproval`, `DXAC07_ReleaseChecksum`, `DXAC08_VersionRule` (group `pins`).
- ACs: DX-AC5, DX-AC6, DX-AC7, DX-AC8.
- Verify: `go test -count=1 ./internal/pins/... ./internal/deps/...`. Expected: `ok`.
- Done when: bootstrap never changes an installed version.

## D3.2 Tailscale serve rule

- Goal: Detect Tailscale, add its address to the certificate, and apply the serve rule of PRD-devx 5.8.
- Owns: see the ownership table.
- Reads: PRD-devx 5.4, 5.7, 5.8, 17.1.
- Depends on: `D3.0`.
- Method:
  - Conditions: command in `PATH`, `tailscale ip -4` gives an address in `100.64.0.0/10`, and on Ubuntu `OperatorUser` of `tailscale debug prefs` is the current user.
  - Skip reasons: `command absent`, `down`, `address invalid`, `operator not set`. One line `skip tailscale <reason>; loopback only`.
  - Plan: read `tailscale serve status --json`. Another `TCP."8443"` target or a true `AllowFunnel` key for `:8443` gives exit 1 before any change.
  - Step 13: run `tailscale serve --bg --tcp 8443 tcp://127.0.0.1:8443` only when `TCP."8443"` is absent. Never run `tailscale funnel`.
  - Ubuntu operator step: prompt on a terminal. With approval, run `sudo tailscale set --operator=<user>` before the serve rule.
  - The fake `tailscale` answers from test files and records its arguments. A `serve` call stores the rule.
- Tests first: `tailscale_test.go`: `TestAddressRange`, `TestSkipReasons`, `TestServeRuleMatch`, `TestOtherTargetConflict`, `TestFunnelConflict`, `TestNoServeWhenRulePresent`, `TestNeverFunnel`, `TestOperatorPrompt`.
- e2e: `DXAC50_TailscaleFake`.
- ACs: DX-AC50.
- Verify: `go test -count=1 ./internal/tailscale/... ./e2e/devx/cmd/faketailscale/...`. Expected: `ok`.
- Done when: SAG publishes `127.0.0.1:8443` only in each case.

## D3.3 Port check and idempotence

- Goal: Name the holder of `127.0.0.1:8443` before a change. Prove that a second bootstrap is idempotent.
- Owns: see the ownership table.
- Reads: PRD-devx 5.2, 6.
- Depends on: `D3.0`.
- Method:
  - First `podman ps`: a container that publishes 8443 is the holder.
  - Else `lsof -nP -iTCP:8443 -sTCP:LISTEN` on macOS, `ss -ltnp` on Ubuntu.
  - Bootstrap step 3 and `service start` call the check. The container `sag` gives `ok port-8443`.
- Tests first: `portcheck_test.go`: `TestContainerSagIsOk`, `TestOtherContainerFails`, `TestProcessFails`, `TestFreePortContinues`, `TestSSParse`, `TestLsofParse`.
- e2e: `DXAC53_PortHolder`, `DXAC03_BootstrapIdempotent`. DX-AC3 adds a resource entry with a test secret to `<cfg>/sag.yaml` and issues the JWT with `podman exec -it sag sag jwt generate` on a pseudo-terminal.
- ACs: DX-AC3, DX-AC53.
- Verify: `go test -count=1 ./internal/portcheck/...`. Expected: `ok`.
- Done when: a port conflict changes no file, secret, or container.

## D3.4 Integration and gate

- Goal: Prove the layer 3 criteria in CI and close the layer 3 gate.
- Owns: see the ownership table.
- Depends on: `D3.1` to `D3.3`.
- Steps: README section 6. Set the devx gate layer of `ci.yml` to 3. Record that `apt-get install -y podman` installs `uidmap` on Ubuntu 24.04 (PRD-devx 19 step 2).
- ACs: DX-AC3, DX-AC5, DX-AC6, DX-AC7, DX-AC8, DX-AC50, DX-AC53.
- Verify: CI run. Expected: `ac35` `PASS`. `devx-report` shows each DX-AC of layers 2 and 3 `PASS`.
- Done when: the tag `sag-devx-3` exists.

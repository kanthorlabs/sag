# Layer 9 gate: devx

Phase A, macOS development machine. Gate commit `53a2b98`.

PRD-devx 18.1 not run; no macOS machine without Podman (ruling Q32).

## Tasks

| Task | Merge |
|---|---|
| `D9.0` contract: `Mac` suite, `User` stubs, `versions.go` | `90afc1e` |
| `D9.1` `User` run guard, snapshot, teardown, compare | `6d1d750` |
| `D9.2` macOS bootstrap, credential, pkg checksum (DX-AC34, DX-AC35, DX-AC38) | `3f51373` |
| `D9.3` real Claude Code and pi (DX-AC47, DX-AC48) | `4f79cab` |
| `D9.4` real Tailscale serve (DX-AC52) | `d28ba19` |
| `D9.5` removal of the development scripts | `4cd6dd4` |
| `D9.6` User run fixes, review fixes | `894096c`, `2070785`, `75e3f06`, `53a2b98` |

## Decisions

- D9-a: package `dxh` has the type `UserSnapshot` and the function `Snapshot(dir string) (UserSnapshot, error)`. Go does not allow a type and a function with one name in one package.
- D9-b: the `User` teardown removes only the exact SAG values: the three deny rules of PRD-devx 12.7, `env.NODE_EXTRA_CA_CERTS` equal to `<cfg>/ca.pem`, the two profile lines of PRD-devx 12.5. The suite fails and changes nothing when a SAG entry exists before the run. Each write follows the JSON write rule of PRD-devx 12.4.
- D9-c: DX-AC47 and DX-AC48 run each real harness child with the README 2.1 allow list. PATH holds the real program link directory first. DX-AC48 adds only `SHELL`.
- D9-d: DX-AC52 is the only test that runs the real `tailscale`. It runs `tailscale serve --tcp 8443` and its `off`.
- D9-e: while a `claude` process runs, the `User` suite exits 1 with a warning that names R24. `SAG_TEST_ACCEPT_CLAUDE_RUNNING=1` accepts the risk; the suite writes the warning to the report and continues.

## Findings fixed at integration

- B130 - status:FIXED - action:YES - DX-AC47 and DX-AC48 assertions could not fail - fix:exact version token; parsers for the connected line, the result object, the `permission_denials` entry, the pi MCP JSON and the pi tool result event, each with unit tests and mutants; `credential list --json`; pi run from a new shell - why:a test that cannot fail does not count
- B131 - status:FIXED - action:YES - teardown used wrong pi paths - fix:`agentsetup.PiPaths`, `ClaudeCodePaths`, `ShellProfile` - why:PRD-devx 15 step 10
- B132 - status:FIXED - action:YES - `settings.json` rewrite replaced a symbolic link and escaped HTML - fix:write the link target, keep mode, `SetEscapeHTML(false)` - why:PRD-devx 12.4
- B133 - status:FIXED - action:YES - deny rules matched by prefix - fix:exact match of the three rules - why:only SAG entries may change
- B134 - status:FIXED - action:YES - every `NODE_EXTRA_CA_CERTS` value removed - fix:remove only `<cfg>/ca.pem` and the two exact profile lines - why:another value belongs to the operator
- B135 - status:FIXED - action:YES - no check for SAG entries before the run - fix:fail before the group `harness` - why:stop on an unsafe state
- B136 - status:FIXED - action:YES - the real program link directory was not in PATH - fix:put it first in the child PATH - why:README 2.1
- B137, B138 - status:FIXED - action:YES - DX-AC38 wrong-hash case ran without the stop hook; weak DX-AC34 checks - fix:stop hook in both cases; Podman path, version, size and mtime unchanged - why:DX-AC34 and DX-AC38 texts
- B139 - status:FIXED - action:YES - the real serve rule stayed after a failure - fix:cleanup runs `tailscale serve --tcp 8443 off` when a TCP 8443 entry exists - why:the machine must be the same after the run
- B140 - status:FIXED - action:YES - messages could print the Tailscale IP - fix:mask each error text - why:never print the Tailscale IP
- B141 - status:FIXED - action:YES - shell profile rewrite replaced a symbolic link - fix:link-safe write - why:PRD-devx 12.4
- B142 - status:FIXED - action:YES - DX-AC47 and DX-AC48 passed the full run environment - fix:`dxh.UserChildEnv` - why:README 2.1
- B143 - status:FIXED - action:YES - mask with an empty address broke the text - fix:return the text unchanged - why:readable errors

## Findings from the User runs

- B144 - status:FIXED - action:YES - DX-AC39 default path case was never written - fix:real `agent setup claude-code` without `CLAUDE_CONFIG_DIR`; check the entry, the mode, the skill and the JWT `sub` - why:a skip is not a pass
- B145 - status:FIXED - action:YES - DX-AC47 asked Claude Code before the resource was live - fix:wait for LaunchAgent idle and for `/mcp` to list the resource - why:the criterion tests Claude Code, not restart timing
- B146 - status:FIXED - action:YES - the variadic `--allowedTools` took the prompt - fix:prompt directly after `-p`, `--allowedTools` last - why:DX-AC47 text
- B147 - status:FIXED - action:YES - the DX-AC47 resource pointed at a closed port - fix:canary PostgreSQL DSN - why:PRD 9.1: a resource that fails preflight is not served
- S102 - status:FIXED - action:YES - no way to run with an open Claude Code session - fix:D9-e - why:operator ruling
- S103 - status:FIXED - action:YES - DX-AC48 hid a pi model error - fix:name the model error; 16 MiB line buffer - why:a failure must name its cause
- S104 - status:FIXED - action:YES - the default pi model of the operator failed - fix:the operator set a working model - why:DX-AC48 needs a model that calls the bash tool

## Independent review

The review covered `9af72bb..75e3f06`.

- B148 - status:FIXED - action:YES - teardown dropped a failed backup remove - fix:return read and remove errors - why:a SAG entry left behind must be reported
- S105 - status:FIXED - action:YES - DX-AC48-B dropped the shell exit and stderr - fix:print them on failure - why:a failure must name its cause

## Gate 1 on `6d1d750`

All checks exit 0. Main run 1 failed DX-AC10: the key scan of `$HOME` passed its 5-minute limit. Main run 2 passed.

- S101 - status:OPEN - action:YES - DX-AC10 key scan timeout in one run - fix:none in code; a second timeout opens a blocker - why:the cause is the walk time of `$HOME`, not the code under test

## Final gate on `53a2b98`

```
gofmt -l internal e2e cmd                    → empty
go vet ./... and -tags devxe2e               → exit 0
go test -tags devxtest ./internal/...        → exit 0
make contract-check                          → exit 0
make test                                    → exit 0
make e2e-devx-unit LAYER=9                   → exit 0
make e2e-devx SUITE=Mac LAYER=9              → 11 pass events, no fail, no skip
make e2e-devx SUITE=User LAYER=9             → 11 pass events, no fail, no skip
make e2e-devx SUITE=Main LAYER=9             → two runs, 140 pass events each, no fail, no skip
make e2e-devx SUITE=Pins LAYER=9             → exit 0, 6 pass events, report over all suites at 53a2b98
make e2e-matrix LAYER=6 PGS="14 17 19"       → exit 0
```

The Mac, User and Main runs exit 2 only because the report of an earlier suite named an older commit. The Pins run wrote the report over all five suites at `53a2b98`. Gate layer 9 phase A: PASS. DX-AC10 took 164 s and 164 s.

```
PASS DX_Harness_Skeleton  PASS DXAC01  PASS DXAC02  PASS DXAC03  PASS DXAC04
PASS DXAC06  PASS DXAC07  PASS DXAC08  PASS DXAC09  PASS DXAC10
PASS DXAC11  PASS DXAC12  PASS DXAC13  PASS DXAC14  PASS DXAC15
PASS DXAC16  PASS DXAC17  PASS DXAC18  PASS DXAC19  PASS DXAC20
PASS DXAC21  PASS DXAC22  PASS DXAC23  PASS DXAC24  PASS DXAC25
PASS DXAC26  PASS DXAC27  PASS DXAC28  PASS DXAC29  PASS DXAC30
PASS DXAC31  PASS DXAC32  PASS DXAC33  PASS DXAC34  PASS DXAC35
PASS DXAC36  PASS DXAC37  PASS DXAC38  PASS DXAC39  PASS DXAC40
PASS DXAC41  PASS DXAC42  PASS DXAC43  PASS DXAC44  PASS DXAC45
PASS DXAC46  PASS DXAC47  PASS DXAC48  PASS DXAC49  PASS DXAC50
PASS DXAC51  PASS DXAC52  PASS DXAC53
```

DX-AC05 is phase B.

After the runs:

- `~/.config/sag`, the LaunchAgent and each container with label `sag.devx-e2e=1` are absent.
- The harness files of the operator hold no SAG entry: `~/.claude.json`, `~/.claude/settings.json`, `~/.pi/agent/mcp.json`, the shell profile. The SAG skills and `sag-guard.ts` are absent. The compare passed and removed its copy.
- `tailscale serve status --json` has the same hash before and after the `Mac` suite.
- The reports and logs hold no JWT and no Tailscale address.

## Disk

```
Images 98, reclaimable 11.84GB; Local Volumes 14, reclaimable 667.6MB
/var free after the gate: 11879152 KiB
```

## Env

```
OS          macOS 26.3.1
Podman      5.5.1
Claude Code 2.1.332
pi          1.6.1
Tailscale   1.98.5
```

## Open items

- S101 - status:OPEN - action:YES - see gate 1.
- S96, S98 - status:OPEN - action:NO - verify search wording of D9.0; the blocked pi tool result text is not documented by pi 1.6.1, and DX-AC48 passed with the real text - fix:none - why:no behavior change

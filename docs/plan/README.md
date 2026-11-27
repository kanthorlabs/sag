# SAG v1 implementation plan

| Field | Value |
|---|---|
| Owner | Ulrich |
| Author | Aelita |
| Date | 2026-11-25 |
| Source of truth | `docs/PRD.md`, revision 5 |
| Scope | v1: PostgreSQL adapter, Podman container, MCP Streamable HTTP over TLS, JWT (PRD section 11.2), test-only image `sag-conformance` |

The PRD wins on any conflict with this plan.

## Files

| File | Content |
|---|---|
| `README.md` | Rules for all agents, task graph, worktree and branch model, claim and merge procedure, layer gate, estimate, risks |
| `foundation.md` | Toolchain, dependencies, module layout, core boundary, fixed design decisions, critical security tasks |
| `testing.md` | Test tiers, PostgreSQL in Podman, e2e harness, AC report, CI, AC map |
| `dev-setup.md` | macOS development setup and Claude Code configuration |
| `layer-1.md` to `layer-6.md` | Goal, ACs, gate, and tasks of each layer |
| `v1.1-mysql.md` | Required items of the MySQL release |

## 1. Rules for all agents

- Call the owner "Ulrich". Call the author "Aelita".
- Read `foundation.md`, `testing.md`, and the file of your layer before you claim a task.
- Write each test first. Run it and see it fail. Then write the code.
- Write no code comments. Names and types carry the meaning.
- Give each package one responsibility. Do not fold unrelated logic into one package.
- Keep the core free of SQL. Only `internal/sql` and `internal/adapter/postgres` know SQL.
- Change only the files that your task owns. If you need a change in another file, stop and ask the contract owner of the layer.
- Do not change `go.mod` or `go.sum`. Only contract tasks and integration tasks change them.
- Do not run `go mod tidy`. Only the integration task runs it.
- Do not weaken a security control to make a test pass. Report the conflict to Ulrich.
- Write technical text in ASD-STE100 style: max 20 words per sentence in instructions, 25 in descriptions, imperative steps, active voice, simple tenses, no contractions.
- Write review findings in this format: `<B1/S1> - status:<FIXED/OPEN> - action:<YES/NO> - <name> - <description> - fix:<...> - why:<...>`.

An AC is reached only when its e2e test passes (`testing.md` section 3). A unit test alone does not reach an AC.

## 2. Layer rule

- The build runs in 6 layers. Each layer ends with a working, verified product.
- No task of layer N+1 starts before the gate of layer N closes.
- Each layer starts with a contract task. It freezes the Go interfaces, types, file layout, and intra-package function signatures of the layer. It hands each stub file to one task.
- Each layer ends with an integration task. It runs the e2e matrix and the layer gate.
- All other tasks of a layer run in parallel after the contract task merges.
- Release only after layer 6.

Track tasks: `X.1` (CI skeleton) and `X.2` (development stack) belong to layer 1. No AC depends on them. They start after `T1.0` and merge before `T1.10`. The layer 1 gate reviews them. They do not break the layer rule, because they own no file of a later layer.

## 3. Task graph

Notation: `A -> B` means that B starts after A merges. `B (verify after C)` means that B starts after the contract, but its verify command needs C merged.

```
Bootstrap
  B.1 -> T1.0

Layer 1 (AC1-AC4)
  T1.0 -> T1.1 T1.2 T1.3 T1.4 T1.5 T1.6 T1.7 T1.8 T1.9 X.1 X.2
  T1.5 (verify after T1.6)
  T1.7 (verify after T1.6)
  X.2  (verify after T1.9)
  all  -> T1.10 (integration, gate)

Layer 2 (AC5-AC12)
  T1.10 -> T2.0
  T2.0 -> T2.1 T2.2 T2.3 T2.4 T2.5 T2.6 T2.7
  T2.3 (verify after T2.2, T2.4)
  T2.6 (verify after T2.1)
  T2.7 (verify after T2.1, T2.2, T2.6)
  all  -> T2.8 (integration, gate)

Layer 3 (AC13-AC19)
  T2.8 -> T3.0
  T3.0 -> T3.1 T3.2 T3.3 T3.4 T3.5 T3.6
  all  -> T3.7 (integration, gate)

Layer 4 (AC20-AC27)
  T3.7 -> T4.0
  T4.0 -> T4.1 T4.2 T4.3 T4.4
  all  -> T4.5 (integration, gate)

Layer 5 (AC28-AC31)
  T4.5 -> T5.0
  T5.0 -> T5.1 T5.2 T5.3
  all  -> T5.4 (integration, gate)

Layer 6 (AC32-AC35)
  T5.4 -> T6.0
  T6.0 -> T6.1 T6.2 T6.3
  all  -> T6.4 (integration, gate, CI run)
```

Available parallelism is the number of tasks that the graph allows at the same time. The pool rule (section 3.1) caps the agents at once at 3.

| Layer | Tasks | Available parallelism | Agents at once | Critical path | Critical path length | Wall-clock with pool of 3 |
|---|---|---|---|---|---|---|
| 1 | 13 (`T1.0` to `T1.10`, `X.1`, `X.2`) | 11 | 3 | `T1.0` -> `T1.6` -> `T1.7` verify -> `T1.10` | 8 h | 11 h |
| 2 | 9 (`T2.0` to `T2.8`) | 7 | 3 | `T2.0` -> `T2.2` -> `T2.3` verify -> `T2.8` | 8 h | 10 h |
| 3 | 8 (`T3.0` to `T3.7`) | 6 | 3 | `T3.0` -> `T3.1` -> `T3.7` | 9 h | 9 h |
| 4 | 6 (`T4.0` to `T4.5`) | 4 | 3 | `T4.0` -> `T4.1` -> `T4.5` | 6.5 h | 7.5 h |
| 5 | 5 (`T5.0` to `T5.4`) | 3 | 3 | `T5.0` -> `T5.2` -> `T5.4` | 4.5 h | 4.5 h |
| 6 | 5 (`T6.0` to `T6.4`) | 3 | 3 | `T6.0` -> `T6.3` -> `T6.4` (CI run) | 5.5 h | 5.5 h |
| Total | 46 | | | | 41.5 h | 47.5 h |

### 3.1 Scheduling rule: pool of 3 agents

Ulrich's rule (`CLAUDE.local.md`): run a pool of a maximum of 3 sub-agents at the same time. When one sub-agent finishes, start the next ready task in a new sub-agent. Keep this limit until Ulrich explicitly increases it.

- The coordinator owns the pool. Each task runs in one sub-agent.
- The review sub-agent of a gate counts as one agent of the pool.
- A task is ready when each of its start dependencies is merged (section 5 step 2).
- If more than one task is ready, start the task with the longest remaining path to the integration task first. Then start the task with the longest estimate.
- A task with a "verify after" dependency can start before that dependency merges. It waits for the merge only before its verify command.
- No task of layer N+1 is ready before the gate of layer N closes.

Task estimates and start order with a pool of 3 (Aelita estimate, hours):

| Layer | Contract | Parallel tasks in start order | Parallel phase | Integration and gate |
|---|---|---|---|---|
| 1 | `T1.0` 1 | `T1.6` 2.5, `T1.7` 3.5, `T1.3` 2, `T1.4` 2, `T1.9` 2, `T1.1` 1.5, `T1.2` 1.5, `T1.5` 1.5, `T1.8` 1, `X.2` 1, `X.1` 0.5 | 6.5 | `T1.10` 3.5 |
| 2 | `T2.0` 1 | `T2.2` 3, `T2.4` 2, `T2.1` 2.5, `T2.3` 3, `T2.6` 2, `T2.5` 1.5, `T2.7` 1.5 | 6 | `T2.8` 3 |
| 3 | `T3.0` 1 | `T3.1` 5, `T3.2` 2, `T3.4` 2, `T3.5` 2, `T3.3` 1.5, `T3.6` 1 | 5 | `T3.7` 3 |
| 4 | `T4.0` 1 | `T4.1` 2.5, `T4.3` 2, `T4.4` 2, `T4.2` 1.5 | 3.5 | `T4.5` 3 |
| 5 | `T5.0` 0.5 | `T5.2` 2, `T5.1` 1, `T5.3` 1 | 2 | `T5.4` 2 |
| 6 | `T6.0` 0.5 | `T6.3` 2, `T6.1` 1.5, `T6.2` 1.5 | 2 | `T6.4` 3 |

## 4. Worktree and branch model

| Item | Value |
|---|---|
| Integration branch | `feat/v1-postgres`, created from `main` |
| Integration worktree | `.dev/integration` |
| Task branch | `feat/v1-postgres--<task-id>`, for example `feat/v1-postgres--T2.3`, created from `feat/v1-postgres` |
| Task worktree | `.dev/<task-id>`, for example `.dev/T2.3` |
| Claims | `.dev/claims/<task-id>/` |
| Merge lock | `.dev/claims/merge.lock/` |
| Local state | `.dev/local/` (certificates, review diffs) |
| Layer tag | `sag-layer-<N>` on `feat/v1-postgres` after the gate of layer N closes |

`.dev/` is in `.git/info/exclude`. Task agents commit on their task branch. The integration agent commits on `feat/v1-postgres`. Only Ulrich pushes. The first push is in `T6.4`.

### 4.1 Bootstrap (B.1)

Run these commands one time at the repository root:

```sh
git add docs && git commit -m "docs: PRD revision 5 and implementation plan"
echo '.dev/' >> .git/info/exclude
git branch feat/v1-postgres main
git tag sag-layer-0 main
git worktree add .dev/integration feat/v1-postgres
mkdir -p .dev/claims .dev/local
```

## 5. Claim a task

1. Read the task in the layer file.
2. Check each start dependency: `.dev/claims/<dep>/merged` exists.
3. Claim the task: `mkdir .dev/claims/<task-id>`. If `mkdir` fails, another agent owns the task. Pick another task.
4. Write your agent name and the UTC time to `.dev/claims/<task-id>/owner`.
5. Create the worktree: `git -C .dev/integration worktree add -b feat/v1-postgres--<task-id> ../<task-id> feat/v1-postgres`.
6. Work only in `.dev/<task-id>`.
7. If you stop before the task is done, remove `.dev/claims/<task-id>/` and tell the coordinator.

## 6. Merge a task

Do these steps when the verify command of the task passes:

1. Commit on the task branch.
2. Acquire the lock: `mkdir .dev/claims/merge.lock`. If it fails, wait 30 s and try again.
3. In the task worktree, run `git merge --no-edit feat/v1-postgres`.
4. Run `make test` and the verify command of the task.
5. If step 3 or step 4 fails, release the lock with `rmdir .dev/claims/merge.lock`. Fix the cause, then start again at step 1.
6. In `.dev/integration`, run `git merge --ff-only feat/v1-postgres--<task-id>`.
7. Run `touch .dev/claims/<task-id>/merged`.
8. Release the lock: `rmdir .dev/claims/merge.lock`.
9. Remove the worktree: `git -C .dev/integration worktree remove ../<task-id>`.

## 7. Integration task and layer gate

The integration task of layer N owns every file of layer N after all other tasks of the layer merge.

1. Check that each task of the layer has `.dev/claims/<task-id>/merged`.
2. In `.dev/integration`, run `go mod tidy`. Check that no module version changes.
3. Run `make e2e-matrix LAYER=<N> PGS="14 17 19"`. It runs `make test-integration` for each major before the e2e suite.
4. If `acreport` exits 1, fix the cause and run step 3 again.
5. Write the diff: `git diff sag-layer-<N-1> > ../local/layer-<N>.diff`.
6. Spawn one review sub-agent with `model: fable` and `effort: high`. Give it the diff, the PRD sections of the layer, `e2e/report/ac-report.md`, and the layer file. Ask for correctness and security findings in Ulrich's format.
7. Store the review output in `docs/reviews/layer-<N>.md`.
8. Fix every `action:YES` item. Add a test for each fixed defect.
9. Run step 3 again.
10. Update `docs/reviews/layer-<N>.md`. Each `action:YES` item has `status:FIXED`. Each `action:NO` item keeps `status:OPEN` with its `why`. Append the final `ac-report.md` table.
11. Commit on `feat/v1-postgres`. Tag `sag-layer-<N>`.
12. Tell the coordinator that the gate of layer N is closed.

The gate closes when step 11 is complete and every AC with `layer <= N` is `PASS`.

## 8. Estimate

Aelita estimate, not a commitment:

| Item | Value |
|---|---|
| Wall-clock with the pool of 3 agents | about 48 h (section 3 table) |
| Wall-clock without the pool limit | about 42 h (sum of the critical path lengths) |
| Agent-hours | about 62 h |
| Serial floor | about 22 h: 6 contract tasks, 6 integration tasks, 6 review gates |
| Serial plan with one agent | about 52 h |

The pool of 3 adds about 6 h, in layers 1, 2, and 4. The 6 serial gates limit the gain from more agents. Each e2e matrix run takes about 8 minutes. AC11 uses `preflight.recheck_interval: 10s`. The failed-resource retry stays at 60 s (PRD section 9.1). Layer 6 needs a push by Ulrich for the CI run.

## 9. Risks

- The field allowlist for pg_query nodes (`T3.1`) is the longest task. A missing field rejects valid queries.
- The cgo build of libpg_query adds minutes to each image build. The Containerfile uses a build cache mount.
- 3 parallel agents share one Podman machine. Integration tests and e2e runs can compete for CPU and memory.
- A contract that misses a signature blocks parallel tasks. A contract amendment goes through the contract owner and the merge lock.
- The go-sdk `auth` bridge and stateless mode are not tested with Claude Code yet. AC31 is the first proof.
- `SET LOCAL standard_conforming_strings = on` on PostgreSQL 19 is not tested (PRD section 9.3).
- Rootless Podman secrets with `uid=65532` on the macOS Podman machine are not tested.
- AC31 needs a Claude Code login. AC35 needs a remote repository and a push.

Cut order if time runs short. No cut removes a security control or its AC:

1. Run AC31 by hand one time and record the output. Keep the e2e subtest.
2. Gate layers 1 to 5 on PostgreSQL 17 only. Run 14 and 19 only at layer 6.
3. Drop task `X.2`. Use the commands in `dev-setup.md` by hand.
4. Drop the `linux/arm64` and `darwin/amd64` builds. Keep `linux/amd64` and `darwin/arm64`.
5. Defer the operator name report (AC12). It is report only.
6. Defer column redaction (AC25). PRD section 10.4 states that it is not a security boundary.
7. Defer `max_cell_bytes`. No AC depends on it.
8. Defer `sag extension revoke`. The operator deletes the record file.

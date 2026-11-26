# Layer 11: Removal of the development scripts

| Item | Value |
|---|---|
| Goal | The devx commands replace the development scripts. The documents describe the devx flow. |
| ACs | None. Each DX-AC stays `PASS`. |
| PRD-devx sections | 14, 16, 19 step 9 |
| Gate | `D11.3`: README section 6. The final merged report holds 53 `PASS` rows. |
| Agents at once | 2. Start order: `D11.1`, `D11.2` |
| Wall-clock | `D11.0` 0.5 h + parallel phase 1.5 h + `D11.3` 2 h = 4 h |

## File ownership

| Task | Owns |
|---|---|
| `D11.0` | The removal list in this file |
| `D11.1` | `scripts/dev-up.sh`, `scripts/dev-down.sh`, `Makefile` (targets `dev-up` and `dev-down`), `docs/plan/dev-setup.md` |
| `D11.2` | `docs/PRD.md` |
| `D11.3` | Every layer-11 file after `D11.1` and `D11.2` merge. `docs/reviews/devx-layer-11.md`. |

## D11.0 Contract

- Goal: Freeze the removal list and confirm that no code depends on it.
- Owns: see the ownership table.
- Reads: PRD-devx 14.
- Depends on: `D10.4`.
- Content: the removal list is `scripts/dev-up.sh`, `scripts/dev-down.sh`, and the `dev-up` and `dev-down` targets of `Makefile`. `deploy/dev/sag.yaml` stays.
- Tests first: none. The contract has no logic.
- ACs: none.
- Verify: `grep -rn 'dev-up\|dev-down\|sag-dev-pg' --include='*.go' --include='*.yml' --include=Makefile .`. Expected: only the `Makefile` targets.
- Done when: the search result is recorded in `docs/reviews/devx-layer-11.md`.

## D11.1 Script removal and development setup

- Goal: Remove the scripts and their targets. Rewrite the development setup.
- Owns: see the ownership table.
- Reads: PRD-devx 4.4, 14.
- Depends on: `D11.0`.
- Method: `docs/plan/dev-setup.md` holds the steps of PRD-devx 4.4, one `sag credential create`, and one `sag agent setup`. It names no JWT in a command argument and no mkcert step.
- Tests first: none. The task changes no Go code.
- ACs: none.
- Verify: `make test`. Expected: exit 0.
- Done when: `make dev-up` gives `No rule to make target`.

## D11.2 PRD changes

- Goal: Apply the rows of PRD-devx 16 to `docs/PRD.md`.
- Owns: see the ownership table.
- Reads: PRD-devx 16; `docs/PRD.md`.
- Depends on: `D11.0`.
- Method: change each named PRD section as its row states. Add the revision line. Show the diff to Ulrich before the merge. Ulrich approves each PRD change.
- Tests first: none. The task changes no code.
- ACs: none.
- Verify: Ulrich approves the diff.
- Done when: each row of PRD-devx 16 has a matching change in `docs/PRD.md`.

## D11.3 Integration and final gate

- Goal: Prove that the removal breaks no criterion, and close the devx plan.
- Owns: see the ownership table.
- Depends on: `D11.1`, `D11.2`.
- Steps: README section 6 steps 1 to 4. Set the devx gate layer of `ci.yml` to 11. Merge the CI report with the dev-machine report of `D10.4`. The code under test of that report is unchanged in this layer.
- ACs: DX-AC1 to DX-AC53 (regression).
- Verify: the merged report. Expected: 53 DX-AC rows `PASS`. `ac35` `PASS`.
- Done when: the tag `sag-devx-11` exists. The merge of `feat/devx` into `main` is a decision of Ulrich.

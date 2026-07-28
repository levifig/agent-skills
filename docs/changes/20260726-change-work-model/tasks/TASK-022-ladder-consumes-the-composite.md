---
change: change-work-model
id: TASK-022
title: The state ladder consumes the composite tier
blocked-by:
  - TASK-021
---

# TASK-022 — The state ladder consumes the composite tier

## Objective

Close C4-3: `deriveChangeState` consults only `evaluateChangeNode` before promoting a fresh receipt to `verified`, so a cohort member with banned task frontmatter or a conversion violation can display `verified` while `check` and the gate reject it. The round-3 lanes' file boundaries left `change_state.go` unowned — the M1/M2 display-disagreement class recurring one tier up. After this task, no surface can display `verified` where the gate would refuse.

## Scope boundaries

**In:** `deriveChangeState` in `internal/cli/change_state.go`; fixtures in `internal/cli/change_state_test.go`.

**Out:** The gate itself (TASK-021 lands first; consume its composite, do not re-implement). The ladder's other rungs (`captured`/`shaped`/`executable`/`executing`/`complete` semantics unchanged — this task guards the `verified` promotion only). Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`.

## Context pointers

- Round-4 board finding C4-3; TASK-011's agreement doctrine this completes: "the state the gate reads must not be displayable when the gate would refuse it."

## Acquisition

```bash
loaf journal log "skill(implement): TASK-022 — ladder consumes the composite"
```

## Steps

- [ ] `verified` requires the same structural composite the gate applies (TASK-021's lineage-inclusive report) to be clean, in addition to the fresh all-passing HEAD receipt; a structurally rejected member derives its state as if unverified.
- [ ] Fixture: a targeted change with banned task frontmatter, flips, and a fresh committed passing receipt — `list`, `show`, and `show --json` all report a non-`verified` state while `check` shows the violation; removing the violation flips all surfaces to `verified` together.
- [ ] The ladder-agreement fixture (`TestChangeListShowStateAgreementAcrossLadder`) extends to cover the structurally-rejected-receipt rung.

## Verification

- `go test ./internal/cli/ -run 'ChangeState|ChangeList|ChangeShow'` green including the new rung; `loaf change list` on this repo still reports `change-work-model` as `complete` and the sweep carrier as `captured`.

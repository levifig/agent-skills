---
change: change-work-model
id: TASK-027
title: Structural load errors surface instead of silently demoting
---

# TASK-027 — Structural load errors surface instead of silently demoting

## Objective

Close C5-5: `changeStructurallyCleanForState` converts load errors to `false`, so an unreadable machine file silently demotes a verified change's displayed state with no trace. Fail-closed stays (the conservative state is correct); silence goes.

## Scope boundaries

**In:** the error path of the state guard in `internal/cli/change_state.go` (return the reason, not just `false`); warning plumbing in `internal/cli/change_tasks.go` (`show`) and `internal/cli/change_list.go` so the demotion reason appears as a `warn:` line and in the JSON `warnings`; fixtures in `internal/cli/change_state_test.go`.

**Out:** The demotion itself (correct direction, unchanged). `check`'s reporting (already surfaces its own errors). The do-not-touch list: `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`.

## Context pointers

- Round-5 board C5-5; TASK-011's agreement doctrine — surfaces that disagree silently are the defect class this ladder exists to end.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-027 — structural load errors surface"
```

## Steps

- [x] The state guard returns its failure reason; `show` and `list` print `warn: structural evaluation failed: <reason>` for the affected member and include it in JSON warnings, while the displayed state stays conservative.
- [x] Fixture: an unreadable/corrupt machine file on one member demotes its state and surfaces the warning on both `show` and `list`; other members unaffected.

## Verification

- `go test ./internal/cli/ -run 'ChangeState|ChangeShow|ChangeList' -count=1` green including the warning fixture.

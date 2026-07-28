---
change: change-work-model
id: TASK-029
title: Every silent demotion surfaces, and the fixture that pretended to check does
---

# TASK-029 — Every silent demotion surfaces

## Objective

Close O6-3 and O6-5: two load-error paths beside TASK-027's fix still demote silently — `changeFolderExecuted`'s error folds into the executable rung and `receiptErr` is captured but never surfaced (`change_state.go:40-45`, `change_list.go:78`), so a truncated committed receipt shows `executing` with no warning while the gate errors loudly on the same file. And the TASK-027 fixture carries an empty conditional (`change_state_test.go:412-413`) that reads as an assertion and can never fail.

## Scope boundaries

**In:** the two error paths in `internal/cli/change_state.go` plus the discard in `internal/cli/change_list.go`; the `check --json` state-field demotion noted on the board (same warning treatment); the empty conditional in `internal/cli/change_state_test.go` made into a real assertion; fixtures.

**Out:** The demotion direction (conservative states stand). Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`.

## Context pointers

- Round-6 board O6-3, O6-5, and the `check --json` note; TASK-027's warning plumbing (extend, do not re-invent).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-029 — every silent demotion surfaces"
```

## Steps

- [x] Provenance and receipt errors on the state path surface through the same warning plumbing TASK-027 built, on `show`, `list`, and `check --json`; displayed states stay conservative.
- [x] Fixture: a truncated committed receipt yields the conservative state plus a visible warning on all three surfaces.
- [x] The empty conditional becomes a real assertion (state line present and readable), and the surrounding fixture's claims match what it checks.

## Verification

- `go test ./internal/cli/ -run 'ChangeState|ChangeList|ChangeShow|ChangeCheck' -count=1` green including the new warning fixture.

---
change: change-work-model
id: TASK-011
title: One derived-state ladder across every surface
blocked-by:
  - TASK-007
relates-to:
  - TASK-003
---

# TASK-011 — One derived-state ladder across every surface

## Objective

Give the change model a single derived-state derivation shared by every surface, completing the ladder the shape declares. Today `loaf change list` and `loaf change show` disagree about the same change (`executed` versus `executable`), `show --json` carries no state at all, and the two states the document names last — `complete` and `verified` — exist nowhere in the code, so the one state the release gate reads cannot be displayed.

## Scope boundaries

**In:** one exported derivation used by `runChangeListUnits`, `runChangeShow`, `runChangeCheck` and both JSON payloads; a `state` field on `change show --json`; the Observable Workflow paragraph in `shape.md`.

**Out:** Gate behaviour — the gate keeps computing its own predicates; this task makes the *display* agree with them, and must not become the gate's input path. No progress percentages, no burn-down (Rabbit Holes: check is not a workflow engine).

## Context pointers

- `docs/changes/20260726-change-work-model/shape.md` — Observable Workflow's ladder; Decision 15 (criteria gate, tasks are the route).
- `reports/20260727-201619-review-codex-r3.html` finding 1.3 — the ladder split was accepted there as the fix for untargeted changes having no coherent done state.
- Review board findings M1 and M2, including the decided disposition: build the ladder, keep `executable`, amend the doc to name it.
- `internal/cli/change_list.go:75-88` and `internal/cli/change_tasks.go:210-216` — the two disagreeing derivations.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-011 — single derived-state ladder"
```

One clarification settled at review, so no one chases an impossible predicate: shape.md reads "checkboxes done **or descoped** → complete", but descoping is only legible through verification (Decision 15 — an unchecked task on a verified change is descoped work). Mechanically, `complete` means every checkbox checked; the "or descoped" clause belongs to the verified path, and the doc amendment should make that explicit.

## Steps

- [x] Single derivation function returning the ladder: `captured` → `shaped` → `executable` → `executing` → `complete`, plus `verified` for changes declaring a `target_release`.
- [x] `complete` = every task checkbox checked. `verified` = cohort member with a fresh receipt whose criteria all passed — reuse the predicate TASK-007 introduces rather than restating it.
- [x] `list`, `show`, `check` and both JSON payloads consume that one function; `change show --json` gains a `state` field so the text and JSON surfaces cannot drift again.
- [x] Amend shape.md's Observable Workflow to name `executable` in the ladder and to state that `complete` is all-boxes-checked while descoping resolves only through verification. Documentation follows shipped behaviour, never ahead of it.
- [x] Fixture asserting `list` and `show` report identical state for the same fixture change across every rung of the ladder.

## Verification

- `go test ./internal/cli/ -run 'ChangeList|ChangeShow|ChangeCheck'` green, including the agreement fixture.
- `loaf change list` and `loaf change show` agree on `change-work-model`, and the sweep carrier still reads `captured`.
- `loaf change check` reports zero violations on the amended `shape.md`.

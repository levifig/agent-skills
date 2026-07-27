---
change: change-work-model
id: TASK-009
title: Flip grammar matches the documented transition
relates-to:
  - TASK-004
---

# TASK-009 — Flip grammar matches the documented transition

## Objective

Make flip-grade provenance detect an actual `- [ ]`→`- [x]` transition outside code fences, as the Planning Contract's provenance-precision section specifies, and ship the negative fixtures V1 promises. Today the detector pairs any removed unchecked box with any added checked box anywhere in the same file, and it never tracks fence state.

## Scope boundaries

**In:** `diffContainsCheckboxFlip` and its regexes in `internal/cli/change_provenance.go`; the fixtures.

**Out:** The path grade (unchanged — a commit still needs a companion path outside `docs/changes/` entirely). Receipt semantics (`TASK-007`). Derived state names (`TASK-011`).

## Context pointers

- `docs/changes/20260726-change-work-model/shape.md` — Planning Contract, "Provenance precision"; V1's "each with a negative fixture".
- Review board findings M3 (fences) and M4 (pairing), including the decided disposition: tighten the detector rather than restate the grammar.
- `internal/cli/change_provenance.go:108-133` — the current approximation, whose own comment concedes fenced regions are not handled.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-009 — flip grammar exactness"
```

Note the accepted failure direction before choosing a heuristic: shape.md's Risks section declares that work which does not register as executed is the safe error, because it blocks stable until a one-commit remedy. A commit that flips a box *and* rewords its label in the same edit may therefore stop counting — that is acceptable; a reword alone counting as execution is not.

## Steps

- [x] Track fence state across the patch so a checkbox inside a fenced block is a non-event in both directions, rather than only skipping the fence marker lines themselves.
- [x] Require the removed-unchecked and added-checked lines to occupy the same hunk and carry the same normalized label text, so a transition is detected rather than inferred from unrelated additions and deletions.
- [x] Negative fixtures, one per non-event named in V1: reverse flip (`- [x]`→`- [ ]`), an added already-unchecked box, a whitespace-only or title-only edit, a fenced-block flip, and a delete-unchecked-plus-add-checked pair that shares no label.
- [x] Positive fixtures still pass: a plain flip, a flip in a commit that also touches unrelated task-file prose, and a squash-collapsed batch whose diff carries several flips.

## Verification

- `go test ./internal/cli/ -run 'Provenance|ReleaseCohortGate'` green with every fixture above.
- `loaf change list` still reports `change-work-model` as flip-executed at HEAD — the tightened detector must not un-execute this change's real history.

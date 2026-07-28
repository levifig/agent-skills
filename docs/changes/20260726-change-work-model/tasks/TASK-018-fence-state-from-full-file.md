---
change: change-work-model
id: TASK-018
title: Fence state from the full file, not the patch window
---

# TASK-018 — Fence state from the full file, not the patch window

## Objective

Close C3-4: fence tracking reads the `--unified=3` patch, so a checkbox inside a fenced block whose opening fence sits more than three unchanged lines above is scored as execution — the code's own comment ("unified=3 keeps nearby fence markers as context") states the assumption this breaks. After this task, fence state derives from the complete file images, and a flip inside any fenced region is a non-event regardless of distance.

## Scope boundaries

**In:** `commitFlipsTaskCheckboxes` and `diffContainsCheckboxFlip` in `internal/cli/change_provenance.go`; fixtures in `internal/cli/change_provenance_test.go` **only**.

**Out:** The flip grammar itself (same-hunk, same-normalized-label — unchanged, TASK-009 settled it). The path grade. Gate and verify files (other lanes own them). Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`, or any test file other than `change_provenance_test.go`.

## Context pointers

- Round-3 board finding C3-4; shape.md Provenance precision: "only `- [ ]`→`- [x]` transitions **outside code fences** count".
- The safe failure direction (shape.md Risks): a flip that fails to register blocks until a one-commit remedy; a fenced flip that registers opens a gate on nothing — err toward non-events.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-018 — fence state from full file"
```

## Steps

- [x] Compute fenced line ranges from the complete pre-image and post-image files (`git show <commit>^:<path>` / `git show <commit>:<path>` via the existing `outputCommand` seam), tracking hunk line numbers so each removed/added checkbox line maps to its file position; a transition counts only when the removed line is unfenced in the pre-image and the added line is unfenced in the post-image.
- [x] File-creation and file-deletion edges (no pre-image / no post-image) handled without error; added-checked in a new file stays a non-event as today.
- [x] Fixture: fence opened ten unchanged lines above the flipped checkbox — non-event; the round-2 near-fence fixture stays green; a genuine flip with a fenced example elsewhere in the file still counts.

## Verification

- `go test ./internal/cli/ -run 'Provenance'` green including the distant-fence fixture.
- `loaf change list` still reports `change-work-model` flip-executed at integration — real history must not un-execute.

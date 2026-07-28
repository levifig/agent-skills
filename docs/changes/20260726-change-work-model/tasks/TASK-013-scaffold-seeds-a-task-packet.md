---
change: change-work-model
id: TASK-013
title: Scaffold seeds a real task packet
relates-to:
  - TASK-002
---

# TASK-013 — Scaffold seeds a real task packet

## Objective

Make `loaf change init` emit a real first task packet instead of an empty `tasks/` directory holding a `.gitkeep`. The delegation-packet format carries the whole execution model, and nothing currently puts it in front of a shaper — the template ships inside the binary, invisible at scaffold time.

## Scope boundaries

**In:** `internal/cli/change_scaffold.go`; the seeded file's name and placeholder body; `internal/cli/change_task_template.md` if the template needs adjusting to work as a seed.

**Out:** `--brief` capture mode, which still emits `change.json` + `brief.md` only and no `tasks/`. The task frontmatter schema (closed and shipped). Do not add fields toward tracker parity.

## Context pointers

- `docs/changes/20260726-change-work-model/shape.md` — TASK-002's "a seeded `tasks/`"; Decision 7 (each file is a self-sufficient delegation packet); V3 (fresh scaffold reports zero violations).
- Review board finding m3 and its decided disposition.
- `internal/cli/change_task_template.md` — the format to seed from.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-013 — scaffold seeds a task packet"
```

## Steps

- [x] `loaf change init <slug>` seeds `tasks/TASK-001-first-slice.md` from the task template with placeholder brackets and **unchecked** boxes, and no longer writes `.gitkeep`.
- [x] The seeded packet's placeholders are tolerated by `loaf change check` exactly as `shape.md`'s brackets are, so V3 stays green — a fresh scaffold reports zero violations and no deprecation notice.
- [x] The seeded slug cites no other work unit and its name survives renaming (the author is expected to rename it; say so in the body).
- [x] `--brief` still emits no `tasks/` directory at all.
- [x] Fixture: fresh scaffold yields a parseable packet in `loaf change tasks --json` with derived completion false.

## Verification

- `go test ./internal/cli/ -run 'ChangeInit|ChangeScaffold|ChangeTasks'` green.
- In a throwaway repo: `loaf change init x` then `loaf change check <folder>` reports zero violations, and `loaf change tasks --json` lists TASK-001.

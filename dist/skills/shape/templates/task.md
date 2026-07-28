---
change: [slug]
id: TASK-NNN
title: [short title]
# Relations (closed set; targets are TASK-NNN within this change only):
# parent: TASK-NNN
# blocks:
#   - TASK-NNN
# blocked-by:
#   - TASK-NNN
# relates-to:
#   - TASK-NNN
---

# TASK-NNN — [Title]

## Objective

[What this task delivers when its checkboxes are flipped.]

## Scope boundaries

**In:** [What this task may touch.]

**Out:** [What this task must not touch — other tasks, other changes, deferred work.]

## Context pointers

- Contract: `shape.md` — [relevant sections]
- Research: [paths under research/ when cited]

## Acquisition

```bash
loaf journal log "skill(implement): TASK-NNN — [short intent]"
# [commands or files to load before editing]
```

## Steps

- [ ] [Atomic step — one commit's worth when flipped with the delivering work]
- [ ] [Next step]

## Verification

- [Commands or checks that prove this task alone]
- The slug never cites other work units (`TASK-…`, `SPEC-…`, issue keys) — identity is local; provenance is in frontmatter and the change folder.

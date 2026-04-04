---
id: TASK-059
title: 'Create SOUL.md, canonical template, and 3 profile definitions'
spec: SPEC-014
status: done
priority: P0
created: '2026-04-04T16:41:22.302Z'
updated: '2026-04-04T16:41:22.302Z'
completed_at: '2026-04-04T16:41:22.302Z'
---

# TASK-059: Create SOUL.md, canonical template, and 3 profile definitions

Create the Warden identity file and the 3 functional profile agent definitions that replace the 8 role agents.

## SOUL.md

Create `SOUL.md` at project root (or `.agents/SOUL.md` — confirm placement):
- Warden identity (Arandil) and behavioral principles
- Fellowship profiles by lore + concept name (Smith/Implementer, Sentinel/Reviewer, Ranger/Researcher)
- Council conventions (composition rules)
- Keep to ~30 lines — principles only, not procedures

Also create `content/templates/soul.md` as the canonical build-time source.

## Profile definitions

Create 3 agent files in `content/agents/`:

### `implementer.md` (Smith / Dwarf)
- Tool access: Full write (Read, Write, Edit, Bash, Glob, Grep)
- Behavioral contract: Forges code, tests, config, docs. Speciality via skills at spawn time.
- Lore: Dwarvish instance names (hard, short)
- No skill preloads — skills provided at spawn time

### `reviewer.md` (Sentinel / Elf)
- Tool access: Read-only (Read, Glob, Grep)
- Behavioral contract: Watches, guards, verifies. Cannot modify what it reviews.
- Lore: Elvish instance names (flowing, musical)

### `researcher.md` (Ranger / Human)
- Tool access: Read + Web (Read, Glob, Grep, WebSearch, WebFetch — no Write, no Bash)
- Behavioral contract: Scouts far, gathers intelligence, reports back as structured findings.
- Lore: Mannish instance names (Anglo-Saxon)

Each profile gets:
- `{name}.md` with frontmatter + content
- `{name}.claude-code.yaml` sidecar
- `{name}.opencode.yaml` sidecar

## Test
- SOUL.md exists and defines Warden + fellowship
- 3 profile files exist with correct tool boundaries
- Reviewer is mechanically read-only
- Researcher has no Write or Bash access
- `loaf build` succeeds

## Relates to
- R7, R8, R9, R10, R11, R12

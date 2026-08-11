---
change: spec-conversion-and-guidance-sweep
id: TASK-008
title: Dogfood migration and archive
blocked-by:
  - TASK-007
relates-to:
  - TASK-001
---

# TASK-008 — Dogfood migration and archive

## Objective

Loaf's own database holds zero open legacy rows, the 24 top-level SPEC files sit in `archive/` as historical evidence, and the Change carries a committed `loaf change verify` receipt.

## Scope boundaries

**In:** Run `loaf migrate work-records` against Loaf's production database (the dogfood); review the migration output — any SPEC file holding unharvested content beyond what the mechanical migration captured gets a note on its Intent (fog entry resolved here); move the 24 top-level `.agents/specs/*.md` into `archive/` with frontmatter statuses normalized; run `loaf change verify` and commit the receipt.

**Out:** Triaging the migrated Intents — that happens later in the queue with everything else (Scope Out). Editing archived spec bodies (Decision 11). Other projects' migrations.

## Context pointers

- Contract: `shape.md` — Definition of Done, Open Questions
- Live-row inventory as of shaping: 15 open specs, 17 open tasks (`loaf spec list` / `loaf task list`, 2026-08-11)

## Acquisition

```bash
loaf journal log "skill(implement): TASK-008 — dogfood migration and archive"
```

## Steps

- [ ] Migration run on Loaf's database; converted-record report captured in the journal
- [ ] Harvest review: Intents annotated where the mechanical capture missed context worth keeping
- [ ] SPEC files archived, statuses normalized; `.agents/specs/` top level holds only `archive/`
- [ ] `loaf change verify` green; receipt committed

## Verification

- `loaf intake list` shows the migrated Intents with provenance
- `loaf change check docs/changes/20260727-spec-conversion-and-guidance-sweep` clean; receipt present and fresh

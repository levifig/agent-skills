---
change: hooks-entry-reconciliation
id: TASK-005
title: Canary acceptance evidence
blocked-by:
  - TASK-002
  - TASK-003
  - TASK-004
---

# TASK-005 — Canary acceptance evidence

## Objective

The real upgrade on the canary machine, recorded as change evidence: zero drift refusals, the Codex disable-intent absorbed as a queryable record, every non-Loaf entry preserved value-identical, an idempotent second run, a verb round-trip, and enablement-aware `config check` output.

## Scope boundaries

**In:** Running the built binary against this machine's live `~/.codex` and `~/.cursor`, capturing before/after snapshots and command output under `research/`, and logging the journal evidence entries.

**Out:** Any cleanup of non-Loaf entries; changes to code (defects found here reopen the owning task instead).

## Context pointers

- Contract: `shape.md` — Hypothesis, Verification Contract H1–H4.
- Baseline: `research/hook-entry-classification.json` (pre-change live state, 52 entries) and the sanitized raw fixtures under `research/fixtures/`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — canary acceptance evidence"
# Snapshot ~/.codex/hooks.json and ~/.cursor/hooks.json before running anything.
```

## Steps

- [ ] Snapshot both live files; run the built `loaf upgrade`; capture output.
- [ ] Run `research/compare_hook_files.py` on each before/after pair and assert its reported differences name only expected Loaf entries (per the upgrade output and `loaf hooks list`); no drift-refusal text; Codex `session-start-loaf` absorbed as disabled (record present with `absorbed_at`, entry still absent, absorption marker durable); every non-Loaf entry value-identical and order-stable — the Codex herdr entry and all 33 Cursor foreign entries (32 legacy-generation plus one herdr).
- [ ] Re-run upgrade; assert no actions reported and no second absorption.
- [ ] Verb round-trip on Codex: `enable` restores exactly the `session-start-loaf` entry, `disable` removes exactly it; herdr untouched throughout; `absorbed_at` unchanged by the toggles.
- [ ] `loaf config check`: disabled Codex hook reports healthy-absent (H4).
- [ ] Store snapshots, diffs, and outputs under `research/` (named for what they are); log `finding(hooks)` journal entries for the evidence.

## Verification

- The recorded evidence satisfies H1–H4 as written in the Verification Contract.
- The comparator output for both files is part of the evidence; every difference it reports is an expected Loaf entry, and the foreign population (33 Cursor, 1 Codex) appears in neither the removed nor changed sets.

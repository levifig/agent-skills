---
id: report-spec-053-taxonomy-signoff
report_kind: gate
state_id: "report:d4e082ada4a9b3428efaa254"
status: final
title: Spec 053 Taxonomy Signoff
---

# SPEC-053 Taxonomy Sign-Off Packet

Date: 2026-06-25
Spec: SPEC-053
Task: TASK-394
Status: signed off

## Purpose

This packet records the accepted taxonomy decisions for the SPEC-053 breaking-change gate. The decisions avoid destructive cleanup where the intended product direction is vendor/optional packaging, and they wire `librarian` as Loaf's durable artifact handler instead of retiring it.

## Accepted Decisions

1. Vendor `thermo-nuclear-code-quality-review`.
   - Decision: do not retire or delete old installed copies.
   - Implementation: add an `externalized_skills` manifest entry that reports old installed copies as moved out of Loaf core and points at the vendor source.
   - Vendor source: https://github.com/cursor/plugins/tree/main/cursor-team-kit/skills/thermo-nuclear-code-quality-review
   - Follow-on: a real `loaf skill add`/update/remove installer needs its own package-management spec with source pinning, provenance, and update semantics.

2. Keep `debugging` description-only for now.
   - Decision: do not set `disable-model-invocation` in SPEC-053.
   - Decision: do not use `user-invocable: false` as a routing fix.
   - Follow-on: SPEC-051 owns validated description rewrites and routing evals.

3. Model optional skill packaging as recommended, curated, and vendor.
   - Decision: language/domain packs are not retired in SPEC-053.
   - Decision: they should eventually become recommended optional skills, while third-party skills become vendor skills and Loaf-hosted optional packs become curated/recommended.
   - Follow-on: install profiles and de-selection cleanup remain out of this slice.

4. Wire `librarian` as the durable artifact handler.
   - Decision: keep `librarian`.
   - Implementation: add Cursor/OpenCode sidecars and workflow caller guidance from wrap, housekeeping, and orchestration.
   - Boundary: `librarian` remains scoped to SQLite-backed Loaf state and `.agents/` artifacts; it does not write code, review code, research, or orchestrate.

## Production Manifest Shape

The active production entry is non-destructive:

```json
{
  "externalized_skills": [
    {
      "skill": "thermo-nuclear-code-quality-review",
      "since": "v2.0.0-pre.20260614235428",
      "reason": "externalized by SPEC-053 taxonomy decision; install as a vendor skill instead of a Loaf core skill",
      "signoff": "report-spec-053-taxonomy-signoff",
      "source": "https://github.com/cursor/plugins/tree/main/cursor-team-kit/skills/thermo-nuclear-code-quality-review",
      "skill_homes": [
        "${HOME}/.agents/skills",
        "${HOME}/.config/agents/skills",
        "${HOME}/.config/opencode/skills"
      ]
    }
  ]
}
```

## Gate Status

TASK-394 and TASK-395 are closed. SPEC-053's signed taxonomy gate is ready for downstream breaking specs to consume, with destructive cleanup still requiring explicit `--yes` confirmation and the broader migration/rollback mechanism already proven by the earlier SPEC-053 tasks.

<!-- loaf:render kind=report contract=durable-doc-v1 -->

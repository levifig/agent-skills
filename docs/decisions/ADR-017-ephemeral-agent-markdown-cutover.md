---
id: ADR-017
title: "Ephemeral Agent Markdown Cutover"
status: Accepted
date: 2026-06-25
supersedes: null
superseded_by: null
amended_by: ADR-019
---

# ADR-017: Ephemeral Agent Markdown Cutover

## Context

ADR-013 established that agentic state is project-scoped rather than
branch-scoped, and SPEC-040 moved Loaf operational state into a single global
SQLite database. ADR-016 then clarified the storage rule: nouns live in SQLite,
verbs live in git, and Markdown is a render/export surface.

The remaining tracked Markdown under `.agents/tasks/`, `.agents/sessions/`,
`.agents/ideas/`, `.agents/drafts/`, `.agents/sparks/`, and
`.agents/brainstorms/` predates that model. Those files are no longer the right
operational source of truth: they duplicate SQLite rows, create stale
cross-branch provenance, and leave agents unsure whether to mutate Markdown or
the database. (`.agents/sessions/` is a historical migration source; the
session entity itself was later removed by SPEC-056 / ADR-019.)

## Decision

Loaf will cut over ephemeral agent artifacts to SQLite-only operational state.
The tracked Markdown files in the ephemeral roots above, plus `.agents/TASKS.json`,
are migration sources and rollback material, not durable source documents.

The cutover must be reversible and gated:

1. Create an out-of-tree rollback backup that stores the original file bytes and
   SHA-256 manifest.
2. Verify the byte barrier before deletion: each ephemeral file must still match
   the rollback backup bytes.
3. Remove the tracked ephemeral Markdown and `.agents/TASKS.json` in one
   guarded git change.
4. Keep durable specs, ADRs, reports, skills, docs, and code in git.

This corrects the ADR-013-era assumption that session/task Markdown files remain
the operational routing surface. Project-scoped state still stands; the state is
now the SQLite noun model, with Markdown only as compatibility import, export,
or deterministic render.

This also corrects a factual error in ADR-013's Decision, which lists "ADRs,
knowledge" among the artifact kinds that "all live in one place" under
`.agents/`. They do not: ADRs live in `docs/decisions/` and knowledge lives in
`docs/` (Tier-2, git-native). ADR-013's project-scoped-resolution rule governs
only the artifact kinds that actually live under `.agents/`; the `docs/` tree is
git-native and outside that resolver.

## Consequences

### Positive

- Agents have one mutation surface for operational artifacts: Loaf SQLite
  commands.
- Branches stop carrying divergent copies of task, session, idea, spark, and
  brainstorm state (the session entity was later removed outright by
  SPEC-056 / ADR-019).
- Rollback remains byte-exact because restore uses stored backup bytes, not a
  renderer.
- ADR-016 has a concrete enforcement point for legacy agent Markdown.

### Negative

- Historical Markdown references need to be rewritten, tombstoned, or ignored by
  explicit policy before the destructive cutover.
- Some docs and skills that taught direct Markdown writes need follow-up cleanup
  so users do not reintroduce the old source model.

### Neutral

- Durable authored Markdown remains in git. Specs and ADRs are not part of the
  ephemeral cutover.
- Compatibility import/export can continue to read and produce Markdown, but it
  is no longer authoritative operational state.

## Alternatives Considered

### Keep ephemeral Markdown as a compatibility mirror

Rejected. A mirror still creates two apparent sources of truth and lets branches
drift. It also makes state repair harder because agents can mutate the mirror
without updating SQLite.

### Delete without rollback

Rejected. The cutover is intentionally breaking, so it needs a byte-exact backup
and one-command restore path.

### Convert every historical reference before deletion

Rejected as the blocking criterion. Historical sessions and archived artifacts
can remain as history; the required gate is that surviving active provenance does
not point at deleted operational files.

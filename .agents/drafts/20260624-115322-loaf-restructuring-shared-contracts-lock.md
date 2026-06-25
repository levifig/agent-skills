---
title: "Shared Contracts Lock: Loaf Restructuring Program"
type: contract-lock
created: 2026-06-24T11:53:22Z
status: draft
source: restructuring-program
governed_by: ADR-016
related_specs:
  - SPEC-043
  - SPEC-044
  - SPEC-045
  - SPEC-046
  - SPEC-047
  - SPEC-048
  - SPEC-049
  - SPEC-052
  - SPEC-053
  - SPEC-054
---

# Shared Contracts Lock: Loaf Restructuring Program

This draft locks the cross-spec contracts that must stay stable while executing
SPEC-043...054. It is intentionally narrow: implementation specs may add detail,
but must not contradict these contracts without first updating this lock and the
dependent specs.

## 1. Artifact Body Contract

ADR-016 governs storage boundaries: nouns are SQLite rows, verbs stay in git,
and Markdown/JSON/CSV/HTML are renders.

SPEC-043 uses a separate `artifact_bodies` table, not one body column per entity:

```sql
artifact_bodies(
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  entity_kind TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  body_kind TEXT NOT NULL DEFAULT 'markdown',
  content TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(project_id, entity_kind, entity_id, body_kind)
)
```

Contract rules:

- `sources` becomes provenance-only: path, hash, line range, import time. It
  never stores body content.
- Existing `body_source_id` columns remain nullable provenance/fallback links.
- Read precedence is: `artifact_bodies` current row wins; otherwise fall back to
  `body_source_id` and the existing Markdown file reader until SPEC-045 removes
  ephemeral files.
- Writes to body-capable entities update `artifact_bodies` and FTS rows in the
  same transaction. No trigger-maintained FTS.
- Body content must be UTF-8 text. Code, generators, scripts, and hook logic are
  never body content; store only provenance pointers to them.
- SPEC-043 stays non-breaking: no file deletion, no status vocabulary rewrite,
  and no durable render finalization.

## 2. Search And Entity Contract

SPEC-043 owns Tier-1 search over SQLite-resident bodies and journals. SPEC-046
adds Tier-2 `docs/` indexing as a rebuildable cache.

Contract rules:

- FTS5 is used through SQL DDL (`CREATE VIRTUAL TABLE ... USING fts5`); no new
  Go import or module dependency is introduced for FTS.
- Tier-1 hits are entity-addressed (`kind:alias` or stable row ID). Tier-2 hits
  are file-addressed (`path:line`). JSON output carries a `tier` discriminator.
- Default search scope is the current project. `--all-projects` is explicit.
- `docs/` indexing reads the invoking checkout's working tree, not the main
  worktree, because `docs/` is git-native source.
- SPEC-054 adds `finding`, `verdict`, and `run` rows on top of SPEC-043. `run`
  stores generator ref/version/hash, never generator code.

## 3. Status Contract

SPEC-043 and SPEC-054 may add local validators for new write paths, but they do
not rewrite existing status vocabularies.

Contract rules:

- Existing status strings remain unchanged until SPEC-049.
- SPEC-049 owns the canonical lifecycle vocabulary, per-entity subsets, data
  migration, and event normalization.
- SPEC-049 data rewrite is breaking and must consume SPEC-053 backup/rollback.
- SPEC-048 may cite the runtime session enum as interim truth, but must not
  invent another session status vocabulary.
- New entities added before SPEC-049 must define local status sets in code and
  document their mapping candidates for SPEC-049.

## 4. Durable Render Contract

SPEC-044 owns committed durable Markdown renders. The global SQLite database is
absent in a clean CI checkout, so the gate is self-consistency, not DB mirroring.

Contract rules:

- Deterministic durable rendering is a new code path, separate from live summary
  exports that contain counts, timestamps, or absolute paths.
- Deterministic renders contain no `time.Now()`, live counts, absolute DB paths,
  or map-iteration-dependent ordering.
- Normalized output uses LF line endings and locked section/field order.
- Scratch renders go under XDG cache, namespaced by project ID and branch.
- Committed durable renders carry a trailing stamp:
  `<!-- loaf:render kind=<kind> contract=durable-doc-v1 -->`.
- The drift gate parses the committed render, re-renders deterministically from
  the parsed representation, and byte-compares with no SQLite DB required.
- Hand-edited committed renders are rejected with guidance to edit via `loaf
  <entity> edit`, then render/finalize. The gate never re-imports hand edits.

## 5. Migration And Breaking-Change Contract

SPEC-053 is the gate for all breaking/user-visible removals and relocations.

Contract rules:

- Breaking actions require `--dry-run` output, explicit confirmation or `--yes`,
  and a verified backup before mutation.
- Backups live under `$XDG_DATA_HOME/loaf/backups/<timestamp>-<slug>/` and
  include a manifest with SHA-256 hashes for every file or DB copied.
- `loaf migrate markdown --rollback <backup-id>` restores original file bytes
  from the backup manifest; rollback never uses a render.
- Deprecations and relocations are declared in `config/deprecations.yaml`.
  Entries include kind, id/path, replacement, introduced_version,
  removal_version, and reason.
- `loaf install --upgrade` consumes the deprecation manifest for orphan cleanup
  and path relocation. It removes only Loaf-managed entries.
- SPEC-045, SPEC-049 data migration, SPEC-052 relocation, retired skills,
  opt-in packs, and user-side Gemini cleanup do not ship before SPEC-053 and
  user sign-off.

## 6. Harness Parity Contract

SPEC-047 locks the first-class harness matrix to five targets: Claude Code,
Codex, Cursor, OpenCode, and Amp. Gemini is removed from in-repo build output;
user-side cleanup is SPEC-053.

Parity means equivalent capability by native idiom, not identical files:

- Claude Code and Amp reach workflow skills by native skill loading.
- Cursor, Codex, and OpenCode reach workflow skills by generated command files.
- Advisory hooks stay advisory; enforcement hooks stay enforcing on every
  supported hook surface.
- Harness-specific language is expressed with source tokens and resolved per
  target. Non-Claude outputs must not leak raw Claude Code tool names, agents
  file names, or subagent wording.
- The parity matrix test derives expectations from `content/skills`, sidecars,
  and `config/hooks.yaml`; it must not be a stale hand-maintained checklist.

## 7. Execution Rules

- TASK-197 is the foundation branch for markdown-migration preview fidelity.
- SPEC-047 and SPEC-043 may proceed in parallel only after this contract is
  reviewed against their open questions.
- Every spec branch must include source, tests, and generated artifacts together.
- Push, PR creation, merge, release tagging, and publication require explicit
  user confirmation.

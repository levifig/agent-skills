---
change: pitch-entrypoint
id: TASK-006
title: Captured-folder promotion — init completes capture-only folders safely
---

# TASK-006 — Captured-folder promotion

## Objective

`loaf change init <slug>` completes a structurally valid capture-only folder in place — preserving `brief.md` and every `change.json` value verbatim while instantiating `shape.md` and the seeded `tasks/` — through an explicit, failure-safe transition protocol, shipped with the rebuilt runtime and converged CLI guidance, so pitch's "shape now" handoff reaches the full Change anatomy. Duplicate rejection is unchanged for fully-materialized folders.

## Scope boundaries

**In:** `internal/cli/change.go` (init duplicate-detection branch), `internal/cli/change_scaffold.go` (completion path), `internal/cli/change_test.go` (promotion regression), the `init` contract surfaces — CLI help and generated reference (`internal/cli/agent_help.go`, `internal/cli/cli_reference.go`) and `content/skills/shape/references/cli-boundary.md` — plus the rebuilt runtime and mirrors: `bin/native`, `plugins/`, `dist/`, and any hash-bound receipt the artifact verifier demands, all in this task's delivering commit.

**Out:** no new CLI verbs or flags (completion rides the existing `init` verb — Decision 12 and the Cut list), no changes to `--brief` capture-mode scaffolding itself, no template content changes (TASK-001), no skill text beyond the cli-boundary reference (TASK-004 instructs shape to rely on this path), no edits to `docs/knowledge/task-system.md` (TASK-005's convergence sweep).

## Context pointers

- Contract: `shape.md` — Scope (captured-folder promotion bullet), Decisions 11–12, Planning Contract (Approach, Sequencing).
- Verified gap: capture then re-`init` currently dead-ends with `change slug "<slug>" already exists` — reproduce with an isolated `LOAF_DB` and a scratch git directory before coding.
- Current behavior: duplicate detection at init, the sequential overwrite-capable scaffold writer, and the layout loader's captured/shaped derivation (note: the loader currently derives captured even for a `change.json`-only folder — promotion must not).
- Shipping discipline: tracked `bin/native` binaries, `cli/scripts/verify-go-artifacts.mjs` (rejects stale binaries), and ADR-023's consequence that Go code commits rebuild and refresh hash-bound receipts.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-006 — captured-folder promotion in change init"
# Reproduce the dead-end in isolation first; read the init duplicate-detection, scaffold-writer, and layout-loader paths
```

## Conventions

Markdown paragraphs are single lines — never hard-wrap. Match the existing Go error-message voice; table-driven tests beside the existing scaffold tests.

## Steps

- [ ] Define the promotion state matrix in code, with exactly three outcomes. Promote: a structurally valid brief-only new-layout folder (valid `change.json` + `brief.md` + no `shape.md`, no `tasks/` content). Resume: a partial promotion — no `shape.md`, and any existing `tasks/` content byte-identical to the would-be seed instantiation — completes by filling only the missing pieces. Fail clearly and untouched: everything else — repeated `init --brief` on a capture, `change.json`-only folders (missing brief), hybrid legacy/new folders (`change.md` beside `change.json`), fully-materialized folders (duplicate rejection unchanged), diverged partial `tasks/` content, and malformed or semantically invalid metadata (fail closed, never silent fallback).
- [ ] Make the transition atomic-by-publication, not merely ordered: every destination file is written to a temp path in the same directory and atomically renamed into place (refusing when the destination exists — resume fills gaps only), so a destination file either does not exist or is complete; `shape.md` is renamed last as the promotion marker. A plain sequential `os.WriteFile` is insufficient — a process death mid-write would leave a partial marker the loader classifies as materialized, recreating the dead-end this task eliminates.
- [ ] Completion preserves `brief.md` bytes and `change.json` values verbatim (including `target_release` and `created`), instantiates `shape.md` and the seeded `tasks/` from the embedded templates, and prints a promotion message distinct from fresh-scaffold output.
- [ ] Promotion regression tests covering the matrix and the publication protocol: capture → promote → `loaf change check` derives shaped-or-better with the brief intact; injected interruption before the marker rename followed by a successful resume; interruption fixtures simulating a partial task write and a partial marker write (stray temp files, half-written bytes) proving neither can strand the folder as materialized or unresumable; promoting a materialized folder still fails; repeated `--brief` fails untouched; missing-brief, hybrid-layout, invalid-schema, and diverged-partial-`tasks/` fixtures fail closed; a stamped `target_release` carries through untouched; no existing file is ever overwritten.
- [ ] Converge the `init` contract surfaces in the same commit: CLI help and generated reference name capture promotion alongside duplicate rejection, and `content/skills/shape/references/cli-boundary.md` replaces "init fails when the folder exists" with the promotion semantics and documents the check JSON envelope's `state` field, which the landing guard reads.
- [ ] Ship the behavior: `npm run build` (rebuilds the native runtime and targets), run the manual smoke through the branch-built `./bin/loaf`, and commit the rebuilt `bin/native` mirrors, `plugins/`, `dist/`, and any hash-bound receipt updates the artifact verifier demands together with the source — one atomic task commit.

## Verification

- `go test ./... -count=1` passes including the promotion matrix and failure-protocol regressions.
- `npm run build` passes, including the artifact verifier's stale-binary check, in the same commit as the Go change.
- Manual proof through the branch-built binary in an isolated scratch repo (`LOAF_DB` on a throwaway database): `./bin/loaf change init demo --brief && ./bin/loaf change init demo` completes the folder instead of erroring, and `./bin/loaf change check docs/changes/<date>-demo` — explicit folder path, since bare check resolves by branch and the scratch repo stays on its default branch — reports it shaped-not-captured; a third `./bin/loaf change init demo` then rejects the materialized folder.
- CLI help output and the cli-boundary reference both name capture promotion — confirmed by reading the built output, not the source.
- The slug never cites other work units — identity is local; provenance is in frontmatter and the change folder.

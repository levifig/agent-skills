---
id: ADR-024
title: "Receipt validity binds to a masked root-tree content digest"
status: Accepted
date: 2026-07-29
supersedes: null
superseded_by: null
related:
  - ADR-023
---

# ADR-024: Receipt validity binds to a masked root-tree content digest

## Context

ADR-023's receipt freshness walked `verified_commit..HEAD` and exempted only the change's own receipt path. Merge strategies that destroy commit identity (squash, rebase) make that walk machine-dependent: the author's object store may still hold the commit while a protocol clone exits 128 into `cannot inspect receipt`. The walk also made N≥2 cohorts unsatisfiable — member B's receipt stales member A — and left touch-then-revert as a load-bearing property that squash merges already erase on main.

Council 2026-07-28 (`docs/changes/20260728-receipt-tree-binding/reports/20260728-222942-council-receipt-freshness.html`) settled on binding validity to content.

## Decision

**Gating digest.** `scopeDigest(T, X)` over `git ls-tree -r -z --full-tree <T>` entries whose path matches no glob in `X`: emit `path\0mode\0oid\n`, byte-sort ascending, prefix `loaf/change-evidence-digest\nv1\n`, SHA-256. Entries come from ls-tree only — never the filesystem. Mode is included. Matching is byte-exact and case-sensitive.

**Exclusion set (one exported boundary).** `X = ChangeEvidenceExclusions()` = `docs/changes/*/receipts/**` ∪ `docs/changes/*/reports/**` ∪ `ReleaseMetadataAllowlist` (version files, `CHANGELOG.md`, `dist/**`, `plugins/**`, `bin/**`). Designation-legal ≡ receipt-neutral: the promotion Change imports the same constant. Masking regenerated outputs obliges the designation check to prove they are the deterministic rebuild of source.

**Glob grammar.** Component-anchored: `*` is one segment; trailing `**` is zero-or-more remaining segments. Not prefix match (`dist/**` must not match `distributor/…`).

**Freshness predicate.** A pure function of receipt fields and the pinned HEAD tree — supported schema ∧ current exclusions ∧ current digest_spec ∧ criteria digest (text included) ∧ scope digest ∧ results cover criteria IDs ∧ all ok. Reachability never participates in the verdict. `verified_commit` is provenance only. `scope_sections` refine drift messages, never the verdict.

**Schema v2 clean break.** No dual-read of v1; unsupported schema blocks with a re-verify remedy. Never recompute an absent digest from the current tree to "upgrade" a receipt — an attestation whose subject is manufactured by its reader is not evidence.

**Touch-then-revert detection dropped.** Endpoint digests cannot see it; squash already erases it; the claim is about content, not history. A byte-identical restore un-stales deliberately.

**Verify refuses a dirty tracked worktree.** A receipt must not attest execution against bytes no commit holds. Dirty checks (pre- and post-criteria) exempt exactly the receipt and report masks — never the release-metadata allowlist — so a criterion that mutates tracked `dist/`/`bin/`/version files fails closed. Post-run divergence still writes the receipt (write-on-failure) with `worktree_clean: false`; the freshness predicate rejects that field as a typed void-execution verdict.

## Consequences

Same HEAD yields the same verdict on every clone under any merge strategy. Cohort receipts coexist because receipts are masked. The promotion model can cut rc / promote without expiring evidence by construction. Cost accepted: any real code landing expires cohort receipts until an rc-point re-verify sweep.

**ADR-023 successor note.** The receipt attests criteria-against-declared-scope; tree-is-green at cut belongs to CI (the promotion change lands that assertion).

ADR-023's freshness section (commit-walk staleness, touch-then-revert), v1 receipt schema (including `cwd`), and "any later commit stales" cost claim are superseded by this ADR; provenance grades, verify-as-only-runner, and success-required rules remain.

Provenance: `docs/changes/20260728-receipt-tree-binding/` shape.md Decisions 1–8; council board 2026-07-28.

<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Receipt Tree Binding — Content-Digest Freshness, Merge-Strategy-Agnostic

## Problem

Receipt freshness is anchored to commit identity, and merge strategies are licensed to destroy commit identity. `receipts/verify.json` records `verified_commit`, and the gate's freshness check walks `verified_commit..HEAD` (`change_verify.go:493-525`); after a squash or rebase merge plus branch deletion, that commit is unreachable on every machine except the author's, so the same repository content yields a reasoned block on one machine and a `release blocked: cannot inspect receipt … exit status 128` crash on any fresh clone (stderr discarded by `commandOutput`). The council reproduced this end-to-end, and found two more defects the commit walk hides:

1. **Cohorts of two or more are unsatisfiable.** The walk exempts only the change's own receipt path, so member B's committed receipt stales member A's — proven in every commit ordering, including both receipts in one atomic commit. The cohort gate has never been satisfiable for the multi-change cohort it exists to gate.
2. **Verify attests worktree results under a HEAD label.** Criteria run against the working tree while the receipt records `git rev-parse HEAD` — an uncommitted fix produces a green committed receipt for code no commit holds.
3. **`criteria_digest` drops the criterion's prose**, so a V-entry's claim text can be rewritten without expiring anything; `schema_version` is written and never read; the receipt commits an absolute local path (`cwd`); and reason strings are load-bearing control flow (`strings.HasPrefix` dispatch in `formatChangeReceiptBlock`).

It bites the first cohort member that verifies on a branch — the sweep carrier, en route to 2.0.0 — and Loaf installs into arbitrary consumer repos whose merge policy it cannot dictate, so the gate must be merge-strategy-agnostic.

## Hypothesis

Binding receipt validity to content digests — the whole tree minus a declared, receipt-recorded exclusion set — makes the gate's verdict a pure function of receipt fields and HEAD content: same HEAD, same verdict, on any clone, under squash, merge commit, or rebase. The exclusion set (receipts repo-wide, report boards, and the release-metadata allowlist) makes **designation-legal equivalent to receipt-neutral**, so rc cuts and the promotion commit can never expire cohort receipts — the promotion model's by-construction guarantee — while any real code change anywhere expires evidence, preserving the work model's Decision 13 precedent that evidence must not survive a revert of the verified work.

## Scope

**In**

- Canonical scope digest: `git ls-tree -r -z --full-tree <pinned-sha>` entries minus the exclusion set, byte-sorted, mode included, domain-separation header, SHA-256, spec-versioned; the exclusion set defined as one exported boundary (receipts + reports masks ∪ release-metadata allowlist) for the promotion change to consume.
- Receipt `schema_version: 2`: `scope_digest`, `scope_sections` (per-top-level-directory sub-digests for drift naming), `exclusions` verbatim, `digest_spec`, `verified_root_tree` and `repo_tree_digest`-style provenance (recorded, never gating), `tool_version`, toolchain, `worktree_clean`; `verified_commit` demoted to provenance with a non-gating comment; `cwd` dropped; `criteria_digest` extended to cover criterion text.
- Verify hardening: refuse a dirty tracked worktree; drop the `-l` from `bash -lc` (login-shell environment divergence).
- Freshness read path rewrite: the commit walk deleted; a pure predicate over (receipt fields, HEAD tree); every checker state maps to a reasoned block verdict via a typed reason enum — unparseable, unsupported schema, digest mismatch (naming drifted sections), criteria mismatch, failing results, uncommitted, missing; the unreachable-commit inspection error becomes unreachable code.
- Regression fixtures: post-squash verification (verify on branch, squash-merge, delete branch, protocol-clone simulation → verified), the N≥2 cohort composability fixture, and the inverse of the touch-then-revert fixture asserting the new contract (byte-identical restore un-stales, deliberately).
- Message surfaces: `commandOutput` captures stderr into returned errors; block messages name the folder, the cause, and the copy-pasteable remedy.

**Out** (deferred, not rejected)

- The CI-green-at-HEAD assertion before tagging, rc-cut gating, and the designation diff check — the promotion-model change (`docs/changes/20260728-release-promotion-model/`) consumes this change's exclusion-set boundary for them.
- Per-change declared `evidence_paths` — coherent additive-only extension; ship only when a real case argues for it (council: medium-low, deliberately).
- Command dedup across an rc-sweep (`go test ./...` re-runs per member) — an optimization for cohorts big enough to feel it.
- A read-only receipt-status surface in `loaf change check` output — fold into the promotion change's skill work where the ceremony is defined.

**Cut** (explicitly rejected)

- Any reachability fallback when the verified commit happens to exist — a verdict that differs by object-store state is the defect formalized (council unanimous).
- Folder-scoped gating — the criteria read the repo, not the folder; rejected 3:2 with the owner settling on masked root (board: `reports/20260728-222942-council-receipt-freshness.html`).
- A configurability knob for expiry scope.
- `git mktree` for digest construction — writes objects into the store; an audit gate must not have side effects.
- Recomputing an absent digest from the current tree to "upgrade" a v1 receipt — an attestation whose subject is manufactured by its reader is not evidence (named anti-pattern for the ADR).
- "N commits old" drift metrics — they need reachability; drift is named by sections, machine-independently.

## Observable Workflow

```
# verify writes a v2 receipt bound to content, refusing dirty trees
loaf change verify docs/changes/20260727-spec-conversion-and-guidance-sweep
# error: working tree differs from HEAD; commit before verifying

# after squash merge + branch deletion, on a fresh clone:
loaf release --dry-run --bump release
# change "spec-conversion-and-guidance-sweep": verified (V1–V7 green)   ← same verdict as the author's machine

# a later code commit expires evidence with a named reason:
# release blocked: change "…" targets 2.0.0 but content changed since verification
# (content changed under internal/, content/). Run: loaf change verify docs/changes/…, then commit the receipt

# two cohort members' receipts coexist — committed in one sweep commit, neither stales the other
```

## Rabbit Holes and No-Gos

- **Do not grow the digest into semver-style cleverness or partial-tree negotiation.** One construction, one spec version, spec change expires everything — the same discipline `criteria_digest` already has.
- **Do not consult the worktree in the gate path.** The one surviving worktree read (`changeReceiptExistsInWorkingTree`) refines a reason between two blocking verdicts, never a verdict; reasons become a typed enum precisely so prose can't become control flow again.
- **Do not solve promotion here.** This change exports the exclusion-set boundary; the promotion change wires it into rc gating and the designation check.
- **Do not let the exclusion set accrete silently.** It is recorded verbatim in every receipt and any edit to it expires all receipts — correct, because the domain of the claim changed.

## Decisions

Provenance: five-member advisory council 2026-07-28, decision by the owner after board review (`reports/20260728-222942-council-receipt-freshness.html`); defect reproductions by the council against the real gate code; interview decisions logged as journal `decision(shape)`/`decision(council)` entries the same day.

1. **The gating digest is the masked root tree.** Whole tree minus the declared exclusion set; folder-scoped gating rejected because the criteria's input domain is the repository (`go test ./...`, `npm run build`, `loaf check`) and Decision 13 of the work model already rejected evidence that survives a revert of the verified work.
2. **The exclusion set is one exported boundary: receipts (repo-wide) ∪ report boards ∪ the release-metadata allowlist.** Designation-legal ⟺ receipt-neutral — rc cuts and promotion cannot expire cohort receipts by construction; the promotion change consumes the same constant for its designation diff check. If the allowlist ever grows to admit paths criteria genuinely read, the sets diverge with the digest exclusion strictly smaller.
3. **Reachability leaves the verdict entirely; no fallback.** `verified_commit` is provenance. Diagnostics may consult extra objects when present but degrade to shorter messages — never different verdicts, never errors.
4. **Touch-then-revert detection is dropped deliberately.** Endpoint digests cannot see it, squash merges already erase it on main, and the receipt's claim is about content, not history; the fixture inverts to assert the new contract.
5. **Verify refuses a dirty tracked worktree.** A receipt from a dirty tree attests an execution against bytes no commit holds; three council members found this independently.
6. **Schema v2 is a clean break, shipped in the next alpha.** Zero receipts exist anywhere; v1 refused with a named re-verify remedy; no dual-read path; `schema_version` is read and enforced from now on.
7. **Drift is named by content, not counted by commits.** Per-top-level-directory sub-digests let every machine say "content changed under `internal/`" with no ancestry; wall-clock and cycle-relative signals stay advisory material for the promotion ceremony.
8. **Digest construction details are load-bearing and pinned in the ADR**: `-z` (quotePath immunity), byte-sort (traversal-order independence), mode included (executable-bit changes behavior), tree-path matching byte-exact and case-sensitive, entries from `ls-tree` never the filesystem (autocrlf/case-folding immunity), domain-separated hash, no `mktree`.

## Planning Contract

### Digest construction

`scopeDigest(T, X)`: over every entry of `git ls-tree -r -z --full-tree <T>` whose path matches no glob in `X`, emit `path \0 mode \0 oid \n`, byte-sort ascending, prefix `loaf/change-evidence-digest\nv1\n`, SHA-256. `X = {docs/changes/*/receipts/**, docs/changes/*/reports/**} ∪ releaseMetadataAllowlist` where the allowlist names version files (`package.json`, `.claude-plugin/marketplace.json`), `CHANGELOG.md`, and the regenerated output roots (`dist/**`, `plugins/**`, `bin/**`). The boundary lives in one exported place; the composition obligation is stated in the ADR: masking regenerated outputs means the promotion designation check must independently prove they are the deterministic rebuild of source (the existing drift check does).

### Freshness predicate

`fresh(receipt, HEAD) ≡ schema_version supported ∧ exclusions = current ∧ digest_spec = current ∧ criteria_digest matches shape.md@HEAD (text included) ∧ scope_digest = scopeDigest(HEAD, X) ∧ results cover the criteria ID set ∧ all ok`. Every input derives from receipt fields and the pinned HEAD tree — no refs, no reachability, no worktree, no clock. `scope_sections` mismatches refine the block message, never the verdict.

### Failure-mode table

Missing → block `missing receipt`; present-in-worktree-only → block `receipt not committed at HEAD` (worktree stat is a hint layer refining prose only); unparseable → block `receipt unreadable — re-verify` (today an inspection error); unsupported schema → block naming the version and remedy; criteria mismatch → block `criteria changed (receipt expired)`; scope mismatch → block naming drifted sections; result-set mismatch or any `ok: false` → block naming IDs. Reasons are a typed enum; rendering derives from the type; no consumer parses prose.

### Sequencing

Digest + boundary first (TASK-001), then the write path (TASK-002) which needs the digest, then the read path (TASK-003) which needs both, then messages/fixture sweeps (TASK-004) which exercise everything. This change lands before the sweep carrier runs its first `loaf change verify` on a branch — the whole point — and before the promotion change's gate work, which imports the boundary.

## Implementation Units

- **TASK-001 — Scope digest and the evidence boundary.** The canonical digest construction, the exported exclusion-set constant, determinism tests (quotePath, sort, mode, case-sensitivity), and the sub-digest sections.
- **TASK-002 — Receipt schema v2 and the verify write path.** New fields, criteria-text digesting, dirty-worktree refusal, login-shell fix, `cwd` dropped, tool/toolchain recorded.
- **TASK-003 — Freshness predicate and typed verdicts.** Commit walk deleted, pure predicate, reason enum, every state a reasoned block; post-squash, cohort-composability, and inverted touch-then-revert fixtures.
- **TASK-004 — Messages and error capture.** `commandOutput` stderr capture, block wordings with copy-pasteable remedies, section-named drift in gate output.

## Verification Contract

- **V1.** The digest is deterministic and correctly masked: identical trees digest identically across quotePath/case/mode variations; excluded paths never participate; spec or exclusion change expires. Command: `go test ./internal/cli -run 'TestChangeScopeDigest' -count=1`. Expect: exit 0.
- **V2.** Verify writes v2 receipts: new fields present, criteria text digested, dirty tree refused, no absolute paths in the artifact. Command: `go test ./internal/cli -run 'TestChangeVerifySchemaV2' -count=1`. Expect: exit 0.
- **V3.** Freshness is machine-independent and reasoned: post-squash protocol-clone fixture verifies green; N≥2 cohort receipts coexist; every failure state yields a typed block, never an inspection error; v1 receipts refused with the remedy. Command: `go test ./internal/cli -run 'TestChangeReceiptFreshness' -count=1`. Expect: exit 0.
- **V4.** The full suite is green. Command: `go test ./...`. Expect: exit 0.

<!-- Human review (H-tier): review material, never gate input. -->

- **H1.** A gate transcript over the fixtures reads as reasoned verdicts with named remedies — no raw git errors surface anywhere.
- **H2.** The council board in `reports/` accurately records the decision and its provenance; the ADR pins the digest serialization exactly.

## Definition of Done

- All V-entries green at HEAD via `loaf change verify` with the receipt committed — this change's own receipt is the first v2 receipt and the first post-squash survivor, its own proof.
- `loaf change check` reports zero violations and the change derives executable.
- The `cannot inspect receipt` error path is deleted, not just avoided; `grep` finds no reachability consult in the freshness path.

## Durable Outputs

- ADR: receipt validity binds to content — the masked root-tree digest, the exported evidence boundary, the exact serialization, the dropped touch-then-revert property and why, the no-fallback rule, and the named anti-pattern (never recompute an absent digest from the current tree).
- ADR-023 successor note: the receipt attests criteria-against-declared-scope; tree-is-green at cut belongs to CI (the promotion change lands that assertion).
- `docs/knowledge/work-model.md`: freshness section updated from commit-walk to content-digest semantics, including the rc-sweep verification rhythm.

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]/[UU] with a route. Tags are convention, never parsed by check. -->

- [KU] Exact glob grammar for the exclusion constant (component-anchored vs prefix match) → TASK-001's tactical choice; pinned in the ADR with the serialization.

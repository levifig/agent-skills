<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Self-Carried Release Guardrail

## Problem

`loaf release --post-merge` guardrail 5 (`checkReleasePostMergeDiffFiles`, internal/cli/release_post_merge.go:240) demands that the release commit's own diff touch both `CHANGELOG.md` and at least one version file. That demand assumes the version flip happens *in* the release commit. It does not hold for a self-carrying release — one whose version flip already landed as Change content, so the version files sit at the candidate before the ceremony starts. For that shape no honest release commit can satisfy guardrail 5: the changelog is the only thing left to write, and the guardrail reads the absent version-file diff as "this does not look like a release commit". Cutting 0.2.20 hit this empirically — the versioning reset flipped every version file to `0.2.20` as its own deliverable, and the ceremony that would publish it is now blocked by a guardrail asking the release commit to redo work that is already in the tree. The only ways past it today are dishonest: a no-op version-file touch, or a manufactured re-flip commit.

## Hypothesis

Guardrail 4 already asserts the stronger fact — every version file at HEAD equals the candidate being tagged — and it runs first, aborting the whole check when it does not hold. If guardrail 5 accepts a changelog-only release commit exactly when that assertion has been made, the version-file demand is dropped only where it is provably redundant, and the self-carrying shape releases honestly without any other guardrail moving.

## Scope

**In**

- Guardrail 5 accepts a changelog-only release-commit diff when the caller can attest version-file/candidate consistency; the abort survives for callers that cannot.
- The attestation is guardrail 4's own verdict, carried forward at the call site — one place decides consistency, and it is the place that already refuses without it.
- Test matrix pinning the relaxation as conditional: changelog-only accepted under attestation, changelog-only still rejected without it, missing-changelog still rejected either way, and the ordinary both-present release commit unchanged.
- Rebuild of the tracked artifact trees plus a capability-evidence re-record against the rebuilt binary, so the 0.2.20 ceremony's evidence gate stays green.
- A `[Unreleased]` changelog line riding into the 0.2.20 section.

**Out** (deferred, not rejected)

- The 0.2.20 ceremony itself — it runs after this lands, under the reset Change's runbook.
- Any advisory in the release skill about which release shapes are self-carrying; the guardrail now accepts both shapes silently, which is the point.

**Cut** (explicitly rejected)

- Weakening any other guardrail. Guardrails 1–4 and 6–9 are untouched, and the changelog half of guardrail 5 stays unconditional — a release commit that does not write release notes is still not a release commit.
- Dropping guardrail 5 entirely. The changelog demand is the half that is not redundant with anything: guardrail 6 checks that the changelog *file at HEAD* has a populated section for the candidate, which a commit that never touched the changelog can inherit unchanged from its parent.
- Manufactured provenance — a no-op version-file rewrite or a re-flip commit staged only to satisfy the shape check. That workaround class is what this Change exists to make unnecessary.

## Observable Workflow

- A release whose version files already carry the candidate — because the flip shipped as Change content — passes `loaf release --post-merge` with a release commit that touches only `CHANGELOG.md`.
- A release commit that omits the changelog is refused exactly as before, with the same message.
- Nothing changes for a conventional release: the release commit bumps the version files and writes the changelog, and guardrail 5 reads both.

## Rabbit Holes and No-Gos

- **Re-deriving consistency inside guardrail 5.** Guardrail 4 has already read every version file and compared it to the candidate; re-reading them would add I/O that can only confirm what the caller already proved, and would create a second place where "consistent" is defined.
- **Inferring the self-carrying shape from history.** Whether the parent commit already carried the candidate is derivable from what guardrail 4 established plus the absent version-file diff; reaching for `git show HEAD^:package.json` would buy nothing and add a failure mode.
- **Turning the attestation into a flag operators can pass.** It is an internal precondition between two guardrails in one pipeline, not a `--force` in disguise.

## Decisions

Provenance: incident diagnosis while cutting 0.2.20 (this session), plus the operator's instruction to relax exactly this collision and nothing adjacent.

1. **A changelog-only release commit is accepted only under attested version-file/candidate consistency.** Redundancy is the entire justification: guardrail 4 asserts every version file equals the candidate and aborts otherwise, so demanding that the release commit *also* have moved one of those files asks for evidence of a fact already proven. Where that proof is absent the demand is not redundant, and the abort stays.
2. **The attestation is a parameter, not an assumption.** `checkReleasePostMergeDiffFiles` takes the verdict from its caller rather than inferring it, so the function keeps a complete contract, the conditional is directly testable, and a future second caller has to state what it knows instead of inheriting a silent precondition.
3. **In the composed pipeline the attestation is always true, and that is the honest reading.** Guardrail 4 refuses before guardrail 5 can run, so the version-file half of guardrail 5 becomes unreachable in production and guardrail 5 is now, in effect, the release commit's changelog-shape check. This Change records that plainly rather than pretending the branch still guards the pipeline; it guards the function, and the tests hold it.
4. **The changelog half stays unconditional.** It is the only part of guardrail 5 that no other guardrail restates, and it is what keeps the repair-commit refusals (evidence-only repair tests) failing at guardrail 5 rather than sliding through.

## Planning Contract

### Approach

`checkReleasePostMergeDiffFiles` gains a trailing `versionFilesAtCandidate bool`. The "missing both" and "missing CHANGELOG.md" aborts are untouched; the "missing a version-file diff" abort becomes conditional on the attestation being absent. At the call site, guardrail 4's equality comparison is named (`versionFilesAtCandidate := snapshot.Candidate == prepared`), used for its existing abort, and then handed to guardrail 5 — so the value carried forward is the same comparison that already decides the release's fate rather than a literal.

### Placement

One function and its one call site in internal/cli/release_post_merge.go. Tests join the existing post-merge guardrail tests in internal/cli/release_test.go, beside the helpers (`seedReleasePostMergeFiles`, `releasePostMergeHappyResponses`, `scriptedReleasePostMergeRunner`) they need.

### Risks

- **Silently admitting a non-release commit.** Bounded by the unconditional changelog demand plus guardrail 6's populated-section check on the candidate; a commit that writes no release notes still cannot pass.
- **Go changes stale the binary-pinned capability receipts.** The re-record rides this Change before merge, and nothing may be built after it — the same discipline the two sibling 0.2.20 members followed.

## Implementation Units

- **TASK-001 — Relax the release-commit diff shape.** The conditional abort, the four-case test matrix, the rebuild, the evidence re-record, and the verify receipt.

## Verification Contract

- **V1.** The post-merge guardrail suite, including the new relaxation matrix, is green. Command: `go test ./internal/cli -run TestReleasePostMerge -count=1`. Expect: exit 0.
- **V2.** The whole suite is green. Command: `npm run test`. Expect: exit 0.

Human review (H-tier):

- **H1.** A reviewer confirms no guardrail other than 5's version-file demand was weakened — guardrails 1–4 and 6–9 unchanged, and guardrail 5's changelog demand still unconditional.

## Definition of Done

- V1 and V2 pass, and the verify receipt lands as the branch's final content-free commit.
- The three 0.2.20 cohort folders each report state verified after their receipts are re-cut.
- `loaf release --dry-run` derives a candidate without the guardrail-5 refusal standing in the way of the ceremony.

## Durable Outputs

- None expected. The relaxation is a conditional inside one guardrail, recorded by this Change; if a reviewer judges the "content-carried version flip" release shape architecturally significant beyond it, that is an ADR written after the fact.

## Open Questions

- [KU] Whether the release skill should describe self-carrying releases as a named shape once one has actually shipped → revisit after 0.2.20 publishes.

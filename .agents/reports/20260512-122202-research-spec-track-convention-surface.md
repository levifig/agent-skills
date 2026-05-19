---
title: "Report: SPEC / PLAN / Track convention surface inventory"
type: research
created: 2026-05-12T12:22:02Z
status: draft
source: ad-hoc
tags: [spec, conventions, output-leakage, document-model, pre-shape]
---

# SPEC / PLAN / Track convention surface inventory

**Question:** Where do SPEC IDs, Track terminology, and PLAN references appear across Loaf's surface? Separate "output leakage" (PR titles, commits, branches) from "document model" (SPEC durability, sub-spec splitting) so the planned redesign SPEC can address them independently.

## Summary

Loaf's current convention surface is **not silent on output leakage** — the rules already exist and already prohibit SPEC IDs in commit subjects and PR titles. The friction the user observed in GridSight (`feat: ... (SPEC-007 Track 1)` style PRs) is an **enforcement gap**, not a missing-rule gap. The validating hook (`check-commit-msg.sh`) does not scan for SPEC/TASK IDs in the subject, and Loaf's own canonical close-out commit example in `implement/SKILL.md:325` violates the rule it documents.

"Track" is real Loaf vocabulary inherited from Shape Up, defined in `orchestration/references/planning.md` and surfaced in `shape/SKILL.md`. It is not formally introduced as PR/commit terminology anywhere; its appearance in PR titles is **emergent vocabulary** rather than a documented suggestion. However, the *template* already uses "Part A/Part B" while the *skills* use "Track N" — that mismatch invites projects to invent their own (`Track 1`).

The document model is **partially specified**: SPECs are first-class durable docs (`.agents/specs/`), Linear parent issues carry `[SPEC-NNN]` prefixes by design, and sub-spec splitting is mentioned but vague. Multi-PR-per-SPEC and SPEC v1→v2 lifecycle are **not documented as first-class patterns** — which is plausibly the structural cause of the leakage (authors reach for `Track N` in the PR title precisely because no other shorthand exists to say "n-th slice of this SPEC").

**Confidence: High** for the inventory (all citations verified). **Medium** for the leakage→cause mapping (one observed project; a brainstorm pass should test the hypothesis against other Loaf-using teams).

## Key Findings

### Output Leakage Surface (PR titles, commits, branches)

1. **The "no SPEC/TASK IDs in subject/title" rule already exists** — `content/skills/git-workflow/references/commits.md:223` (under **Never**: "Put SPEC or TASK IDs in commit subject"), `commits.md:227-231` ("IDs belong in footer, not subject line" with explicit good/bad examples), `content/hooks/instructions/pre-pr-format.md:3` ("No scope prefixes, no SPEC/TASK IDs"), `content/hooks/instructions/pre-pr-checklist.md:47` ("No scope prefixes. No SPEC/TASK IDs in the title."). **Confidence: High.**

2. **The enforcement hook does not catch the rule it claims to enforce.** `content/skills/foundations/scripts/check-commit-msg.sh` checks format, scope syntax, length, imperative mood, agent attribution, and file lists — but never scans for `SPEC-\d+` or `TASK-\d+` patterns in `$SUBJECT` (file:34-69). The `orchestration-validate-commit.py` hook (`content/hooks/pre-tool/orchestration-validate-commit.py:114-126`) delegates entirely to that script, so the gap is at the policy layer, not the dispatch layer. **Confidence: High.**

3. **Loaf's canonical close-out commit example violates its own rule.** `content/skills/implement/SKILL.md:325` prescribes `chore: close SPEC-XXX — archive tasks, spec, and session` as the post-spec close-out commit. SPEC ID in subject. Anyone following the canonical implement workflow ends up with at least one rule-violating commit per spec. **Confidence: High.**

4. **Branch naming guidance contradicts saved user feedback.** `content/skills/git-workflow/references/commits.md:119` shows `feat/spec-010-task-management-cli` as a recommended branch name; `commits.md:114` also shows `<type>/TASK-123-description` as a pattern. Saved memory (`feedback_branch_pr_granularity` in this project's MEMORY.md) reads: *"Branch names: feat/{slug}, no IDs"*. Content is on the pre-feedback convention. **Confidence: High.**

5. **"Track N" is NOT prescribed as commit/PR subject vocabulary.** No file in `content/` recommends putting `(SPEC-XXX Track N)` in a PR title or commit subject. The GridSight pattern is project-side invention, not Loaf-directed. **Confidence: High.**

### Track Terminology — Vocabulary Surface

6. **"Track" is first-class Loaf vocabulary, inherited from Shape Up.** Origin: `content/skills/orchestration/references/planning.md:73-103` ("Ship tracks in priority order. Drop from the end, not the middle.", "Set explicit gates between priority tracks", "Track progress visually: uphill (figuring out) → downhill (executing)"). Surfaced in `content/skills/shape/SKILL.md:36, 85, 131, 151` and `content/skills/orchestration/SKILL.md:53`. **Confidence: High.**

7. **Internal mismatch: skills say "Track", template says "Part".** `content/skills/shape/templates/spec.md:70-77` instructs authors to write `1. **Part A** — ...`, `2. **Part B** — ...`. The surrounding skill prose (`shape/SKILL.md:36, 85`) uses the word "track". A project trying to follow Loaf has to pick: the template's "Part" or the skill's "Track". GridSight picked Track. **Confidence: High.**

8. **Tracks have no canonical ID format.** Nothing says `Track-1` vs `Track 1` vs `Part A` vs `Slice 1`. The PR title `(SPEC-007 Track 1)` invented a numbering scheme on the fly. **Confidence: High.**

### Document Model Surface (SPEC durability, splitting, multi-PR)

9. **SPECs are durable artifacts by design.** `content/skills/shape/templates/spec.md` and `content/skills/orchestration/references/specs.md` treat SPECs as canonical-in-`.agents/specs/`. Status lifecycle is `drafting → approved → implementing → complete → archived` (`shape/SKILL.md:125`). **No documented lifecycle for v1 → v2 iteration** of an implemented SPEC — once `archived`, the model is silent on what happens when work resumes 18 months later. **Confidence: High.**

10. **Sub-spec splitting exists but is vague.** `content/skills/shape/SKILL.md:131` ("When scope exceeds a single track, split into sub-specs or use priority ordering within the spec"). `content/skills/orchestration/references/specs.md:140-144` shows `SPEC-001-user-auth.md` → `SPEC-001a-oauth-integration.md` + `SPEC-001b-session-management.md` + `SPEC-001c-login-ui.md`. Two splitting modes (sub-specs vs priority ordering) with no guidance on which to pick when. **Confidence: High.**

11. **Multi-PR-per-SPEC is not a documented pattern.** `implement/SKILL.md:326` describes the single-PR-per-spec flow: "If on a feature branch: push and create PR (`gh pr create`)". The breakdown skill creates tasks/sub-issues, the implement skill closes them, but **nothing prescribes how to land them across multiple PRs**. **Confidence: High.** This matters: when no shorthand exists for "this PR ships slice N of the SPEC," authors reach for `(SPEC-XXX Track N)` to fill the gap.

12. **Linear parent issue intentionally embeds SPEC ID.** `content/skills/breakdown/SKILL.md:213` mints Linear parent titles as `[SPEC-NNN] <spec title>`. This is *Linear-internal labeling* (the `spec` label group, parent rollup convention) — it does not appear in git history or PRs unless someone copies it. This is the dual-ID system the user observed: SPEC IDs live in `.agents/` and in Linear parent titles by design; ENG-XXX (sub-issue) IDs are the workflow ledger. **The leakage was authors hand-typing both in PR copy.** Nothing in Loaf prescribes that. **Confidence: High.**

### Where SPEC IDs are documented to live

| Surface | Carries SPEC ID? | Prescribed By |
|---|---|---|
| `.agents/specs/SPEC-NNN-*.md` filename | Yes | `shape/SKILL.md:108`, `shape/templates/spec.md:7,17` |
| Spec file H1 (`# SPEC-XXX: [Title]`) | Yes | `shape/templates/spec.md:17` |
| Linear parent issue title `[SPEC-NNN] <title>` | Yes | `breakdown/SKILL.md:213` |
| Linear sub-issue title | No | `breakdown/SKILL.md:240` (just "task title") |
| Session frontmatter `spec:` | Yes | `content/templates/session.md:7` |
| Commit subject | **No** (rule violated by Loaf's own example) | `commits.md:223` |
| PR title | **No** | `pre-pr-format.md:3`, `pre-pr-checklist.md:47` |
| Commit body / PR body footer | Permitted (via `Refs SPEC-XXX`-style line — not currently documented) | Inferred from `commits.md:88-108` Linear magic words pattern |
| Branch name | Currently yes (`feat/spec-010-...`) — **contradicts saved memory** | `commits.md:119` |
| CHANGELOG entries | **No** (curated draft must strip) | `commits.md:174-201` |

## Methodology

1. **Project context first.** Read `content/skills/shape/SKILL.md`, `breakdown/SKILL.md`, `implement/SKILL.md`, `orchestration/SKILL.md`, `git-workflow/references/commits.md`, `shape/templates/spec.md`, hook instruction files (`pre-pr-format.md`, `pre-pr-checklist.md`, `pre-merge.md`), validating hook (`check-commit-msg.sh` + `orchestration-validate-commit.py`), `orchestration/references/planning.md`.
2. **Pattern search.** Grep for `SPEC-[A-Z0-9]+`, `Track\s+\d+`, `PLAN-`, `commit|PR title` patterns across `content/` (29 files surfaced `track|Track`; 50+ surfaced SPEC patterns; 8 surfaced PR/commit prescription).
3. **Cross-check against saved memory.** Compared `feedback_branch_pr_granularity` (memory) to `commits.md:119` (content) — found contradiction.
4. **Verify enforcement claim.** Read `check-commit-msg.sh` end-to-end to confirm no SPEC/TASK regex.

All findings carry verified file:line citations. Confidence levels reflect inventory completeness, not interpretation.

## Recommendations

### Cleanup that can ship today (no SPEC needed)

- **R1: Fix the canonical example** — Rewrite `content/skills/implement/SKILL.md:325` to remove `SPEC-XXX` from the subject. Move it to footer or body. Estimated change: one line.
- **R2: Reconcile branch naming with feedback** — Update `content/skills/git-workflow/references/commits.md:114, 119` to drop SPEC/TASK IDs from branch examples. Match the `feat/<slug>` convention from saved memory.
- **R3: Resolve Part-vs-Track template mismatch** — Either update `content/skills/shape/templates/spec.md:70-77` to use "Track" (matching skill prose) or update the skill prose to use "Part". Pick one. **Recommend "Track"** since it is the Shape Up-rooted vocabulary already in `orchestration/references/planning.md`.

### Address in the redesign SPEC

- **R4: Codify Track scoping.** "Track" is internal vocabulary for use within SPEC docs and Linear parent/sub-issue groupings — never in commit subjects or PR titles. Spell this out explicitly.
- **R5: Document multi-PR-per-SPEC as a first-class pattern.** Add to `breakdown/SKILL.md` and `implement/SKILL.md`: a SPEC may land across multiple PRs; each PR has a descriptive subject; the SPEC↔PR mapping lives in the SPEC doc (or its Linear parent), not in PR titles. This removes the structural pressure that produced `(SPEC-007 Track 1)`.
- **R6: Define a SPEC durability lifecycle.** Add states or annotations for "implemented in v1; resuming for v2 work" so a SPEC genuinely outlives one implementation cycle. Possibly a `lifecycle:` field listing the cycles, or a `superseded-by` / `extended-by` relation.
- **R7: Define the one permitted place for SPEC references in public artifacts.** Most likely a one-line footer pattern like `Refs SPEC-024` in the PR body, mirroring Linear magic-word convention. Document explicitly that title never carries it.
- **R8: Add SPEC/TASK-ID enforcement to `check-commit-msg.sh`.** Warn (yellow) by default; opt-in to block. The rule is already documented; teach the validator to read it.
- **R9: Codify Linear Project-per-cross-SPEC-initiative + parent-per-SPEC + child-per-unit.** Currently `breakdown/SKILL.md` describes parent-per-SPEC + child-per-task but is silent on the Project layer.

### Defer / Open

- **R10: Worktree-per-SPEC ergonomics.** The user's mental model says "orchestrator stays on `main` as control room, one worktree per concurrent SPEC". Loaf has no command surface for this today. Probably scope for a separate SPEC after the conventions one lands.

## Sources

- `content/skills/git-workflow/references/commits.md` (lines 119, 174-201, 223-231) — **High**, primary policy doc
- `content/skills/foundations/scripts/check-commit-msg.sh` (lines 34-69) — **High**, validating script
- `content/hooks/pre-tool/orchestration-validate-commit.py` (lines 114-126) — **High**, hook dispatcher
- `content/hooks/instructions/pre-pr-format.md` (line 3) — **High**
- `content/hooks/instructions/pre-pr-checklist.md` (line 47) — **High**
- `content/skills/implement/SKILL.md` (line 325) — **High**, canonical-example-violates-rule evidence
- `content/skills/breakdown/SKILL.md` (lines 213, 240) — **High**, Linear parent/sub title formats
- `content/skills/shape/SKILL.md` (lines 36, 85, 131, 151) + `shape/templates/spec.md` (lines 7, 17, 68-77) — **High**, shape vocabulary + template
- `content/skills/orchestration/references/planning.md` (lines 73-103) — **High**, Track-vocabulary origin
- `content/skills/orchestration/references/specs.md` (lines 36-44, 140-144) — **High**, splitting convention
- Saved memory: `feedback_branch_pr_granularity` — **High**, user-stated convention (contradicts content)
- GridSight session wrap-up (referenced by user, not read in this pass — could verify additional context if needed) — **Medium**

## Open Questions

These are the questions to take into the brainstorm pass — not to answer here.

- **Q1: Track vs Part vs Slice — which name wins, and what's its scope?** "Track" has lineage but is also overloaded ("audio track", "code track", project-management "track"). "Part" is what the template already uses. "Slice" maps cleanly onto multi-PR-per-SPEC mental model.
- **Q2: One PR per Track, or N PRs per Track?** GridSight ran one PR per Track. Is that prescriptive, or a coincidence? Multi-PR-per-Track is also reasonable.
- **Q3: What signals "this PR is the n-th in a series for this SPEC" if the PR title can't say so?** Options: a `Refs SPEC-XXX` footer; PR description preamble (`Part 2 of N for the deployment identity SPEC`); the SPEC doc's progress section; Linear sub-issue labels; nothing (descriptive title is enough — let SPEC↔PR mapping live in the SPEC doc).
- **Q4: Sub-spec splitting vs priority ordering — when does each apply?** The current docs offer both with no decision rule.
- **Q5: SPEC durability — what marks a SPEC as "alive across v2/v3 cycles"?** Status field? Separate `lifecycle:` field? Inline section listing implementation cycles?
- **Q6: Linear Project layer — required, optional, or convention?** Current `breakdown/SKILL.md` is silent.
- **Q7: Migration path for projects mid-stream.** GridSight has SPEC-007 through SPEC-010 partially shipped. What changes apply only to new SPECs vs. retroactive cleanup?
- **Q8: Does the enforcement hook block or warn?** Blocking is teeth, warning is forgiving. Both are reasonable for different users; the SPEC should pick or make configurable.

# Devil's advocate report — mattpocock/skills vs the 20 decisions

Evidence (Decision 18): verbatim final report from the adversarial lens of the three-agent review of github.com/mattpocock/skills, 2026-07-06. Brief: attack the decisions, steelman the case against, rate KILL/WOUND/GLANCING; attack the meta-level; clean bills are valid findings. Adjudication: Decision 21.

---

## The unifying thesis I found

There is one pattern under most of my attacks, and Matt's repo is the perfect foil for it:

**Loaf repeatedly chooses "put it in git/the repo and govern its lifecycle with a rule or a gate," where Matt chooses "keep it out of git by construction, or let a native surface carry it."**

- HTML evidence → Loaf commits it into `research/` and archives it (D18/D20); Matt writes it to `$TMPDIR` "so nothing lands in the repo" (`improve-codebase-architecture/SKILL.md:34`, `HTML-REPORT.md:2`).
- Handoff baton → Loaf puts `handoff.md` in the change folder and relies on a delete-at-ship rule (D18); Matt saves handoffs "to the temporary directory of the user's OS - not the current workspace" (`handoff/SKILL.md:8`).
- Reasoning log → Loaf keeps every archived change folder in-tree forever (D2); Matt's reasoning log is closed issues + a deduplicated `.out-of-scope/` KB, out of the working tree.
- Deferrals → Loaf builds harvest + spark capture + publish ledger + two-lane rule (D17); Matt writes one markdown file per rejected concept, or files a fresh issue.
- Invariant enforcement → Loaf builds a bespoke `loaf change check` CLI (D7/D14); Matt explicitly *rejected* adding a check code path (`.out-of-scope/setup-skill-verify-mode.md`).

Matt's README states the counter-thesis directly: *"Approaches like GSD, BMAD, and Spec-Kit try to help by owning the process. But while doing so, they take away your control and make bugs in the process hard to resolve. These skills are designed to be small, easy to adapt, and composable."* (`README.md:17-19`). His shipped `implement` skill is **9 lines** (`implement/SKILL.md`). His `tdd` skill was actively *shrunk* to reference-only because the workflow steps "were largely restating the loop" (`.changeset/tdd-reference-only-seams.md`). That is the mirror Loaf's 595-line/20-decision doc should be held up against.

---

## Decision-level attacks

### D3 — "Draft PR opens at shaping, by default" — **KILL** (of the default; the capability survives)

**Steelman against.** A draft PR opened at shaping has, by the doc's own account, *no code and no review*. Decision 15 explicitly says review does **not** happen on the draft during shaping — "anchored comments on a still-churning document only orphan," so churn stays conversational + journal until the draft→ready flip. So during the entire shaping phase the draft PR's only function is to be a row in `gh pr list`. But Decision 14 already gives the CLI "listing branch-local Changes." The one justification for the default (a cross-branch index) is **redundant with a capability Loaf is building anyway** — while the draft PR imposes CI runs, review-request routing, and notification noise on every shaping session.

**Ground it.** Matt never opens a PR to plan. His flow puts the *plan* in the issue tracker (`to-prd/SKILL.md` publishes a PRD to the tracker; `wayfinder` puts the whole map in the tracker as issues) and reserves PRs for code. GitHub's draft-PR primitive is built around a diff; a PR whose diff is only `change.md` is a docs-PR cosplaying as a planning surface. Matt's `wayfinder` is the direct counter-design for "work in flight, needs cross-session/cross-person visibility" — and he uses **child issues + native blocking edges + assignee-as-claim + a frontier query** for it, not draft PRs.

**Failure scenario.** Solo dev shapes three ideas in a week → three draft PRs, three CI pipelines firing on doc-only branches, three sets of review-request nudges, and a `gh pr list` polluted with things that are 20% formed. The "index" becomes noise, and the dev learns to ignore it — the exact fate the doc fears for rotting lifecycle verbs.

**Verdict.** The draft-PR-at-shaping *default* should flip to **off/opt-in** (open a draft only when you actually want early external eyes), with `loaf change list` as the real index. The capability is fine; the default posture is the thing to change.

---

### D17 — "The package carries deferrals; ship harvests them" (harvest step + publish ledger + two-lane rule) — **WOUND**, KILL on the sub-machinery

**Steelman against.** This is the doc's most-built decision for its least-exercised ceremony. The doc's own governing principle (Background, and repeated in the Critique Gate) is: *"if a command or state cannot name the ceremony that uses it, do not build it yet."* Harvest's ceremony is **ship** — and ship's harvest step is *not in this pilot*; it's routed to the `spec-conversion-and-guidance-sweep` follow-up. `archive` was deferred for precisely this reason ("a command that cannot yet name its ceremony is not built yet"). Yet D17 fully specifies the harvest step, the `backend_mappings` publish ledger, the two-lane rule, and tracker-election config *now*, before the ceremony that would exercise or falsify any of it exists. By the doc's own rule, that is premature specification.

**Ground it.** Matt solves the same problem with almost nothing. Deferrals/rejections go to `.out-of-scope/<concept>.md` — one file per concept, deduplicated during triage (`triage/OUT-OF-SCOPE.md`). And he draws the exact distinction Loaf's "two-lane rule" draws, in one sentence: *"Avoid referencing temporary circumstances ('we're too busy right now') — those aren't real rejections, they're deferrals"* (`OUT-OF-SCOPE.md:68`). No ledger, no harvest step, no federation. His 18 pending `.changeset/*.md` files show the same instinct: pending/deferred work is *just a markdown file in git*, one per atomic change, doubling as changelog input.

**Failure scenario.** The publish ledger and two-lane semantics get designed, argued, and written into the contract now; then the ship ceremony gets built later and its real shape doesn't match — so the D17 spec is reworked or quietly ignored, becoming exactly the "unused lifecycle verb that rots" the doc warns against.

**Verdict.** Keep the genuinely useful **two-lane insight** (abandon-surviving work → capture now; change-adjacent → ride the doc's Follow-ups). Defer the publish ledger + harvest mechanics to the ship follow-up that owns the ceremony — as the doc's own principle demands.

---

### D2 + D19 — "Archive over delete" / "Finalize merges deltas into the spec corpus" — **WOUND**

**Steelman against.** These two decisions collide. D19 says finalize *extracts everything durable*: behavior deltas → `docs/specs/`, durable `research/` → `docs/knowledge/`, `handoff.md` deleted. So by construction, what remains in the archived folder is the **residue** — `notes.md` scratch, the non-durable `research/` HTML, and a change.md whose durable content now lives in specs. D2 then commits that residue into the working tree *forever*, and the "temporary vs durable" line the doc leans on is exactly what D19 already honored by promotion. Archiving the residue in-tree is keeping the part you decided wasn't worth keeping.

**Ground it.** Matt's durable knowledge lands in `CONTEXT.md`/ADRs/specs (`domain-modeling/SKILL.md`), and the *reasoning log* is closed issues + the deduplicated `.out-of-scope/` KB — out of the working tree. Nothing accumulates a growing pile of stale planning folders in the repo. D2's stated rationale (squash merges collapse branch history) is real, but the merge commit that lands the finalize *is* the retrievable archaeology; you don't need the folder to persist in `HEAD` to retrieve it.

**Failure scenario.** After 40 changes, `docs/changes/archive/` is 40 stale folders of post-finalize residue that every `grep`, every agent `Explore`, and every human `ls docs/` now wades through — precisely the entropy Matt's `improve-codebase-architecture` exists to fight.

**Verdict.** Reconcile D2 with D19: once finalize promotes durables, either delete the folder (the merge commit is the archive) or archive out-of-tree. In-tree `archive/` should hold, at most, changes that were *abandoned before finalize* — where nothing was promoted.

---

### D20 + D18 — "Contracts in markdown, evidence in HTML in `research/`" / ephemera in the change folder — **WOUND**

**Steelman against.** The markdown-vs-HTML split is right, but the *placement* contradicts the very sources cited to justify it. Loaf commits HTML into `research/` and then archives it with the folder. Thariq's cited caveat is *"HTML diffs are noisy and hard to review compared to Markdown"* — which is an argument against **versioning HTML in the repo at all**, not just against using it for contracts. The cited practitioners honor that by keeping HTML *out* of the tree: Matt writes the architecture report to `$TMPDIR` and opens it (`improve-codebase-architecture/SKILL.md:34`); his handoff goes to the OS temp dir (`handoff/SKILL.md:8`); his `prototype` is "throwaway... delete or absorb when done" (`prototype/SKILL.md:26`). None of the three sources Loaf cites for D20 (Thariq, Delba, and Matt's own practice) commit rendered HTML into the repo.

**Ground it.** Same as above, plus D18's "ephemeral by rule" (`handoff.md` deleted at ship) is strictly weaker than Matt's "ephemeral by location" (temp dir). "By rule" fails on the unhappy path: a change abandoned without a ship, or a ship that skips the delete, strands `handoff.md` and noisy `research/` HTML in `archive/` forever.

**Failure scenario.** `git log -p` and PR diffs on any implementation branch that touched `research/` are polluted with thousands of lines of un-reviewable Tailwind/Mermaid HTML — the noise Thariq warned about, imported into the exact surface (the diff) the split was supposed to protect.

**Verdict.** Keep HTML evidence out of the versioned tree by construction (temp dir, or a git-ignored `research/`), promoting only the durable *findings* (as markdown/knowledge) at finalize. Make ephemera ephemeral by location, not by a delete rule.

---

### D7 + D12 + V1 — "The pilot gate bans status-like frontmatter" — **WOUND**

**Steelman against.** The first gate Loaf chose to ship polices a field class that the design already makes impossible to produce. If the template has no `readiness:`/`status:` field and the shape skill never writes one, then V1 catches nothing, ever — it's a regression guard against a crime the template forecloses. As a *proof that gates-derived-done works* (D7's stated purpose, the "convergence-test pattern at birth"), it's near-tautological: it validates plumbing on a null input rather than exercising judgment. The decision that actually carries information — V2's derived-executability report — is the one demoted to non-failing.

**Ground it.** Matt made the opposite bet *and wrote down why*: `.out-of-scope/setup-skill-verify-mode.md` rejects adding a `--verify` flag or check skill because it "would duplicate work the existing setup skill already handles in conversation... Adding a flag or a sibling skill would split the surface area of a feature that's already expressible through the natural-language entry point." His determinism comes from leading words + checkable completion criteria in the skill, not a bespoke gate (`writing-great-skills/SKILL.md`: "A skill exists to wrangle determinism out of a stochastic system"). writing-great-skills' **no-op test** applies to V1: does it change behavior versus the default? On the happy path, no.

**Honest counter (why it's a WOUND, not a KILL).** Loaf's Background premise — "skills are weak at invariants; the CLI is better at deterministic, repeatable operations" — is legitimate, and `loaf change check` is genuinely tiny. Folder-naming/identity-mismatch checks (also in V1) *do* catch real drift that a prompt would miss. The status-frontmatter ban is the weak sub-clause.

**Verdict.** Make the *executability report* (V2) the pilot gate that proves gates-derived-done — it's the check that varies with real input. Keep the structural checks (naming, identity). Treat the status-frontmatter ban as cheap insurance, not as the flagship proof.

---

### D13 + D14 — Rigid section contract + two-tier check — **GLANCING** (survives, eyes open)

**Steelman against.** The fixed tail (Implementation Units + Verification Contract + Definition of Done) is heavier and more rigid than any comparator, including the two the doc cites. Matt's `to-prd` template has *no* verification-contract section — testing is prose ("Testing Decisions"), and Gokul Rajaram's spec (also cited) is lighter still. The rigidity is load-bearing *only* for the CLI check (V2 derives executability from the fixed names) — so D13's rigidity inherits whatever doubt attaches to D7/D14's CLI. Pull the "deterministic gate beats prompt check" thread and D12→D13→D14 wobble together as a dependent chain.

**Why it survives.** The "flat product / contained planning / fixed tail" shape is internally coherent, and free-form `###` subsections inside the Planning container is a genuinely nice concession to the "don't over-structure what the model handles anyway" principle. It's defensible *if* the CLI check is kept. Note the coupled-fate, don't kill it.

---

### D5 — "Breakdown folds into shape" — **WOUND** (right for small/solo, under-powered for the case the doc admits exists)

**Steelman against.** Matt keeps decomposition as a *separate, first-class* artifact and clearly found value there: `to-issues` produces "independently-grabbable" vertical-slice issues with `Blocked by` dependency wiring, quizzed with the user for granularity, published in dependency order (`to-issues/SKILL.md`). `wayfinder` goes further with a frontier query over native blocking edges so "the human sees what's takeable at a glance." Loaf collapses all of that into in-doc "Implementation Units" that are explicitly "not tracked entities" with no IDs and no dependency graph. That's fine for one dev doing one PR — but the doc *itself* admits "a larger Change may span multiple PRs," and Loaf's orchestration model spawns parallel implementer agents. For parallel/AFK/multi-PR work, bullet-list units with no IDs and no blocked-by edges are strictly weaker coordination than either Matt artifact.

**Failure scenario.** A large Change spans 4 PRs across 2 parallel agent sessions; the in-doc unit list can't express "U3 blocks U4" in a way any tool can query, so sequencing lives only in prose in one file two agents are both editing — the collision class the whole model was trying to escape, reintroduced at the unit layer.

**Verdict.** Fold breakdown into shape for the small-change common case (correct), but the doc needs an explicit escape hatch for the multi-PR/parallel case it already acknowledges — a wayfinder-style frontier is the proven design, and Loaf has `EnterWorktree` primitives to build it.

---

## Meta-level attack

**Is the 20-decision doc over-shaped? Yes — this is the headline finding.** The deliverable (U1–U3) is: restructure this doc, extract a template, ship two CLI subcommands. Matt ships surface of that size as **one changeset** (`.changeset/research-skill.md` adds a whole skill + README + plugin.json + docs page in ~6 lines of description). Loaf spends 595 lines, 20 decisions, and spins out **5 follow-up Changes** to get there. And the doc is visibly *accreting*: decisions 1–8 on 2026-07-05, 9–16 later that day, 17–20 the next — 12 decisions added after the first 8. writing-great-skills names this exactly: **sediment** ("stale layers that settle because adding feels safe and removing feels risky") and **sprawl**. Matt's counter-discipline is to *delete* (removed `caveman`/`zoom-out` as "went unused in practice," CHANGELOG 1.0.0; shrank `tdd`). The doc has a Cut list, which is good — but it has no pruning pass on its own decisions.

**Too ceremony-heavy for a solo dev? Yes.** The user is solo (per project context). Draft-PR-at-shaping, harvest→ledger→federation, two-tier check with a `--require-executable` CI gate, `reviews/` packets, round-summary PR comments, publish `backend_mappings` — this is team-scale process. Matt's solo path is grill → (local-markdown issues or none) → `implement` → `code-review` → commit. The README indictment lands: process-ownership "takes away your control and makes bugs in the process hard to resolve."

**Is "readiness is derived, therefore lighter" actually true? Partly a sleight.** The doc critiques CE's completeness-field and declares "no field at all" — but the *state still exists*. It's just distributed across (a) PR draft/ready state + (b) required-section presence + (c) a CLI to reassemble them. Eliminating the field didn't eliminate the state; it spread it over three surfaces and built machinery to read it back. Whether that's lighter than one honest `readiness:` line is genuinely contestable, and the doc asserts the win rather than proving it.

**Is dogfooding-while-designing producing self-confirmation? Yes, structurally.** D1 makes the doc the pilot of its own contract — elegant, but a sample of one authored by the believer cannot disconfirm the bet. H3 ("a subsequent Change is shaped from the template without friction") is judged by the same author who wrote the template. The external-inputs.md leans on "this Change already dogfooded the pattern" as evidence *for* the pattern — circular.

**Survivorship bias in the external inputs? Strongly.** Every Source Input is a "validates" or "converges." The SDD Blueprint's "independent convergence" (D19) is the load-bearing claim — but it was synthesized from a NotebookLM notebook the user *curated* from process-heavy methodologies (Shape Up, Spec Kit, Compound Engineering, RAC/Lore). Convergence from sources you selected because they share your priors is resonance, not independence. The Codex review is cited as "external" but it checked "branch integrity, source integrity, scope trim, decision provenance" — process hygiene, not the core bet. **Not one cited input argues against the model.** mattpocock/skills is the first genuinely adversarial input in this whole exercise, and it lands squarely opposite (small/composable/anti-process-ownership, HTML-out-of-tree, no bespoke check CLI, deferrals-as-a-file, decomposition-kept-separate). That asymmetry — 8 confirming sources, 0 disconfirming, until an outsider is deliberately introduced — is the tell.

---

## Clean bills (no manufactured disagreement)

- **D9 (index not `artifact_bodies`)** — sound and well-motivated by the SPEC-056 drift history; forecloses the body/render drift class by construction. Survives cleanly.
- **D10 (cite journal entries by ID)** — append-only stable IDs can't go stale. Correct.
- **D4 (coexistence by conversion; conversion strictly before sweep)** — proper migration discipline; aligns with Matt's `deprecated/` bucket and "no stranded work." Survives.
- **D16 (the artifact is a Change; "spec" = durable post-impl contract)** — reasonable naming resolution.
- **D6 (noun CLI `loaf change` / verb skill `/shape`)** — coherent with existing Loaf grammar. (Footnote: Matt has *no CLI at all* — determinism from leading words + native tracker semantics. Loaf has already committed to a CLI, so this is out of scope to relitigate, but it's worth knowing an entire high-functioning skills repo disproves the necessity.)
- **D11 (scope trimmed to core)** — genuinely good, and ironically the most Matt-aligned decision in the set. The irony: it's stated in a 595-line doc.

---

## If I had to change three things

1. **Flip D3's default to opt-in.** `loaf change list` is the index; a draft PR is for when you want early eyes, not the default cost on every idea.
2. **Get ephemera and HTML out of the versioned tree (D18/D20), and reconcile D2 with D19.** Match the sources you cite: temp dir or git-ignored `research/`; after finalize promotes durables, delete the residue folder — the merge commit is the archive.
3. **Defer D17's harvest/ledger/two-lane mechanics to the ship follow-up that owns the ceremony**, per the doc's own "name the ceremony or don't build it" rule. Keep only the two-lane *insight* now.

And one meta-move: before implementing, run the doc through Matt's own `writing-great-skills` no-op test, decision by decision — "does this change behavior versus the default the model/template already produces?" Several decisions (V1's status ban, parts of D13's rigidity) will fail it.

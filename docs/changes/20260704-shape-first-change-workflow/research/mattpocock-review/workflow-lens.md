# Workflow-process report — mattpocock/skills vs the Change model

Evidence (Decision 18): verbatim final report from the workflow-process lens of the three-agent review of github.com/mattpocock/skills, 2026-07-06. Brief: map every workflow-relevant idea against the 20 decisions, all up for debate; rate challenge strength. Adjudication: Decision 21.

---

## 1. What his repo is, and why the framing matters before any mapping

Matt Pocock's `mattpocock/skills` is a set of **small, composable, deliberately un-opinionated** engineering skills. Its README states the thesis outright: "Approaches like GSD, BMAD, and Spec-Kit try to help by owning the process. But while doing so, they take away your control and make bugs in the process hard to resolve. These skills are designed to be small, easy to adapt, and composable." (`README.md`)

That is the single most important context for this evaluation: **Matt's repo is philosophically the anti-Loaf on process ownership.** Where Loaf's own executive report identified "verification contracts plus gates-derived done" and "intake, continuity, enforcement" as its differentiators, Matt deliberately refuses to own the process at all. So almost none of his skills "challenge" Loaf's machinery in the sense of "Loaf should have less machinery" — he simply declined to build it. The interesting challenges are narrower and sharper: places where he *did* solve a problem Loaf also has, and solved it differently.

His actual process is a chain, documented in `skills/engineering/ask-matt/SKILL.md`:

```
grill-with-docs → [prototype detour, bridged by handoff] → to-prd → to-issues
  → implement (drives tdd) → code-review → commit
    on-ramps: triage, diagnosing-bugs
    upkeep:   improve-codebase-architecture
```

This maps almost one-to-one onto Loaf's `brainstorm/shape → breakdown → implement → ship`, which makes the comparison unusually clean.

A second framing point: Matt's flow assumes **AFK parallel agents grabbing independent issues from a tracker**, and it is governed by an explicit "smart zone" token budget (~120K). Loaf's pilot assumes **one agent, one context, one small change**. Several of the sharpest disagreements reduce to that difference of assumed scale, so I flag it repeatedly below.

## 2. Inventory: every workflow-relevant skill, mapped

| His skill (file) | What it is | Loaf decisions it touches |
|---|---|---|
| `to-prd/SKILL.md` | Synthesize conversation into a PRD (Problem/Solution/User Stories/Impl Decisions/Testing Decisions/Out of Scope). **No interview** — that already happened. Publishes to tracker with `ready-for-agent`. | D5, D13, D19 |
| `to-issues/SKILL.md` | Break a PRD into **tracer-bullet vertical slices**, each a tracked issue with acceptance criteria + `Blocked by`. Quiz user on granularity. | D5, D13, No-Go "no task entity" |
| `implement/SKILL.md` | Drive `/tdd` at pre-agreed seams; typecheck+test; `/code-review`; commit. | D14 gates, Verification Contract |
| `tdd/{SKILL,tests,mocking}.md` | Red-green vertical slices at **pre-agreed seams**; bans **tautological**, **implementation-coupled**, and **horizontal-slicing** tests. | D13/D14 tail, V1–V3 |
| `code-review/SKILL.md` | **Two-axis** review (Standards + Spec) as **parallel sub-agents**, aggregated side-by-side, **never reranked across axes**. Standards carries a Fowler smell baseline. | D15, D14 |
| `codebase-design/{SKILL,DESIGN-IT-TWICE,DEEPENING}.md` | Deep-module vocabulary; **"design it twice"** parallel sub-agents each given a radically different constraint; deletion test; "one adapter = hypothetical seam, two = real." | Planning Contract (D13), Critique Gate, spike |
| `grilling` + `grill-me` + `grill-with-docs` | Relentless **one-question-at-a-time** interview, recommended answer per question; `grill-with-docs` writes glossary + ADRs **inline as decisions land**. | shape interview, D19 |
| `research/SKILL.md` | Background agent, **primary sources only**, cited markdown file in repo. | D18 `research/`, D10, D20 |
| `prototype/{SKILL,LOGIC,UI}.md` | Throwaway code answering **one question**; question decides shape; capture-the-answer is the only durable output; isolate validated logic so it *can* be lifted. | Spike step, D18 |
| `improve-codebase-architecture/{SKILL,HTML-REPORT}.md` | Scan for deepening opps → **self-contained HTML report** (Tailwind+Mermaid, before/after diagrams, strength badges) → grill the chosen one. | **D20**, D18, refactor-deepen |
| `domain-modeling/{SKILL,ADR-FORMAT,CONTEXT-FORMAT}.md` | Active glossary/ADR discipline; ADR gate = hard-to-reverse **and** surprising **and** real-tradeoff; CONTEXT.md is glossary-only. | D19, D16 |
| `triage/{SKILL,AGENT-BRIEF,OUT-OF-SCOPE}.md` | Move raw incoming work through a role state machine; **verify the claim before grilling**; `.out-of-scope/` KB for rejections (dedup + memory); **deferrals ≠ rejections**. | **D17**, D3/D15, D12 |
| `handoff` + `claude-handoff` | Compact conversation → handoff doc **in OS temp dir (not the repo)**; "suggested skills" section; reference other artifacts, don't duplicate. | **D18**, D10 |
| `in-progress/wayfinder/SKILL.md` | Plan work too big for one session as a **shared map of investigation tickets**; map is index-not-store; **fog-of-war vs out-of-scope**; sized to one ~100K session; refer by name not id. | **No-task-entity No-Go**, D9/D10, D13, D17 |
| `setup-matt-pocock-skills/*` + `issue-tracker-local.md` | Per-repo config: **explicitly choose** tracker (GitHub/GitLab/local-md/other), labels, domain layout. | **D17**, D6 |
| `ask-matt/SKILL.md` | Router; documents the chain and **"smart zone" context-hygiene** rule. | **D5**, D6, D8 |
| `.changeset/` + `config.json` + `CHANGELOG.md` | Uses **Changesets**: one `.md` per change (semver + human note), consumed into CHANGELOG at release. | Changesets/release decision, D19 |
| `diagnosing-bugs/SKILL.md` | Build a **tight red-capable feedback loop before hypothesizing**; falsifiable hypotheses; regression test only if a correct seam exists. | Verification ethos, debugging |
| `git-guardrails` + `setup-pre-commit` | Hook blocks dangerous git (push/reset/clean); Husky pre-commit (lint+typecheck+test). | Enforcement hooks, confirm-before-push |
| `deprecated/{qa,request-refactor-plan}` | Retired: conversational issue-filing; heavyweight "plan of tiny commits" doc. | Meta: shedding heavyweight planning |
| `in-progress/writing-shape/SKILL.md` | Article-writing ("exploit: exploring done, mine the pile, grow block-by-block, append as you go"). | Metaphor for D1 "grows toward executability" |

## 3. The challenges that actually matter, rated

I rate each: **Real weakness** (exposes something Loaf got wrong or left undesigned) / **Conscious tradeoff** (Loaf chose the other side on purpose) / **Context difference** (his users differ).

### C1 — Durable docs *during* the work, not only after. Challenges D19. **Real weakness (partial), context-mitigated.** `grill-with-docs` and `domain-modeling/SKILL.md` write the domain **glossary (CONTEXT.md) and ADRs inline as decisions crystallize** — "Don't batch these up — capture them as they happen." His rationale (README §2, and the CONTEXT.md tip): a living glossary is *consumed by* the work — it makes variables/files named consistently, the codebase navigable, and "the agent spends fewer tokens on thinking."

Loaf's D19 defers **all** durable outputs to finalize, relying on journal `decision` entries (D10) during flight. That is right for *decision records* — a journal entry citing a decision is a fine substitute for an ADR-you-write-later. But it is **not** a substitute for a **living glossary**: Loaf's model has no per-Change artifact that is a domain reference *read during* the work. The change.md is a contract, not a glossary; the journal is an event log, not a reference.

Why context-mitigated: Loaf-the-framework already has this — its `.claude/CLAUDE.md` and the repo's `CONTEXT.md` serve as the glossary. Matt's users are building product apps with fresh domains where the glossary must be born during shaping. **Recommendation:** split D19's "durable after" rule by artifact type. Decision records → defer to finalize (keep D19). A living glossary → allow a Change to create/update it inline, as the one deliberate exception. This is the sharpest single gap the repo exposes.

### C2 — For work beyond one context, you need entities. Challenges the "no task entity" No-Go. **Real gap, consciously deferred.** `wayfinder/SKILL.md` is the designed counter-model to Loaf's "implementation units are in-doc guides, not tracked entities." When work exceeds one session, wayfinder makes each unit a **claimable, resolvable, blockable child issue** — i.e., an entity — because parallel AFK sessions need to (a) claim a unit so others skip it, (b) see the unblocked frontier rendered in the tracker's native dependency UI, and (c) never lose a resolution. `ask-matt` supplies the reason: the **"smart zone" (~120K tokens)**; past it, "don't push on degraded — handoff and continue in a fresh thread," and each `implement` starts fresh per issue.

Loaf concedes the shape of this ("A larger Change may span multiple PRs; the Change remains the shared context") but has **not designed the sub-PR, multi-session coordination surface.** `gh pr list` (D3) indexes work at PR granularity; it says nothing about who is working which *unit within* an in-flight Change, or which units are unblocked.

Crucially, wayfinder is *not* a refutation of D9/D10 — it **validates** them: "The map is an **index**, not a store... a decision lives in exactly one place — its ticket — so the map never restates it, only gists it and links." That is D9 (docs_index, never body copy) and D10 (cite by identity) applied to the planning artifact. So the reconciliation is clean: **change.md is the map**; when a Change goes parallel, its implementation units need to graduate into claimable entities, while change.md stays the index. **Recommendation:** keep "no entity" for small single-context Changes (correct for the pilot); design the "Change exceeds the smart zone" escape hatch as wayfinder-style claimable units indexed by change.md. This is deferred, not wrong — but it's the biggest undesigned area.

### C3 — Breakdown as a separate step behind a context boundary. Challenges D5. **Conscious tradeoff, moderate.** Matt keeps `to-prd` (synthesize spec, no interview) and `to-issues` (decompose) as **distinct skills separated by a context boundary**, because issues are independent and each should be implemented in a **fresh context** (`ask-matt` "Context hygiene"). D5 folds breakdown *into* shape.

The tension is not "separate vs folded" in the abstract — it's the **token budget**. Loaf's `/shape` already does a lot (gather + interview + compare alternatives + bound + critique gate + choose artifact). Adding decomposition risks a shape skill that blows the smart zone on non-trivial changes and then decomposes while degraded. Matt's separation is a direct answer to that failure mode. **Recommendation:** fold breakdown into shape as D5 says, but as a **distinct, skippable step** that can be run in a fresh context — not an inseparable phase. For small changes, one context; for larger, decompose fresh. The "smart zone" concept itself is worth importing into Loaf's shape/implement guidance — Loaf's model is currently silent on context budget.

### C4 — Two-axis review is the mechanism D15 is missing. **Adopt, not a challenge.** D15 says *where* review happens (conversation churns, PR reviews at the draft→ready flip, journal remembers) but not *what* the review checks. `code-review/SKILL.md` supplies it: **Standards** (repo conventions + a fixed Fowler smell baseline) and **Spec** (does the diff faithfully implement the originating contract — missing requirements, scope creep, wrong implementations, *quote the spec line*), run as **parallel sub-agents so they don't pollute each other**, aggregated side-by-side, **never reranked across axes** ("stops one axis from masking the other"). The Spec axis *is* Loaf's contract review — and it reads the change.md as the acceptance contract, reinforcing D3/D15. **Recommendation:** adopt two-axis parallel review as the concrete mechanism for D15's two review moments. Loaf's ship/review currently has no equivalent structural discipline.

### C5 — HTML evidence: ephemeral vs kept. Challenges D18/D20. **Conscious tradeoff.** `improve-codebase-architecture` writes its HTML report to the **OS temp dir "so nothing lands in the repo,"** regenerated on demand. Loaf's D18 keeps HTML in `research/` as **write-once evidence dispositioned at finalize**, and D20 routes "rich evidence — explorations, decision aids, architectural explainers" there. Matt's is the exact practice D20 cites (via the Thariq/Delba inputs) but with the **opposite persistence choice**: a review aid you consume once and discard, not an artifact you keep.

Both are right for different evidence. An architectural walkthrough generated as a review aid at the draft→ready flip is a consume-once regenerable — cluttering `research/` and archiving it forever is waste. A spike finding worth promoting to `docs/knowledge/` is not. **Recommendation:** D20 should not force *all* HTML into `research/`. Allow two lanes: `research/` for evidence worth keeping/promoting, and temp/uncommitted for pure regenerable review aids. Also note a subtler point: Matt's `research/` **findings are markdown, not HTML** (`research/SKILL.md`) — citation-heavy fact-gathering is diffable and belongs in markdown. D20's "evidence in HTML" over-rotates; the real split is **rich/visual → HTML, citation/fact → markdown**.

### C6 — Verification-quality guards. Enriches the Verification Contract. **Adopt.** `tdd/{SKILL,tests}.md` names three failure modes Loaf's Verification Contract has no guard against: **tautological** ("the assertion recomputes the expected value the way the code does... passes by construction and can never disagree with the code"), **implementation-coupled**, and **horizontal-slicing** ("bulk tests verify *imagined* behavior"). Loaf's V1–V3 and the deferred SPEC-017 binary-R format specify criteria but never warn that a criterion can be **vacuous**. A verification criterion whose check restates the implementation is the same disease as D12's banned status field: a record that can't disagree with reality. **Recommendation:** bake "criteria must have an independent source of truth; a tautological criterion is a violation" into the Verification Contract format when SPEC-017 revives.

### C7 — Out-of-scope KB. Enriches D17. **Novel addition.** `triage/OUT-OF-SCOPE.md` distinguishes three fates for non-shipped work, where Loaf's D17 has two lanes (spark-now vs ride-the-package): **deferrals** (temporary, become sparks) vs **rejections** (durable, "avoid referencing temporary circumstances — those aren't real rejections, they're deferrals"), and rejections get a **deduped, one-file-per-concept KB with reasoning**, checked during triage to avoid re-litigating. This change.md itself has a **"Cut (explicitly rejected)"** list (numeric CR IDs, task entity, bidirectional sync, lifecycle state machine) that, under current D17, would simply vanish into the archive. **Recommendation:** harvest a Change's "Cut" list to a durable rejection KB (deduped), so future shaping surfaces "we rejected this before, here's why" instead of re-arguing it. D17's harvest currently drops sparks but forgets rejections.

### C8 — The prototype skill is a finished blueprint for Loaf's deferred spike-harness. **Adopt.** Loaf's spike is defined (isolated worktree, stop at enough-unknowns, discard by default, write findings back) but unbuilt (Open Questions + spike-harness follow-up). `prototype/{SKILL,LOGIC}.md` is the mature version: **state the question first** ("a prototype that answers the wrong question is pure waste"), **the question decides the shape** (logic TUI vs UI variants), **capture-the-answer is the only durable output**, and one refinement to Loaf's "discard by default" — **isolate the validated logic in a portable pure module so it *can* be lifted** while the shell is discarded. That nuances Loaf's blanket "discard implementation output": discard the *shell*, keep the *answer* and optionally the validated core. **Recommendation:** base the spike-harness Change directly on this skill.

### C9 — Handoff belongs outside git. Mild challenge to D18. **Context difference.** Both agree handoff is ephemeral. Matt saves it to the **OS temp dir, explicitly not the workspace** (`handoff/SKILL.md`); D18 commits `handoff.md` to the branch folder then deletes it at ship. Committing-then-deleting creates branch-history noise for something never meant to reach main. But Loaf's counter is legitimate under multi-worktree/multi-agent: a temp-dir handoff isn't discoverable across worktrees or by another agent, and ADR-013 already argues branch context must live where branch context lives. **Verdict:** context difference — Loaf's parallel-agent model justifies committing it; a solo user is better served by temp. Worth noting, not changing.

## 4. Strong convergences — where Matt independently validates Loaf's decisions

These matter because independent arrival is evidence the decision is sound:

- **No IDs / refer by name** (`wayfinder`: "A wall of `#42, #43, #44` is illegible; names read at a glance") independently validates the Cut of numeric CR IDs and `SPEC-*`.
- **Index-not-store; a decision lives in exactly one place** (`wayfinder`) + **reference, don't duplicate** (`handoff`) independently validate **D9** and **D10**.
- **Explicit tracker election** (`setup-matt-pocock-skills`: "pick the place you *actually* track work," proposed from the git remote but never assumed) is verbatim **D17** ("election is explicit configuration, never tool presence — an authed `gh` does not make GitHub the tracker"). Matt even offers local-markdown as first-class for repos with no remote — Loaf's git+SQLite-only default.
- **Behavioral, durable, no-file-paths contracts** (`AGENT-BRIEF.md`, `to-prd`, `to-issues`, deprecated `qa`: "Don't reference file paths — they go stale... describe interfaces, types, behavioral contracts") validate the change-doc-is-behavioral principle under D13/D19.
- **ADR triple-gate** (hard-to-reverse ∧ surprising ∧ real-tradeoff, `ADR-FORMAT.md`) matches Loaf's architecture skill criteria.
- **The deletion test / speculative-generality smell** (`codebase-design`, `code-review`) is Loaf's Critique-Gate rule in different words: "if a command or state cannot name the ceremony that uses it, do not build it yet."
- **Changesets as release input, separate from planning docs** — Matt is a *working instance* of the exact model Loaf describes but hasn't built (`.changeset/config.json`, per-change `.md` notes consumed into `CHANGELOG.md`). Validates the "changesets are release input; Change folders are planning context" decision and hands Loaf a concrete tool (`@changesets/cli`). Note the global rule "ask before adding dependencies" applies. Interesting: his changeset entries read like **delta notes** ("Fog and out-of-scope were conflated... now two sections") — the same "delta against reality" shape as D19's finalize.
- **Deprecating `request-refactor-plan`** (a heavyweight "plan of tiny commits" doc) mirrors Loaf shedding spec/task machinery — both moved from heavy planning artifacts to composable shaping.
- **git-guardrails** (block `push`) validates never-push-without-confirmation.

## 5. Genuinely novel ideas — worth a spike, fit neither adopt nor reject cleanly

1. **Fog-of-war as the mechanic for "change.md grows toward executability."** Wayfinder's "Not yet specified" section holds questions you can *see coming but can't sharply phrase*; resolutions **graduate fog into tickets one at a time.** Loaf's D1 says the change grows toward executability but the growth is a **static** Follow-ups/Out list. Wayfinder's **dynamic graduation** (fog → unit as unknowns clear) is a better-specified answer to *how* a change matures. The "fog or ticket?" test — "can you state the question precisely now, not answer it" — is a clean guard against premature decomposition. Spike: model Loaf's Change maturation as fog-graduation rather than a fixed section.

2. **"Design it twice."** `DESIGN-IT-TWICE.md` spawns 3+ parallel sub-agents, each forced to a **radically different constraint** ("minimize interface" / "maximize flexibility" / "optimize the common caller" / "ports & adapters"), then compares on depth/locality/seam and recommends. This is a forcing function for the Planning Contract's approach/placement subsections and the "compare alternatives" step in Loaf's Observable Workflow — sharper than a single brainstorm pass. Spike: wire into shape's alternative-comparison.

3. **The "smart zone" as an explicit budget concept.** ~120K tokens as the window within which a model reasons sharply, governing when a phase must split into fresh contexts. Loaf's model is entirely silent on context budget, yet folds ever more into `/shape` and `/implement`. Worth making explicit in orchestration guidance.

4. **Recommendation-strength vocabulary** (`Strong` / `Worth exploring` / `Speculative`, `HTML-REPORT.md`) as a rating for sparks, deferrals, and research findings — a small, adoptable grammar Loaf's triage/intake lacks.

5. **The changeset entry as a bridge artifact.** Written at change-time in release-note voice, it doubles as (a) release input and (b) a mini durable-spec *delta*. This could unify Loaf's separate "changesets" and D19 "delta-merge at finalize" mechanics into one write.

## 6. Bottom line

Nothing in Matt's repo refutes the architecture of the 20 decisions — the git-canonical, index-not-body, no-status, gates-derived spine is **independently reinvented** in his best work (wayfinder, setup, agent-brief, code-review), which is strong corroboration. His repo challenges the model in exactly four places worth acting on, plus offers ready blueprints for two things Loaf has deferred:

- **Fix (real gap):** Loaf has no *living glossary* consumed during the work (C1). Split D19 by artifact type.
- **Design (deferred gap):** the "Change exceeds one context" coordination surface (C2/C3), where wayfinder's claimable-units-indexed-by-the-map pattern is the answer, and the "smart zone" is the trigger.
- **Enrich:** two-axis review as D15's missing mechanism (C4); tautological-criterion guard for the Verification Contract (C6); a rejection KB for the harvest (C7); dual-lane HTML persistence + markdown-for-citations correction to D20 (C5).
- **Blueprint (adopt for deferred work):** the `prototype` skill *is* the spike-harness (C8); `@changesets/cli` *is* the changesets decision, already running in his repo.

The one philosophical caution: Matt's minimalism ("don't own the process") is a deliberate different bet, not a critique of Loaf's machinery — but it's a standing reminder that every gate, folder member, and CLI verb must name its ceremony, which the Critique Gate and D11's scope-trim already enforce. His deprecation of `request-refactor-plan` and the fact that his heaviest artifact (wayfinder) is still `in-progress` both suggest the same lesson Loaf's own SPEC-056 taught: process weight that isn't exercised rots.

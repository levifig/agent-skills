---
title: "Report: Loaf Skill Suite — Deep Evaluation & Restructuring Plan"
type: audit
created: 2026-06-21T02:03:42Z
status: final
source: workflow
report_alias: report-loaf-skill-suite-deep-evaluation
supersedes: report-loaf-skills-deep-audit
tags:
  - skills
  - harnessing
  - sqlite-state
  - build-targets
  - taxonomy
  - triggering
  - skill-creator
---

# Report: Loaf Skill Suite — Deep Evaluation & Restructuring Plan

**Question:** What should change across all Loaf-shipped skills to tighten focus, improve
harnessing, reduce clutter/slop, and make the shipped experience reliable — going beyond the
2026-06-20 audit, using the skill-creator evaluation lens?

## How this was produced

This extends `report-loaf-skills-deep-audit` (2026-06-20). Method:

1. **Independent re-verification** of the prior report's headline claims against current source.
2. A **14-agent evaluation workflow**: 8 skill-family clusters + 4 cross-cutting deep dives
   (harness/build root-cause, session-model drift map, agent/hook layer, mechanical lint) →
   an opinionated synthesis → an **adversarial critique** that stress-tested the synthesis.
3. **Manual verification of the net-new load-bearing claims** before promoting them — which
   caught and corrected one overstatement in the audit itself (see cli-reference).

The skill-creator lens (descriptions *are* the router; progressive disclosure; anti-slop;
verb/noun fit) was applied to every skill.

Confidence: High on the mechanical/correctness findings (source-verified). **Medium on the
routing/triggering claims — they are textual-overlap inferences, not measured.** Closing that
gap is itself a recommendation (§6).

---

## 1. The one-paragraph verdict

**The taxonomy is healthy; stop trying to fix it.** Every one of the 8 family audits
independently concluded the verbs are distinct and *no merges are warranted*. The suite's real
diseases are three, and none of them is "too many skills":

1. **A half-shipped migration.** SPEC-040 made one global SQLite database the canonical
   operational store and demoted markdown in `.agents/` to compatibility/export. Exactly **one**
   skill — `wrap` — converged. Everything else still teaches the markdown-era model as primary,
   and in one case (`housekeeping`) drives a command that is now a **no-op**.
2. **A build that validates nothing.** The target transformers ship broken artifacts (the Amp
   plugin fails `node --check`), and the test suite *codifies* the breakage — it uses a fake
   `node` and asserts TypeScript syntax is present in a `.js` file. This is the root cause of
   four separate cross-target defects shipping at once.
3. **Duplicated reference mass.** ~1,600 of `orchestration`'s ~4,656 reference lines re-document
   five dedicated skills; several reference packs over-claim each other's territory in their
   descriptions. This — not skill count — is the actual clutter.

The single highest-leverage move is **#2: make the build prove its own output.** It is the
documented cause of the most damaging fact in the audit (a shipped plugin that won't parse), it
gates the credibility of every other build fix, and — extended with a content lint — it becomes
the durable guard that stops the session-drift and Claude-ism leakage from ever shipping again.
Session-model convergence is the right *second* move. Description-tuning and packaging are
real but second-order, and **must not proceed without measurement** (§6).

---

## 2. Verification of the prior report

Re-checked against current `main` (only change since the prior report: the ship/release split,
commit `af3549fc`).

### Confirmed (source-verified)
- **Amp plugin is invalid JS** — `dist/amp/plugins/loaf.js:38` `interface HookResult`;
  `node --check` fails. ✓
- **Codex hardens advisory hooks** — `dist/codex/.codex/hooks.json` emits `failClosed: true`
  for all five hooks, including advisory `validate-push` and `workflow-pre-pr`. ✓
- **OpenCode drops commands** — no `ship`/`release`/`bootstrap`/`refactor-deepen` (also
  `debugging`); gated on optional `SKILL.opencode.yaml` presence. ✓
- **Session-model split (F4), transcript-archival conflict (F5), self-logging gap (F6),
  oversized roots (F7), unwired scripts (F9)** — all confirmed.
- **Stale refs (F10):** git-workflow commit format, `database-design → infrastructure`,
  `power-systems-modeling → database-patterns`, `foundations/code-style.md` python/typescript/rails,
  `knowledge-base → CLAUDE.md`, `triage` stray fence, `shape` missing `source_sessions`. ✓

### Refuted / corrected (the prior report was wrong here)
- **"refactor-deepen links an archived SPEC-034"** — refuted. The SKILL.md citation is a prose
  concept reference and resolves; the real dangling SPEC-034 link is in `interface-design.md`.
- **"documentation-standards has a broken ADR link"** — refuted. The ADR-013 link is an
  illustrative example in a fenced block and the target exists. (The *real* defect is a
  contradictory duplicate ADR **spec** — see §5.)
- **"11 broken template links"** — refuted as a false positive: they resolve at build time via
  `shared-templates` in `targets.yaml`. Reading `content/skills/` source directly misleads.
- **"prompt+if hook gotcha"** — refuted as a live issue: zero `type: prompt` hooks remain in
  `hooks.yaml`. Latent in build code only.

### Net-new (missed by the prior report) — manually verified
- **`housekeeping` drives a dead command. (RELEASE-BLOCKING)** Following the skill does nothing:
  `loaf session housekeeping` returns `Action: "skipped"` (`cli.go:7096`, reason: *"markdown
  session housekeeping … is not run in SQLite mode"*). The real artifact scanner is the
  top-level `loaf housekeeping`, which the skill never names; the "Session Enrichment" section
  is built on `loaf session enrich <file>`, also skipped (`cli.go:7030`).
- **The build test suite codifies the bugs.** `build_test.go:31` installs a fake `node` and
  asserts (`:56`) it is *never invoked* — so `node --check` never runs on output. Worse,
  `:283` asserts the Amp `loaf.js` *contains* `const postToolHooks: Record<string, HookEntry[]> = {`
  — i.e. the test **requires** TypeScript syntax in the `.js` file. The test guarantees the
  broken plugin.
- **`librarian` is a dead agent profile** that loses its `.agents/`-only tool boundary on
  opencode/cursor (no sidecar, no frontmatter description) and is spawned by zero skills.
- **Session status enum is itself inconsistent** across three files: `active` (`session.md`)
  vs `in_progress|paused` (`sessions.md`) vs `active/stopped/done/blocked/archived` (CLAUDE.md).
- **`foundations` description over-claims three siblings** (commit conventions / documentation
  standards / security patterns) — the largest *textual* trigger collision in the suite.
- **`interface-design` now collides with the newer harness design skills** (`artifact-design`,
  `frontend-design`) over "build distinctive UI."

### Correction to *this* evaluation's own audit (caught by verifying)
- **cli-reference was OVERSTATED.** The synthesis called it "release-blocking" and claimed
  `loaf doctor` is "absent." False: it's catalogued as `loaf state doctor`
  (`cli-reference/SKILL.md:145`) — the actual command. The genuine, narrower defect: the
  canonical `loaf session` *family* (start/list/show/log/end) isn't catalogued even though the
  doc's own decision guide tells readers to run `loaf session start` (`:898`), and the
  `cliReferenceSubcommands` "session" special-case (`cli_reference.go:599`) is dead code (no
  top-level `session` entry exists to trigger it). **Quality bug, not release-blocking.** This
  is the audit's one self-caught error — and the reason §6 matters.

---

## 3. Severity-ranked fix plan

### RELEASE-BLOCKING — ship first, independently

| # | Fix | Root cause | Guard |
|---|-----|-----------|-------|
| B1 | **Make the build validate output.** Drop the fake `node`; run real `node --check` on `loaf.js` (and `node --check`/`tsc` on opencode `hooks.ts`); **delete the assertion that requires TS syntax in `loaf.js`** (`build_test.go:283`). | `build_test.go:31,56,283` | This *is* the guard — it's the keystone for B2–B5. |
| B2 | **Amp plugin → valid artifact.** Emit `loaf.ts` (mirror OpenCode) *or* strip type syntax to real JS — one source of truth. Fix the handler reading undefined `call` (param is `input`). | `build_amp.go:103, 355-405` | B1's `node --check`. |
| B3 | **Codex: stop hardening advisory hooks.** Default `failClosed:false`; parse `value=="true"`; add `blocking`/`if` to the struct + switch. | `build_codex.go:629,646,24-30,638-649` | Test: `validate-push`/`workflow-pre-pr` are `failClosed:false` in codex output. |
| B4 | **git-workflow commit format.** `SKILL.md:31,40` teach `type(scope):` but `check.go:198,249` (a `failClosed` hook, proven by `check_test.go:339`) **rejects scoped commits**. Change to unscoped `type: description`; add a note that scope is allowed in PR titles + journal entries, banned in commit subjects. | `git-workflow/SKILL.md` | Content lint: commit examples unscoped. |
| B5 | **`housekeeping` dead command.** Replace `loaf session housekeeping` with `loaf housekeeping` (`--dry-run/--sessions/--specs/--drafts`); delete the Session Enrichment section. | `housekeeping/SKILL.md` + 7 dist copies | — |

B1+B4 are the lowest-risk, highest-certainty items (a test-infra change and a 2-line doc fix with
a binary-level guard already in place) — they can go out today.

### HIGH

- **H1 — `implement` session-model rewrite (the worst offender, 361-line root).** Replace
  "MANDATORY: Create session file BEFORE any other work" (`:251,261`) with `loaf session start`;
  adopt the `wrap` model (`list/show --json` + `log` + `end --wrap`). Delete the **fabricated
  orchestration flags** (`--continue/--skip/--abort/--parallel/--dry-run` in
  `batch-orchestration.md` — none exist natively; they will make the model hallucinate commands).
  Fix the broken `linear-workflow` ref (`session-management.md:62`), the double `6.` numbering,
  and the duplicate work-type table. **Must change in the same set as the `implementer.md`
  profile** (which still demands a "session file" — see R2).
- **H2 — Transcript-archival removal (SPEC-040 scope violation).** Delete the Transcript Archival
  section (`implement/references/session-management.md:220-254`) and the `transcripts:` rows +
  layout in `orchestration` (`SKILL.md:131`, `sessions.md:73-87,206-208`). Data-handling
  liability, not a style nit.
- **H3 — OpenCode command coverage.** Drive generation off `user-invocable` (the signal used
  everywhere else), not sidecar presence. `build_opencode.go:94`. Test: every user-invocable
  workflow yields a command.
- **H4 — Cross-target harness-language transformation.** `transformMd` is a no-op for
  codex/gemini/amp; ~60–81 Claude-isms leak per target ("Claude Code", "AskUserQuestion", "Task
  tool", "TodoWrite", ".claude"). Tokenize at source (`{{HARNESS_NAME}}`, `{{INTERVIEW_TOOL}}`,
  `{{SUBAGENT_MECHANISM}}`, `{{TODO_TOOL}}`, `{{AGENTS_FILE}}`) and resolve per target; add a
  cross-target content lint that fails the build on raw Claude-isms (the durable extension of B1).
- **H5 — `orchestration` → thin hub (the real clutter cut).** Collapse the ~1,600 reference
  lines that duplicate `council`/`shape`/`breakdown`/`research` into pointers; stop shipping the
  10 unwired helper scripts to all 6 targets. Converge its session model (H1 family).
- **H6 — `brainstorm` SQLite reconvergence.** It treats `.agents/drafts/*.md` as canonical while
  its consumer `triage` reads `loaf spark list`/`loaf brainstorm list` from SQLite. Capture via
  `loaf spark capture`; the draft doc becomes export. Fix the broken `strategy/references/` link
  (`:70`) and delete the false "`/idea` scans brainstorm docs" claim (`:63` — that's triage's job).
- **H7 — `architecture` = single ADR source of truth.** `documentation-standards/references/documentation.md:41-74`
  defines a *second*, conflicting ADR spec (4 statuses vs 5; `ADRXXX` vs the repo's real
  `ADR-NNN`); `architecture/SKILL.md:201` even routes readers *at* the stale doc. Strip the ADR
  spec from documentation-standards; fix the cross-link.

### MEDIUM

- **M1 — Self-logging, done right (NOT 130 edits as a standalone workstream).** Only 2 of ~19
  user-invocable workflows log invocation first. But the *only proven functional failure* is that
  `wrap`'s "no housekeeping this session" nudge (`wrap:100`) mis-fires because `housekeeping`
  never writes its `skill(housekeeping)` marker. **Fix that one marker now (with B5); batch the
  rest into the convergence pass.** The other "violations" log useful `decision()` outcomes — keep
  them. Fix the AGENTS.md exemplars (`shape`, `housekeeping`, `implement`) first — they don't
  follow their own rule, which means the rule or the examples is the real bug (§6e).
- **M2 — `research` de-scope.** Delete its "Brainstorming" mode (collides with `brainstorm`) and
  "Vision Evolution" mode (third skill claiming "update VISION", after `reflect`/`strategy`).
  Reduce to State Assessment + Topic Investigation. Fix stale `docs/specs/` → `.agents/specs/`.
- **M3 — `interface-design` reposition** as the WCAG-policy + design-token *reference*; add
  negative routing to `artifact-design`/`frontend-design`; merge the two near-identical a11y refs.
- **M4 — `foundations` de-scope** to code quality / naming / TDD / verification / review only
  (description rewrite in §6); fix stale language-skill names in `code-style.md:96-98`; merge the
  dual code-review references; retire the 5 dead scripts (enforcement is native now).
- **M5 — Unrunnable tooling.** `infrastructure-management/scripts/validate-k8s-manifest.py:11`
  imports undeclared PyYAML; `power-systems-modeling` sidecar allows only `Bash(python:*)` but
  ships a `.sh`. Fix imports or delete scripts; align sidecar tool grants.
- **M6 — `cli-reference` (corrected scope).** Add the `loaf session` family to the catalog (or
  make the generator introspect the command tree) and delete the dead session special-case. Do
  **not** bundle the larger "collapse with agent-help" refactor into the same change (R5).
- **M7 — Structure/lint hygiene.** Add `## Contents` to architecture/brainstorm/council/database-design/housekeeping;
  fix `knowledge-base` → `.agents/AGENTS.md` (4 locations) and its banned deep-links; fix
  `database-design:76 → infrastructure-management` and `power-systems-modeling:93 → database-design`;
  fix the `triage:136` stray fence; add `source_sessions:` to `shape/templates/spec.md`;
  correct `targets.yaml:26,37` (it documents `.cursor.yaml`/`.codex.yaml` sidecars that are
  neither present nor read).
- **M8 — `librarian` profile: decide.** Retire it (native session CLI covers its role) or add
  `librarian.opencode.yaml`/`librarian.cursor.yaml` and wire it from orchestration/wrap. Don't
  leave it half-wired and boundary-less.

### LOW / decisions deferred to §7
`thermo-nuclear-code-quality-review` (retire), `debugging` invocability (corrected — see §6b),
language/domain **install profiles** (needs a migration spec — §7), `wrap` description verb-first
touch-up, CHANGELOG duplication (`documentation-standards` owns it; `git-workflow` links).

---

## 4. Proposed end-state taxonomy

**35 → 34 skills.** The only outright cut is `thermo-nuclear-code-quality-review`. No merges, no
renames — the audits unanimously confirmed the verbs are distinct. The clutter reduction comes
from **de-bloating** (orchestration, oversized roots) and **packaging** (opt-in language/domain
packs), *not* from cutting skills.

| Tier | Skills | Visible |
|------|--------|:--:|
| **Core workflow** | bootstrap, idea, brainstorm, triage, shape, breakdown, implement, ship, release, wrap, housekeeping | yes |
| **Advanced workflow** | architecture, council, research, strategy, reflect, handoff, refactor-deepen | yes |
| **Reference pack (base)** | foundations, git-workflow, documentation-standards, security-compliance, knowledge-base, database-design, infrastructure-management, interface-design | model-invoked, hidden from `/` |
| **Reference pack (opt-in)** | go-development, python-development, typescript-development, ruby-development, power-systems-modeling | install-gated |
| **Internal** | orchestration, cli-reference | hidden |
| **Retired** | ~~thermo-nuclear-code-quality-review~~ | — |
| **Visibility-only change** | debugging | see §6b |

---

## 5. What's genuinely slop vs genuinely valuable

- **Slop to cut:** `thermo-nuclear-code-quality-review` (imported third-party, no sidecar,
  non-standard frontmatter, restates model-known review wisdom, collides with `foundations` +
  `refactor-deepen`). The critique's correction stands: **just delete it — do not ceremonially
  "fold its heuristics in,"** because the audit's own justification for cutting it is that the
  heuristics are model-known (folding them back in would re-introduce the anti-slop violation).
- **Mass to thin, not cut:** `orchestration` (~1,600 duplicating ref lines + 10 unwired scripts);
  the language packs' references are 40–60% generic restatement around a thin opinionated core
  (Stack/Defaults tables + Always/Never lists) — slim each root to a router and keep the core.
- **Valuable, just mis-placed:** `power-systems-modeling` is the *highest-quality* content in its
  cluster but a niche personal domain (transmission-line thermal modeling) — the canonical "great
  skill, wrong default surface." Opt-in, don't delete.

---

## 6. The measurement gap (the most important methodological point)

Every routing/collision claim in this evaluation — `foundations` vs three siblings, `research`
vs `brainstorm`, `interface-design` vs `artifact-design` — is **textual overlap, not a measured
mis-route.** The cli-reference self-correction (§2) proves the risk: string-inspection produced a
confident, wrong, "release-blocking" claim. Two consequences:

**a) Do not churn descriptions on assertion alone.** The user invoked `skill-creator` for exactly
this — it has a routing/triggering eval loop. Before rewriting any description across 7 targets,
build a small routing eval (user-utterance → expected-skill, current vs proposed description) and
the conflict pairs the prior report named (idea/triage, research/brainstorm, strategy/reflect,
ship/release, architecture/shape, foundations/git-workflow/documentation-standards). Then the
rewrites below are *validated*, not hoped-for.

**b) `user-invocable: false` does NOT stop model invocation** (the synthesis got this wrong). Per
the project's own sidecar table, it only hides from the `/` menu; the model still autonomously
routes to the skill on its description. So "hide `debugging`" does not fix the verb/noun mismatch
— the model will route to it exactly as before. The real options for `debugging` are: (i) tighten
its *description* triggers, (ii) set `disable-model-invocation: true` if it should be reference-only,
or (iii) fold it into `foundations/references/debugging-methodology.md`. The cluster audit argued
*against* folding (it would lose the hypothesis-tracking methodology), so (i)/(ii) are preferred.
This same conflation underwrites the opt-in-pack idea: only **not shipping the files** removes a
skill from a harness; flags do not.

**Candidate description rewrites (to be eval-gated, not shipped blind):** `foundations` (drop the
three over-claimed domains), `research` (drop ideation/vision modes), `brainstorm` (SQLite-aware +
research negative-routing), `interface-design` (reference framing + design-skill negative routing),
`strategy` (add persona/market/competitor triggers — currently under-triggers). Full before/after
strings are in the workflow synthesis artifact.

---

## 7. Risk, sequencing, and the breaking-change gate

1. **Keystone first:** B1 (validate the build) gates the credibility of B2/B3/H3/H4. Do it before
   or with the Amp fix — not last.
2. **Ship the cheap blockers today:** B1 + B4 are low-risk, high-certainty, independent.
3. **Land the session spine atomically (R1).** The suite is *currently consistent in being wrong*
   (everyone teaches markdown). A half-done convergence is **worse** — some skills SQLite-first,
   others markdown-first, cross-referencing each other. Order: `templates/session.md` (the schema
   everyone cites) → `orchestration` (SKILL + sessions/context-management/background-agents refs)
   → `implement` **+ `implementer.md` profile together (R2)** → `bootstrap` → `brainstorm` →
   read-side second wave (`reflect`, `research`, `background-runner`). Reconcile the status enum
   against `internal/state/session_*.go`. Add a lint: session schema appears in exactly one file.
4. **The breaking-change gate (R3 — CLAUDE.md "ask before breaking changes").** Retiring `thermo`,
   changing `debugging`, and moving 5 skills to opt-in packs all affect users who already ran
   `loaf install --to all`. **There is no orphan-cleanup / upgrade-migration story today.** Does
   `loaf install --upgrade` remove a retired skill from a user's `~/.claude`/cursor dir? Silently
   drop opt-in packs they relied on? This needs a small spec (deprecation window, tombstone/alias,
   upgrade semantics) **before** any retire/relocate. Treat as SPEC work, not a free edit.
5. **Don't bundle refactors with data fixes (R5).** Add `session` to the cli-reference catalog
   (M6) ≠ rewrite the generator to introspect + collapse with `agent-help`. Ship the data fix;
   spec the generator refactor separately.

---

## 8. Recommended PR sequence

1. **PR 1 — Build validates itself + ship the cheap blockers.** B1, B2, B3, B4, H3, H4 (+ the 5
   regression tests: amp JS validity, codex advisory `failClosed`, opencode command coverage,
   cross-target Claude-ism lint, sidecar-merge). *This is the keystone PR.*
2. **PR 2 — `housekeeping` un-break + its self-log marker.** B5 + M1's one real fix.
3. **PR 3 — Session-model spine (atomic).** `session.md` → orchestration → implement+profile →
   bootstrap → brainstorm, + transcript removal (H2), + status-enum reconciliation + the
   "one canonical session file" lint. Batched self-logging rides along.
4. **PR 4 — De-bloat + de-dupe.** orchestration thin-hub (H5), ADR SoT (H7), research de-scope
   (M2), interface-design reposition (M3), foundations de-scope (M4), structure/lint hygiene (M7),
   unrunnable tooling (M5), cli-reference data fix (M6).
5. **PR 5 — Routing eval + validated description rewrites** (§6). Gated on the eval harness.
6. **PR 6 — Packaging spec + migration** (§7.4): install profiles, opt-in packs, `thermo` retire,
   `debugging` decision, `librarian` decision. **Needs sign-off — breaking changes.**

PRs 1–4 are pure correctness/hygiene and need no taxonomy decisions. PRs 5–6 carry the contested
calls and the breaking changes.

---

## 9. Open decisions (yours)

1. **Scope of my next action:** produce the implementation (starting with the keystone PR 1), or
   is this evaluation the deliverable?
2. **Packaging (PR 6):** opt-in language/domain packs — yes, and is a migration spec acceptable
   as the gate?
3. **`thermo`:** retire outright (recommended) vs keep-with-edits (add sidecar, conform frontmatter)?
4. **`debugging`:** tighten description (recommended) vs `disable-model-invocation` vs fold into
   `foundations`?
5. **Routing eval:** build the `skill-creator` eval harness before touching descriptions
   (recommended), or accept asserted rewrites?

## Sources
- `report-loaf-skills-deep-audit` (2026-06-20) — extended and corrected here.
- 14-agent evaluation workflow (8 family clusters + harness-build / session-model / agents-hooks /
  structure-lint cross-dives + synthesis + adversarial critique).
- Source-verified: `internal/cli/cli.go:7030,7075,7096`, `internal/cli/build_test.go:31,56,283`,
  `internal/cli/cli_reference.go:44,101,599`, `internal/cli/build_amp.go`, `build_codex.go`,
  `build_opencode.go`, `internal/cli/check.go`, `config/hooks.yaml`, `config/targets.yaml`,
  `content/skills/*/SKILL.md` + sidecars, `content/agents/*`, `docs/knowledge/skill-architecture.md`,
  `.agents/specs/SPEC-040-sqlite-backed-loaf-operational-state.md`.

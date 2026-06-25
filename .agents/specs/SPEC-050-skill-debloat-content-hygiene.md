---
id: SPEC-050
title: Skill De-bloat & Content Hygiene
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-D)"
created: 2026-06-22T09:13:21Z
status: drafting
branch: feat/skill-debloat-content-hygiene
source_sessions:
  - id: 20260621-001541-session
    role: shaped
---

# SPEC-050: Skill De-bloat & Content Hygiene

## Problem Statement

The deep skill-suite evaluation (`report-loaf-skills-deep-audit`,
`.agents/reports/20260620-214448-audit-loaf-skills-deep-audit.md`) found that Loaf's taxonomy is
healthy but the corpus carries excess reference mass, duplicated authority, over-claimed skill
descriptions, unrunnable tooling, and a layer of stale cross-references that erode trust in the
shipped content. None of these break the build, but together they make the skill suite harder to
maintain, route through, and reason about.

Concretely, the audit verified (and this spec re-verified against source):

- **Orchestration has thin a root but heavy references.** `content/skills/orchestration/SKILL.md`
  is already 154 lines (a good router), but its `references/` total ~4,156 lines across 16 files —
  several duplicate authority owned by dedicated workflow skills (`references/councils.md` 404,
  `references/planning.md` 422, `references/sessions.md` 523, `references/specs.md` 245,
  `references/product-development.md` 247). Fourteen scripts live in
  `content/skills/orchestration/scripts/` (e.g. `new-council.sh`, `validate-roadmap.py`,
  `suggest-team.py`, `extract-decisions.py`) and most are not wired into `loaf check` or any hook —
  `references/script-surface.md` does not even reference them.
- **ADR authority is split.** `content/skills/architecture/SKILL.md` is the deep, correct ADR
  source of truth (template at `templates/adr.md`, triage gate, supersession discipline,
  numbering — `architecture/SKILL.md:86-204`). Yet `documentation-standards` re-publishes a
  competing ADR template and format
  (`content/skills/documentation-standards/references/documentation.md:47-50` "ADR Template" with a
  full `# ADR-XXX: Title` block, plus `:30-31`, `:45`). Two ADR specs invite drift.
- **Over-claimed descriptions.** `research` advertises four modes — State Assessment, Topic
  Investigation, Brainstorming, Vision Evolution (`research/SKILL.md:51-56`, `:69-74`) — colliding
  with `brainstorm` (ideation) and `strategy`/`reflect` (vision). `interface-design` is a
  `user-invocable: false` reference skill (`interface-design/SKILL.claude-code.yaml:4`) but is not
  negative-routed toward `artifact-design`/`frontend-design`. `foundations` claims it
  "Establishes code quality, commit conventions, documentation standards, and security patterns"
  (`foundations/SKILL.md:4-5`) — three of those are owned by sibling skills (`git-workflow`,
  `documentation-standards`, `security-compliance`).
- **Unrunnable tooling.** `infrastructure-management/scripts/validate-k8s-manifest.py:10` does
  `import yaml` with no declared/bundled PyYAML dependency (the audit's local `import yaml` failed).
  `power-systems-modeling/SKILL.claude-code.yaml` allows only `Bash(python:*)` while the skill
  ships `scripts/check-standard-refs.sh` (a shell script the sidecar can never run).
- **Structure/lint hygiene & stale references** verified at source:
  - `database-design/SKILL.md:76` points to a nonexistent `infrastructure` skill (real name:
    `infrastructure-management`).
  - `power-systems-modeling/SKILL.md:93` points to a nonexistent `database-patterns`.
  - `foundations/references/code-style.md:96-98` references `python`, `typescript`, `rails`
    instead of `python-development`, `typescript-development`, `ruby-development`.
  - `knowledge-base/SKILL.md:7,53,56,85` routes agent instructions to `CLAUDE.md` rather than the
    current `.agents/AGENTS.md` entrypoint.
  - `triage/SKILL.md` carries a stray closing code fence (the audit's finding #10).
  - `cli-reference/SKILL.md` has effectively no `loaf session` family catalog — only a single
    incidental mention (`:898`) of `loaf session start`; `start/log/end/list/show/archive` are
    absent from the generated reference.

## Strategic Alignment

**Vision/Architecture.** This is the low-risk content-hygiene workstream of the Loaf restructuring
roadmap (`.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md` §2 WS-D, §5). It does not
change the SQLite-native runtime, targets, or install convention; it tightens the shipped content
so the structural work landing in earlier specs is not re-buried under duplicated prose.

**Coordinates-with:**

- **SPEC-048 (Session-Model Convergence).** WS-D is explicitly sequenced *after* SPEC-048 to avoid
  churn collisions on shared files — `orchestration/SKILL.md` and `orchestration/references/
  sessions.md` / `context-management.md` are rewritten by SPEC-048's session convergence. This spec
  removes/relocates orchestration's *non-session* duplicated references and unwired scripts only
  after that rewrite settles. The session references themselves are SPEC-048's domain, not this
  one's.
- **SPEC-051 (Routing Eval & Validated Description Rewrites).** The description rewrites here
  (`research` de-scope, `interface-design` negative-routing, `foundations` de-scope) are *content*
  edits; SPEC-051 owns the routing-eval harness that *validates* description changes measurably
  improve routing. This spec proposes the edits; SPEC-051 gates whether they ship. The two run in
  parallel (roadmap §3: D ∥ E) and must coordinate so a description is not rewritten twice.
  Per the roadmap decision, *no blind rewrites* — description changes here are staged behind
  SPEC-051's eval where they affect routing.
- **SPEC-043 (SQLite-Native Artifact Bodies).** The `cli-reference` catalog gap is a *content*
  symptom; SPEC-043 introduces uniform `new/edit/show/list/link` verbs and a cli-reference
  regeneration build-step. This spec only ensures the `loaf session` family is cataloged; it does
  not re-author the generation mechanism (that is SPEC-043's).

**Prior specs:**

- **SPEC-040 (`.agents/specs/SPEC-040-sqlite-backed-loaf-operational-state.md`).** This spec must
  not reintroduce markdown-as-source-of-truth assumptions; orchestration reference trimming keeps
  the SQLite-native model intact.
- **ADR-013 (`docs/decisions/ADR-013-agentic-state-storage-model.md`).** The `knowledge-base` fix
  (route to `.agents/AGENTS.md`) aligns with the agentic state model.

**Supersedes:** nothing. This spec retires duplicated content and corrects references; it does not
supersede any prior spec or ADR.

## Solution Direction

Treat WS-D as a series of small, independently-reviewable content edits, each preserving build
green (`loaf build`, `npm run typecheck`, `npm run test`) and the in-tree `dist/`+`plugins/` drift
gate. Group the work into four clusters that can land as separate commits/PRs:

1. **Reference de-duplication.** Make `orchestration` a thin hub: keep `SKILL.md` as the router,
   and remove or stub references whose authority lives in a dedicated workflow skill
   (`councils.md`→`council`, `planning.md`→`breakdown`/`shape`, `specs.md`→`shape`,
   `product-development.md`→appropriate workflow). Leave session references to SPEC-048.
   Retire or relocate the ~14 orchestration scripts: classify each as CLI-owned (promote to
   `loaf check`), hook-owned (wire in `config/hooks.yaml`), skill-local helper (document as such),
   or retired. Strip the competing ADR template/format from
   `documentation-standards/references/documentation.md` and replace it with a link to
   `architecture` as the single ADR source of truth.

2. **Description de-scope & repositioning** (staged behind SPEC-051's eval where routing-affecting):
   drop `research`'s ideation/vision modes (it becomes investigation-only; ideation→`brainstorm`,
   vision→`strategy`/`reflect`); add negative-routing to `interface-design`'s description toward
   `artifact-design`/`frontend-design` and keep it `user-invocable: false`; narrow `foundations`'
   description to its actual surface (code quality + naming + TDD + verification), dropping the
   git/docs/security over-claim and negative-routing to the sibling skills.

3. **Tooling correctness.** Remove the undeclared PyYAML dependency from
   `infrastructure-management`'s k8s validator — rewrite without `import yaml` (stdlib-only parse or
   document an explicit opt-in dependency) — and reconcile `power-systems-modeling`'s sidecar
   `allowed-tools` with the scripts it actually ships (add `Bash(*.sh)` semantics or convert the
   shell helper to Python). Add a script smoke check so a skill cannot ship a script its sidecar
   cannot execute.

4. **Structure/lint hygiene.** Fix every verified stale reference, add missing `## Contents`
   headers where absent, remove `triage`'s stray fence, and ensure the `loaf session` family
   appears in the `cli-reference` catalog (coordinate with SPEC-043's regeneration build-step).

## Scope

### In Scope

- Trim `orchestration` references that duplicate `council`/`shape`/`breakdown`/`research`
  authority; retire or wire the unwired orchestration scripts.
- Strip the duplicate ADR template/format from `documentation-standards`; link to `architecture`.
- De-scope `research` (drop ideation/vision modes → investigation-only).
- Reposition `interface-design` as a reference skill with negative-routing to
  `artifact-design`/`frontend-design`.
- De-scope `foundations` description (drop the three over-claimed sibling domains).
- Fix unrunnable tooling: `infrastructure-management` PyYAML import; `power-systems-modeling`
  sidecar/script mismatch.
- Stale skill-name reference fixes: `database-design`→`infrastructure-management`,
  `power-systems-modeling`→`database-design`, `foundations/code-style` →
  `python-development`/`typescript-development`/`ruby-development`.
- `knowledge-base` → route agent instructions to `.agents/AGENTS.md` not `CLAUDE.md`.
- `triage` stray-fence fix; add missing `## Contents` headers where absent.
- `cli-reference` `loaf session` family catalog gap (content-side; mechanism is SPEC-043).
- Rebuild affected `dist/`+`plugins/` artifacts and commit them with the source changes.

### Out of Scope

- Session-model rewrites of `orchestration`/`implement`/`bootstrap` and the session references
  (`sessions.md`, `context-management.md`) — owned by **SPEC-048**.
- The routing-eval harness and the decision of *whether* a rewrite ships — owned by **SPEC-051**.
- The cli-reference *generation mechanism* and uniform entity verbs — owned by **SPEC-043**.
- Retiring `thermo-nuclear-code-quality-review`, `debugging` disposition, opt-in install packs,
  `librarian` profile — owned by **SPEC-053** (taxonomy decisions).
- Any breaking change, install-path relocation, or schema change.

### Rabbit Holes

- **Rewriting the entire orchestration reference set.** Only remove *duplicated* authority; do not
  re-architect what remains. Resist turning trimming into a rewrite.
- **Inventing new tooling for the unwired scripts.** Classify-and-decide; do not build a new script
  runner. Promotion to `loaf check` is allowed only where a script is genuinely an enforcement
  gate.
- **Bikeshedding description wording.** Stage routing-affecting wording behind SPEC-051's eval
  rather than iterating prose blind.

### No-Gos

- Do not start before SPEC-048 lands (avoids churn on shared `orchestration`/session files).
- Do not reintroduce markdown-as-source-of-truth (SPEC-040).
- Do not change skill *names* (only fix references *to* skills); name changes are taxonomy
  decisions (SPEC-053).
- Do not ship a routing-affecting description rewrite that SPEC-051's eval has not validated.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Trimming orchestration references breaks a link that another skill depends on | Medium | Medium | Grep for inbound references before deletion; run the link-integrity lint; rebuild and `git diff --exit-code -- dist plugins` |
| Churn collision with SPEC-048 on shared orchestration/session files | Medium | Medium | Hard-sequence after SPEC-048; touch only non-session references here |
| Description rewrite degrades routing | Medium | Medium | Stage routing-affecting rewrites behind SPEC-051's eval; no blind rewrites |
| Removing PyYAML breaks the k8s validator's parse behavior | Low | Low | Validate against existing manifest fixtures; stdlib-only or documented opt-in |
| cli-reference session catalog edit conflicts with SPEC-043's regeneration | Low | Medium | Coordinate; if SPEC-043 lands first, the gap closes there and this item is dropped |
| Stale-reference fix misses a generated copy in `dist/` | Low | Low | Always rebuild after source edits; drift gate catches divergence |

## Open Questions

1. Should the unwired orchestration scripts that are genuinely useful (`suggest-team.py`,
   `new-council.sh`) be promoted to `loaf` subcommands, or retired and folded into the relevant
   skill prose? (Likely defers to whether SPEC-043/SPEC-048 absorb their function.)
2. Does the `research` de-scope require a new template/mode removal, or only description + Quick
   Reference table edits? (Verify `research/templates/` after SPEC-051's eval.)
3. For `power-systems-modeling`: convert the shell helper to Python (matching the sidecar) or widen
   the sidecar to allow shell? (Convention favors Python parity with the existing `*.py` scripts.)
4. Is the `cli-reference` session gap fully closed by SPEC-043's regeneration build-step, making
   this spec's catalog item a no-op? (Coordinate at breakdown time.)

## Test Conditions

- [ ] `loaf build` succeeds and `git diff --exit-code -- dist plugins` is clean after each cluster.
- [ ] `npm run typecheck` and `npm run test` pass.
- [ ] `content/skills/orchestration/references/` no longer contains references duplicating
      `council`/`shape`/`breakdown`/`research` authority (session references untouched, owned by
      SPEC-048).
- [ ] Every script under `content/skills/orchestration/scripts/` is either wired (hook or
      `loaf check`), documented as a skill-local helper, or removed.
- [ ] `documentation-standards/references/documentation.md` no longer publishes an ADR template;
      it links to `architecture` as the single ADR source of truth.
- [ ] `research/SKILL.md` no longer advertises Brainstorming or Vision Evolution modes; routing
      to `brainstorm`/`strategy`/`reflect` is negative-routed in its description.
- [ ] `interface-design`'s description negative-routes to `artifact-design`/`frontend-design` and
      it remains `user-invocable: false`.
- [ ] `foundations`'s description no longer claims commit conventions, documentation standards, or
      security patterns as its own surface.
- [ ] `infrastructure-management/scripts/validate-k8s-manifest.py` runs without an undeclared
      `import yaml` (or the dependency is explicitly declared/opt-in).
- [ ] `power-systems-modeling` sidecar `allowed-tools` matches the scripts it ships (no
      shell-script-without-shell-permission mismatch).
- [ ] `rg -n 'infrastructure\b|database-patterns' content/skills/database-design content/skills/power-systems-modeling`
      returns no stale skill-name references.
- [ ] `rg -n '`python` skill|`typescript` skill|`rails` skill' content/skills/foundations` returns
      nothing.
- [ ] `knowledge-base/SKILL.md` routes agent instructions to `.agents/AGENTS.md`, not `CLAUDE.md`.
- [ ] `triage/SKILL.md` has no stray/unbalanced code fence.
- [ ] Every source `SKILL.md` over 100 lines has a `## Contents` header.
- [ ] The `loaf session` family (`start/log/end/list/show/archive`) appears in the generated
      `cli-reference` catalog (or the gap is closed by SPEC-043).

## Priority Order

All tracks are **non-breaking** content edits. None require a migration gate. Sequence after
SPEC-048 has landed.

1. **Structure/lint hygiene & stale references** (non-breaking) — triage stray fence, stale
   skill-name fixes, `knowledge-base`→`.agents/AGENTS.md`, missing `## Contents` headers. Lowest
   risk; clears sharp edges before larger edits create churn.
2. **Tooling correctness** (non-breaking) — PyYAML removal, power-systems sidecar/script
   reconciliation, script smoke check.
3. **ADR de-duplication** (non-breaking) — strip the competing ADR spec from
   `documentation-standards`; link to `architecture`.
4. **Orchestration reference trim & script disposition** (non-breaking)
   — **go/no-go gate: SPEC-048 must be merged** to avoid churn on shared orchestration/session
   files.
5. **Description de-scope & repositioning** (non-breaking)
   — **go/no-go gate: SPEC-051's routing eval** must validate that routing-affecting rewrites
   measurably improve routing before they ship (no blind rewrites).
6. **cli-reference session catalog** (non-breaking) — **conditional on SPEC-043**: drop if
   SPEC-043's regeneration build-step already closes the gap.

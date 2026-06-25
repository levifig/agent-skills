---
id: SPEC-050
title: Skill De-bloat & Content Hygiene
source: "roadmap:20260621-020342-loaf-restructuring-roadmap (WS-D)"
created: 2026-06-22T09:13:21Z
status: complete
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
roadmap (`roadmap:20260621-020342-loaf-restructuring-roadmap` §2 WS-D, §5). It does not
change the SQLite-native runtime, targets, or install convention; it tightens the shipped content
so the structural work landing in earlier specs is not re-buried under duplicated prose.

**Coordinates-with:**

- **SPEC-048 (Session-Model Convergence).** WS-D is explicitly sequenced *after* SPEC-048 to avoid
  churn collisions on shared files — `orchestration/SKILL.md` and `orchestration/references/
  sessions.md` / `context-management.md` are rewritten by SPEC-048's session convergence. This spec
  removes/relocates orchestration's *non-session* duplicated references and unwired scripts only
  after that rewrite settles. The session references themselves are SPEC-048's domain, not this
  one's.
- **SPEC-051 (Routing Eval & Validated Description Rewrites).** SPEC-051 owns routing-affecting
  `description:` rewrites and the eval harness that validates them. SPEC-050 may trim duplicate
  body prose, stale references, broken helper contracts, and duplicated authority, but it must not
  edit skill frontmatter descriptions or ship routing text that SPEC-051 has not validated. The two
  run in parallel (roadmap §3: D ∥ E); this spec supplies hygiene evidence and leaves description
  decisions to SPEC-051.
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

2. **Body de-scope only.** Remove or replace duplicate body sections when they restate authority
   owned by another skill, but do not change YAML `description:` fields. `research` mode wording,
   `interface-design` negative routing, and `foundations` description scope are SPEC-051 decisions;
   SPEC-050 can record evidence and clean non-frontmatter duplication after confirming it is not a
   routing rewrite.

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
- Remove non-frontmatter duplicate body prose only when ownership is unambiguous; record evidence
  for SPEC-051 where the cleanup would require a routing-affecting description rewrite.
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
- YAML `description:` rewrites for `research`, `interface-design`, or `foundations` that change
  routing scope — owned by **SPEC-051**. Stale path-name corrections that preserve routing intent
  are allowed here.
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

1. **Orchestration scripts:** classify each helper as hook-owned, CLI-owned, skill-local, or
   retired. SPEC-050 may document skill-local helpers and remove clearly dead legacy helpers, but it
   should not create new `loaf` subcommands.
2. **Research/foundations/interface descriptions:** defer frontmatter description changes to
   SPEC-051. SPEC-050 may only trim duplicate body prose when the edit does not change routing.
3. **Power-systems helper contract:** widen the sidecar to include the shipped shell helper unless
   implementation evidence shows conversion to Python is smaller and clearer.
4. **cli-reference session gap:** re-check after SPEC-043. If the generated session family catalog
   is already present, mark this item as closed-by-dependency and do not hand-edit generated docs.

## Test Conditions

- [x] `loaf build` succeeds and generated `dist/` / `plugins/` changes are refreshed with source.
- [x] `npm run typecheck` and `npm run test` pass.
- [x] `content/skills/orchestration/references/` no longer contains references duplicating
      `council`/`shape`/`breakdown`/`research` authority (session references untouched, owned by
      SPEC-048).
- [x] Every script under `content/skills/orchestration/scripts/` is either wired (hook or
      `loaf check`), documented as a skill-local helper, or removed.
- [x] `documentation-standards/references/documentation.md` no longer publishes an ADR template;
      it links to `architecture` as the single ADR source of truth.
- [x] SPEC-050 does not ship the `research` / `interface-design` / `foundations` routing rewrites;
      those remain for SPEC-051. Stale path-only description corrections preserve routing intent.
- [x] `infrastructure-management/scripts/validate-k8s-manifest.py` runs without an undeclared
      `import yaml` (or the dependency is explicitly declared/opt-in).
- [x] `power-systems-modeling` sidecar `allowed-tools` matches the scripts it ships (no
      shell-script-without-shell-permission mismatch).
- [x] `rg -n 'infrastructure\b|database-patterns' content/skills/database-design content/skills/power-systems-modeling`
      returns no stale skill-name references.
- [x] `rg -n '`python` skill|`typescript` skill|`rails` skill' content/skills/foundations` returns
      nothing.
- [x] `knowledge-base/SKILL.md` routes agent instructions to `.agents/AGENTS.md`, not `CLAUDE.md`.
- [x] `triage/SKILL.md` has no stray/unbalanced code fence.
- [x] Every source `SKILL.md` over 100 lines has a `## Contents` header.
- [x] The `loaf session` family (`start/log/end/list/show/archive`) appears in the generated
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
5. **Body-only de-scope evidence** (non-breaking) — collect duplicate-body evidence for SPEC-051
   and trim only non-frontmatter prose whose ownership is unambiguous.
6. **cli-reference session catalog** (non-breaking) — **conditional on SPEC-043**: drop if
   SPEC-043's regeneration build-step already closes the gap.

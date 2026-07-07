# Blindspot pass — fog-routing target skills (brainstorm, idea, triage, research)

Read-only reviewer report, 2026-07-07. Question: do the routing destinations' contracts serve the fog-quadrant routing, what do they assume but never state, and where does spec-model residue live. Findings verbatim from the reviewing agent; adjudication in `change.md` (Planning Contract → Blast-radius findings).

## Routing gaps

- [routing-gap] content/skills/research/SKILL.md:111-117 — Research outputs land at `.agents/reports/` or stdout or SQLite report state; it has no mode to hand a bounded answer back INTO an in-flight shaping session. As the known-unknown destination it can't return an answer to a specific named unknown.
- [routing-gap] content/skills/research/SKILL.md:112 — Evidence written to `.agents/reports/` collides with the Change model: ADR-013 routes `.agents/` to the main worktree, but a Change's write-once evidence belongs in branch content `docs/changes/YYYYMMDD-slug/research/` (pilot change.md:84, D18). Research doesn't know the Change's research/ folder exists.
- [routing-gap] content/skills/research/SKILL.md:29,42,104 — "Interview before researching" / never "skip the interview step" / Topic Investigation asks "what decision will this inform?" — when shape routes a pre-scoped known-unknown, shape already interviewed; research re-interviews. No "answer this scoped question" entry path that skips its own interview.
- [routing-gap] content/skills/research/SKILL.md:52-58 — The four modes (State Assessment, Topic Investigation, Brainstorming, Vision Evolution) contain no "resolve a named unknown for a Change being shaped" mode. The known-unknown handoff has no matching contract.
- [routing-gap] content/skills/research/SKILL.md:119-127 — Research's own "Brainstorming" mode shadows the separate brainstorm skill. Shape routing to "brainstorm" is ambiguous (skill vs. this mode) — the shadow the tightening follow-up (pilot change.md:296) is meant to kill.
- [routing-gap] content/skills/brainstorm/SKILL.md:21,28 — Brainstorm has no notion of a reaction/prototype artifact for a specific named unknown. It captures sparks and defers ("Process sparks... capture only, expand later"), the opposite of resolving an unknown-known in the shaping session. No prototype mode exists at all.
- [routing-gap] content/skills/brainstorm/SKILL.md:18,37 — Brainstorm mandates grounding in VISION.md/STRATEGY.md; a tactical named unknown inside a Change may not be strategic. Contract forces a project-level frame onto a Change-local question.
- [routing-gap] content/skills/brainstorm/SKILL.md:21,64 — Brainstorm's byproducts flow forward to intake (spark capture → triage → /idea), away from the current Change into a future queue, not back into the shaping session that needs the resolution.
- [routing-gap] content/skills/triage/SKILL.md:52-54 — Triage's intake taxonomy is sparks/brainstorms/ideas only; no notion of Change-adjacent harvested deferrals or the rejection KB the Change model produces at ship (pilot D17, D21c). Harvested items would have no triage home.
- [routing-gap] (whole set) — The unknown-unknown quadrant routes to a "blindspot pass," but none of these four skills is that destination; research is closest yet has no adversarial/disconfirming mode, despite the pilot (change.md:141) making adversarial input standing practice.

## Unknown knowns

- [unknown-known] content/skills/brainstorm/SKILL.md:21,34,60 — Assumes the reader knows what "SQLite state initialized" means, how to detect it, and how to pick a `--scope`; scope vocabulary is never defined. Same SQLite-vs-markdown-fallback assumption unstated in idea/SKILL.md:36-37,42 and triage/SKILL.md:46,62-64.
- [unknown-known] content/skills/brainstorm/SKILL.md:71 — References bare path `strategy/references/` as a topic source; assumes the reader can resolve a cross-skill path, violating the one-level-deep reference rule.
- [unknown-known] content/skills/research/SKILL.md:29,47,104 & brainstorm/SKILL.md:47 — Both assume an interactive user is present (interview, AskUserQuestion) but never state they are interactive-only; matters when shape delegates them as a subagent/background step.
- [unknown-known] content/skills/idea/SKILL.md:84 vs 78-80 — Step says generate a timestamp, but SQLite mode creates no markdown file; the timestamp only applies to the markdown fallback. Reader must already know when.
- [unknown-known] content/skills/triage/SKILL.md:98,54 — `loaf spark promote <spark> --to-idea <idea>` assumes a target idea already exists; create-during-promote mechanics unstated. "Ideas → Shape, promote, or archive" leaves "promote" undefined for an idea.

## Spec-model residue

- [spec-residue] content/skills/idea/SKILL.md:51,93,100,118 — Lifecycle hardcodes SPEC as the terminal artifact: `shaped | Converted to SPEC`, `raw -> shaping -> shaped (becomes SPEC) -> archived`, "shape -- Develop an idea into a SPEC."
- [spec-residue] content/skills/triage/SKILL.md:177 — Related skills: "shape -- Develop an idea into a SPEC."
- [spec-residue] content/skills/research/SKILL.md:96 — State Assessment runs `loaf spec list --json`; the spec surface is being removed (pilot D4).
- [spec-residue] content/skills/research/templates/state-assessment.md:35-38 — "In Flight" table keyed on Spec/Task with a status lifecycle — the exact SPEC-ID + status-lifecycle model the Change model kills.
- [spec-residue] content/skills/research/templates/report.md:17,12-18,56-65 — `source: SPEC-XXX | TASK-XXX` and `status: draft | done | archived` frontmatter; the status-frontmatter class is what pilot V1 bans on Changes — residue/tension even if reports are a separate artifact.
- [spec-residue] content/skills/brainstorm/templates/brainstorm.md:16 & content/skills/idea/templates/idea.md:15 — `related:` frontmatter comments still cite "spec IDs" as the reference type.

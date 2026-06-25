---
id: SPEC-048
title: Session-Model Convergence to SQLite
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-C)"
created: 2026-06-22T09:13:21Z
status: drafting
branch: feat/session-model-convergence
source_sessions:
  - id: 20260621-001541-session
    role: shaped
related_specs:
  - SPEC-040
  - SPEC-043
  - SPEC-044
  - SPEC-049 # sequenced-with SPEC-049 (soft; SPEC-048 may land first)
  - SPEC-029
  - SPEC-037
---

# SPEC-048: Session-Model Convergence to SQLite

## Problem Statement

SPEC-040 made one global SQLite database the canonical operational store and
demoted markdown in `.agents/` to compatibility/export. The skill suite never
followed. Per the deep evaluation
(`.agents/reports/20260621-020342-loaf-skill-suite-deep-evaluation.md`, §1.1),
**exactly one skill — `wrap` — converged.** Everything else still teaches the
markdown-era session model as primary, and in one case drives a command that is
now a no-op:

- `loaf session housekeeping` returns `Action: "skipped"` in SQLite mode
  (`internal/cli/cli.go:7096`, reason: *"markdown session housekeeping … is not
  run in SQLite mode"*). The `housekeeping` skill drives this dead command at
  `content/skills/housekeeping/SKILL.md:21,49,50,62`. The real artifact scanner
  is the top-level `loaf housekeeping`, which the skill never names.
- Because `housekeeping` never runs, it never writes its `skill(housekeeping)`
  self-log marker, so `wrap`'s "no housekeeping this session" nudge mis-fires
  (eval report §3 M1).
- `content/templates/session.md` — the schema *everyone cites* — still documents
  a markdown file at `.agents/sessions/YYYYMMDD-HHMMSS-session.md` with YAML
  frontmatter as the routing surface (template lines 3–12, 57–59).
- `implement` teaches "**MANDATORY: Create session file BEFORE any other work**"
  (`content/skills/implement/SKILL.md:251,261`), and the `implementer.md`
  profile (`content/agents/implementer.md:8`) still demands "a session file …
  provided in your spawn prompt." These cross-reference each other; a half-fix
  desynchronizes the spine.
- `content/skills/implement/references/session-management.md:220-254` documents
  **transcript archival to `.agents/transcripts/`** — a SPEC-040 scope violation
  (raw transcript capture is explicitly out of scope; SPEC-040:88-89,162) and a
  data-handling liability.

The taxonomy is healthy (eval §1). The disease is a half-shipped migration. The
governing constraint from the evaluation (§7.3): **a half-migrated spine — some
skills SQLite-first, others markdown-first, cross-referencing each other — is
*worse* than the current consistent-but-wrong state.** This spec finishes the
convergence and lands it atomically.

## Strategic Alignment

**Vision / Architecture.** The loaf CLI is the harness; operational truth lives
in one global SQLite store; markdown is a projection. This spec brings the
session model — the most-cited operational surface in the skill suite — into
line with that architecture, eliminating the last large pocket of markdown-era
guidance.

**Supersedes / completes:**
- **SPEC-040** (`.agents/specs/SPEC-040-sqlite-backed-loaf-operational-state.md`,
  status `complete`). SPEC-040 shipped the SQLite session model
  (`session start/end/log/archive`, journal rows, SQLite-only lifecycle) but
  Priority Order Track E ("Skill and hook migration", SPEC-040:520) was only
  partially delivered — `wrap` alone converged. SPEC-048 completes Track E for
  the session-model surface. It also enforces SPEC-040's own out-of-scope
  decision (SPEC-040:162 — no raw transcript ingestion) by *removing* the
  transcript-archival guidance that violates it.

**Coordinates with:**
- **SPEC-043** (WS-B, bodies into SQLite). Sessions become SQLite-only **bodies**
  per SPEC-043's body store — there is no in-tree session `.md` as source after
  this spec; the markdown view is render-on-demand. SPEC-048 depends on SPEC-043
  landing the session-body storage and `session show/list` read path. SPEC-048
  does the **skill/profile/template** convergence; SPEC-043 does the **storage**
  convergence. They must be coherent: this spec's guidance assumes SPEC-043's
  body store exists.
- **SPEC-044** (WS-B, deterministic render + drift gate). The render-on-demand
  markdown view of a session uses SPEC-044's deterministic body renderer. SPEC-048
  benefits from but does not strictly require SPEC-044 — until it lands, sessions
  are SQLite-only with the existing compatibility export.
- **SPEC-049** (WS-C, status-vocabulary unification) — **sequenced-with SPEC-049
  (soft; SPEC-048 may land first)**. The session status enum is
  inconsistent across three files today: `active` (`content/templates/session.md`),
  `in_progress`/`paused` (`content/skills/orchestration/references/sessions.md`),
  and `active/stopped/done/blocked/archived` (CLAUDE.md). SPEC-049 owns the
  canonical enum and the "schema lives in exactly one file" lint. SPEC-048 must
  not invent a fourth vocabulary — it cites whatever SPEC-049 anoints. If SPEC-048
  lands first, it uses the SPEC-040 runtime enum (`internal/state/session_*.go`)
  as the interim source of truth and SPEC-049 reconciles.
- **SPEC-029** (librarian journal enrichment). SPEC-029 enriches session journals
  from JSONL; the `housekeeping` skill currently drives `loaf session enrich`
  (`content/skills/housekeeping/SKILL.md:52,79`). SPEC-048 keeps enrichment but
  reroutes/cleans the guidance so it does not depend on the dead markdown
  housekeeping path. Conflict to name: SPEC-029 edits session `.md`; once sessions
  are SQLite-only bodies, enrichment writes journal **rows**, not file lines.
- **SPEC-037** ("specs are mutable internal work definitions"). Unaffected here,
  noted because the same markdown-vs-SQLite tension recurs; sessions are not specs
  and are unambiguously SQLite-only (ephemeral tier).

## Solution Direction

Finish the migration the suite never followed, landing it **as one atomic
change** so the session spine is never internally inconsistent. Anoint `wrap` as
the canonical session model and propagate it outward in strict dependency order.

The convergence is content-and-CLI, not new storage (SPEC-043 owns storage):

1. **Anoint the model.** Cite `wrap` as the canonical session model in
   `.claude/CLAUDE.md` and `.agents/AGENTS.md`. The model is: `loaf session
   start` (find/create + emit context) → `loaf session log "type(scope): desc"`
   → `loaf session end --wrap`. No hand-authored session file, no mandated
   frontmatter editing, no transcript archival.

2. **Rewrite in dependency order (one PR/commit set):**
   `content/templates/session.md` (schema everyone cites) → `orchestration`
   (SKILL + `references/sessions.md`, `references/context-management.md`,
   `references/background-agents.md`) → `implement` **+ the `implementer.md`
   profile in the same change** → `bootstrap` → `brainstorm` (SQLite-first sparks
   via `loaf spark capture`) → read-side second wave (`reflect`, `research`,
   `background-runner`).

3. **Sessions become SQLite-only bodies** (per SPEC-043). No in-tree session
   `.md` as source. The markdown view is render-on-demand (`loaf session show`,
   and SPEC-044's renderer once available). `content/templates/session.md`
   becomes a **render template / journal-format reference**, not a file scaffold
   — the entry-type vocabulary and `[timestamp] type(scope): desc` format are
   still authoritative; the "create this file" framing is removed.

4. **Remove transcript-archival guidance** entirely
   (`content/skills/implement/references/session-management.md:220-254` and any
   `transcripts:` rows/layout in `orchestration`). It violates SPEC-040 scope and
   is a data-handling liability.

5. **Fix the housekeeping dead command.** Replace `loaf session housekeeping`
   with `loaf housekeeping` (the real scanner) throughout
   `content/skills/housekeeping/SKILL.md`, and ensure the `skill(housekeeping)`
   self-log marker is written on invocation — which fixes `wrap`'s mis-firing
   nudge. Reroute the `loaf session enrich` guidance so it does not present the
   skipped markdown path as the primary flow.

6. **Add first-action self-logging** to user-invocable workflow skills (the
   `loaf session log "skill(<name>): …"` first action, per `.claude/CLAUDE.md`
   "Session Journal Self-Logging"). Fix the AGENTS.md exemplars (`shape`,
   `housekeeping`, `implement`) first so the rule and its examples agree
   (eval §3 M1). Keep existing `decision()` self-logs that carry useful outcomes.

The "all 5 first-class harnesses" parity contract (WS-A / SPEC-047) applies:
these are content edits in `content/`, regenerated to all five harness outputs.
No Claude-isms should be introduced; rely on SPEC-047's harness-language
tokens where harness-specific terms appear.

## Scope

### In Scope

- Cite `wrap` as the canonical session model in `.claude/CLAUDE.md` and
  `.agents/AGENTS.md`.
- Rewrite `content/templates/session.md` from a file-scaffold to a render
  template / journal-format reference (keep entry-type table + format rules;
  drop the "create `.agents/sessions/*.md`" framing and YAML-frontmatter-as-source).
- Rewrite `orchestration`: `SKILL.md` session sections + `references/sessions.md`,
  `references/context-management.md`, `references/background-agents.md` to the
  SQLite/`wrap` model; remove `transcripts:` rows/layout.
- Rewrite `implement` `SKILL.md` (replace MANDATORY-session-file framing at
  `:251,261`, the "session file exists" success criteria at `:49,51`, and the
  scattered "create session file" steps with `loaf session start` flow) **and**
  `content/agents/implementer.md:8` in the same change.
- Delete the Transcript Archival section
  (`content/skills/implement/references/session-management.md:220-254`) and the
  list of transcript triggers (`:245-247`).
- Rewrite `bootstrap` session guidance to the SQLite/`wrap` model.
- Rewrite `brainstorm` to capture sparks via `loaf spark capture` (SQLite-first)
  rather than treating draft `.md` as canonical.
- Rewrite read-side skills (`reflect`, `research`, `background-runner`) that read
  session state to the SQLite/`wrap` model.
- Fix `housekeeping` skill: `loaf session housekeeping` → `loaf housekeeping`
  everywhere; reroute `loaf session enrich` guidance; ensure
  `skill(housekeeping)` self-log marker is written.
- Add first-action `loaf session log "skill(<name>): …"` self-logging to the
  user-invocable workflow skills that lack it; fix the AGENTS.md exemplars first.
- Regenerate all built target outputs (`dist/*`, `plugins/loaf/`) and commit them
  with the source changes.
- Verification: a lint asserting no skill references `loaf session housekeeping`
  or `.agents/transcripts/`; a check that the canonical session model is cited in
  exactly the anointed places.

### Out of Scope

- The session **storage** model (body store, `session show/list` read path,
  SQLite-only bodies mechanics) — owned by **SPEC-043**.
- The deterministic markdown render of a session body and its drift gate — owned
  by **SPEC-044**.
- Canonicalizing the session **status enum** and the "schema in exactly one file"
  lint — owned by **SPEC-049** (this spec cites, does not define, the enum).
- Build-integrity / harness-parity / harness-language tokenization — owned by
  **SPEC-047** (WS-A); SPEC-048 consumes its tokens, does not build them.
- General skill de-bloat (orchestration thin-hub, research mode cuts) beyond the
  session-model surface — owned by **SPEC-050** (WS-D).
- Routing/description rewrites — owned by **SPEC-051** (WS-E).
- Changing `loaf session`/`loaf housekeeping` CLI *behavior* — this spec aligns
  guidance to existing commands; it does not add or alter command surface.

### Rabbit Holes

- **Re-litigating the SQLite decision.** SPEC-040 is `complete`; sessions are
  SQLite. This spec aligns the suite, not the architecture.
- **Defining a new status enum.** Resist; SPEC-049 owns it. Cite the runtime
  enum (`internal/state/session_*.go`) as interim truth, no more.
- **Touching every skill that merely mentions "session."** Limit to the named
  set in dependency order; a wider sweep risks the non-atomic spine the eval
  warns against (§7.3).
- **Folding transcript capture into SQLite "properly."** No — removal only.
  Raw transcript storage stays out of scope (SPEC-040:162).
- **Self-logging as a 130-edit standalone workstream** (eval §3 M1). Add it to
  the user-invocable workflows touched in this convergence + fix the exemplars;
  do not chase a mechanical line-count target.

### No-Gos

- Do not land a partial convergence. The spine ships atomically or not at all.
- Do not reintroduce hand-authored session `.md` as a source surface.
- Do not document `.agents/transcripts/` or transcript archival anywhere.
- Do not present `loaf session housekeeping` (a no-op in SQLite mode) as a
  working command in any skill.
- Do not invent a session vocabulary that conflicts with SPEC-049 / the runtime
  enum.
- Do not introduce Claude-isms; use SPEC-047 harness-language tokens.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Non-atomic landing leaves the spine internally inconsistent (some SQLite-first, some markdown-first) | Medium | High | One PR/commit set covering template → orchestration → implement+profile → bootstrap → brainstorm → read-side; reviewer checks cross-references resolve to the same model |
| SPEC-043 (session bodies) not landed when this ships | Medium | High | Hard dependency: gate SPEC-048 on SPEC-043's session-body store + `session show/list`; until then guidance references nonexistent storage |
| Status-enum guidance contradicts SPEC-049 | Medium | Medium | Cite runtime enum as interim only; coordinate sequencing so SPEC-049 reconciles; add no fourth vocabulary |
| `implement` and `implementer.md` desynchronize (one fixed, one not) | Medium | High | Explicit In-Scope coupling: both edited in the same change; verification asserts neither mentions "MANDATORY session file" |
| Transcript-removal misses a dist copy, leaving the liability shipped | Medium | High | Lint: zero matches for `.agents/transcripts/`/`Transcript Archival` across `content/` + all built outputs |
| `housekeeping` rewrite reroutes to a command with different flags than the skill assumes | Low | Medium | Verify `loaf housekeeping --dry-run/--sessions/--specs/--drafts` flags against `internal/cli` before rewriting examples |
| Self-logging churn introduces noise without the marker fix actually firing | Low | Medium | Tie the `skill(housekeeping)` marker fix to a `wrap`-nudge test; verify the nudge stops mis-firing |
| Built outputs drift from source (uncommitted regeneration) | Medium | Medium | Run `loaf build`; commit `dist/`/`plugins/` with source per CLAUDE.md "Before Committing" |

## Open Questions

- [ ] Does SPEC-049 land before or after SPEC-048? If after, confirm the interim
  enum source (`internal/state/session_*.go`) is the right citation and that
  SPEC-049 will reconcile the three documents this spec touches.
- [ ] After SPEC-043, is `content/templates/session.md` retained as a *render
  template* consumed by the renderer, or demoted to a pure journal-format
  *reference* doc? (Affects whether it lives in `templates/` or `references/`.)
- [ ] Should `loaf session enrich` guidance survive in `housekeeping` at all once
  sessions are SQLite-only bodies (SPEC-029 writes rows, not file lines), or move
  wholesale into the SQLite enrichment path?
- [ ] Exact set of "read-side" skills beyond `reflect`/`research`/`background-runner`
  that reference session state — confirm by grep before the atomic change so none
  are left markdown-first.
- [ ] Which user-invocable workflows still lack a first-action self-log? Enumerate
  against `content/skills/*` (eval §3 M1 says only 2 of ~19 comply).

## Test Conditions

- [ ] `wrap` is cited as the canonical session model in `.claude/CLAUDE.md` and
  `.agents/AGENTS.md`; no other document presents a competing "create a session
  file" model as primary.
- [ ] `content/templates/session.md` no longer instructs creating
  `.agents/sessions/*.md` as a source surface; it documents the journal entry
  format and entry types as a render template / reference.
- [ ] No skill or agent profile references `loaf session housekeeping`; all
  housekeeping guidance uses `loaf housekeeping`.
- [ ] `content/skills/housekeeping/SKILL.md` writes a `skill(housekeeping)`
  self-log marker on invocation, and `wrap`'s "no housekeeping this session"
  nudge stops mis-firing when housekeeping has run.
- [ ] Zero matches for `Transcript Archival` or `.agents/transcripts/` across
  `content/` and all built outputs (`dist/*`, `plugins/loaf/`).
- [ ] `content/skills/implement/SKILL.md` no longer contains "MANDATORY: Create
  session file BEFORE any other work" or "DO NOT PROCEED WITHOUT A SESSION FILE";
  it teaches `loaf session start`.
- [ ] `content/agents/implementer.md` no longer demands a hand-authored "session
  file" and aligns with the `loaf session start` model — changed in the same
  commit set as `implement`.
- [ ] `orchestration` SKILL + `sessions.md`/`context-management.md`/
  `background-agents.md` present the SQLite/`wrap` model and contain no
  `transcripts:` frontmatter rows/layout.
- [ ] `brainstorm` captures sparks via `loaf spark capture` (SQLite-first); the
  draft `.md` is described as export/projection, not canonical source.
- [ ] Read-side skills (`reflect`, `research`, `background-runner`) reference the
  SQLite session model, not markdown session files as the routing surface.
- [ ] User-invocable workflow skills touched in this convergence include a
  first-action `loaf session log "skill(<name>): …"`; the `shape`, `housekeeping`,
  and `implement` AGENTS.md exemplars match the documented rule.
- [ ] The session vocabulary used in the rewritten content matches the runtime
  enum (`internal/state/session_*.go`) / SPEC-049's canonical enum — no fourth
  vocabulary introduced.
- [ ] `loaf build` succeeds; `npm run typecheck` and `npm run test` pass; built
  outputs committed with source.
- [ ] The whole convergence lands as one atomic change set — no intermediate
  commit leaves some session skills SQLite-first and others markdown-first.

## Priority Order

Tracks land **as a single atomic change** (the spine must never be internally
inconsistent), but in this dependency order within the change. All tracks are
**non-breaking** content/guidance edits over existing commands; the gate is
SPEC-043 (session bodies).

0. **Gate — SPEC-043 session bodies.** Go/no-go: SPEC-043's session-body store
   and `session show/list` read path exist so guidance references real storage.
   *(non-breaking; dependency gate)*
1. **Anoint + schema.** Cite `wrap` in CLAUDE.md/AGENTS.md; rewrite
   `content/templates/session.md` (the schema everyone cites). Go/no-go: the
   canonical model is stated once and the template no longer scaffolds a file.
   *(non-breaking)*
2. **Orchestration.** SKILL + `sessions.md`/`context-management.md`/
   `background-agents.md` to SQLite/`wrap`; strip `transcripts:`. *(non-breaking)*
3. **Implement + profile (coupled).** `implement/SKILL.md` and `implementer.md`
   together; delete `session-management.md` transcript section. Go/no-go: neither
   demands a hand-authored session file. *(non-breaking)*
4. **Bootstrap + brainstorm.** Bootstrap session guidance to SQLite/`wrap`;
   brainstorm to `loaf spark capture`. *(non-breaking)*
5. **Read-side wave.** `reflect`, `research`, `background-runner`. *(non-breaking)*
6. **Housekeeping fix + self-logging.** `loaf session housekeeping` →
   `loaf housekeeping`; write `skill(housekeeping)` marker; add first-action
   self-logging to touched user-invocable workflows; fix AGENTS.md exemplars.
   Go/no-go: `wrap` nudge no longer mis-fires. *(non-breaking)*
7. **Build + verify.** `loaf build`, typecheck, test, commit built outputs; run
   the transcript/dead-command lints. Go/no-go: lints green, outputs committed.
   *(non-breaking)*

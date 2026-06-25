---
title: "Roadmap: Loaf Holistic Restructuring"
type: roadmap
created: 2026-06-21T02:03:42Z
status: draft
source: report-loaf-skill-suite-deep-evaluation
tags:
  - roadmap
  - skills
  - harness
  - targets
  - sqlite-cli-ux
  - install
  - routing-eval
---

# Roadmap: Loaf Holistic Restructuring

A program-level plan that fuses the deep skill-suite evaluation
(`report-loaf-skill-suite-deep-evaluation`, 2026-06-21) with the structural decisions taken in
this session. It defines the **target architecture**, decomposes the work into **seven
workstreams** (each becomes a spec via `/shape`), sequences them with dependencies, and isolates
the **breaking-change/migration gate**.

The governing insight from the evaluation: *the taxonomy is healthy — the diseases are a
half-shipped SQLite migration, a build that validates nothing, and duplicated reference mass.*
The structural decisions below add a deliberate **target-surface simplification** and a
**SQLite CLI-UX investment** on top of finishing that migration.

---

## 1. Target architecture (the end state)

### Harness targets: 6 → 5, simplified
| Target | Before | After | Change |
|--------|--------|-------|--------|
| claude-code | plugin (`plugins/loaf/`) | plugin | unchanged (the one exception to `~/.agents/`) |
| cursor | skills + agents + hooks | skills + agents + hooks | install dir → `~/.agents/` where supported |
| codex | skills + hooks.json | skills + hooks.json | install dir → `~/.agents/`; codex hook semantics fixed |
| opencode | skills + commands + hooks.ts | skills + commands + hooks.ts | install dir → `~/.agents/`; command coverage fixed |
| **amp** | broken `loaf.js` plugin + skills | **fixed TS plugin + skills** | **first-class**: rewrite the plugin as valid TypeScript at Amp's real path (`.amp/plugins/` or `~/.config/amp/plugins/`) using the stable event API |
| **gemini** | skills | **removed** | **dropped entirely** |

### Harness tiers & the parity contract
Parity across the harnesses is a first-class architectural goal, not a nice-to-have.

| Tier | Harnesses | Expectation |
|------|-----------|-------------|
| **First-class (full parity)** | **Claude Code, Codex, Cursor, OpenCode, Amp** | Every user-invocable skill is reachable; hooks/enforcement work via each harness's hook surface; advisory hooks stay advisory; no harness-specific language leaks; behavior is equivalent within each harness's native idioms. |
| **Removed** | Gemini | — |

**Parity = equivalent capability via each harness's native idiom — NOT identical artifacts.**
Skills are reached differently per harness, and that is correct: Claude Code and Amp **auto-load
skills** (no command files needed); Cursor / Codex / OpenCode get **generated command files**.
Hooks likewise use each harness's own surface.

**The parity contract (a testable invariant, owned by WS-A):**
1. Every `user-invocable` workflow skill is reachable on **all five** first-class harnesses —
   as an auto-loaded skill (Claude Code, Amp) or a generated command (Cursor, Codex, OpenCode).
2. Every advisory hook stays advisory on all five; every enforcement hook stays enforcing on all
   five, via each hook surface: Claude Code plugin, Codex `hooks.json`, Cursor `hooks.json`,
   OpenCode `hooks.ts`, **Amp TS plugin** (`.amp/plugins/`).
3. No Claude-isms (or any single-harness terminology) leak into another harness's output.
4. The `~/.agents/` install convention applies to Codex, Cursor, OpenCode, and Amp's *skills*
   (Amp already reads `~/.agents/skills/` natively); Claude Code is the documented exception via
   its plugin mechanism. (Amp's *plugin* uses its own `.amp/plugins/` ⁄ `~/.config/amp/plugins/`
   path, separate from skills.)

A single parity-matrix test enumerates these cells and fails the build on any gap — so the next
skill or hook added cannot silently break parity. This is the structural difference from today,
where each target drifted independently.

### Invariants (unchanged)
- **The loaf CLI is the harness.** Everything routes through it.
- **One global SQLite database is the operational backend AND the source of truth for artifact
  bodies.** Markdown is a projection/export — PR-reviewable in git for durable docs (specs, ADRs),
  never the source (SPEC-040, extended by the decision below). *This migration gets finished.*

### New conventions
- **`~/.agents/` global install convention** for every harness that supports a configurable
  agents/skills directory. Amp already reads `~/.agents/skills/` natively (a free win); Codex,
  Cursor, and OpenCode need their install destinations pointed there. Claude Code keeps its plugin
  mechanism (the documented exception). Per-harness capability must be verified — not every harness
  reads a custom dir today (gemini used `~/.gemini`, codex `~/.codex`, etc.).
- **A first-class CLI UX for browsing SQLite state** — list *and* view reports, drafts, ideas,
  sparks, sessions, tasks, specs with consistent, human-readable output.

### State content & retrieval model (decided)
Today SQLite stores only metadata + relationships + a `sources` row (`path`, `hash`); bodies live
as in-tree `.agents/<type>/*.md`, read from disk (`readImportedSourceBody`). That centralized the
*index* but not the *content* — bodies stay branch/worktree-variant, non-cross-project, and
unsearchable, and writing a `.md` registers nothing (proven: this session's report is absent from
`loaf report list`). Decision:

- **SQLite is the source of truth for ALL artifact bodies** (ideas, sparks, sessions, brainstorms,
  drafts, reports, specs, ADRs, …). Finishes centralization: branch/worktree-immune, cross-project,
  and **searchable** via SQLite FTS5.
- **Durable in-repo docs are also rendered to git as PR artifacts.** Specs and ADRs (and final
  reports — TBD) render deterministically from SQLite into committed markdown (`.agents/specs/…`,
  `docs/decisions/…`) so they are PR-reviewable and persist in history — the same
  generated-artifact-committed-with-source pattern Loaf already uses for `dist/` + `plugins/`.
- **A drift gate guarantees sync:** `loaf check` verifies committed exports match SQLite — the same
  contract as `git diff --exit-code -- dist plugins`. The `.md` is a projection, never hand-edited
  as source.
- **Authoring goes through the CLI/skills**, not hand-edited generated files: `loaf <entity>
  new/edit` writes SQLite (scaffolded from a render template), then re-renders the git export.
  **Templates become render templates, not file scaffolds.**
- **Ephemeral artifacts become SQLite-only** (no in-tree `.md`): ideas, sparks, sessions,
  brainstorms, drafts, tasks. Cleans the `.agents/` tree and removes their branch/worktree variance.

### Consequences of the surface changes (accept explicitly)
- **Amp's plugin is rebuilt, not dropped.** Today's `dist/amp/plugins/loaf.js` is invalid JS
  written against a stale/experimental API shape. It is replaced by a valid TypeScript plugin at
  Amp's real path using the now-stable event API — so Amp keeps full hook enforcement and is
  first-class. The old `loaf.js` artifact goes away (handled by the install change). The cost is
  owning one TS plugin against Amp's API.
- **Dropping gemini and relocating install paths are breaking changes** for already-installed
  users. They require a migration/cleanup mechanism *before* they ship (see WS-G gate).

---

## 2. Workstreams

Each workstream is spec-sized. They are ordered by dependency, not priority.

### WS-A — Build integrity & target simplification  *(keystone; do first)*
The evaluation's keystone (make the build prove its own output) merged with the structural
target changes, because they touch the same Go build code and must be tested together.

- **Validate output:** drop the fake `node` (`build_test.go:31,56`); run real `node --check` /
  `tsc --noEmit` on every emitted JS/TS artifact (opencode `hooks.ts`, the Amp TS plugin);
  **delete the assertion requiring TypeScript syntax inside `loaf.js`** (`build_test.go:283`).
- **Amp plugin → fix it (first-class):** in `build_amp.go`, emit a valid **TypeScript** plugin at
  Amp's real path (`.amp/plugins/` or `~/.config/amp/plugins/`) using the stable event API
  (`session.start`, `tool.call`, `tool.result`), not the broken `dist/amp/plugins/loaf.js`; fix
  the handler that reads an undefined `call` (the param is `input`); drop the
  `@i-know-the-amp-plugin-api-is-wip` header — the API is stable now.
- **Drop gemini:** remove from `targets.yaml`, build wiring, `dist/gemini/`, install detection,
  `install_fenced.go:23`, `install_mcp` gemini handling, and tests.
- **Codex advisory hooks:** default `failClosed:false`; parse `value=="true"`; carry `blocking`/
  `if` (`build_codex.go:629,646,24-30,638-649`).
- **OpenCode command coverage:** drive generation off `user-invocable`, not sidecar presence
  (`build_opencode.go:94`).
- **Cross-target harness-language transform + lint:** tokenize Claude-isms at source
  (`{{HARNESS_NAME}}`, `{{INTERVIEW_TOOL}}`, `{{SUBAGENT_MECHANISM}}`, `{{TODO_TOOL}}`,
  `{{AGENTS_FILE}}`), resolve per target in `transformMd`; add a build-failing content lint over
  the four first-class harnesses.
- **git-workflow commit-format fix** rides here as a cheap correctness blocker
  (`SKILL.md:31,40` → unscoped; the native `check.go:249` hook already rejects scoped commits).
- **Encode the parity contract as a test** (§1): a single parity-matrix test asserting, across all
  five first-class harnesses — skill reachability (auto-loaded for Claude Code/Amp, generated
  command for Cursor/Codex/OpenCode), advisory/enforcement hook semantics preserved on each hook
  surface (incl. the Amp TS plugin), and zero cross-harness language leakage.
- **Regression tests (the durable guard):** JS/TS validity via real `node --check`/`tsc` (opencode
  `hooks.ts`, the Amp TS plugin), codex advisory `failClosed:false`, opencode command
  coverage for all user-invocable skills, cross-harness Claude-ism lint, sidecar-merge
  correctness, and the parity-matrix test above.

*Net: a smaller, self-validating target matrix (Gemini gone; the other five first-class). The Amp
invalid-JS defect is fixed at the source — a valid TS plugin the build actually validates — not
hidden; the build can no longer ship an artifact it hasn't checked.*

### WS-B — State content & retrieval model
The largest investment and the enabler for WS-C. Promote bodies into SQLite, add retrieval, and
wire the git-export projection. (Per §1 "State content & retrieval model".)

- **Bodies into SQLite:** extend the schema so the entity/`sources` rows store body *content*, not
  just `path`+`hash`; `migrate markdown` imports bodies (not just indexes them). SQLite becomes the
  source of truth.
- **Search — the unrealized payoff:** add `loaf search` across all entities via SQLite **FTS5**
  (full-text, filterable by type/tag/status/relationship). Today there is **no** search of
  operational state (only KB's separate QMD).
- **Uniform verbs across every entity:** `list / show / new / edit / archive / link` consistent for
  reports, ideas, sparks, sessions, tasks, specs, brainstorms — and **model the missing types**
  (`draft`, `plan`, `handoff`, `council`). Add `report show` (today list-only) and
  `brainstorm capture` (today missing).
- **Git-export projection for specs + ADRs (+ final reports, TBD):** deterministic render to
  committed markdown as part of the PR, with a `loaf check` drift gate mirroring `dist`/`plugins`.
- **Write → register atomically:** `loaf <entity> new` scaffolds from a render template AND creates
  the row; a `Write`-side hook is the fallback so an artifact can never be written-but-unregistered.
- **Human-readable output by default** (today JSON-first); keep `--json` for agents. Consider a
  unified `loaf browse` / TUI.
- **Fix status-vocabulary drift** (reports surface `active`/`unknown` vs the real
  `draft`/`final`/`archived`); reconcile with the session-status enum (WS-C).
- **Reconcile `cli-reference` + `agent-help`** into one generated catalog.

### WS-C — Session-model convergence  *(finish the migration; land atomically)*
- Anoint `wrap` as the canonical session model (cite it in CLAUDE.md / AGENTS.md).
- Rewrite in dependency order: `content/templates/session.md` (the schema everyone cites) →
  `orchestration` (SKILL + `sessions.md`/`context-management.md`/`background-agents.md`) →
  `implement` **+ the `implementer.md` profile in the same change** → `bootstrap` →
  `brainstorm` (SQLite-first sparks) → read-side (`reflect`, `research`, `background-runner`).
- **Remove transcript-archival guidance** (SPEC-040 scope violation).
- **Fix `housekeeping` dead command** (`loaf session housekeeping` → `loaf housekeeping`) and its
  `skill(housekeeping)` self-log marker (which fixes `wrap`'s mis-firing nudge).
- Reconcile the session **status enum** (3 incompatible vocabularies today) against
  `internal/state/session_*.go`; add a lint: session schema lives in exactly one file.
- **Sessions become SQLite-only bodies** (per the WS-B content model) — no in-tree session `.md`;
  the markdown view is render-on-demand. This is the convergence's end state, not just "point at CLI".
- **Land atomically** — a half-migrated spine (some skills SQLite-first, others markdown-first,
  cross-referencing each other) is worse than the current consistent-but-wrong state.

### WS-D — De-bloat & content hygiene
Low-risk content edits; sequence after WS-C to avoid churn collisions on shared files.

- `orchestration` → thin hub (cut ~1,600 duplicating reference lines + 10 unwired scripts).
- `architecture` = single ADR source of truth (strip the contradictory ADR spec from
  `documentation-standards`).
- `research` de-scope (drop ideation/vision modes); `interface-design` reposition as
  reference + negative-route to `artifact-design`/`frontend-design`; `foundations` de-scope.
- Unrunnable tooling (infra PyYAML import; power-systems sidecar/script mismatch).
- Structure/lint hygiene: `## Contents` headers; `knowledge-base` → `.agents/AGENTS.md`;
  stale skill-name refs; `triage` stray fence; `shape` `source_sessions`; correct
  `targets.yaml` phantom-sidecar docs.

### WS-E — Routing eval + validated description rewrites  *(eval-gated, per decision)*
- Build the **skill-creator routing-eval harness**: user-utterance → expected-skill, current vs
  proposed description, over the named conflict pairs (idea/triage, research/brainstorm,
  strategy/reflect, ship/release, architecture/shape, foundations/git-workflow/docs).
- Refresh the stale `cli/scripts/eval-skill-routing.mjs` (references nonexistent skills).
- **Only ship description rewrites that measurably improve routing** (foundations, research,
  brainstorm, interface-design, strategy, …). No blind rewrites.
- Update `docs/knowledge/skill-architecture.md` (stale: says 33 skills; reality 34–35).
- Add the **self-logging first-action line** to user-invocable workflows (batched here or in WS-C).

### WS-F — `~/.agents/` install convention
- **Per-harness capability investigation** first: which harnesses (cursor, codex, opencode, amp)
  can read a configurable `~/.agents/` skills/agents dir, and how.
- Rework install destination logic accordingly; Claude Code stays on its plugin mechanism.
- **Breaking** (path relocation) → depends on WS-G's migration mechanism.

### WS-G — Breaking-change migration mechanism + taxonomy decisions  *(gated; needs sign-off)*
- **Migration mechanism** (the prerequisite): does `loaf install --upgrade` remove orphaned
  skills (dropped gemini, retired skills) and handle relocated paths? Define deprecation window,
  tombstone/alias, and upgrade semantics. **Nothing breaking ships before this exists.**
- **State/body migration** (gated breaking change, pairs with WS-B): `migrate markdown` imports
  bodies into SQLite and stops treating in-tree ephemeral `.md` as source. Define a one-time,
  reversible, backed-up migration before ephemeral `.md` are removed from the tree.
- **Taxonomy decisions:** retire `thermo-nuclear-code-quality-review`; decide `debugging`
  (tighten description vs `disable-model-invocation` — *not* `user-invocable:false`, which does
  not stop model routing); language/domain **opt-in install packs**; `librarian` profile
  (retire or fully wire).

---

## 3. Sequencing & dependencies

```
WS-A  Build integrity + target simplification   ──┐  (keystone; gemini drop + amp plugin fix here)
WS-B  CLI state-browse UX                        ──┤  (parallel with A; different code layer)
                                                   │
WS-C  Session convergence (atomic)  ←── depends on A (validated build) + B (read commands)
                                                   │
WS-D  De-bloat & hygiene            ←── after C (avoid file churn) ; parallelizable
WS-E  Routing eval + descriptions   ←── independent harness ; parallel with D
                                                   │
WS-F  ~/.agents/ install convention ←── after A (stable matrix) ; needs WS-G migration
WS-G  Migration mechanism + taxonomy ←── gates F and all breaking changes ; USER SIGN-OFF
```

Recommended order of shaping into specs: **A → B → C → (D ∥ E) → G → F**.

- **A and B can start in parallel** (build layer vs command/state layer).
- **C waits on A** (so the build validates the convergence) and benefits from **B** (good CLI
  read commands to point skills at).
- **D and E are parallel** once C lands; both are mostly content/tooling.
- **G is the gate** for every breaking change (gemini drop's user-side cleanup, `~/.agents/`
  relocation, retires, packs). **F and the breaking parts of A** must not reach users until G's
  migration mechanism exists.

---

## 4. Breaking-change ledger (CLAUDE.md: ask before breaking changes)

| Change | Who it breaks | Mitigation (WS-G) |
|--------|---------------|-------------------|
| Drop gemini | gemini users | upgrade removes orphaned `~/.gemini` skills; changelog + announcement |
| Amp plugin rebuilt + relocated | amp users | strictly better — current plugin is broken; upgrade removes old `loaf.js`, installs the TS plugin at Amp's path |
| `~/.agents/` relocation | all non-Claude installed users | upgrade migrates/relinks old path; cleanup of stale dirs |
| Retire `thermo`, opt-in packs | users who installed them | tombstone/alias; upgrade removes; changelog |
| Bodies move into SQLite (ephemeral `.md` no longer source) | anyone reading/editing `.agents/{ideas,sessions,sparks,brainstorms,drafts,tasks}/*.md` directly | `migrate markdown` imports bodies + `state backup`; ephemeral `.md` become render-on-demand; specs/ADRs still rendered to git |

All of the above are **held behind WS-G** and require your explicit sign-off before shipping.

---

## 5. Evaluation-finding → workstream traceability (nothing dropped)

- Build validates nothing / Amp invalid JS / codex hooks / opencode commands / Claude-isms → **WS-A**
- `housekeeping` dead command, transcript removal, session drift, status enum, self-logging → **WS-C**
- bodies-in-SQLite, FTS5 search, read/write/register UX, missing entity types (draft/plan/handoff/
  council), git-export of specs+ADRs, status drift, cli-reference catalog gap → **WS-B**
- orchestration bloat, ADR duplication, research/interface/foundations scope, stale refs, tooling → **WS-D**
- description collisions (foundations/research/brainstorm/interface/strategy), stale routing eval,
  taxonomy doc drift → **WS-E**
- install-path convention → **WS-F**
- thermo / debugging / packs / librarian / migration → **WS-G**

---

## 6. Spec program (WS → SPEC)

The whole process is now captured as a cross-referenced spec program (all `status: drafting`).
WS-B split into five specs by reversibility; status-unification moved from this roadmap's WS-B
into WS-C per the SPEC-043 adversarial review. The whole program is governed by **ADR-016**
(artifact storage trichotomy: nouns→SQLite, verbs→git, markdown→render; the DB stores outputs +
provenance pointers, never code) — surfaced by the parallel gridsight thermonuclear-migration
session and now the citable rule behind SPEC-043/044/050/053/054.

| Spec | WS | Scope | Breaking? | Gated on |
|------|----|-------|:--:|---------|
| **SPEC-043** | B | SQLite-native bodies + uniform verbs + FTS5 search (additive core) | no | — |
| **SPEC-044** | B | Durable-doc render, finalization & drift gate (self-consistency, not DB-mirror) | no | SPEC-043 |
| **SPEC-045** | B | Ephemeral-to-SQLite cutover (`.md` removal) | **yes** | SPEC-053 |
| **SPEC-046** | B | `docs/` Tier-2 indexing & cross-project search | no | SPEC-043 |
| **SPEC-054** | B | Rich artifact entity model (report→finding→verdict+run) & `--format` export | no | SPEC-043 |
| **SPEC-047** | A | Build integrity, parity contract & target simplification (keystone) | no¹ | — (¹user-side gemini drop → SPEC-053) |
| **SPEC-048** | C | Session-model convergence to SQLite (atomic) | no | SPEC-043 |
| **SPEC-049** | C | Status-vocabulary unification | **yes** | SPEC-053 |
| **SPEC-050** | D | Skill de-bloat & content hygiene | no | — |
| **SPEC-051** | E | Routing eval & validated description rewrites | no | — |
| **SPEC-052** | F | `~/.agents` install convention & harness install parity | **yes** | SPEC-053 |
| **SPEC-053** | G | Breaking-change migration mechanism & taxonomy decisions (the gate) | **yes** | self |

Sequencing (unchanged): **A → B → C → (D ∥ E) → G → F**. The four breaking specs (045, 049, 052,
and the user-side gemini cleanup in 047) are hard-gated on SPEC-053.

## 7. Next step

Recommended first spec to break down: **SPEC-047** (build integrity — the keystone, independent,
mostly non-breaking) and/or **SPEC-043** (the WS-B core — additive, immediately valuable: search +
body storage). SPEC-043 is already drafted and reviewed; the other ten are drafting skeletons that
should each get a `/shape`-style refinement + adversarial review (like SPEC-043 got) before
breakdown. Each spec lands on its own `feat/<slug>` branch at breakdown.

Before implementing SPEC-047 or SPEC-043, apply the shared cross-spec contract lock in
`.agents/drafts/20260624-115322-loaf-restructuring-shared-contracts-lock.md`. It fixes the body
store shape, render stamp, migration/deprecation manifest, status boundary, and harness parity
contract so the dependency waves do not re-decide them independently.

*This roadmap is the parent; each spec above is a child.*

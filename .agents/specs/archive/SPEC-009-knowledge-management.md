---
id: SPEC-009
title: Knowledge Management
source: .agents/drafts/brainstorm-loaf-cli-knowledge-harness.md
created: '2026-03-14T19:48:00.000Z'
status: complete
---

# SPEC-009: Knowledge Management

## Problem Statement

Knowledge accumulates across overlapping surfaces (docs/knowledge, docs/decisions,
MEMORY.md, session files), drifts silently, and no tool detects when instructions go
stale. A developer changes `src/pipeline/registry.py` but the knowledge file describing
the pipeline's contract sits unchanged. Agents work with outdated context. Humans don't
know what to review.

Projects need a living knowledge base where **decay is visible** and **maintenance is
woven into the work loop**, not a separate chore.

## Strategic Alignment

- **Vision:** Knowledge Management is Pillar 2 of Loaf's evolution (after Skills &
  Distribution, before Autonomous Execution). This is the next major capability.
- **Personas:** Benefits both the developer (knows what's stale, can review efficiently)
  and the agent (gets nudged to update knowledge as a side-effect of coding).
- **Architecture:** Aligns with ADR-003 (QMD as retrieval backend), ADR-004 (knowledge
  naming convention), ADR-006 (agent-creates, human-curates). The `covers:` field +
  git-based staleness detection is the core innovation no other AI tool provides.

## Solution Direction

`loaf kb` CLI commands for knowledge lifecycle management, a `knowledge-base` skill for
agent guidance, and lifecycle hooks that surface staleness in real-time.

**Core innovation:** The `covers:` frontmatter field links knowledge files to code paths
via globs. Combined with `git log --since={last_reviewed}`, this detects staleness
automatically. Git handles globs natively in pathspecs, so staleness checks are a single
`git log` invocation per knowledge file — no glob expansion needed.

**QMD is a soft dependency.** Core commands (check, validate, status, review) work
without QMD via direct file parsing + git. Commands that manage collections (init
registration, import) use QMD if present and fail gracefully with an install message if
not. This delivers the core value prop immediately while laying the foundation for
QMD-powered retrieval at scale.

## Scope

### In Scope

**CLI Commands** (`loaf kb`):

| Command | Purpose | Requires QMD |
|---------|---------|:---:|
| `kb init` | Scaffold dirs, register QMD collections, update loaf.json | Partial (dirs always, collections if QMD present) |
| `kb check` | Staleness report: covers × git log | No |
| `kb check --file <path>` | Reverse lookup: which knowledge files cover this path? | No |
| `kb validate` | Frontmatter consistency (required fields, valid globs, broken refs) | No |
| `kb status` | Summary view (total files, stale count, avg review age) | No |
| `kb review <file>` | Reset `last_reviewed` to now | No |
| `kb import <name>` | Register external project's QMD collection | Yes |

All display commands support `--json` for hook consumption.

**knowledge-base Skill** (`content/skills/knowledge-base/`):
- Agent guidance for creating, updating, and reviewing knowledge files
- Reference docs: frontmatter schema, naming conventions (per ADR-004)
- Template: knowledge file scaffold with standard frontmatter
- Sidecar: `user-invocable: false` (pure reference, not a workflow)

**Lifecycle Hooks:**
- **SessionStart:** Run `loaf kb status --json`, surface stale count alongside
  existing session context
- **PostToolUse:** On Edit/Write to a file, check if any knowledge file's `covers:`
  matches the path. If the knowledge file is stale, nudge once per session (per-session
  single nudge — temp file tracks already-nudged files, cleaned up at SessionEnd)
- **SessionEnd:** If covered files were modified during the session, prompt for
  knowledge consolidation

**Configuration** (`.agents/loaf.json` expansion):
```json
{
  "knowledge": {
    "local": ["docs/knowledge", "docs/decisions"],
    "staleness_threshold_days": 30,
    "imports": []
  }
}
```

**Knowledge File Schema** (enforced by `kb validate`):
```yaml
---
topics: [tag1, tag2]             # Required (min 1)
last_reviewed: 2026-03-14        # Required (ISO 8601 date)
covers:                          # Recommended (enables staleness detection)
  - "cli/lib/kb/*.ts"
  - "config/hooks.yaml"
consumers: [backend-dev, pm]     # Optional (agent routing hints)
depends_on: [other-file.md]      # Optional (cross-references)
implementation_status: stable    # Optional (in-progress | stable | deprecated)
---
```

`covers:` globs resolve relative to the **git repository root**.

### Out of Scope

- Personal knowledge base / cross-career knowledge (Phase 4+)
- Knowledge repos / cross-project sharing beyond QMD import (Phase 4)
- CI/CD integration (future spec)
- Overnight implementation loop (Phase 5)
- Knowledge versioning / time travel
- Quality scoring of agent-written knowledge
- Search — QMD handles retrieval; this spec handles lifecycle
- `loaf health` / `loaf doctor` — generic project health aggregation (separate spec;
  SPEC-009's SessionStart hook is kb-only, designed to be replaced by `loaf health`)

### Rabbit Holes

- **Glob expansion performance:** Don't expand globs into file lists then query git
  per-file. Use `git log --since={date} -- <glob>` directly — git handles pathspec
  globs natively. Single invocation per knowledge file.
- **Building an index file (like TASKS.json):** Knowledge files are simpler than tasks.
  Frontmatter IS the index. Directory scanning is fast enough for tens of files. Add
  caching only if profiling shows a need.
- **Interactive fuzzy search for import:** Keep `kb import` simple — accept a name
  argument, register the QMD collection. Don't build a TUI picker in v1.
- **TypeScript hooks:** Existing hooks are all bash. Don't switch languages for
  knowledge hooks. Keep bash wrappers thin; heavy logic lives in `loaf kb` commands.
- **Cross-project `local-covers` mapping:** Designed in the knowledge-management-design
  doc but too complex for v1. Import registers the collection; local staleness mapping
  is a follow-up.

### No-Gos

- Don't build a search engine (QMD exists for retrieval)
- Don't auto-commit knowledge changes (agent-creates, human-curates per ADR-006)
- Don't block agent work on stale knowledge (advisory nudges only, never blocking)
- Don't add unnecessary dependencies (glob matching uses `picomatch` — added as direct
  dep; git pathspecs handle forward lookups)

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Broad `covers:` globs cause constant staleness | Medium | Medium | `staleness_threshold_days` config (default 30); `kb validate` warns on very broad globs |
| QMD not installed → confusing errors | Medium | Low | Graceful detection with `which qmd`; helpful install message; core commands work without it |
| PostToolUse hook adds latency to every edit | Low | Medium | Hook shells out to `loaf kb check --file <path> --json` which is a single git query; fast path if no knowledge files have matching covers |
| `covers:` paths reference old/renamed code paths | Medium | Low | `kb validate` reports "no files match this covers: pattern" as a warning; organic discovery of stale paths |
| Alert fatigue from too many nudges | Medium | High | Per-session single nudge (each knowledge file nudged at most once per session via temp file tracking) |

## Resolved Questions

- [x] **Glob matching:** Use `picomatch` (add as direct dep). Only needed for reverse
  lookups (`kb check --file`, PostToolUse hook). Git pathspecs handle forward lookups.
- [x] **Coverage %:** Skip for v1. Show total/stale/fresh counts and avg review age.
- [x] **Spark surfacing:** Out of scope for SPEC-009. Build a generic `loaf health`
  (or `loaf doctor`) command separately that aggregates stale knowledge, unprocessed
  sparks, overdue tasks, etc. SPEC-009's SessionStart hook focuses on kb status only.
  The future `loaf health` command becomes the SessionStart aggregation point.

## Test Conditions

- [ ] `loaf kb init` creates `docs/knowledge/` and `docs/decisions/` if missing; is
  idempotent (re-running doesn't error or duplicate)
- [ ] `loaf kb init` registers QMD collections if QMD is available; skips gracefully if
  not
- [ ] `loaf kb check` correctly identifies stale files (covered paths modified after
  `last_reviewed`)
- [ ] `loaf kb check` reports fresh files when no commits exist since `last_reviewed`
- [ ] `loaf kb check` handles knowledge files without `covers:` (skip, not error)
- [ ] `loaf kb check --file src/foo.ts` lists all knowledge files whose `covers:` match
  that path
- [ ] `loaf kb validate` catches: missing `topics`, missing `last_reviewed`, invalid
  date format, `covers:` globs matching no files
- [ ] `loaf kb status` shows total count, stale count, and average review age
- [ ] `loaf kb review <file>` updates `last_reviewed` to current date
- [ ] `loaf kb import <name>` registers a QMD collection and updates `loaf.json`
  imports; errors helpfully if QMD not installed
- [ ] All display commands support `--json` for structured output
- [ ] PostToolUse hook fires on first covered-file edit and is silent on subsequent
  edits to same coverage area within a session
- [ ] SessionEnd hook prompts for consolidation only if covered files were edited

## Circuit Breaker

**At 50% appetite (end of week 1.5):** Core library + `kb validate`, `kb status`, and
`kb review` must be working with tests. If not, cut `kb import` and simplify hooks to
SessionStart-only.

**At 75% appetite (end of week 2):** Staleness detection (`kb check`) must be working
with hooks. If QMD integration is not started, defer it entirely — the core value
(staleness lifecycle) ships without QMD. Import becomes a follow-up spec.

## Implementation Phasing

**Week 1 — Core library + basic commands:**
- `cli/lib/kb/types.ts`, `resolve.ts`, `loader.ts`, `validate.ts`
- `cli/commands/kb.ts` with: `validate`, `status`, `review`
- Register `registerKbCommand()` in `cli/index.ts`
- Unit tests for loader and validation
- Config expansion in loaf.json

**Week 2 — Staleness detection + hooks:**
- `cli/lib/kb/staleness.ts` (git log integration, pathspec matching)
- `kb check` and `kb check --file <path>` subcommands
- SessionStart, PostToolUse, SessionEnd hooks
- Register hooks in `config/hooks.yaml`
- Unit + integration tests for staleness

**Week 3 — QMD integration + skill + import:**
- `cli/lib/kb/qmd.ts` (availability check, collection CRUD)
- `kb init` (scaffold + QMD collections)
- `kb import <name>` (QMD collection registration)
- `content/skills/knowledge-base/` (SKILL.md, sidecar, references, template)
- Register skill in plugin-groups
- Integration tests for init idempotency

## Key Files

**Patterns to follow:**
- `cli/commands/task.ts` — command registration, ANSI output, `--json`, subcommands
- `cli/lib/tasks/types.ts` — type definitions module pattern
- `cli/lib/tasks/resolve.ts` — project root resolution, `findAgentsDir()`
- `content/hooks/post-tool/orchestration-generate-task-board.sh` — PostToolUse hook
  parsing `$TOOL_INPUT`
- `config/hooks.yaml` — hook registration DSL

**Design references:**
- `docs/knowledge/knowledge-management-design.md` — authoritative design document
- `docs/decisions/ADR-003-qmd-as-retrieval-backend.md` — QMD integration decision
- `docs/decisions/ADR-004-knowledge-naming-convention.md` — naming conventions
- `docs/decisions/ADR-006-agent-creates-human-curates.md` — authoring model

## Notes

- Existing knowledge files already use the `covers:` frontmatter informally (e.g.,
  `build-system.md`). The tooling formalizes what's already in practice.
- Some existing `covers:` paths reference `src/` which has been restructured to
  `content/`, `cli/`, `config/`. Running `kb validate` post-ship will surface these
  organically.
- This spec supersedes the original SPEC-009 draft. See brainstorm context at
  `.agents/drafts/brainstorm-loaf-cli-knowledge-harness.md`.

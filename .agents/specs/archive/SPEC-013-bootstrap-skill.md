---
id: SPEC-013
title: /bootstrap — Interactive project bootstrapping skill
source: direct
created: '2026-03-22T01:28:01.000Z'
status: complete
---

# SPEC-013: /bootstrap — Interactive project bootstrapping skill

## Problem Statement

Starting a new project with Loaf requires multiple CLI steps (`loaf init`, `loaf build`, `loaf install`), then manually populating VISION.md, STRATEGY.md, ARCHITECTURE.md, and AGENTS.md with project context. This is friction at the moment when the user has the most energy and context — they have a brief or concept and want to get moving. There's no intelligent layer that interviews the user, extracts intent from existing materials, and populates project documents with real content. The mechanical setup and the intelligent setup are tangled together with no clear two-step path.

## Strategic Alignment

- **Vision:** Loaf as an agentic harness — `/bootstrap` is the first-contact experience. It sets the tone for how the framework helps.
- **Architecture:** Follows "hooks automate what skills teach" — delegates scaffolding to CLI, intelligence lives in the skill. Claude Code-first with cross-harness fallback instructions.
- **ADR-006:** Agent creates, human curates — the skill drafts, the user iterates.

## Solution Direction

### Two-Step Mental Model

The 0-to-1 experience splits cleanly into **mechanical** (CLI) and **intelligent** (skill):

1. **`loaf setup`** — One CLI command handles all scaffolding: runs `loaf init`, builds targets, installs to detected tools. Prints a clear handoff: "Run `/bootstrap` to set up your project."
2. **`/bootstrap`** — Pure intelligence: interviews the user, reads briefs, populates documents, records decisions.

This separation means `/bootstrap` never needs to worry about directory structure or tool installation — it starts from a clean scaffold and focuses entirely on project understanding.

### `loaf setup` CLI Command

A new CLI command that wraps the mechanical bootstrapping into a single step:

```bash
loaf setup                    # In an existing directory
loaf setup ./my-project       # Creates directory first (optional)
```

**What it does (in order):**
1. Creates target directory if a path argument is provided
2. Runs `loaf init` (scaffolds `.agents/`, `docs/`, templates — idempotent)
3. Runs `loaf build` (compiles skills + agents for all targets)
4. Runs `loaf install --to all` (distributes to detected tools)
5. Prints handoff message: "Setup complete. Run `/bootstrap` in Claude Code to set up your project."

**What it does NOT do:**
- No intelligence, no questions, no document population — that's `/bootstrap`'s job
- No brief reading or analysis — purely mechanical
- No symlink creation — `/bootstrap` handles that after docs are populated

**Error handling:** If any step fails, report the failure clearly and stop. Don't half-install. `loaf setup` should be safe to re-run (all sub-commands are idempotent).

### /bootstrap Skill Flow

1. **Detect state** — scan project (`.agents/`, docs, code, git history, manifests), auto-classify as brownfield / greenfield+brief / greenfield+empty. Briefly confirm findings.
2. **Analyze** — brownfield: deep-read README, code patterns, existing docs. Greenfield+brief: deeply analyze brief/prompt. Greenfield+empty: skip to interview.
3. **Interview** — depth varies by mode. Brownfield: focused gap-filling. Greenfield+brief: challenge assumptions, probe gaps. Greenfield+empty: full exploratory grilling. All use `AskUserQuestion`. Interview structure follows [references/interview-guide.md](references/interview-guide.md).
4. **Persist brief** — save or synthesize `docs/BRIEF.md`. Brownfield may produce a brief from what was learned. Greenfield+empty synthesizes the interview.
5. **Draft documents** — populate VISION.md (always), STRATEGY.md and ARCHITECTURE.md (conditionally based on available info), AGENTS.md (incrementally with detected stack info + recommended Loaf skills)
6. **Bootstrap knowledge base** — if `loaf kb init` is available, delegate to it; otherwise scaffold `docs/knowledge/` directory with a README explaining its purpose
7. **Structured review** — present each document section-by-section for iteration using `AskUserQuestion`
8. **Auto-create symlinks** — per Loaf convention (`.claude/CLAUDE.md → .agents/AGENTS.md`, `./AGENTS.md → .agents/AGENTS.md`)
9. **Record session** — save the bootstrap interview as `.agents/sessions/{timestamp}-bootstrap.md` following the standard session template, preserving project decision rationale
10. **Suggest next steps** — offer pipeline paths (`/brainstorm`, `/idea`, `/research`, `/shape`) and suggest `loaf doctor` to verify setup

### State Detection & Mode Selection

`/bootstrap` intelligently detects the project state — no mode selection prompt. Detection signals:

| Signal | Brownfield | Greenfield + Brief | Greenfield + Empty |
|--------|-----------|-------------------|-------------------|
| `.git` with commit history | Yes | — | — |
| Source code files | Yes | — | — |
| `package.json`, `Gemfile`, `go.mod`, etc. | Yes | — | — |
| README.md or existing docs | Yes | — | — |
| `docs/BRIEF.md` or brief passed as argument | — | Yes | — |
| Empty/minimal directory, no brief | — | — | Yes |

After detection, briefly confirm what was found ("I see a Python/FastAPI project with 40+ commits and a README") and proceed. The user can correct if the detection is wrong, but shouldn't have to choose a mode.

### Three Modes, Varying Interview Depth

**Brownfield** — Analyze the existing repo thoroughly: read README, detect stack, scan code patterns, check for existing docs. Interview focuses on filling gaps and capturing nuances that aren't in the code — project direction, team context, what's working vs. what needs to change. Lighter interview, heavier analysis.

**Greenfield + Brief** — Read and deeply analyze the brief (or prompt). Extract everything possible: goals, users, constraints, technical hints. Interview fills the gaps — challenge assumptions, explore edge cases, probe technical decisions the brief leaves open. Moderate interview depth.

**Greenfield + Empty** — The most exploratory mode. No brief, no code, just a person with an idea (or just a spark). Deep, structured interview to help a nugget flourish — this is product discovery energy, helping the user think through the idea from scratch. Follows the full four-phase interview guide (Excavation → Sharpening → Grounding → Synthesis). Produces `docs/BRIEF.md` as a synthesis of the interview before proceeding to VISION/STRATEGY/ARCHITECTURE.

See [references/interview-guide.md](references/interview-guide.md) for the complete interview framework, including must-ask questions, phase transition signals, depth adaptation per mode, and anti-patterns.

### Brief Intake

Accepts input in four forms:
- **Text in prompt:** `/bootstrap Build a CLI tool that manages knowledge bases across projects`
- **File path:** `/bootstrap ~/Desktop/project-brief.md` or `/bootstrap ./brief.md`
- **Folder path:** `/bootstrap ./docs/` — reads all markdown files in the folder
- **No input:** Fully interactive interview from scratch

Processing: Read → persist as `docs/BRIEF.md` → synthesize key themes → interview about gaps → draft documents.

### Brief as Artifact

The canonical brief location is `docs/BRIEF.md`. Intake behavior varies by source:

- **File already in project** (e.g., `docs/BRIEF.md`): Use in place, don't copy. Add frontmatter if missing.
- **External file path** (e.g., `~/Desktop/project-brief.md`): Copy content to `docs/BRIEF.md`.
- **Inline text**: Persist to `docs/BRIEF.md`.
- **Interview-only**: Synthesize interview responses into `docs/BRIEF.md`.

Frontmatter:

```yaml
---
source: file | text | folder | interview
original_path: ~/Desktop/project-brief.md  # if copied from external source
created: 2026-03-27T01:54:00Z
---
```

This gives other skills (particularly `/shape` and `/brainstorm`) a canonical reference to the original project intent. `docs/BRIEF.md` is a project document — humans may read and update it alongside VISION.md and STRATEGY.md.

### Document Population

| Document | When | Content Source |
|----------|------|----------------|
| docs/BRIEF.md | Always (if not already present) | Intake text, external file, or synthesized interview |
| docs/VISION.md | Always | Brief + interview (purpose, target users, success criteria, non-goals) |
| docs/STRATEGY.md | When enough info available | Interview (current focus, priorities, constraints, open questions) |
| docs/ARCHITECTURE.md | When technical choices are stated | Brief + detected stack (overview, components, technology choices) |
| .agents/AGENTS.md | Always (incremental) | Detected stack (build/test commands, project structure, recommended Loaf skills) |

### AGENTS.md Incremental Population

Start with detected stack info and build up:
- Build commands (from package.json, Makefile, pyproject.toml, etc.)
- Test commands (detected test framework)
- Project structure overview
- Recommended Loaf skills section (based on detected languages/frameworks)
- Paths to docs and knowledge base
- Add more as the interview reveals useful conventions

### Knowledge Base Integration (Soft Dependency)

If `loaf kb init` CLI exists: delegate KB scaffolding to it.
If not: scaffold `docs/knowledge/` directory with a README explaining the knowledge base concept and how it will be used when the tooling is available.

After scaffolding, ask the user if they have other Loaf projects they'd like to import knowledge from (don't auto-detect — ask explicitly).

### Structured Review Rounds

After drafting each document, present section-by-section for iteration:
- "Is the problem statement accurate?"
- "Are these the right non-goals?"
- "Anything missing from success criteria?"
- Revise based on feedback, repeat until user approves each section
- Use `AskUserQuestion` throughout

### Session Recording

The bootstrap interview is recorded as a standard session file: `.agents/sessions/{YYYYMMDD}-{HHMMSS}-bootstrap.md`

This preserves:
- Why specific technical choices were made
- What alternatives were considered and rejected
- The original problem framing and user intent
- Decisions made during greenfield/brownfield mode selection

Future agents can reference the bootstrap session to understand project origins — "why did we choose Go over Python?" has an answer in the session log, not just the resulting ARCHITECTURE.md.

### Cross-Harness Support

Claude Code is the primary experience (uses `AskUserQuestion`, Write/Edit tools).
For other harnesses: include a section in the skill that describes the Bash-based equivalent workflow:
- Run `loaf setup` (or `loaf init` + `loaf build` + `loaf install` manually)
- Manually populate documents
- Run `loaf kb init` if available

## Scope

### In Scope
- Skill file: `content/skills/bootstrap/SKILL.md` + sidecar
- Intelligent auto-detection of brownfield / greenfield+brief / greenfield+empty
- Brief intake from text, file path, or folder path
- Brief persistence/synthesis as `docs/BRIEF.md` artifact (all modes)
- Three interview depths: exploratory (empty), gap-filling (brief), nuance-capturing (brownfield)
- VISION.md population and structured iteration (always)
- Conditional STRATEGY.md and ARCHITECTURE.md population
- Incremental AGENTS.md population (build commands, language, framework, recommended Loaf skills)
- Knowledge base directory scaffolding (with graceful degradation if `loaf kb init` unavailable)
- Symlink auto-creation per convention
- Session recording of bootstrap interview
- Pipeline exit with next-step suggestions (including `loaf doctor`)
- Cross-harness alternative instructions (Bash-based fallback)
- `loaf setup` CLI command (init + build + install in one shot)

### Out of Scope
- Knowledge base content seeding beyond directory scaffolding
- Spec or task creation (that's `/shape` and `/breakdown`)
- CI/CD setup, deployment config, or infra bootstrapping
- Importing knowledge from sibling projects (suggest only, don't execute)
- `loaf doctor` health-check command (separate micro-spec, referenced as next step)
- `loaf adopt` dedicated brownfield command (separate micro-spec — brownfield mode in `/bootstrap` covers the intelligent side; `loaf adopt` would be the CLI complement)
- External skill registry integration (skills.sh) — recommend from Loaf built-ins only

### Rabbit Holes
- Trying to parse every possible brief format (PDF, Notion export, Google Doc) — stick to markdown/text files and folders of markdown
- Building a "project type" template system with per-stack scaffolding beyond what `loaf init` already does
- Deep ARCHITECTURE.md population for greenfield projects where no code exists yet — keep it to stated constraints and choices from the brief

### No-Gos
- Don't overwrite existing documents without explicit confirmation
- Don't skip the interview — even with a rich brief, confirm understanding
- Don't auto-run `/shape` or other pipeline skills — suggest, don't execute

## Dependencies

| Dependency | Type | Status | Notes |
|------------|------|--------|-------|
| SPEC-009 (Knowledge Management) | Soft — enhances KB setup when available | `drafting` | Graceful degradation: scaffold directory without `loaf kb init` |
| `loaf init` CLI | Used by `loaf setup` | Implemented | `loaf setup` wraps init + build + install |
| `loaf build` CLI | Used by `loaf setup` | Implemented | |
| `loaf install` CLI | Used by `loaf setup` | Implemented | |

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Brief parsing produces poor synthesis | Low | Medium | Always interview after synthesis — never trust extraction alone |
| Skill becomes too long/complex for single SKILL.md | Medium | Low | Use references/ for interview guides and document templates |
| Cross-harness experience is significantly degraded | Medium | Low | Document Bash-based alternatives clearly; accept Claude Code as primary |
| Structured review rounds feel tedious for simple projects | Low | Medium | Allow user to approve all remaining sections at once ("looks good, approve all") |

## Open Questions

- [ ] What's the minimum viable brief that still produces useful VISION.md content? (Single sentence? Paragraph? Bullet list?)
- [ ] Should the skill detect and warn about existing `.claude/CLAUDE.md` that isn't a symlink to `.agents/AGENTS.md`?
- [ ] Should `loaf setup` be a new command or should `loaf init` be enhanced to optionally build + install? (`loaf init --setup` vs `loaf setup`)
- [ ] For greenfield+empty bootstraps, should the synthesized `docs/BRIEF.md` be presented for review before proceeding to VISION/STRATEGY/ARCHITECTURE?

## Test Conditions

### `loaf setup` CLI
- [ ] `loaf setup` in a new directory runs init + build + install in sequence
- [ ] `loaf setup` in an existing Loaf project (has `loaf.json`) is idempotent — re-inits safely, rebuilds, reinstalls
- [ ] `loaf setup` prints a clear handoff message directing user to `/bootstrap`

### `/bootstrap` Skill — Detection
- [ ] In an empty directory, auto-detects greenfield+empty mode without asking the user
- [ ] In an empty directory with `docs/BRIEF.md`, auto-detects greenfield+brief mode
- [ ] In a project with git history and source code, auto-detects brownfield mode
- [ ] Briefly confirms what was detected ("I see a Python/FastAPI project...") before proceeding
- [ ] User can correct detection if wrong

### `/bootstrap` Skill — Brownfield
- [ ] Reads and analyzes existing README, code patterns, manifests, and docs
- [ ] Interview focuses on gaps and nuances not captured in code
- [ ] Produces `docs/BRIEF.md` synthesized from existing context + interview
- [ ] Populates VISION.md, STRATEGY.md, ARCHITECTURE.md from analysis + interview

### `/bootstrap` Skill — Greenfield + Brief
- [ ] Running `/bootstrap ~/brief.md` reads the brief, persists it as `docs/BRIEF.md`, interviews about gaps
- [ ] Running `/bootstrap ./docs/` reads all markdown files in the folder and synthesizes them
- [ ] Interview challenges assumptions and probes gaps in the brief
- [ ] Populates VISION.md with content derived from the brief + interview

### `/bootstrap` Skill — Greenfield + Empty
- [ ] Running `/bootstrap` with no input in an empty directory launches full exploratory interview
- [ ] Interview covers: problem, users, existing solutions, unfair advantage, success criteria, scope
- [ ] Synthesizes interview into `docs/BRIEF.md` before proceeding to VISION/STRATEGY/ARCHITECTURE
- [ ] Produces populated VISION.md even from a single-sentence starting idea

### `/bootstrap` Skill — Common
- [ ] User can iterate on each document section via structured review rounds
- [ ] AGENTS.md contains detected stack info (build commands, language, framework) and recommended Loaf skills
- [ ] Symlinks are auto-created per convention without prompting
- [ ] Skill suggests at least 2 relevant next steps (`/brainstorm`, `/idea`, `/research`, `/shape`) and `loaf doctor`
- [ ] When `loaf kb init` is unavailable, `docs/knowledge/` is scaffolded with a README
- [ ] When `loaf kb init` is available, it is used for KB setup
- [ ] On a non-Claude Code harness, the skill provides Bash-based instructions for equivalent setup
- [ ] Existing documents are never overwritten without explicit confirmation

### Brief Artifact
- [ ] Brief from in-project file (e.g., `docs/BRIEF.md`) is used in place, not copied
- [ ] Brief from external file is copied to `docs/BRIEF.md` with `source: file` and `original_path`
- [ ] Brief from inline text is persisted to `docs/BRIEF.md` with `source: text`
- [ ] Brief from folder intake is persisted as concatenated content with `source: folder`
- [ ] Interview-only bootstrap persists a synthesized brief to `docs/BRIEF.md` with `source: interview`

### Session Recording
- [ ] Bootstrap creates a session file at `.agents/sessions/{timestamp}-bootstrap.md`
- [ ] Session captures key decisions: mode selection, technical choices, rejected alternatives
- [ ] Session follows the standard session template format

## Follow-On Work

These emerged from brainstorming (`.agents/drafts/20260327-015423-brainstorm-project-bootstrap-experience.md`) and are explicitly out of scope but worth tracking:

- **`loaf doctor`** — Health-check command: validates symlinks, skills installed, docs populated, KB structure. Natural post-bootstrap verification. Micro-spec.
- **`loaf adopt`** — Dedicated brownfield CLI command: reads existing docs, proposes Loaf-compatible structure, migrates what it can. Complements `/bootstrap`'s brownfield mode with CLI-level intelligence. Micro-spec.
- **skills.sh integration** — Future: query skills.sh registry for community skills during recommendation step. For now, recommend Loaf built-ins only.

## Circuit Breaker

**At 50% appetite:** If brief parsing and structured review are working but cross-harness, KB integration, and session recording aren't ready, ship Claude Code-only with `docs/knowledge/` directory scaffolding only. Skip `loaf setup` — document the manual three-step CLI sequence.

**At 75% appetite:** Drop cross-harness alternative instructions entirely. Ship Claude Code-only with full interview + doc population + brief artifact + session recording. `loaf setup` can ship as a fast-follow.

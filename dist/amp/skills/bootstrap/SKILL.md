---
name: bootstrap
description: >-
  Bootstraps new or existing projects through intelligent state detection,
  structured interviews, and document population. Use when the user asks "how do
  I start a new project?", "set up Loaf," or "bootstrap my project." Produces
  populated project documents and setup recommendations. Not for shaping
  features (use shape), problem discovery for a new concept (use pitch), or
  quick idea capture (use idea).
version: 0.5.0
---

# Bootstrap

First-contact project setup: detect state, interview the builder, populate project documents.

## Contents
- Critical Rules
- Verification
- Topics
- Purpose
- Input Parsing
- State Detection
- Brief Intake
- Interview Flow
- Document Population
- Structured Review
- Finalization
- Cross-Harness Support
- Guardrails
- Related Skills

Series-prep lives under Finalization (phase between Knowledge Base Scaffolding and Next Steps): the initial arc becomes backlog issues, not folders.

**Input:** $ARGUMENTS

---

## Critical Rules

- **Detect, don't ask** -- auto-classify project mode (brownfield/greenfield+brief/greenfield+empty), confirm briefly, let the user correct
- **Never overwrite existing documents** without explicit confirmation -- read first, note what exists, ask before changing
- **Always interview** -- even with a rich brief, confirm understanding through structured questions — one at a time, with a recommendation, using your harness's structured question tool if it has one
- **Pitched BRIEF is discovery-already-done** -- when `docs/BRIEF.md` has `source: pitch`, do not re-excavate the problem space; quote-back and gap-fill only for operating-document population
- **BRIEF is input, not output** -- the BRIEF is raw intake. Extract every useful fact into VISION/STRATEGY/ARCHITECTURE/AGENTS during bootstrap.
- **BRIEF is archeological after bootstrap** -- once extraction completes (including series-prep reading scoped concepts from it), the BRIEF is a frozen historical snapshot. No skill, agent, command, or template should reference `docs/BRIEF.md` post-bootstrap. Operating documents and minted issue bodies must stand on their own.
- **Series-prep never auto-shapes and never creates branches** -- every mint is user-confirmed; no priority, date, or dependency fields; buckets are labels, never bindings; concepts that fail granularity stay BRIEF lines or sparks
- **Prepare an arc of rideable outcomes, not layers** -- series-prep describes the operator journeys that later shape will make concrete; storage, backend, API, UI, and verification are not milestones by themselves
- **Record, don't provision Linear** -- when the project exposes a Linear MCP, record its server name in `.agents/loaf.json`; never install, connect, or authenticate it
- **Suggest, don't execute** -- recommend next skills at the end, never auto-run them
- **Log first** -- log invocation before interviewing: `loaf journal log "skill(bootstrap): <project or intake>"`
- **Log outcome** -- log bootstrap completion to the project journal: `loaf journal log "decision(bootstrap): project bootstrapped, mode detected"`

---

## Verification

- All expected operating documents (`docs/VISION.md`, `AGENTS.md` at minimum) exist and contain populated content
- Useful BRIEF content has been extracted into operating documents (no future reader should need to open the BRIEF)
- When `source: pitch`, the interview was gap-only (no re-excavation of already-specific problem sections)
- When series-prep ran: each minted row is a backlog issue (`loaf issue new "<title>" --body "<problem narrative>" --status backlog`) with a standalone problem-space body and a nameable operator outcome rather than a component layer; an advisory bucket (`loaf issue bucket <ref> now|next|later`) may be set — buckets are labels, never bindings; `loaf issue check <ref>` only when a capture is shaped enough to check, otherwise nothing (a backlog issue with a problem body needs no ceremony); no folders, no docs-only commits; no branches created for the series; no auto-shape
- Root `AGENTS.md` is a real file; on Claude Code, the compatibility symlink `.claude/CLAUDE.md -> ../AGENTS.md` exists (see Finalization)
- When a Linear MCP was active, `.agents/loaf.json` records its exact server name under `integrations.linear.mcp_server_name`; bootstrap did not install or authenticate it
- Key decisions and interview outcomes were logged with `loaf journal log` and are readable with `loaf journal recent`

---

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Interview Guide | [interview-guide.md](references/interview-guide.md) | Conducting the builder interview (all modes) |

---

## Purpose

Bootstrap is the **intelligent half** of the 0-to-1 experience. The mechanical half (`loaf setup`) handles scaffolding, building, and installing. Bootstrap handles everything that requires understanding: reading briefs, interviewing the builder, populating project documents, and recording decisions.

The goal is to go from "I have an idea" (or "I have a codebase") to a populated set of operating documents -- VISION.md, STRATEGY.md, ARCHITECTURE.md, and AGENTS.md -- through a structured but conversational process. The BRIEF is captured as an *intake snapshot* (raw, historical) and its content is then extracted into the *living operating docs*. The pipeline is explicit: BRIEF (raw intake) -> VISION/STRATEGY/ARCHITECTURE/AGENTS (refined operating docs).

---

## Input Parsing

Parse `$ARGUMENTS` to determine brief intake mode. Invocation forms differ by harness — use only your product's row:

| Harness | Invoke |
|---------|--------|
| Claude Code (plugin) | `/loaf:bootstrap` |
| OpenCode, Cursor, Codex, Amp | `/bootstrap` |

| Input Pattern | Mode | Example `$ARGUMENTS` |
|---------------|------|----------------------|
| Text description | Inline brief | `Build a CLI tool that manages knowledge bases` |
| File path | File brief | `~/Desktop/project-brief.md` |
| Folder path | Folder brief | `./docs/` |
| Empty | Interactive | _(empty)_ |

After determining intake mode, proceed to state detection -- they are independent concerns.

---

## State Detection

Automatically classify the project into one of three modes. Do not ask the user to choose -- detect and confirm.

### Detection Signals

| Signal | Brownfield | Greenfield + Brief | Greenfield + Empty |
|--------|-----------|-------------------|-------------------|
| `.git` with commit history | Yes | -- | -- |
| Source code files | Yes | -- | -- |
| `package.json`, `Gemfile`, `go.mod`, etc. | Yes | -- | -- |
| README.md or existing docs | Yes | -- | -- |
| `docs/BRIEF.md` or brief passed as argument | -- | Yes | -- |
| Empty/minimal directory, no brief | -- | -- | Yes |

### Detection Procedure

1. Check for `.git` directory and run `git log --oneline -5 2>/dev/null` to verify commit history
2. Scan for source code files and language manifests (package.json, Gemfile, go.mod, pyproject.toml, Cargo.toml, etc.)
3. Check for existing documentation (README.md, docs/)
4. Check for `docs/BRIEF.md` or brief input from `$ARGUMENTS`

### Confirm Detection

Briefly state what was found and proceed. Examples:

- "I see a Python/FastAPI project with 40+ commits, a README, and pytest tests. I'll treat this as an existing project and focus on what's not captured in the code."
- "This looks like a fresh project with a brief. I'll analyze your brief and interview you about the gaps."
- "Empty project, no brief. Let's start from scratch -- I'll interview you to understand what you're building."

If the user corrects the detection, adjust and proceed.

---

## Brief Intake

The BRIEF is captured at intake time as a *historical snapshot* of how the project entered Loaf. After this section, the skill's job is to extract its content into the operating docs (VISION/STRATEGY/ARCHITECTURE/AGENTS). The BRIEF is never updated again -- it stands as a frozen artifact of the original framing.

The canonical brief location is `docs/BRIEF.md`. Handle each intake form:

### Inline Text

When `$ARGUMENTS` contains a text description (not a file or folder path):

1. Snapshot to `docs/BRIEF.md` (historical record of intake) with frontmatter
2. Analyze the text for themes and gaps
3. Proceed to interview about gaps

### File Path

When `$ARGUMENTS` is a path to a file:

1. Read the file
2. If the file IS `docs/BRIEF.md` -- use in place, add frontmatter if missing
3. If the file is external -- snapshot content to `docs/BRIEF.md` (historical record of intake) with `original_path` in frontmatter
4. Analyze and proceed to interview

### Folder Path

When `$ARGUMENTS` is a path to a directory:

1. Read all markdown files in the folder
2. Synthesize into a single brief
3. Snapshot to `docs/BRIEF.md` (historical record of intake) with `source: folder`
4. Analyze and proceed to interview

### No Input

When `$ARGUMENTS` is empty and no `docs/BRIEF.md` exists:

1. Skip directly to interview
2. After interview, snapshot responses to `docs/BRIEF.md` (historical record of intake) with `source: interview`

### Brief Frontmatter Schema

Follow [templates/brief.md](templates/brief.md) for the full brief template. Frontmatter:

```yaml
---
source: file | text | folder | interview | pitch
original_path: ~/Desktop/project-brief.md  # only if copied from external source
created: 2026-03-27T01:54:00Z              # use actual timestamp
archived: true                             # always true -- BRIEF is a historical snapshot
---
```

When `source: pitch`, discovery is already done by pitch — greenfield+brief mode interviews for gaps only (see Interview Flow).

### Existing Brief

If `docs/BRIEF.md` already exists and no new brief was provided:

1. Read and analyze the existing brief
2. Treat as greenfield+brief mode
3. Do NOT overwrite -- use it as-is, add frontmatter if missing
4. The existing BRIEF is read once for extraction into operating docs and is not consulted again afterward

---

## Interview Flow

Interview depth adapts to the detected mode. All interviews ask one question at a time, with a recommendation, using your harness's structured question tool if it has one. The full interview framework is in [references/interview-guide.md](references/interview-guide.md).

### Brownfield: Nuance-Capturing Interview

The project exists. Code exists. Docs may exist. Lighter interview, heavier analysis.

**Before interviewing:**
1. Read README.md thoroughly
2. Detect stack from manifests (package.json, Gemfile, go.mod, pyproject.toml, etc.)
3. Scan code structure and patterns
4. Read any existing docs (VISION.md, STRATEGY.md, ARCHITECTURE.md)
5. Check for test frameworks and CI configuration

**Interview focus (6-10 questions):**
- What is NOT captured in the code -- intentions, frustrations, future direction
- What the builder wants to CHANGE -- current pain, technical debt, strategic shifts
- Conventions and preferences that exist but are not documented
- Project goals and who the users are

**Opening pattern:** Show the builder what you learned from the code first. "I see a Python/FastAPI project with PostgreSQL and Docker. The test suite uses pytest with 85% coverage. Is that the intended stack going forward?" Let the codebase speak, then fill gaps.

### Greenfield + Brief: Gap-Filling Interview

A brief exists but needs validation and gap-filling. Moderate depth. Depth shrinks further when the brief is pitched.

**Before interviewing:**
1. Read and deeply analyze the brief, including frontmatter `source`
2. Extract: goals, users, constraints, technical hints, scope signals
3. Identify gaps and assumptions

**When `source: pitch` (discovery already done):**

Pitch owned the problem-space grill. Bootstrap does not re-excavate. The pitch→bootstrap handoff must read as one continuous flow — gap mode opens by acknowledging the pitched BRIEF, not by starting a fresh discovery ceremony.

**Session-start opening (required before any gap question):**

1. **Acknowledge the pitch** — name that `docs/BRIEF.md` carries `source: pitch` and that problem discovery is already done.
2. **Summarize what pitch captured** — short section-by-section gist (problem, who, alternatives, value, constraints, sequencing, open questions). The builder should hear continuity with the pitch closing ceremony, not a cold restart.
3. **State what bootstrap will do now** — interview only on gaps for operating-document population (VISION, STRATEGY, ARCHITECTURE, AGENTS), then series-prep the initial arc as backlog issues. Do not re-grill the problem space.

Then continue:

- Quote the BRIEF back section by section only where confirmation is needed; note corrections only
- Interview **gaps only** — signals the pitch skeleton does not cover for operating docs: stack preferences, conventions, deployment constraints, success metrics phrasing, non-goals for VISION, anything left blank or marked open
- Skip full Excavation and full Sharpening re-runs; do not re-ask "who has this problem?" when Who Has It is already specific
- Expect 4–8 questions total, concentrated on gaps; zero questions is allowed if the BRIEF and builder confirmation fully feed VISION/STRATEGY/ARCHITECTURE

**When `source` is file, text, folder, or interview (or missing):**

**Interview focus (8-12 questions):**
- Confirm extracted understanding ("Here's what I got from your brief -- is this right?")
- Challenge assumptions ("Your brief says X, but have you considered Y?")
- Fill gaps in whichever interview sections are weakest
- Don't re-ask what the brief already answers well

**Opening pattern:** Quote the brief back, confirm accuracy, then pivot to gaps.

### Greenfield + Empty: Full Exploratory Interview

No code, no brief, just a person with an idea. Deepest interview.

**Run all four sections from [references/interview-guide.md](references/interview-guide.md):**

1. **Excavation (The Spark)** -- understand the problem, who has it, what they do today
2. **Sharpening (The Shape)** -- define scope, boundaries, no-gos, complexity
3. **Grounding (The Architecture)** -- technical direction, build vs. buy, hard problems
4. **Synthesis (The Documents)** -- transition to drafting

**Expect 15-25 questions across all sections.** Follow the transition signals in the interview guide. Be patient -- the builder may circle back or contradict themselves. That is normal.

**Opening pattern:** "Tell me about what you're building. What problem are you solving?"

### Interview Anti-Patterns

Avoid these across all modes:

- **The Form** -- don't run through questions mechanically like a survey
- **The 45-Minute Interrogation** -- if the builder is losing energy, cut to synthesis
- **Premature Architecture** -- don't ask about databases during Excavation
- **The Echo Chamber** -- challenge, don't just agree
- **Asking for Permission to Proceed** -- transition between sections naturally
- **Over-Indexing on Frameworks** -- use frameworks as lenses, not vocabulary

---

## Document Population

### Population Order

Draft documents in this order. Each document gets a structured review before moving to the next.

These are the *operating documents*. The BRIEF was captured during intake and is no longer modified.

| Document | When | Content Source |
|----------|------|----------------|
| `docs/VISION.md` | Always | Brief + interview (purpose, target users, success criteria, non-goals) |
| `docs/STRATEGY.md` | When enough info available | Interview (current focus, priorities, constraints, open questions) |
| `docs/ARCHITECTURE.md` | When technical choices stated | Brief + detected stack (overview, components, technology choices) |
| `AGENTS.md` | Always (incremental) | Detected stack (build/test commands, project structure, Loaf skills) |

### Conditional Logic

- **STRATEGY.md** -- Draft only if the interview surfaced personas, market context, priorities, or competitive landscape. If not, skip and note it as a future exercise.
- **ARCHITECTURE.md** -- Draft only if the builder stated technical choices or the codebase was analyzed (brownfield). For greenfield projects where no stack is decided, capture constraints only.
- **Never force a document.** If there is not enough signal, say so and suggest revisiting later.

### AGENTS.md Incremental Population

Start with detected/discussed stack info and build up:

- Build commands (from package.json scripts, Makefile targets, pyproject.toml, etc.)
- Test commands (detected test framework and runner)
- Project structure overview
- Recommended Loaf skills section (based on detected languages, frameworks, project type)
- Paths to project docs and knowledge base
- Any conventions or preferences from the interview

### Existing Documents

**Never overwrite existing documents without explicit confirmation.** If a document already exists:

1. Read it
2. Note what is already covered
3. Ask whether to update, merge, or leave as-is
4. If updating, show the proposed changes before writing

---

## Structured Review

After drafting each document, present it section-by-section for iteration — one question at a time, using your harness's structured question tool if it has one.

### Review Pattern

For each document:

1. Present the first section (e.g., problem statement)
2. Ask a specific question: "Is this problem statement accurate?"
3. Revise based on feedback
4. Move to next section: "Are these the right non-goals?"
5. Continue until all sections reviewed

### Specific Review Prompts

| Document | Section | Prompt |
|----------|---------|--------|
| VISION.md | Problem | "Is this problem statement accurate?" |
| VISION.md | Target users | "Did I capture the right target users?" |
| VISION.md | Purpose | "Does this capture why this project exists?" |
| VISION.md | Non-goals | "Are these the right non-goals?" |
| VISION.md | Success criteria | "Anything missing from success criteria?" |
| STRATEGY.md | Priorities | "Are these the right current priorities?" |
| ARCHITECTURE.md | Tech choices | "Are these the right technical constraints, or did I add assumptions?" |

### Approve All

If the builder is satisfied, they can say "looks good" or "approve all" to skip remaining sections. Accept this gracefully and move on.

---

## Finalization

After all documents are reviewed and approved:

### 1. Knowledge Base Scaffolding

Check if `loaf kb init` CLI command is available:

```bash
loaf kb init --help 2>/dev/null
```

- **If available:** Run `loaf kb init` to scaffold the knowledge base
- **If not available:** Create `docs/knowledge/` directory with a README explaining its purpose:

```markdown
# Knowledge Base

This directory will hold the project's knowledge base -- decisions, patterns,
and context that accumulate over the project's lifetime.

When `loaf kb` tooling is available, run `loaf kb init` to scaffold the full
knowledge base structure.
```

After scaffolding, ask the builder if they have other Loaf projects they would like to import knowledge from. Don't auto-detect -- ask explicitly.

### 2. Claude Code Compatibility Symlink

Root `AGENTS.md` is the canonical project-instructions file on every harness. The `.claude/CLAUDE.md` path is Claude Code-specific — create and verify it only on that product. Read only the labeled section for the harness you are running.

### Claude Code

Create the compatibility symlink so Claude Code also loads project instructions:

```bash
# .claude/CLAUDE.md -> ../AGENTS.md
mkdir -p .claude
ln -sf ../AGENTS.md .claude/CLAUDE.md
```

If the symlink already exists and points at `../AGENTS.md`, skip silently. If it exists but points elsewhere, warn the user and ask before changing.

### Other harnesses

Do not create `.claude/CLAUDE.md`. Ensure root `AGENTS.md` exists and is populated; that is the file every harness reads.

Before journal recording, when `.agents/loaf.json` exists, inspect the Linear-capable MCP servers exposed by the current harness. With exactly one, preserve an existing matching value or record its exact name as `integrations.linear.mcp_server_name` and set `integrations.linear.enabled` to `true`; with several, ask which belongs to the project; with none, leave the integration unchanged. Ask before replacing a different recorded name, preserve unrelated fields, and never install, connect, or authenticate the MCP.

### 3. Journal Recording

Record the bootstrap interview in the project journal:

1. Log mode detection and key interview decisions with `loaf journal log`.
2. Use `loaf journal recent` to inspect what was captured.

The journal should capture:
- Mode detected and why
- Key decisions made during the interview
- Key interview exchanges that informed those decisions
- Technical choices and rejected alternatives
- The original problem framing and user intent
- Any open questions or deferred decisions

Use [templates/journal.md](templates/journal.md) only as the rendered entry
format reference; do not hand-author journal markdown as the source of truth.

### 4. Series-Prep (initial arc as backlog issues)

After operating documents are populated (and Knowledge Base Scaffolding above has run), close bootstrap by minting the BRIEF's initial arc as **backlog issues** — SQLite rows with a problem-space body and an optional advisory bucket label (`loaf issue bucket <ref> now|next|later`). Buckets are labels, never bindings. Series-prep is not roadmap planning: no milestone entities, no dates, no priorities, no dependency fields. Sequencing is prose in each issue body. No folders, no docs-only commits per capture — rows, not files.

**When to run**

- Always offer series-prep when a project BRIEF exists and names more than one scoped concept (typical after a pitched BRIEF; also after a rich non-pitch brief).
- If the BRIEF is a single atomic concept with no series, say so and skip to Next Steps — one future shape or a single issue later is enough.
- Series-prep **reads** the BRIEF during this phase only. After bootstrap ends, nothing references `docs/BRIEF.md` again; minted issue bodies and operating docs stand alone.

**Procedure**

1. **Enumerate concepts** with the builder from the BRIEF's scoped problem space (Sequencing and Relationships, Open Questions, and distinct problem threads in Problem Statement). List candidates as recommendation-first options using your harness's structured question tool if it has one.
2. **Apply granularity** per [references/interview-guide.md](references/interview-guide.md) (Series-Prep Granularity): a concept earns its own backlog issue when it names a complete operator outcome that is independently shippable **and** its problem can be **stated precisely now** (the mint-time specifiability test — not answered now, stated now) without the others; otherwise it stays a BRIEF line or becomes a spark — never a half-minted row. Component layers remain inside the future consuming slice.
3. **Per confirmed concept (one at a time — never batch):**
   1. Confirm mint with the builder (title, optional advisory bucket, one-line problem restatement). Buckets are labels, never bindings — a missing bucket does not block mint. If the concept fails granularity, do not mint — park as spark or BRIEF line.
   2. Propose a **working title** that names the concept, never another work unit (issue aliases, task ids). Confirm the title.
   3. **Seed a problem-space-only narrative** from the BRIEF's content for that concept (Problem Statement, Who Has It, Current Alternatives, Value Proposition, Constraints, Sequencing and Relationships as prose order relative to the arc, Sources if any, Open Questions). Do not copy solution design. The seeded body must stand alone as intent for later shape — cold-read without the project BRIEF or this session.
   4. Mint the backlog issue:

      ```bash
      loaf issue new "<title>" --body "<problem narrative>" --status backlog
      ```

      Creates a SQLite row, not a folder. Use `--body -` or `--body-file <path>` when the narrative is long (see `loaf issue new --help`).
   5. **Optionally set an advisory bucket** (a label, never a binding):

      ```bash
      loaf issue bucket <ref> now|next|later
      ```
   6. **Validate** only when the capture is shaped enough to check:

      ```bash
      loaf issue check <ref>
      ```

      A backlog issue with a problem body is capture-only and needs no ceremony — skip the check. Do not add criteria or an out-of-scope statement during series-prep (that is shape).
   7. **Do not land a docs-only commit.** The row is the artifact. Never push; never open a PR; never create a branch.
4. **Guards (hard):**
   - Every mint is user-confirmed — never auto-mint the whole list
   - Never auto-run shape and never create branches during series-prep
   - No priority, date, estimate, or dependency fields on issues
   - No folders, no docs-only commits per capture — rows, not files
   - Concepts that fail granularity stay BRIEF lines or sparks; buckets are labels, never bindings

**After the series**

Log the arc: `loaf journal log "decision(bootstrap): series-prep minted <n> backlog issues"`. Hand off by naming the first issue ref for shape when the builder is ready.

### 5. Next Steps

Suggest relevant next steps based on what was learned:

- shape -- on a series-prep backlog issue (or any ready concept) to bound implementation
- pitch -- if a new concept still needs problem discovery (not for re-grilling the BRIEF)
- idea -- if specific feature ideas emerged during the interview and should not become issues yet
- research -- if there are open questions that need investigation
- `loaf doctor` -- to verify the setup is healthy

Suggest at least 2 relevant paths. Don't auto-run any of them.

---

## Manual Setup Fallback

When the interactive interview path is unavailable, bootstrap the operating documents manually:

1. Run `loaf setup` (or `loaf init && loaf build && loaf install --to all` manually)
2. Create `docs/VISION.md`, `docs/STRATEGY.md`, `docs/ARCHITECTURE.md` manually -- these are the load-bearing operating documents
3. Populate `AGENTS.md` with build commands, test commands, and project structure
4. Optionally snapshot intake (problem, users, constraints) to `docs/BRIEF.md` as a historical record -- not referenced again after bootstrap
5. Create the Claude Code compatibility symlink when that harness is in use: `.claude/CLAUDE.md -> ../AGENTS.md`
6. Run `loaf kb init` if available, or create `docs/knowledge/` with a README

---

## Guardrails

1. **Detect, don't ask** -- auto-classify mode, confirm briefly, let user correct
2. **Always interview** -- even with a rich brief, confirm understanding; when `source: pitch`, gap-fill only
3. **Never overwrite** -- existing documents require explicit confirmation
4. **Draft, then review** -- present documents section-by-section
5. **Extract, don't preserve** -- pull every useful fact from the BRIEF into operating docs (and series-prep seeds issue bodies from it once). The BRIEF is archeological after bootstrap; nothing should reference it again.
6. **Record the session** -- decisions and rationale are preserved
7. **Suggest, don't execute** -- recommend next skills, don't auto-run them; series-prep never auto-shapes or creates branches
8. **Interview structured** -- one question at a time, with a recommendation, using your harness's structured question tool if it has one
9. **Series-prep is not roadmap planning** -- advisory bucket labels (never bindings) + prose sequencing only; no dates, priorities, or dependency fields

---

## Related Skills

- **pitch** -- Authors a project-scale `docs/BRIEF.md` with `source: pitch` (or an issue-scale problem narrative); bootstrap consumes the pitched BRIEF with gap-only interview and series-prep
- **shape** -- Bound a backlog issue into a contract (often follows series-prep)
- **explore** -- Agent technique when a concept that emerges during bootstrap is still undecided (not a user front door; prefer pitch for human problem discovery)
- **research** -- Investigate topics and open questions
- **idea** -- Quick-capture feature ideas that emerge during bootstrap
- **strategy** -- Deep persona and market context work
- **architecture** -- Detailed technical decision-making

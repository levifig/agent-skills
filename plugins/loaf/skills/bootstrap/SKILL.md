---
name: bootstrap
description: >-
  Bootstraps new or existing projects through intelligent state detection,
  structured interviews, and document population. Use when the user asks "how do
  I start a new project?", "set up Loaf," or "bootstrap my project." Produces
  populated project d...
user-invocable: true
argument-hint: '[brief or path]'
allowed-tools: 'Read, Write, Edit, Bash, Glob, Grep, AskUserQuestion'
version: 2.0.0-dev.28
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

**Input:** $ARGUMENTS

---

## Critical Rules

- **Detect, don't ask** -- auto-classify project mode (brownfield/greenfield+brief/greenfield+empty), confirm briefly, let the user correct
- **Never overwrite existing documents** without explicit confirmation -- read first, note what exists, ask before changing
- **Always interview** -- even with a rich brief, confirm understanding through structured questions using `AskUserQuestion`
- **Persist the brief** -- every bootstrap must produce a `docs/BRIEF.md`
- **Suggest, don't execute** -- recommend next skills at the end, never auto-run them
- **Log outcome** -- log bootstrap completion to session journal: `loaf session log "decision(bootstrap): project bootstrapped, mode detected"`

---

## Verification

- All expected documents (`docs/BRIEF.md`, `docs/VISION.md`, `.agents/AGENTS.md` at minimum) exist and contain populated content
- Symlinks are correct: `.claude/CLAUDE.md -> .agents/AGENTS.md` and `./AGENTS.md -> .agents/AGENTS.md`
- A session file was saved in `.agents/sessions/` capturing key decisions and interview exchanges

---

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Interview Guide | [interview-guide.md](references/interview-guide.md) | Conducting the builder interview (all modes) |

---

## Purpose

Bootstrap is the **intelligent half** of the 0-to-1 experience. The mechanical half (`loaf setup`) handles scaffolding, building, and installing. Bootstrap handles everything that requires understanding: reading briefs, interviewing the builder, populating project documents, and recording decisions.

The goal is to go from "I have an idea" (or "I have a codebase") to a populated set of project documents -- BRIEF.md, VISION.md, STRATEGY.md, ARCHITECTURE.md, and AGENTS.md -- through a structured but conversational process.

---

## Input Parsing

Parse `$ARGUMENTS` to determine brief intake mode:

| Input Pattern | Mode | Example |
|---------------|------|---------|
| Text description | Inline brief | `/loaf:bootstrap Build a CLI tool that manages knowledge bases` |
| File path | File brief | `/loaf:bootstrap ~/Desktop/project-brief.md` |
| Folder path | Folder brief | `/loaf:bootstrap ./docs/` |
| Empty | Interactive | `/loaf:bootstrap` |

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

The canonical brief location is `docs/BRIEF.md`. Handle each intake form:

### Inline Text

When `$ARGUMENTS` contains a text description (not a file or folder path):

1. Persist to `docs/BRIEF.md` with frontmatter
2. Analyze the text for themes and gaps
3. Proceed to interview about gaps

### File Path

When `$ARGUMENTS` is a path to a file:

1. Read the file
2. If the file IS `docs/BRIEF.md` -- use in place, add frontmatter if missing
3. If the file is external -- copy content to `docs/BRIEF.md` with `original_path` in frontmatter
4. Analyze and proceed to interview

### Folder Path

When `$ARGUMENTS` is a path to a directory:

1. Read all markdown files in the folder
2. Synthesize into a single brief
3. Persist to `docs/BRIEF.md` with `source: folder`
4. Analyze and proceed to interview

### No Input

When `$ARGUMENTS` is empty and no `docs/BRIEF.md` exists:

1. Skip directly to interview
2. After interview, synthesize responses into `docs/BRIEF.md` with `source: interview`

### Brief Frontmatter Schema

Follow [templates/brief.md](templates/brief.md) for the full brief template. Frontmatter:

```yaml
---
source: file | text | folder | interview
original_path: ~/Desktop/project-brief.md  # only if copied from external source
created: 2026-03-27T01:54:00Z              # use actual timestamp
---
```

### Existing Brief

If `docs/BRIEF.md` already exists and no new brief was provided:

1. Read and analyze the existing brief
2. Treat as greenfield+brief mode
3. Do NOT overwrite -- use it as-is, add frontmatter if missing

---

## Interview Flow

Interview depth adapts to the detected mode. All interviews use `AskUserQuestion`. The full interview framework is in [references/interview-guide.md](references/interview-guide.md).

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

A brief exists but needs validation and gap-filling. Moderate depth.

**Before interviewing:**
1. Read and deeply analyze the brief
2. Extract: goals, users, constraints, technical hints, scope signals
3. Identify gaps and assumptions

**Interview focus (8-12 questions):**
- Confirm extracted understanding ("Here's what I got from your brief -- is this right?")
- Challenge assumptions ("Your brief says X, but have you considered Y?")
- Fill gaps in whichever interview phases are weakest
- Don't re-ask what the brief already answers well

**Opening pattern:** Quote the brief back, confirm accuracy, then pivot to gaps.

### Greenfield + Empty: Full Exploratory Interview

No code, no brief, just a person with an idea. Deepest interview.

**Run all four phases from [references/interview-guide.md](references/interview-guide.md):**

1. **Excavation (The Spark)** -- understand the problem, who has it, what they do today
2. **Sharpening (The Shape)** -- define scope, boundaries, no-gos, complexity
3. **Grounding (The Architecture)** -- technical direction, build vs. buy, hard problems
4. **Synthesis (The Documents)** -- transition to drafting

**Expect 15-25 questions across all phases.** Follow the phase transition signals in the interview guide. Be patient -- the builder may circle back or contradict themselves. That is normal.

**Opening pattern:** "Tell me about what you're building. What problem are you solving?"

### Interview Anti-Patterns

Avoid these across all modes:

- **The Form** -- don't run through questions mechanically like a survey
- **The 45-Minute Interrogation** -- if the builder is losing energy, cut to synthesis
- **Premature Architecture** -- don't ask about databases in Phase 1
- **The Echo Chamber** -- challenge, don't just agree
- **Asking for Permission to Proceed** -- transition between phases naturally
- **Over-Indexing on Frameworks** -- use frameworks as lenses, not vocabulary

---

## Document Population

### Population Order

Draft documents in this order. Each document gets a structured review before moving to the next.

| Document | When | Content Source |
|----------|------|----------------|
| `docs/BRIEF.md` | Always (if not already present) | Intake text, external file, or synthesized interview |
| `docs/VISION.md` | Always | Brief + interview (purpose, target users, success criteria, non-goals) |
| `docs/STRATEGY.md` | When enough info available | Interview (current focus, priorities, constraints, open questions) |
| `docs/ARCHITECTURE.md` | When technical choices stated | Brief + detected stack (overview, components, technology choices) |
| `.agents/AGENTS.md` | Always (incremental) | Detected stack (build/test commands, project structure, Loaf skills) |

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

After drafting each document, present it section-by-section for iteration using `AskUserQuestion`.

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
| BRIEF.md | Problem | "Is this problem statement accurate?" |
| BRIEF.md | Users | "Did I capture the right target users?" |
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

### 2. Symlink Creation

Create symlinks per Loaf convention:

```bash
# .claude/CLAUDE.md -> .agents/AGENTS.md
mkdir -p .claude
ln -sf ../.agents/AGENTS.md .claude/CLAUDE.md

# ./AGENTS.md -> .agents/AGENTS.md (root convenience symlink)
ln -sf .agents/AGENTS.md ./AGENTS.md
```

If symlinks already exist and point to the right targets, skip silently. If they exist but point elsewhere, warn the user and ask before changing.

### 3. Session Recording

Save the bootstrap interview as a session file:

```
.agents/sessions/{YYYYMMDD}-{HHMMSS}-bootstrap.md
```

The session should capture:
- Mode detected and why
- Key decisions made during the interview
- Key interview exchanges that informed those decisions
- Technical choices and rejected alternatives
- The original problem framing and user intent
- Any open questions or deferred decisions

Follow [templates/session.md](templates/session.md) for structure and frontmatter schema.

### 4. Next Steps

Suggest relevant next steps based on what was learned:

- `/loaf:brainstorm` -- if the idea needs more exploration
- `/loaf:idea` -- if specific feature ideas emerged during the interview
- `/loaf:research` -- if there are open questions that need investigation
- `/loaf:shape` -- if a specific feature is ready to be bounded into a spec
- `loaf doctor` -- to verify the setup is healthy

Suggest at least 2 relevant paths. Don't auto-run any of them.

---

## Cross-Harness Support

This skill is designed for Claude Code (uses `AskUserQuestion`, Write/Edit tools). For other harnesses, the equivalent workflow is:

1. Run `loaf setup` (or `loaf init && loaf build && loaf install --to all` manually)
2. Create `docs/BRIEF.md` manually with project description
3. Create `docs/VISION.md`, `docs/STRATEGY.md`, `docs/ARCHITECTURE.md` manually
4. Populate `.agents/AGENTS.md` with build commands, test commands, and project structure
5. Create symlinks: `.claude/CLAUDE.md -> .agents/AGENTS.md` and `./AGENTS.md -> .agents/AGENTS.md`
6. Run `loaf kb init` if available, or create `docs/knowledge/` with a README

---

## Guardrails

1. **Detect, don't ask** -- auto-classify mode, confirm briefly, let user correct
2. **Always interview** -- even with a rich brief, confirm understanding
3. **Never overwrite** -- existing documents require explicit confirmation
4. **Draft, then review** -- present documents section-by-section
5. **Persist the brief** -- every bootstrap produces a `docs/BRIEF.md`
6. **Record the session** -- decisions and rationale are preserved
7. **Suggest, don't execute** -- recommend next skills, don't auto-run them
8. **Use AskUserQuestion** -- structured, conversational interaction throughout

---

## Related Skills

- **shape** -- Bound an idea into a spec (often follows bootstrap)
- **brainstorm** -- Explore ideas more deeply (when the builder wants to diverge)
- **research** -- Investigate topics and open questions
- **idea** -- Quick-capture feature ideas that emerge during bootstrap
- **strategy** -- Deep persona and market context work
- **architecture** -- Detailed technical decision-making

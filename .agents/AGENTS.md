# Loaf Development Guidelines

Guidelines for maintaining and extending Loaf - An Opinionated Agentic Framework.

See [README.md](../README.md) for what Loaf is and how to install it.

## Quick Start

```bash
npm install && npm run build   # Build CLI + all targets
loaf build                     # After initial build, use the CLI directly
loaf install --to all          # Install to detected tools
```

## Project Structure

```
cli/                            # CLI tool (TypeScript, bundled by tsup)
├── index.ts                    # Entry point (Commander.js)
├── commands/
│   ├── build.ts                # loaf build
│   ├── check.ts                # loaf check
│   ├── install.ts              # loaf install
│   └── session.ts              # loaf session
└── lib/
    ├── build/                  # Build system
    │   ├── types.ts            # Shared types
    │   ├── targets/            # Target transformers (claude-code, opencode, cursor, codex, gemini, amp)
    │   └── lib/                # Build utilities (version, sidecar, shared-templates, etc.)
    ├── detect/                 # Tool detection
    └── install/                # Installation logic

content/                        # Distributable content
├── skills/{name}/SKILL.md      # Domain knowledge + references/ + templates/
├── templates/                  # Shared templates (distributed at build time)
├── agents/{name}.md            # Functional profiles (tool boundaries + behavioral contracts)
└── hooks/{pre,post}-tool/      # Hook scripts

config/                         # Build configuration
├── hooks.yaml                  # Hook definitions
└── targets.yaml                # Target defaults, sidecars, shared-templates
```

**Output:** `plugins/` (Claude Code), `dist/{target}/` (others)

## Quick Reference

| Component | Location | Key File |
|-----------|----------|----------|
| Skills | `content/skills/{name}/` | `SKILL.md` |
| Agents | `content/agents/{name}.md` | - |
| Hooks | `content/hooks/{pre,post}-tool/` | - |
| Config | `config/` | `hooks.yaml`, `targets.yaml` |
| CLI | `cli/` | `index.ts` |

## Agent Profiles

See [SOUL.md](../SOUL.md) for the Warden identity and fellowship conventions.

| Profile | Concept | Tool Access | Use For |
|---------|---------|-------------|---------|
| **implementer** | Smith (Dwarf) | Full write | Code, tests, config, docs — speciality via skills |
| **reviewer** | Sentinel (Elf) | Read-only | Audits, reviews — mechanical independence |
| **researcher** | Ranger (Human) | Read + web | Research, comparison — structured reports |
| **librarian** | Librarian (Ent) | Read + Edit (.agents/) | Session lifecycle, state, wrap, pre-compaction preservation |
| **background-runner** | System | Read + Edit | Async non-blocking tasks |

## Common Tasks

**Add skill:** Create `content/skills/{name}/SKILL.md`, register hooks in `hooks.yaml`

**Add hook:** Create script in `content/hooks/{pre,post}-tool/`, register in `hooks.yaml`

**Add template:** Create in `content/skills/{name}/templates/` (skill-specific) or `content/templates/` + register in `shared-templates` in `targets.yaml`

**Add target:** Create `cli/lib/build/targets/{target}.ts`, register in `cli/commands/build.ts`

## Skill Development

### Session Journal Self-Logging

User-invocable workflow skills must log their invocation to the session journal as their first action. Include context — arguments, intent, or what triggered the invocation:

```bash
loaf session log "skill(shape): shaping auth token rotation idea into spec"
loaf session log "skill(housekeeping): routine cleanup, no specific trigger"
loaf session log "skill(wrap): end-of-session summary"
loaf session log "skill(implement): TASK-042 — add PAUSE headers to session end"
```

This creates an audit trail of which skills ran during a session. `/wrap` uses these entries to check whether housekeeping or other periodic skills were run.

### Naming Conventions

Use domain-focused names in gerund or noun-phrase form:

| Pattern | Examples | Use For |
|---------|----------|---------|
| `{lang}-development` | `python-development`, `typescript-development` | Language skills |
| `{domain}-{activity}` | `database-design`, `infrastructure-management` | Domain skills |
| Single word | `foundations`, `orchestration`, `research` | Workflow/process skills |

**Constraints:**
- Lowercase letters, numbers, hyphens only
- Max 64 characters
- No reserved words: `anthropic`, `claude`
- Directory name must match `name` field

### SKILL.md Structure

Follow the [Agent Skills](https://agentskills.io) open standard:

```yaml
---
name: skill-name
description: >-
  Third-person description starting with action verb. Covers X, Y, Z.
  Use when [context triggers] or when the user asks "[natural language examples]".
---

# Skill Title

Brief intro paragraph.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules
...

## Verification
...

## Quick Reference
...

## Topics
...
```

**Standard section order:**
1. **Critical Rules** — Must-follow constraints, guardrails, delegation patterns
2. **Verification** — How to confirm success, test conditions, validation steps
3. **Quick Reference** — Tables, decision trees, command cheatsheets
4. **Topics** — Detailed references linked from SKILL.md

#### Verb/Noun Principle

Use action verbs for workflows, nouns for knowledge:

| Pattern | Use For | Examples |
|---------|---------|----------|
| **Verb** (gerund) | Workflow skills | `implement`, `breakdown`, `research` |
| **Noun** (domain) | Knowledge skills | `typescript-development`, `database-design` |

**Why:** Skills that DO things get verbs. Skills that ARE things get nouns. This makes the distinction between "use me to act" and "reference me to know" immediately clear.

#### Frontmatter Fields (Standard)

Only these fields belong in `SKILL.md`:

| Field | Required | Notes |
|-------|----------|-------|
| `name` | Yes | Must match directory name |
| `description` | Yes | Max 1024 chars, third-person, action verbs |
| `license` | No | License name or file reference |
| `compatibility` | No | Environment requirements |
| `metadata` | No | Arbitrary key-value pairs |
| `allowed-tools` | No | Space-delimited tool list (experimental) |

#### Description Best Practices

**Two-tier structure** for Claude's 250-char truncation vs full description:

1. **First sentence (≤250 chars):** Action verb, what it covers, key trigger phrases
2. **Rest:** User-intent examples, negative routing, success criteria

```yaml
description: >-
  Covers Python 3.12+ with FastAPI, Pydantic, async patterns, pytest, SQLAlchemy.
  Use when building APIs, or when the user asks "how do I validate data?"
  or "what's the best way to structure a Python project?"
  Not for schema design decisions (use database-design).
```

**Additional guidelines:**

1. **Start with action verb** (third-person):
   - Good: "Covers...", "Establishes...", "Coordinates..."
   - Bad: "Use for...", "I can help...", "You can use this..."

2. **Include user-intent phrases**:
   ```yaml
   description: >-
     Covers Python 3.12+ development... Use when building APIs,
     or when the user asks "how do I validate data?" or "what's
     the best way to structure a Python project?"
   ```

3. **Be specific** - Claude uses this to choose from 100+ skills

4. **Include negative routing** for disambiguation between confusable skills:
   ```yaml
   description: >-
     Covers Python 3.12+ development... Not for schema design
     decisions (use database-design) or deployment infrastructure
     (use infrastructure-management).
   ```

5. **Add success criteria** for workflow skills (what the skill produces):
   ```yaml
   description: >-
     Conducts project assessment... Produces state assessments,
     research findings with ranked options, or vision change
     proposals. Not for multi-agent coordination (use orchestration).
   ```

### Templates

Artifact format templates (session files, specs, ADRs, task files) live in `templates/` directories. SKILL.md references them with links instead of embedding inline.

**Skill-specific templates:** `content/skills/{name}/templates/` — templates unique to one skill.

**Shared templates:** `content/templates/` — distributed to multiple skills at build time via `shared-templates` config in `targets.yaml`.

```yaml
# targets.yaml
shared-templates:
  session.md: [implement, orchestration, review-sessions]
  plan.md: [implement, council]
  adr.md: [architecture, reflect]
```

**Reference pattern in SKILL.md:**
```markdown
Create session file following [templates/session.md](templates/session.md).
```

**Templates vs references:**
- `templates/` — structural artifacts (frontmatter schema, section headings, required fields)
- `references/` — knowledge documents (conventions, patterns, decision records)

### Sidecar Files

Claude Code-specific fields go in `SKILL.claude-code.yaml`:

```yaml
# Claude Code extensions
user-invocable: false
allowed-tools: Read, Write, Edit, Bash, Glob, Grep
```

| Field | Purpose |
|-------|---------|
| `user-invocable` | `false` for pure reference skills (hide from `/` menu) |
| `disable-model-invocation` | `true` for manual-only workflows |
| `argument-hint` | Autocomplete hint: `"[topic]"`, `"[file]"` |
| `context` | `fork` to run in subagent |
| `model` | Override model for this skill |
| `hooks` | Skill-scoped lifecycle hooks |

### Reference Files

#### Organization

```
skill-name/
├── SKILL.md              # Overview + navigation (< 500 lines)
├── SKILL.claude-code.yaml # Claude-specific overrides
├── references/           # Knowledge docs (loaded on demand)
│   ├── topic-a.md
│   └── topic-b.md
└── templates/            # Artifact format templates (loaded on demand)
    ├── artifact-a.md
    └── artifact-b.md
```

### Reference Table Pattern

Use "Use When" (action-oriented), not "Coverage" (content-oriented):

```markdown
## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Core | [core.md](references/core.md) | Setting up projects, naming conventions |
| Testing | [testing.md](references/testing.md) | Writing tests, debugging failures |
```

#### Table of Contents

Files over 100 lines must have a TOC after the title:

```markdown
# Reference Title

## Contents
- Section One
- Section Two
- Section Three

Brief intro paragraph.

## Section One
...
```

#### Reference Rules

- **One level deep** - All references link from SKILL.md, not from other references
- **No nested chains** - Avoid `SKILL.md → advanced.md → details.md`
- **Forward slashes only** - `references/guide.md`, never `references\guide.md`

### Skill Categories

| Category | `user-invocable` | Example |
|----------|------------------|---------|
| Reference/Knowledge | `false` | `python-development`, `database-design` |
| Workflow/Process | `true` (default) | `orchestration`, `research` |

Reference skills provide background knowledge Claude loads automatically.
Users shouldn't invoke `/python-development` directly.

### Session Journal Vocabulary

Session journals in `.agents/sessions/` use a **compact inline format** — append-only structured logs. Think "conventional commits meets bullet journal."

| Term | Meaning |
|------|---------|
| **Session** | A markdown file with compact inline journal entries |
| **Session File** | Named `YYYYMMDD-HHMMSS-description.md` in `.agents/sessions/` |
| **Frontmatter** | YAML header with `spec`, `branch`, `status`, `created`, `last_entry` |
| **Journal Entry** | `[YYYY-MM-DD HH:MM] type(scope): description` |
| **Entry Type** | `session`, `commit`, `decision`, `discover`, `block`, `unblock`, `spark`, `todo`, etc. |
| **Burst** | Entries grouped without blank lines |
| **Archive** | Completed sessions moved to `.agents/sessions/archive/` |

**Session Status Values:** `active`, `stopped`, `done`, `blocked`, `archived`

**Entry Format:**
```markdown
[YYYY-MM-DD HH:MM] session(start):  === SESSION STARTED ===
[YYYY-MM-DD HH:MM] decision(scope): description
[YYYY-MM-DD HH:MM] commit(abc1234): message
[YYYY-MM-DD HH:MM] session(end): at commit abc1234, 3 commits, 1 decision
[YYYY-MM-DD HH:MM] session(stop):   === SESSION STOPPED ===

[YYYY-MM-DD HH:MM] session(resume): === SESSION RESUMED ===
[YYYY-MM-DD HH:MM] session(context): from commit abc1234
```

**Blank line rules:**
- `session(stop)` has one blank line after, not before
- `session(start)` and `session(resume)` have no blank line after
- No other automatic blank lines

## Build System

### Commands

```bash
loaf build                     # Build all targets
loaf build --target claude-code  # Specific target
loaf install --to all          # Install to all detected tools
loaf install --to cursor       # Install to specific tool
loaf install --upgrade         # Update already-installed targets
loaf check                     # Run enforcement hooks manually
loaf session list              # List active sessions
loaf session start <desc>      # Start new session
loaf session end <file>        # End/archive session
```

### Development

```bash
npm install                    # Install dependencies
npm run build:cli              # Build CLI only (tsup)
npm run build                  # Build CLI + all content targets
npm run typecheck              # Type check (tsc --noEmit)
npm link                       # Make `loaf` available globally
```

### Targets

| Target | Output | Notes |
|--------|--------|-------|
| claude-code | `plugins/loaf/` | Merges sidecars into output |
| opencode | `dist/opencode/` | Skills, agents, and commands (from skills) |
| cursor | `dist/cursor/` | Skills, agents, and hooks |
| codex | `dist/codex/` | Skills only |
| gemini | `dist/gemini/` | Skills only |
| amp | `dist/amp/` | Skills, runtime plugin |

### Before Committing

- [ ] `loaf build` succeeds
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes
- [ ] If tracked build artifacts in `dist/` or `plugins/` changed, commit them with the source changes that produced them
- [ ] Frontmatter has required fields
- [ ] New skills registered in `hooks.yaml`
- [ ] Sidecar file for Claude-specific fields
- [ ] Reference files >100 lines have TOC
- [ ] Template links resolve (no broken `templates/` paths)
- [ ] No Windows-style paths

## Configuration

### Hook Model

Two types of hooks serve different purposes:

**Enforcement Hooks** — Quality gates that block bad actions:
- Run automatically at git lifecycle points (pre-commit, pre-push)
- Example: Secrets scanning, linting, type checking
- Can be run manually via `loaf check`
- Exit non-zero to block the action
- Hooks without explicit `script:` or `command:` auto-dispatch as `loaf check --hook <id>`

**Skill Instruction Hooks** — Context injection at tool invocation:
- Triggered when specific tools are invoked
- Inject relevant skill instructions based on context
- Example: When `Edit` is used, inject language-specific style guide
- Registered in `hooks.yaml` with `matcher` patterns

### Hook Dispatch Mechanisms

Three dispatch types control how a hook executes:

| Type | Field | Behavior |
|------|-------|----------|
| `script` (default) | `script:` | Runs a shell script |
| `command` | `command:` | Runs a CLI command (e.g., `loaf session log --from-hook`) |
| `prompt` | `prompt:` | Injects text directly to the AI model |

### Hook Fields

| Field | Required | Notes |
|-------|----------|-------|
| `id` | Yes | Unique hook identifier |
| `skill` | Yes | Owning skill name |
| `type` | No | `script` (default), `command`, or `prompt` |
| `matcher` | No | Tool name filter: `"Edit\|Write\|Bash"` |
| `if` | No | Conditional matcher, e.g., `"Bash(git commit:*)"` — hook only runs when invocation matches |
| `failClosed` | No | `true` to block the action on hook failure (enforcement hooks) |
| `blocking` | No | `true` if hook can block tool execution |
| `timeout` | No | Timeout in milliseconds |

### hooks.yaml

Register hooks with their `skill:` field pointing to the relevant skill:

```yaml
hooks:
  pre-commit:
    - id: scan-secrets
      skill: security-compliance
      script: hooks/pre-commit/scan-secrets.sh
  pre-tool:
    - id: check-secrets
      skill: foundations
      type: command
      command: loaf check --hook check-secrets
      failClosed: true
      matcher: "Bash"
    - id: session-nudge
      skill: orchestration
      type: prompt
      prompt: "Log important decisions to the session journal."
      if: "Bash(git commit:*)"
```

### targets.yaml

Configure target-specific behavior and sidecars.

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Put Claude fields in SKILL.md | Use `.claude-code.yaml` sidecar |
| Use "Coverage" in reference tables | Use "Use When" (action-oriented) |
| Skip TOC for long files | Add `## Contents` after title |
| Use backslash paths | Use forward slashes: `references/file.md` |
| Nest references deeply | Link all references from SKILL.md |
| Start descriptions with "Use for..." | Start with action verb: "Covers...", "Establishes..." |
| Make reference skills user-invocable | Set `user-invocable: false` in sidecar |
| Skip negative routing for confusable skills | Add "Not for..." in description |
| Leave success criteria undefined for workflow skills | Add "Produces..." in description |
| Embed artifact templates inline in SKILL.md | Extract to `templates/` and link |
| Restate general knowledge models already have | Document only decisions and constraints |

## Version Management

- Version in `package.json`
- Build injects version into output files
- Bump version before release commits
- Use semantic versioning

## Related Documentation

- [Agent Skills Specification](https://agentskills.io/specification)
- [Claude Code Skills Best Practices](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices)
- [Claude Code Skills Documentation](https://code.claude.com/docs/en/skills)

<!-- loaf:managed:start v2.0.0-dev.24 -->
<!-- Maintained by loaf install/upgrade — do not edit manually -->
## Loaf Framework

**Session Journal Entry Types:**
- `decision(scope)`: Key decisions with rationale
- `discover(scope)`: Something learned
- `block(scope)` / `unblock(scope)`: Blockers and resolutions
- `spark(scope)`: Ideas to promote via `/idea`
- `todo(scope)`: Action items to promote to tasks

**CLI Commands:**
- `loaf session start/end/log/archive` — Session management
- `loaf check` — Run enforcement hooks
- `loaf task/spec/kb` — Task and knowledge management

**Journal Discipline:**
Before completing any response that includes significant decisions or discoveries, log journal entries using `loaf session log "type(scope): description"`. Entry types: `decision`, `discover`, `conclude`. Do not defer journaling — log before responding.
Git events (commits, PRs, merges) are auto-logged by PostToolUse hooks — do not log them manually.

See [orchestration skill](skills/orchestration/SKILL.md) for full details.
<!-- loaf:managed:end -->

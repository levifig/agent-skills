# Loaf Development Guidelines

Guidelines for maintaining and extending Loaf - An Opinionated Agentic Framework.

See [README.md](README.md) for what Loaf is and how to install it.

<!-- TRANSITIONAL: remove with spec-conversion-and-guidance-sweep (grep-gated by its convergence check) -->
> **Transition in progress — the Change model is landing.** New work is shaped as a Change: `/shape` → `docs/changes/YYYYMMDD-slug/change.md`, validated by `loaf change check` (see `loaf change --help`). The contract lives in `docs/changes/20260704-shape-first-change-workflow/change.md`. Existing `SPEC-*` work continues under `loaf spec`/`loaf task` until the conversion pass; skill guidance below that still reads "spec-first" predates the sweep and loses to this note.

## Quick Start

```bash
npm install && npm run build   # Build CLI + all targets
loaf build                     # After initial build, use the CLI directly
loaf install --to all          # Install to detected tools
```

## Project Structure

```
cmd/loaf/main.go                # CLI entry point (func main → internal/cli Runner)

internal/                       # CLI implementation (Go)
├── cli/                        # Command surface: Runner dispatch (cli.go), one runX per command
│   ├── build.go                # loaf build
│   ├── build_{target}.go       # Per-target builders (claude-code, opencode, cursor, codex, amp)
│   ├── check.go                # loaf check
│   ├── install.go              # loaf install
│   └── journal.go              # loaf journal
├── state/                      # SQLite-backed state (journal, specs, tasks, findings, ...)
└── project/                    # Project identity and root resolution

cli/                            # Node-side build tooling (not the CLI itself)
├── runtime/loaf-launcher.cjs   # Node launcher shim (copied to bin/loaf)
└── scripts/                    # build-go, build-release, verify-go-artifacts, etc.

bin/loaf                        # Installed Node launcher; execs the native binary in bin/native/

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
| CLI | `internal/cli/` | `cli.go` |

## Agent Profiles

See [SOUL.md](../SOUL.md) for the Warden identity and fellowship conventions.

| Profile | Concept | Tool Access | Use For |
|---------|---------|-------------|---------|
| **implementer** | Smith (Dwarf) | Full write | Code, tests, config, docs — speciality via skills |
| **reviewer** | Sentinel (Elf) | Read-only | Audits, reviews — mechanical independence |
| **researcher** | Ranger (Human) | Read + web | Research, comparison — structured reports |
| **librarian** | Librarian (Ent) | Read + Edit (.agents/) | Project journal, durable artifacts, wrap checkpoints, pre-compaction preservation |
| **background-runner** | System | Read + Edit | Async non-blocking tasks |

## Common Tasks

**Add skill:** Create `content/skills/{name}/SKILL.md`, register hooks in `hooks.yaml`

**Add hook:** Create script in `content/hooks/{pre,post}-tool/`, register in `hooks.yaml`

**Add template:** Create in `content/skills/{name}/templates/` (skill-specific) or `content/templates/` + register in `shared-templates` in `targets.yaml`

**Add target:** Create `internal/cli/build_{target}.go`, add the name to `defaultBuildTargets` and the target switch in `internal/cli/build.go` (see `build_amp.go` for the pattern)

## Skill Development

### Journal Self-Logging

User-invocable workflow skills must log their invocation to the project journal as their first action. Include context — arguments, intent, or what triggered the invocation:

```bash
loaf journal log "skill(shape): shaping auth token rotation idea into spec"
loaf journal log "skill(housekeeping): routine cleanup, no specific trigger"
loaf journal log "skill(wrap): end-of-conversation checkpoint"
loaf journal log "skill(implement): TASK-042 — journal-first hook rewrite"
```

There is no start step and no "active session" to find — the current branch and an opaque `harness_session_id` are attached automatically. This creates an audit trail of which skills ran; `/wrap` reads recent entries to check whether housekeeping or other periodic skills were run.

### Canonical Session Model

The project journal is the canonical session model, and it is the only session-related structure. Journal entries are project-scoped events stored in SQLite (`journal_entries`, `project_id NOT NULL`), each tagged with a `harness_session_id` that correlates one conversation's entries. There is no session entity, no lifecycle, and no status — nobody opens, closes, or transitions anything, so concurrent conversations across branches, worktrees, and harnesses are safe by construction. Log entries with `loaf journal log "type(scope): desc"`; read with `loaf journal recent`, `loaf journal context`, `loaf journal search`, and `loaf journal show`. Rendered markdown is a projection, not a hand-authored source surface.

**Wrap is an optional checkpoint, not a lifecycle transition.** Write a `wrap` entry only when a conversation holds synthesis worth saving — "tried X, abandoned because Y, next is Z" — the connective narrative that evaporates with the context window. Nothing is ever "unwrapped"; a conversation that ends without one leaves a perfectly valid journal.

**Continuity is derived and ephemeral.** At conversation start the SessionStart hook runs `loaf journal context --from-hook` to emit a layered digest — the latest project wrap, recent branch entries, and open tasks — computed at read time and never persisted. Subagent invocations exit silently and write nothing.

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

Artifact format templates (session renders, specs, ADRs, task files) live in `templates/` directories. SKILL.md references them with links instead of embedding inline.

**Skill-specific templates:** `content/skills/{name}/templates/` — templates unique to one skill.

**Shared templates:** `content/templates/` — distributed to multiple skills at build time via `shared-templates` config in `targets.yaml`.

```yaml
# targets.yaml
shared-templates:
  journal.md: [implement, orchestration, housekeeping, bootstrap]
  adr.md: [architecture, reflect]
```

**Reference pattern in SKILL.md:**
```markdown
Use [templates/journal.md](templates/journal.md) for the project journal render and entry-format reference.
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

### Journal Vocabulary

The project journal lives in SQLite and renders to a **compact inline format** — append-only structured logs. Think "conventional commits meets bullet journal." It is project-scoped, not session-scoped; there is no session entity, status, or lifecycle.

| Term | Meaning |
|------|---------|
| **Project Journal** | The SQLite `journal_entries` record for a project — every conversation writes into it |
| **Journal Render** | Markdown projection produced from SQLite state (`loaf journal export`) |
| **Journal Entry** | `[YYYY-MM-DD HH:MM] type(scope): description`, tagged with an opaque `harness_session_id` |
| **Entry Type** | `skill`, `commit`, `decision`, `discover`, `block`, `unblock`, `spark`, `todo`, `finding`, `wrap` |
| **Wrap** | Optional end-of-conversation checkpoint entry — synthesis worth saving, never a transition |
| **Burst** | Entries grouped without blank lines |

There are no session statuses: nothing is `active`, `paused`, `stopped`, `done`, or `archived`, because there is no session to be in a state.

**Entry Format:**
```markdown
[YYYY-MM-DD HH:MM] skill(implement): implementing TASK-042
[YYYY-MM-DD HH:MM] decision(scope): chose X because Y
[YYYY-MM-DD HH:MM] commit(abc1234): message
[YYYY-MM-DD HH:MM] discover(scope): learned Z from file/path
[YYYY-MM-DD HH:MM] wrap(scope): tried X, abandoned because Y, next is Z
```

See [templates/journal.md](../content/templates/journal.md) for the full entry-type table and format rules.

## Build System

### Commands

```bash
loaf build                     # Build all targets
loaf build --target claude-code  # Specific target
loaf install --to all          # Install to all detected tools
loaf install --to cursor       # Install to specific tool
loaf install --upgrade         # Update already-installed targets
loaf check                     # Run enforcement hooks manually
loaf journal log "type(scope): desc"  # Append a project journal entry
loaf journal recent            # Show the recent journal timeline
loaf journal context           # Emit the layered continuity digest
```

### Development

```bash
npm install                    # Install dependencies
npm run build:go               # Build the native Go binary only (cli/scripts/build-go.mjs)
npm run build                  # Build binary + CLI reference + all content targets, then verify
npm run typecheck              # Compile check (go test ./... -run=^$)
npm run test                   # Run Go tests (go test ./...)
npm link                       # Make `loaf` available globally (symlinks bin/loaf onto PATH)
```

### Dev Isolation (avoid polluting the global DB)

The global SQLite DB resolves via `XDG_DATA_HOME` to `~/.local/share/loaf/loaf.sqlite`.
Any real `loaf` command — dogfooding, manual smokes, scratch experiments — writes to
that production database unless redirected. Before running the real binary against
throwaway state, set `LOAF_DB` to an absolute path on a temp file:

```bash
export LOAF_DB="$(mktemp -d)/loaf.sqlite"
loaf journal recent      # operates on the isolated DB
rm -f "$LOAF_DB"         # clean up when done
```

When `LOAF_DB` is set to an absolute path it is used verbatim as the SQLite file
and overrides `XDG_DATA_HOME`. A relative value is ignored (standard XDG resolution
applies). Go unit tests isolate via temp dirs and `t.Setenv`, so no global state is
touched during `go test`.

### Targets

| Target | Output | Notes |
|--------|--------|-------|
| claude-code | `plugins/loaf/` | Merges sidecars into output |
| opencode | `dist/opencode/` | Skills, agents, and commands (from skills) |
| cursor | `dist/cursor/` | Skills, agents, and hooks |
| codex | `dist/codex/` | Skills only |
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
| `command` | `command:` | Runs a CLI command (e.g., `loaf journal log --from-hook`) |
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
    - id: journal-nudge
      skill: orchestration
      type: prompt
      prompt: "Log important decisions to the project journal."
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

<!-- loaf:managed:start sha256=ac6debb93fcd1b2d7806681c446f3b7d9691a43a872831a969c82a7470b0b30d -->
<!-- Maintained by loaf install/upgrade - do not edit manually -->
## Loaf Framework

**Journal Entry Types:**
- `decision(scope)`: Key decisions with rationale
- `discover(scope)`: Something learned
- `block(scope)` / `unblock(scope)`: Blockers and resolutions
- `spark(scope)`: Ideas to promote via `/idea`
- `todo(scope)`: Action items to promote to tasks

**CLI Commands:**
- `loaf journal log/recent/search/context` - Project journal
- `loaf check` - Run enforcement hooks
- `loaf task/spec/kb` - Task and knowledge management

**Journal Discipline:**
Before completing any response that includes edits, commits, or significant decisions, log journal entries using `loaf journal log "type(scope): description"`. Entry types: `decision`, `discover`, `wrap`. Do not defer journaling - log before responding.
In Codex Auto mode, when the user explicitly installed the managed basic-command policy, use the exact path-pinned Loaf executable in the managed `CODEX_HOME/AGENTS.md` block; do not substitute a bare `loaf`. The policy authorizes only explicitly classified basic Loaf command leaves and does not grant unclassified/operator commands, a bare Loaf namespace, or general filesystem access. Other harness adapters are not implied.

See the Loaf `orchestration` skill for full details.
<!-- loaf:managed:end -->

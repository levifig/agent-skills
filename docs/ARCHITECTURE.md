# Loaf Architecture

## Current Architecture (v2.0)

```
cli/                            # CLI tool (TypeScript, bundled by tsup)
├── index.ts                    # Commander.js entry point
├── commands/
│   ├── build.ts                # loaf build
│   ├── check.ts                # loaf check (enforcement backend)
│   ├── install.ts              # loaf install
│   ├── session.ts              # loaf session (start/end/log/list/archive)
│   └── spec.ts                 # loaf spec (list/archive)
└── lib/
    ├── build/
    │   ├── types.ts            # Shared types (BuildContext, HooksConfig, etc.)
    │   ├── targets/            # Target transformers (claude-code, cursor, opencode, codex, gemini, amp)
    │   └── lib/                # Utilities (version, sidecar, shared-templates, etc.)
    ├── detect/                 # AI tool + MCP detection
    ├── install/                # Target-specific installers + fenced-section management
    ├── tasks/                  # Task/spec types, parser, migration, archival
    ├── release/                # Version bump, changelog generation
    ├── housekeeping/           # Artifact scanning, stale detection
    └── kb/                     # Knowledge base loader, staleness, resolution

content/                        # Distributable content (separated from tooling)
├── skills/{name}/SKILL.md      # Domain knowledge (Agent Skills standard)
├── agents/{name}.md            # Functional profiles (tool boundaries + behavioral contracts)
├── hooks/session/              # Session lifecycle scripts (compact.sh)
└── templates/                  # Shared templates (distributed at build time)

config/
├── hooks.yaml                  # Hook definitions (enforcement, instruction, session lifecycle)
└── targets.yaml                # Target defaults + shared-templates mapping

Output:
├── dist-cli/                   # Bundled CLI (single JS file via tsup)
├── plugins/loaf/               # Claude Code plugin (hooks, skills, agents, binary)
└── dist/{target}/              # Other targets (cursor, opencode, codex, gemini, amp)
```

### Build Flow

```
cli/index.ts → tsup → dist-cli/index.js (bundled CLI)
content/ + config/ → loaf build → dist/ + plugins/
```

Each target transformer reads content (skills/agents/hooks) and config, then produces target-specific output. Skills get sidecar files merged. Hooks get registered in plugin manifests. Shared templates get distributed to specified skills.

All TypeScript, bundled into a single file by tsup. No dynamic imports. The `loaf` binary in `plugins/loaf/bin/` is a self-contained copy of the bundled CLI with all npm dependencies inlined.

### Targets

| Target | Output | Agents | Skills | Hooks | Runtime Plugin |
|--------|--------|:------:|:------:|:-----:|:--------------:|
| claude-code | plugins/loaf/ | Yes | Yes | Yes | plugin.json |
| cursor | dist/cursor/ | Yes | Yes | Yes | hooks.json |
| opencode | dist/opencode/ | Yes | Yes | Yes | hooks.ts |
| codex | dist/codex/ | No | Yes | Yes | hooks.json |
| gemini | dist/gemini/ | No | Yes | No | — |
| amp | dist/amp/ | No | Yes | No | loaf.js |

### Agent Model: Functional Profiles

Loaf uses **functional profiles** defined by tool access boundaries, not role-based agents defined by domain identity. Skills provide all domain knowledge; profiles provide the tool sandbox.

**The Warden (Coordinator):**

The main session is the Warden — a persistent coordinator identity defined in `SOUL.md`. The Warden orchestrates, advises, and delegates but does not forge, review, or scout directly. `SOUL.md` lives at the project root; a SessionStart hook validates its presence and restores it from the canonical template (`content/templates/soul.md`) if missing.

**4 Functional Profiles:**

| Profile | Concept | Race | Tool Access | Purpose |
|---------|---------|------|-------------|---------|
| Smith | Implementer | Dwarf | Full write | Forges code, tests, config, docs. Speciality via skills at spawn time. |
| Sentinel | Reviewer | Elf | Read-only | Watches, guards, verifies. Cannot modify what it reviews. |
| Ranger | Researcher | Human | Read + Web | Scouts far, gathers intelligence, reports back. No write or execute. |
| Keeper | Librarian | Ent | Read + Edit (.agents/) | Tends session lifecycle, state, wrap summaries. Does not forge code or scout. |

Each profile is defined in `content/agents/{implementer,reviewer,researcher,librarian}.md` — a minimal behavioral contract and tool boundary, not domain knowledge. A spawned Smith becomes a backend engineer, DBA, or devops engineer depending entirely on the skills loaded at spawn time.

**1 System Agent:**

| Agent | Purpose |
|-------|---------|
| background-runner | Async non-blocking tasks (haiku model) |

**Council Composition:**

Councils convene Smiths and Rangers for deliberation. Rangers advocate for users, informed by their scouting. Sentinels come after, not during — they verify the outcome. The Warden orchestrates but never votes.

**Skills as Universal Knowledge Layer:**

Skills are the only knowledge mechanism that works across all targets (Claude Code, Cursor, Codex, Gemini, Amp). Profiles are Claude Code infrastructure — other targets activate knowledge through skills alone. This makes skills the primary investment surface: better skill descriptions and organization improve all targets simultaneously.

## Execution Model

The execution model is a three-artifact pipeline. No separate "plan" artifact — the journal serves as both execution trace and resumption protocol.

```
/shape → SPEC file → /breakdown → TASK files → /implement → Session/Journal → Done
```

### Task System

```
.agents/specs/SPEC-XXX.md       # Bounded work definitions (scope, test conditions, priority order)
.agents/tasks/TASK-XXX-slug.md  # Individual work items (criteria, file hints, verification)
.agents/tasks/TASKS.json        # Programmatic index (CLI readable)
.agents/sessions/               # Active session journals
.agents/sessions/archive/       # Completed sessions
```

**Specs** define *what* to build — problem, solution direction, boundaries, test conditions. Multi-part specs use priority ordering with go/no-go gates between tracks (ship in order, drop from end). Sized by complexity (small/medium/large), not time.

**Tasks** define *what to do* — one concern per task, file hints, verification command, observable done condition. Created by `/breakdown`, worked by `/implement`.

**Sessions** capture *what happened* — the journal records decisions, discoveries, commits, and progress as structured entries. The `## Current State` section provides handoff-ready context for compaction recovery and cross-conversation resumption.

### Session Lifecycle

Sessions are branch-scoped and managed programmatically by `loaf session start` (SessionStart hook) and `loaf session end` (SessionEnd hook).

**Subagent detection:** Hook JSON from Claude Code includes `agent_id` only for subagents. `loaf session start` checks for this and exits silently — subagents are session-unaware, preventing the session churn that occurs when Task tool spawns trigger SessionStart.

**Cross-conversation continuity:** `session_id` from hook JSON is stored as `claude_session_id` in session frontmatter. On SessionStart, if the incoming session_id differs from the stored one, the session knows it's a new conversation and writes resume entries. `loaf session end` writes the `--- PAUSE ---` separator with the correct timestamp.

**Compaction resilience:** The session journal is external memory that survives context compaction. PreCompact requires flushing unrecorded entries and writing a state summary to `## Current State`. PostCompact nudges the model to re-read the session file for resumption context. No separate snapshot mechanism needed.

### Journal Entry Sources

Session journals receive entries from multiple layered sources:

| Source | Mechanism | When |
|--------|-----------|------|
| Skills | `loaf session log` in skill Critical Rules | Self-logging on invocation |
| Git events | PostToolUse command hooks | Commits, PRs, merges (automatic) |
| Task events | TaskCompleted session hook | Task completed/cancelled (automatic) |
| Context | UserPromptSubmit command hook | Every user prompt |
| Compaction | PreCompact prompt hook | Emergency journal flush |

Skills self-log as their first action. Git and task events are captured automatically by hooks. The UserPromptSubmit hook injects session context and orchestration conventions on every prompt.

**Session management policy:**

| Scenario | Action |
|----------|--------|
| Same scope, continuing work | Compact (journal survives) |
| Different scope entirely | New conversation (new session) |
| Finished and archived a spec | New conversation |
| Context full mid-task | Auto-compact |
| Quick unrelated question | New conversation |

## Hook Architecture

Hooks are defined in `config/hooks.yaml` and distributed to target-specific formats at build time. For Claude Code, the canonical hook registration file is `hooks/hooks.json` (inside the plugin output directory). `plugin.json` silently drops non-matcher session lifecycle events — all hooks should be registered in `hooks.json`.

### Dispatch Types

| Type | Field | Behavior |
|------|-------|----------|
| script | `script:` | Runs a shell script |
| command | `command:` | Runs a CLI command (e.g., `loaf check --hook <id>`) |
| prompt | `prompt:` | Injects text directly to the AI model |

### Hook Type Behavioral Constraints

Hard-won constraints validated during SPEC-030 implementation:

- **`type: prompt`** — Binary gate. Any non-empty LLM response is treated as rejection (`ok: false`). Cannot express "this looks fine, proceed" — the response itself blocks. Unusable for advisory hooks or hooks requiring LLM judgment. Use only for validation that returns empty on success.
- **`type: agent`** — Read-only tool access (Read, Grep, Glob, WebFetch, WebSearch). No Edit, Write, or Bash. Max 50 turns. Useful for observation, not mutation.
- **`type: command`** — Correct primitive for context injection and side effects. Exit 0 with stdout for context injection. Exit 1 for non-blocking warning. Exit 2 to block the action.
- **Stop event circularity** — Writing to session files from a Stop hook can re-trigger the hook chain. State writes must be idempotent or guarded against re-entry.
- **PreCompact prompt hooks** — Not supported outside REPL sessions. Use `type: command` for PreCompact context injection.
- **`plugin.json` drops non-matcher events** — Session lifecycle events (SessionStart, SessionEnd, TaskCompleted) must be registered in `hooks/hooks.json`, not `plugin.json`.
- **UserPromptSubmit has no matcher** — Fires on every user message, cannot be filtered by tool name or input.
- **Session events use different JSON shape** — `hook_event_name` field instead of `tool_name`. TaskCompleted passes `task_subject` and `task_description`.
- **Plugin caching** — Cached plugin versions serve stale hook handlers during development. Marketplace remove/re-add is the reliable cache-busting path.

### Hook Categories

**Enforcement hooks** — quality gates that block bad actions. Run by `loaf check` as a unified TypeScript backend. Exit non-zero to block. `failClosed: true` means failures block the action. `validate-push` (pre-push) restricts direct pushes to the default branch to `.agents/` and `docs/` files only. Code changes require a feature branch and pull request.

**Instruction hooks** — context injection at tool invocation. Triggered by `matcher` patterns (tool name) and optionally filtered by `if` conditions (tool input). Inject relevant skill instructions or nudges.

**Session lifecycle hooks** — tied to events (`SessionStart`, `SessionEnd`, `PreCompact`, `PostCompact`, `Stop`). Manage session journals, compaction, and SOUL.md validation.

### Hook JSON Data Model

Claude Code passes JSON to hooks via stdin. Key fields for post-tool hooks:

| Field | Description |
|-------|-------------|
| `session_id` | Current Claude conversation ID |
| `agent_id` | Present only for subagents — the discriminator for session-aware hooks |
| `tool_name` | Name of the tool invoked (e.g., `"Bash"`) |
| `tool_input` | Arguments sent to the tool |
| `tool_response` | Result/output returned by the tool (post-tool only) |
| `cwd` | Working directory |

`loaf session log --from-hook` uses `tool_input.command` to detect commit/PR/merge patterns and `tool_response` to extract PR numbers from output.

## Knowledge Management

```
docs/knowledge/          # Knowledge files with frontmatter (covers:, topics:, etc.)
docs/decisions/          # ADRs (immutable decision records)
.agents/loaf.json        # Project config (local KB dirs, imports, integration toggles)
```

Knowledge files are managed by `loaf kb` — staleness detection compares file modification time against configurable thresholds. SessionStart surfaces stale file counts. The `/housekeeping` skill flags stale files for review.

## Config

```
.agents/loaf.json               # Project-level (knowledge dirs, integration toggles, settings)
~/.local/state/loaf/            # User-level (registered KBs, default settings)
~/.config/loaf/                 # User preferences
```

Integration toggles in `loaf.json` gate runtime features (Linear magic-word detection, MCP recommendations) without rebuilding.

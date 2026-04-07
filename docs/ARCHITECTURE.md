# Loaf Architecture

## Current Architecture (v2.0)

```
cli/                            # CLI tool (TypeScript, bundled by tsup)
├── index.ts                    # Commander.js entry point
├── commands/
│   ├── build.ts                # loaf build
│   └── install.ts              # loaf install
└── lib/
    ├── build/
    │   ├── types.ts            # Shared types (BuildContext, HooksConfig, etc.)
    │   ├── targets/            # Target transformers (*.ts)
    │   └── lib/                # Utilities (version, sidecar, substitutions, etc.)
    ├── detect/tools.ts         # AI tool detection
    └── install/installer.ts    # Target-specific installers

content/                        # Distributable content (separated from tooling)
├── skills/{name}/SKILL.md      # Domain knowledge (Agent Skills standard)
├── agents/{name}.md            # Agent definitions (frontmatter: model, skills, tools)
├── hooks/{pre,post}-tool/      # Lifecycle hooks (shell/Python scripts)
└── templates/                  # Shared templates (distributed at build time)

config/
├── hooks.yaml                  # Hook definitions + plugin-groups
└── targets.yaml                # Target defaults + shared-templates mapping

Output:
├── dist-cli/                   # Bundled CLI (single JS file via tsup)
├── plugins/loaf/               # Claude Code plugin (with hooks, MCP servers)
└── dist/{target}/              # Other targets (cursor, opencode, codex, gemini)
```

### Build Flow

```
cli/index.ts → tsup → dist-cli/index.js (bundled CLI)
content/ + config/ → loaf build → dist/ + plugins/
```

Each target transformer reads content (skills/agents/hooks) and config, then produces target-specific output. Skills get sidecar files merged. Hooks get registered in plugin manifests. Shared templates get distributed to specified skills.

All TypeScript, bundled into a single file by tsup. No dynamic imports.

### Targets

| Target | Output | Agents | Skills | Hooks | MCP |
|--------|--------|:------:|:------:|:-----:|:---:|
| claude-code | plugins/loaf/ | Yes | Yes | Yes | Yes |
| cursor | dist/cursor/ | Yes | Yes | Yes | No |
| opencode | dist/opencode/ | Yes | Yes | No | No |
| codex | dist/codex/ | No | Yes | No | No |
| gemini | dist/gemini/ | No | Yes | No | No |

### Agent Model: Functional Profiles

Loaf uses **functional profiles** defined by tool access boundaries, not role-based agents defined by domain identity. Skills provide all domain knowledge; profiles provide the tool sandbox.

**The Warden (Coordinator):**

The main session is the Warden — a persistent coordinator identity defined in `SOUL.md`. The Warden orchestrates, advises, and delegates but does not forge, review, or scout directly. `SOUL.md` lives at the project root; a SessionStart hook validates its presence and restores it from the canonical template (`content/templates/soul.md`) if missing.

**3 Functional Profiles:**

| Profile | Concept | Race | Tool Access | Purpose |
|---------|---------|------|-------------|---------|
| Smith | Implementer | Dwarf | Full write | Forges code, tests, config, docs. Speciality via skills at spawn time. |
| Sentinel | Reviewer | Elf | Read-only | Watches, guards, verifies. Cannot modify what it reviews. |
| Ranger | Researcher | Human | Read + Web | Scouts far, gathers intelligence, reports back. No write or execute. |

Each profile is defined in `content/agents/{implementer,reviewer,researcher}.md` — a minimal behavioral contract and tool boundary, not domain knowledge. A spawned Smith becomes a backend engineer, DBA, or devops engineer depending entirely on the skills loaded at spawn time.

**2 System Agents (unchanged):**

| Agent | Purpose |
|-------|---------|
| background-runner | Async non-blocking tasks (haiku model) |
| context-archiver | Session preservation before context compaction |

**Council Composition:**

Councils convene Smiths and Rangers for deliberation. Rangers advocate for users, informed by their scouting. Sentinels come after, not during — they verify the outcome. The Warden orchestrates but never votes.

**Skills as Universal Knowledge Layer:**

Skills are the only knowledge mechanism that works across all targets (Claude Code, Cursor, Codex, Gemini). Profiles are Claude Code infrastructure — other targets activate knowledge through skills alone. This makes skills the primary investment surface: better skill descriptions and organization improve all targets simultaneously.

## Planned Architecture

### Loaf CLI

A unified `loaf` command that wraps build operations, knowledge management, task management, and multi-harness distribution. Language TBD (Node.js likely, same runtime as build system).

### Knowledge Management Layer

```
docs/knowledge/          # Knowledge files with frontmatter (covers:, topics:, etc.)
docs/decisions/          # ADRs (immutable decision records)
.agents/loaf.json        # Project config (local KB dirs, imports, settings)

QMD (retrieval backend):
  Collections → indexed directories
  Context → semantic metadata
  Search → BM25 + optional vector + reranking
  MCP server → agent access

Loaf (lifecycle layer):
  Skill → agent guidance (when to create, update, search)
  Hooks → staleness detection, growth prompts
  CLI → loaf kb check, validate, status, review
```

### Task System

```
.agents/specs/SPEC-XXX.md       # Shape Up specs (scope, test conditions, priority order)
.agents/tasks/TASK-XXX-slug.md  # Task details (criteria, context, verification)
.agents/tasks/TASKS.json        # Programmatic index (CLI/TUI readable)
```

### Config

```
.agents/loaf.json               # Project-level (knowledge, implementation settings)
~/.local/state/loaf/            # User-level (registered KBs, default settings)
~/.config/loaf/                 # User preferences
```

# APT - Agentic Product Team

Your personal agentic product team for AI coding assistants. Skills contain knowledge, agents route to skills.

**Version:** 1.7.0

## Philosophy

**Skills contain knowledge. Agents route to skills.**

- **Skills**: Domain-specific patterns, best practices, and reference materials
- **Agents**: Thin routing layers that detect context and load appropriate skills
- **Hooks**: Automated quality gates that run before/after tool execution
- **Commands**: Portable workflows that work across tools
- **MCP Servers**: External integrations (Serena, Sequential Thinking, Linear)
- **LSP Servers**: Language intelligence (gopls, pyright, typescript-language-server, solargraph)

---

## Available Agents

### Implementation Agents

These agents write code and make direct changes to the codebase:

| Agent | Role | When to Use |
|-------|------|-------------|
| `backend-dev` | Backend Services | Python, Ruby/Rails, Go, or TypeScript backend development |
| `frontend-dev` | Frontend Development | React, Next.js, UI components |
| `devops` | Infrastructure | Docker, Kubernetes, CI/CD, deployment |

### Advisory/Council Agents

These agents provide domain expertise in councils and delegate implementation to implementation agents:

| Agent | Role | When to Use |
|-------|------|-------------|
| `dba` | Database Administration | Schema design, migrations, query optimization |
| `qa` | Quality Assurance | Testing, code review, security audits |
| `design` | UI/UX Design | Design systems, accessibility, visual patterns |
| `power-systems` | Energy Systems | Grid design, power electronics, renewable integration |

### Orchestrator

| Agent | Role | When to Use |
|-------|------|-------------|
| `pm` | Orchestrator | Coordinating multi-agent work, session management |

The PM agent delegates research to Explore/Plan agents, then delegates implementation to implementation agents. Advisory agents participate in councils and delegate their implementation needs to the appropriate implementation agent.

## Available Skills

| Skill | Domain | Key Topics |
|-------|--------|------------|
| `orchestration` | PM Workflow | Sessions, councils, Linear integration, Shape Up methodology |
| `foundations` | Quality | Code style, docs, security, commits |
| `python` | Backend | FastAPI, Pydantic, pytest, async |
| `typescript` | Full-stack | TypeScript/JavaScript, React, Next.js, type safety, testing |
| `ruby` | Backend | Ruby idioms, Rails 8, Hotwire, Minitest, gem dev, CLI tools, DHH/37signals philosophy |
| `go` | Backend | Go services, concurrency, testing |
| `database` | Data | Schema design, migrations, query optimization |
| `infrastructure` | DevOps | Docker, Kubernetes, Terraform |
| `design` | UI/UX | Accessibility, design tokens, responsive |
| `power-systems` | Energy | Grid design, power electronics, renewable integration |

Skills are guidelines, not tasks. They provide domain knowledge that agents reference when doing work.

---

## Plugin Scoping

Commands and agents use scoped naming to avoid conflicts:

```bash
# Commands are scoped to the plugin
/apt:start-session
/apt:council-session
/apt:review-sessions

# Agents are scoped when spawning tasks
Task(apt:backend-dev)
Task(apt:frontend-dev)
Task(apt:dba)
```

This allows multiple plugins to coexist without name collisions.

---

## Repository Structure

```
agent-skills/
├── AGENTS.md                    # This file - universal instructions
├── CLAUDE.md -> AGENTS.md       # Symlink for Claude Code
├── src/                         # Source files (canonical)
│   ├── skills/                  # Domain knowledge
│   │   ├── {skill}/
│   │   │   ├── SKILL.md         # Main skill file with frontmatter
│   │   │   ├── reference/       # Detailed reference docs
│   │   │   └── scripts/         # Utility scripts (optional)
│   ├── agents/                  # Thin routing agents
│   │   ├── pm.md
│   │   ├── backend-dev.md
│   │   └── ...
│   ├── commands/                # Portable commands
│   │   ├── start-session.md
│   │   └── ...
│   ├── hooks/                   # Hook scripts
│   │   ├── pre-tool/            # Run BEFORE tool execution
│   │   ├── post-tool/           # Run AFTER tool execution
│   │   ├── session/             # Session lifecycle hooks
│   │   └── lib/                 # Shared utilities
│   └── config/
│       ├── hooks.yaml           # Hook definitions for build
│       └── targets.yaml         # Target definitions and defaults
├── build/                       # Build scripts
│   ├── build.js                 # Main orchestrator
│   ├── lib/                     # Shared utilities
│   │   └── sidecar.js           # Sidecar loading and transforms
│   └── targets/                 # Target-specific transformers
│       ├── claude-code.js
│       ├── opencode.js
│       └── agentskills.js
├── .github/workflows/
│   └── build.yml                # CI: builds and commits outputs
├── install.sh                   # Curl-pipeable installer
├── plugins/                     # Claude Code marketplace (at root, committed by CI)
├── .claude-plugin/              # Claude Code marketplace manifest (at root)
└── dist/                        # Other distributions (committed by CI)
    ├── opencode/                # OpenCode skills, agents, plugins
    └── agentskills/             # Generic format (Codex, Cursor, Copilot, Gemini)
```

---

## Build System

### Commands

```bash
# Install dependencies
npm install

# Build all targets
npm run build

# Build specific target
npm run build:claude-code
npm run build:opencode
npm run build:agentskills
```

### How Build Works

The build system reads canonical content from `src/` and transforms it for each target:

1. **Reads** configuration files:
   - `src/config/hooks.yaml` for hook definitions and plugin groups
   - `src/config/targets.yaml` for target defaults and sidecar configuration
2. **Loads sidecars** (optional per-file overrides like `pm.opencode.yaml`)
3. **Transforms** canonical structure into target-specific format
4. **Outputs**:
   - Claude Code: `plugins/` and `.claude-plugin/` at repo root
   - Others: `dist/{target}/`

| Target | Output Location | Hook Support |
|--------|-----------------|--------------|
| Claude Code | `plugins/` at root | Full (PreToolUse, PostToolUse, Session*) |
| OpenCode | `dist/opencode/` | Full (tool.execute.*, session.*) |
| Agent Skills | `dist/agentskills/` | None (instructions only) |

The `agentskills` target produces a generic format shared by Codex, Cursor, Copilot, Gemini, and other tools that support simple skills/instructions.

### Adding a New Target

1. Create `build/targets/{target}.js`
2. Export `build({ config, targetConfig, targetsConfig, rootDir, srcDir, distDir, targetName })` function
3. Add target defaults to `src/config/targets.yaml`
4. Add to `TARGETS` in `build/build.js`
5. Add npm script to `package.json`

### Sidecar Metadata System

Sidecars provide target-specific overrides without modifying canonical source files.

**How it works:**
- Defaults defined in `src/config/targets.yaml`
- Optional sidecar files override defaults per-file
- Body content stays canonical across all targets

**Sidecar naming:**
- Agents: `{agent}.{target}.yaml` (e.g., `pm.opencode.yaml`)
- Skills: `SKILL.{target}.yaml` (e.g., `SKILL.agentskills.yaml`)

**Example agent sidecar (`src/agents/pm.opencode.yaml`):**

```yaml
# Override frontmatter only - body stays canonical
frontmatter:
  name: PM                    # Override display name
  mode: primary               # Set agent mode
  tools:
    remove: ["Task(*)"]       # Remove tools matching pattern
    add: { Task: true }       # Add new tools
```

**Example skill sidecar (`src/skills/python/SKILL.agentskills.yaml`):**

```yaml
# Agent Skills-specific metadata (used by Codex, Cursor, Copilot, Gemini)
globs:
  - "**/*.py"
  - "**/pyproject.toml"
```

**Target defaults (`src/config/targets.yaml`):**

```yaml
targets:
  claude-code:
    output: plugins/

  opencode:
    output: dist/opencode/
    defaults:
      agents:
        frontmatter:
          mode: subagent
          tools:
            transform: array-to-record
            remove: ["Task(*)"]

  agentskills:
    output: dist/agentskills/
    # Generic format shared by Codex, Cursor, Copilot, Gemini
```

---

## Distribution Architecture

### Overview

```
Source (src/skills/, src/agents/, src/hooks/)
         │
         ▼
    npm run build
         │
         ├──► Claude Code: plugins/, .claude-plugin/ at repo root
         │
         └──► Others: dist/ (for OpenCode, Agent Skills)
         │
         ▼
   Committed by CI
         │
         ├──► Claude Code: fetches plugins/ directly from GitHub
         │
         └──► Others: installer downloads dist/ to local cache, then installs
```

### CI/CD Pipeline

GitHub Actions (`.github/workflows/build.yml`) automatically:
1. Triggers on push to `main` (ignores `dist/**`, `plugins/**`, `.claude-plugin/**`, `*.md`)
2. Runs `npm ci && npm run build`
3. Commits and pushes `plugins/`, `.claude-plugin/`, and `dist/` if changed

This ensures outputs in the repo are always up-to-date with source changes.

### Target-Specific Behavior

#### Claude Code

- **No local installation needed**
- Users add the marketplace directly: `/plugin marketplace add levifig/agent-skills`
- Claude Code fetches `plugins/` from repo root
- Claude Code handles its own caching and updates
- Updates happen automatically when users interact with `/plugin`

#### OpenCode, Agent Skills (Codex, Cursor, Copilot, Gemini)

- **Requires local cache**
- Installer downloads pre-built `dist/` from GitHub
- Caches to `~/.local/share/agent-skills/` (only dist contents, not full repo)
- Installs to target-specific locations:
  - OpenCode: `~/.config/opencode/{skill,agent,command,plugin}/`
  - Codex: `~/.codex/skills/`
  - Cursor: Project `.cursor/rules/`
  - Copilot: Repo `.github/copilot-instructions.md`

### Installer Behavior

The installer (`install.sh`) behaves differently based on context:

#### Remote Install (curl-piped)

```bash
curl -fsSL .../install.sh | bash
```

1. Only requires `git` (no node/npm needed)
2. Clones repo to temp directory
3. Copies only `dist/` contents to `~/.local/share/agent-skills/`
4. Stores `.version` file for update detection
5. Installs to selected targets

#### Local Development Install

```bash
./install.sh  # Run from cloned repo
```

1. Requires `git`, `node 18+`, `npm`
2. Detects it's running from a local repo (has `.git`, `package.json`, `src/skills/`)
3. Builds all targets locally first (`npm install && npm run build`)
4. Syncs `dist/` to `~/.local/share/agent-skills/`
5. Installs to selected targets

This allows developers to test local changes before pushing.

### Cache Structure

The local cache at `~/.local/share/agent-skills/` contains only built distributions:

```
~/.local/share/agent-skills/
├── .version              # Git commit hash for update detection
├── opencode/
└── agentskills/
```

No source code, no `node_modules` - just the pre-built output.

### Design Decisions

1. **Claude Code at root**: Marketplace expects plugins at repo root, not subdirectory
2. **Source in src/**: Clear separation between source and built outputs
3. **CI builds on push**: Keeps outputs in sync with source automatically
4. **Claude Code uses remote**: No local cache needed, simpler UX
5. **Others use local cache**: Required for tools that don't fetch from GitHub
6. **Installer detects context**: Same script works for both remote and local dev

---

## Adding a New Skill

### 1. Create Skill Directory

```bash
mkdir -p src/skills/{skill-name}/reference
```

### 2. Create SKILL.md

Create `src/skills/{skill-name}/SKILL.md` with YAML frontmatter:

```markdown
---
name: skill-name
description: What this skill covers
version: 1.0.0
---

# Skill Name

Brief overview of the skill domain.

## Core Principles

Key principles and patterns for this domain.

## Patterns

### Pattern Name

Description and examples.

## Anti-patterns

What to avoid.

## Review & QA

Quality checklist specific to this skill domain.

## References

Links to external documentation.
```

### 3. Add Reference Docs

Create detailed reference files in `src/skills/{skill-name}/reference/`:

```markdown
# reference/core.md
# reference/testing.md
# reference/patterns.md
```

Each file covers one topic. Use consistent heading structure.

### 4. Add Scripts (Optional)

For validation or utility scripts, add to `src/skills/{skill-name}/scripts/`:

```bash
src/skills/{skill-name}/scripts/validate.sh
src/skills/{skill-name}/scripts/check-style.py
```

### 5. Register Hooks (If Any)

Add hook definitions to `src/config/hooks.yaml`:

```yaml
hooks:
  pre-tool:
    - id: skill-name-check
      skill: skill-name
      script: hooks/pre-tool/skill-name-check.sh
      matcher: "Edit|Write"
      blocking: false
      timeout: 60000
      description: Description of what this hook does
```

### 6. Add to Plugin Group

Add the skill to an appropriate plugin group in `src/config/hooks.yaml`:

```yaml
plugin-groups:
  skill-name:
    description: Description for marketplace
    agents: [relevant-agent]
    skills: [skill-name, foundations]
    hooks:
      pre-tool: [skill-name-check]
```

### 7. Build and Test

```bash
npm run build
```

---

## Adding a New Agent

### 1. Create Agent File

Create `src/agents/{agent-name}.md` with YAML frontmatter:

```markdown
---
name: agent-name
description: When to use this agent (shown in listings)
model: sonnet
skills: [skill1, skill2]
conditional-skills:
  - skill: optional-skill
    when: "detection pattern (e.g., pyproject.toml)"
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
  - TodoWrite
  - TodoRead
---

# Agent Name

Brief description of agent's role.

## What You Do

Clear responsibilities.

## What You Delegate

Work that goes to other agents.

## Workflow

Step-by-step process.

## Quality Checklist

Before marking work complete:
- [ ] Check 1
- [ ] Check 2

Reference `{skill}` skill for detailed patterns.
```

### 2. Keep Agent Thin

Agents should:
- Route to appropriate skills
- Define tools and permissions
- Provide high-level workflow

Agents should NOT:
- Duplicate skill content
- Contain detailed reference material
- Define patterns (that's what skills are for)

### 3. Add to Plugin Group

In `src/config/hooks.yaml`:

```yaml
plugin-groups:
  existing-group:
    agents: [existing-agent, agent-name]  # Add here
```

Or create a new group if appropriate.

### 4. Build and Test

```bash
npm run build
```

---

## Adding Hooks

### 1. Create Hook Script

Create script in appropriate directory:

```bash
# Pre-tool (runs before tool execution, can block)
src/hooks/pre-tool/{skill}-{action}.sh

# Post-tool (runs after tool execution, informational)
src/hooks/post-tool/{skill}-{action}.sh

# Session (lifecycle hooks)
src/hooks/session/{event}.sh
```

### 2. Script Structure

```bash
#!/usr/bin/env bash
set -euo pipefail

# Load shared libraries
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/json-parser.sh"

# Get tool input from environment
TOOL_NAME="${TOOL_NAME:-}"
TOOL_INPUT="${TOOL_INPUT:-{}}"
FILE_PATH="${FILE_PATH:-}"

# Your logic here
# ...

# Exit codes:
# 0 = success (proceed)
# 1 = failure (block if blocking=true)
```

### 3. Use Shared Libraries

Available in `src/hooks/lib/`:

| Library | Purpose |
|---------|---------|
| `json-parser.sh` | Parse Claude Code input JSON |
| `config-reader.sh` | Read configuration files |
| `agent-detector.sh` | Detect active agent context |
| `timeout-manager.sh` | Handle hook timeouts |

### 4. Register in Config

Add to `src/config/hooks.yaml`:

```yaml
hooks:
  pre-tool:
    - id: unique-hook-id
      skill: skill-name
      script: hooks/pre-tool/skill-action.sh
      matcher: "Edit|Write"  # Tool pattern to match
      blocking: true         # Block on failure?
      timeout: 60000         # Milliseconds
      description: Human-readable description
```

### 5. Add to Plugin Group

```yaml
plugin-groups:
  target-group:
    hooks:
      pre-tool: [existing-hooks, unique-hook-id]
```

---

## Modifying Existing Content

### Skills

1. Edit `src/skills/{name}/SKILL.md` directly
2. Add/update reference docs in `reference/`
3. Run `npm run build` to propagate changes
4. All targets will receive updates on next build

### Agents

1. Edit `src/agents/{name}.md` directly
2. Keep changes consistent with loaded skills
3. Update frontmatter if tools/skills change
4. Run build to verify

### Hooks

1. Edit scripts in `src/hooks/`
2. Update `src/config/hooks.yaml` if behavior changes
3. Test with target tool before committing
4. Hooks execute in declared order

### Config

1. Edit `src/config/hooks.yaml`
2. Validate YAML syntax
3. Run build to verify changes apply correctly

---

## Agent Selection Guide

### PM (Orchestrator)

Use for:
- Breaking down complex tasks into agent-sized work
- Managing work sessions and handoffs
- Running council deliberations
- Coordinating Linear issues
- Applying Shape Up methodology (appetite over estimates, shaping, circuit breakers, hill charts)

**Never implements directly** - delegates research to Explore/Plan agents, then delegates implementation to implementation agents.

### Implementation Agents

These agents write code and make direct changes:

#### Backend Developer

Auto-detects stack and loads appropriate skill:

| Signal | Stack |
|--------|-------|
| `pyproject.toml`, `*.py` | Python |
| `Gemfile`, `app/models/` | Ruby |
| `package.json` with backend deps | TypeScript |
| `go.mod`, `*.go` | Go |

#### Frontend Developer

For all React and Next.js work:
- Server and Client Components
- Type-safe props and state
- Accessibility compliance
- Responsive design

#### DevOps Engineer

For infrastructure work:
- Docker multi-stage builds
- Kubernetes manifests and Helm charts
- CI/CD pipelines
- GitOps patterns

### Advisory/Council Agents

These agents provide expertise in councils and delegate implementation to implementation agents:

#### Database Administrator

Provides expertise on:
- Schema design and normalization
- Migration safety (reversible, backward-compatible)
- Query optimization with EXPLAIN ANALYZE
- Index strategy

Has SQL safety validation hooks. Delegates implementation to `backend-dev`.

#### Quality Assurance

Provides expertise on:
- Unit and integration testing
- Code review (architecture, security, style)
- Security audits (OWASP Top 10)
- Documentation review

Delegates implementation to `backend-dev` or `frontend-dev`.

#### Design

Provides expertise on:
- Accessibility audits (WCAG 2.1 AA)
- Design token systems
- Component patterns
- Responsive layouts

Delegates implementation to `frontend-dev`.

#### Power Systems

Provides expertise on:
- Grid design and power flow
- Power electronics and converters
- Renewable energy integration
- Protection and control systems

Delegates implementation to `backend-dev`.

---

## Workflow Patterns

### Standard Task Flow

```
1. PM receives task
2. PM breaks down into agent-sized work
3. PM spawns appropriate agents
4. Agents load skills and complete work
5. PM tracks progress and outcomes
6. PM reports completion
```

### Council Deliberations

For decisions requiring multiple perspectives:

```
1. PM identifies decision requiring council
2. PM composes council (5-7 agents, always odd)
3. PM spawns all agents in parallel
4. Each agent provides individual report (recommendation, pros, cons, suggestions)
5. PM presents individual perspectives, then synthesizes
6. PM presents options and waits for user decision
7. PM records decision and delegates implementation
```

**Council composition rules:**
- Any agent can participate (implementation and advisory agents)
- Ad-hoc specialist personas can be created when domain expertise is needed
- PM coordinates but does NOT vote
- Always 5 or 7 agents (odd number to prevent ties)

See `orchestration` skill for detailed council patterns.

---

## Integrations

### MCP Servers

The plugin integrates with Model Context Protocol (MCP) servers for external tool access:

| Server | Purpose |
|--------|---------|
| **Serena** | Code intelligence, semantic search, symbol navigation |
| **Sequential Thinking** | Structured reasoning, problem decomposition |
| **Linear** | Issue tracking, project management, sprint planning |

### LSP Servers

Language Server Protocol integration provides intelligent code assistance:

| Server | Languages |
|--------|-----------|
| **gopls** | Go |
| **pyright** | Python (type checking, completion) |
| **typescript-language-server** | TypeScript, JavaScript |
| **solargraph** | Ruby |

---

## Quality Checklist

Before committing changes to this repository:

### Structure
- [ ] Skills have consistent frontmatter (name, description)
- [ ] Agents reference valid skills
- [ ] Hooks registered in src/config/hooks.yaml
- [ ] Plugin groups include all necessary components

### Content
- [ ] SKILL.md files follow template structure
- [ ] Reference docs cover one topic each
- [ ] Agent instructions are thin (defer to skills)
- [ ] Hook scripts use shared libraries

### Build
- [ ] `npm run build` succeeds for all targets
- [ ] Generated output matches expected structure
- [ ] No build warnings or errors

### Testing
- [ ] Hooks execute correctly in target tool
- [ ] Agents load appropriate skills
- [ ] Commands work as expected

### Documentation
- [ ] README updated if structure changed
- [ ] AGENTS.md reflects current agents/skills
- [ ] Skill SKILL.md files are complete

---

## Installation

### Claude Code

No installation needed. Add the marketplace directly:

```
/plugin marketplace add levifig/agent-skills
```

Then browse and install the `apt` plugin via `/plugin`. Updates are automatic.

### OpenCode, Agent Skills (Codex, Cursor, Copilot, Gemini)

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/agent-skills/main/install.sh | bash
```

The installer detects installed tools and guides you through target selection.

### Update

Run the installer again:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/agent-skills/main/install.sh | bash
```

### Development

For contributors testing local changes:

```bash
git clone https://github.com/levifig/agent-skills.git
cd agent-skills
npm install
./install.sh  # Builds and shows dev-specific instructions
```

When run from a local clone, the installer:
1. Shows "DEVELOPMENT MODE" banner
2. Builds all targets from source
3. Outputs Claude Code plugins to repo root (`plugins/`)
4. Syncs other distributions to `~/.local/share/agent-skills/`
5. Shows how to test with Claude Code using local path

**Testing Claude Code locally:**
```
/plugin marketplace add /path/to/agent-skills
```

**Rebuild after changes:**
```bash
npm run build
```

---

## Universal Quality Standards

All agents follow these quality checks:

### Before Changes
- [ ] Understand existing code patterns
- [ ] Load appropriate skill for guidance
- [ ] Consider backward compatibility

### During Changes
- [ ] Follow skill-specific conventions
- [ ] Add type annotations/hints
- [ ] Write tests for new functionality

### After Changes
- [ ] Run linters and type checkers
- [ ] Verify tests pass
- [ ] Update documentation if needed

---

## License

MIT

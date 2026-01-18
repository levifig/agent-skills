# Universal Agent Skills

Universal skills for AI coding assistants. Skills contain knowledge, agents route to skills.

## Philosophy

**Skills contain knowledge. Agents route to skills.**

- **Skills**: Domain-specific patterns, best practices, and reference materials
- **Agents**: Thin routing layers that detect context and load appropriate skills
- **Hooks**: Automated quality gates that run before/after tool execution
- **Commands**: Portable workflows that work across tools

---

## Available Agents

| Agent | Role | When to Use |
|-------|------|-------------|
| `pm` | Orchestrator | Coordinating multi-agent work, session management |
| `backend-dev` | Backend Services | Python, Rails, or TypeScript backend development |
| `frontend-dev` | Frontend Development | React, Next.js, UI components |
| `dba` | Database Administration | Schema design, migrations, query optimization |
| `devops` | Infrastructure | Docker, Kubernetes, CI/CD, deployment |
| `qa` | Quality Assurance | Testing, code review, security audits |
| `design` | UI/UX Design | Design systems, accessibility, visual patterns |

## Available Skills

| Skill | Domain | Key Topics |
|-------|--------|------------|
| `orchestration` | PM Workflow | Sessions, councils, Linear integration |
| `foundations` | Quality | Code style, docs, security, commits |
| `python` | Backend | FastAPI, Pydantic, pytest, async |
| `typescript` | Full-stack | React, Next.js, type safety, testing |
| `rails` | Backend | Rails 8, Hotwire, Minitest |
| `infrastructure` | DevOps | Docker, Kubernetes, Terraform |
| `design` | UI/UX | Accessibility, design tokens, responsive |

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
│       └── hooks.yaml           # Hook definitions for build
├── build/                       # Build scripts
│   ├── build.js                 # Main orchestrator
│   └── targets/                 # Target-specific transformers
│       ├── claude-code.js
│       ├── opencode.js
│       ├── cursor.js
│       └── copilot.js
├── .github/workflows/
│   └── build.yml                # CI: builds and commits outputs
├── install.sh                   # Curl-pipeable installer
├── plugins/                     # Claude Code marketplace (at root, committed by CI)
├── .claude-plugin/              # Claude Code marketplace manifest (at root)
└── dist/                        # Other distributions (committed by CI)
    ├── opencode/                # Skills, agents, plugins
    ├── cursor/                  # .cursor/rules/*.mdc
    └── copilot/                 # .github/copilot-instructions.md
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
npm run build:cursor
npm run build:copilot
```

### How Build Works

The build system reads canonical content from `src/` and transforms it for each target:

1. **Reads** `src/config/hooks.yaml` for hook definitions and plugin groups
2. **Transforms** canonical structure into target-specific format
3. **Outputs**:
   - Claude Code: `plugins/` and `.claude-plugin/` at repo root
   - Others: `dist/{target}/`

| Target | Output Location | Hook Support |
|--------|-----------------|--------------|
| Claude Code | `plugins/` at root | Full (PreToolUse, PostToolUse, Session*) |
| OpenCode | `dist/opencode/` | Full (tool.execute.*, session.*) |
| Cursor | `dist/cursor/` | None (instructions fallback) |
| Copilot | `dist/copilot/` | None (instructions fallback) |

### Adding a New Target

1. Create `build/targets/{target}.js`
2. Export `build({ config, rootDir, srcDir, distDir })` function
3. Add to `TARGETS` in `build/build.js`
4. Add npm script to `package.json`

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
         └──► Others: dist/ (for OpenCode, Cursor, Copilot)
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

#### OpenCode, Cursor, Copilot

- **Requires local cache**
- Installer downloads pre-built `dist/` from GitHub
- Caches to `~/.local/share/agent-skills/` (only dist contents, not full repo)
- Installs to target-specific locations:
  - OpenCode: `~/.config/opencode/{skill,agent,command,plugin}/`
  - Cursor: Instructions to copy to project `.cursor/rules/`
  - Copilot: Instructions to copy to repo `.github/`

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
├── cursor/
└── copilot/
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

**Never implements directly** - always delegates to specialized agents.

### Backend Developer

Auto-detects stack and loads appropriate skill:

| Signal | Stack |
|--------|-------|
| `pyproject.toml`, `*.py` | Python |
| `Gemfile`, `app/models/` | Rails |
| `package.json` with backend deps | TypeScript |

### Frontend Developer

For all React and Next.js work:
- Server and Client Components
- Type-safe props and state
- Accessibility compliance
- Responsive design

### Database Administrator

For database-focused work:
- Schema design and normalization
- Migration safety (reversible, backward-compatible)
- Query optimization with EXPLAIN ANALYZE
- Index strategy

### DevOps Engineer

For infrastructure work:
- Docker multi-stage builds
- Kubernetes manifests and Helm charts
- CI/CD pipelines
- GitOps patterns

### Quality Assurance

For quality gates:
- Unit and integration testing
- Code review (architecture, security, style)
- Security audits (OWASP Top 10)
- Documentation review

### Design

For UI/UX work:
- Accessibility audits (WCAG 2.1 AA)
- Design token systems
- Component patterns
- Responsive layouts

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
2. PM spawns 5-7 relevant agents in parallel
3. Each agent provides perspective
4. PM synthesizes and presents recommendation
5. User approves direction
6. PM records decision and delegates implementation
```

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

Then browse and install plugins via `/plugin`. Updates are automatic.

### OpenCode, Cursor, Copilot

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

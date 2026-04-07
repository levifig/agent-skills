# 🍞 Loaf

> "Why have just a slice when you can get the whole loaf?"

Loaf is an opinionated agentic framework that gives AI coding assistants structured knowledge, enforced tool boundaries, and a complete pipeline from idea to implementation to learning. Write your skills once, deploy to Claude Code, OpenCode, Cursor, Codex, and Gemini.

## Why Loaf?

**Portable knowledge** — 31 skills (29 active, 2 deprecated) covering workflows, engineering standards, and language expertise. Build once, deploy to five AI coding tools without rewriting anything.

**Session journal model** — Session files in `.agents/sessions/` capture state, decisions, and handoff context with frontmatter tracking. Work survives context loss, compaction, and `/clear`.

**Spec-first pipeline** — Ideas are shaped into bounded specs before any code is written. Every change flows: Idea → Spec → Tasks → Code → Learnings. Nothing gets lost.

**Profile-based agents** — Three functional profiles defined by tool access, not job titles. A Smith with `python-development` skills becomes a backend engineer; the same Smith with `infrastructure-management` becomes a DevOps engineer. Skills determine what an agent knows; the profile determines what it can touch.

**Session continuity** — Pick up exactly where you left off with full traceability. Session journals capture state, decisions, and handoff context in `.agents/sessions/`.

**Hooks as quality gates** — Two hook types: enforcement hooks (pre-commit secrets scanning, pre-push linting) block bad commits automatically; skill instruction hooks inject context at tool invocation time. Language-aware and automatic.

## The Pipeline

Loaf's commands form a three-phase workflow that mirrors how good software gets built:

```
┌─────────────────────────────────────────────────────────────┐
│                       PHASE 1: SHAPE                        │
│                                                             │
│        /idea  →  /brainstorm  →  /shape  →  SPEC         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                       PHASE 2: BUILD                        │
│                                                             │
│          /breakdown  →  /implement  →  /release           │
│                                                             │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                       PHASE 3: LEARN                        │
│                                                             │
│          /housekeeping  →  /reflect  →  /wrap             │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Phase 1: Shape

Transform raw ideas into implementable specs with clear boundaries.

| Command | What It Does |
|---------|--------------|
| `/idea` | Quick capture of rough ideas into `.agents/ideas/` |
| `/brainstorm` | Deep exploration of a problem space |
| `/shape` | Rigorous shaping into bounded spec (what's IN and OUT) |
| `/strategy` | Discover and document strategic direction |

### Phase 2: Build

Decompose specs into atomic tasks and execute with specialized agents.

| Command | What It Does |
|---------|--------------|
| `/breakdown` | Split spec into agent-sized atomic tasks |
| `/implement` | Execute tasks with orchestrated agent delegation |
| `/release` (`/ship`) | Orchestrate the merge ritual: pre-flight, docs check, version bump, squash merge, cleanup |

### Phase 3: Learn

Integrate outcomes into strategic knowledge.

| Command | What It Does |
|---------|--------------|
| `/housekeeping` | Review completed sessions, archive artifacts |
| `/reflect` | Integrate learnings into strategic documents |
| `/wrap` | Session summary: what shipped, what's pending, what's next |

### Pipeline Commands

CLI commands that support the workflow pipeline:

| Command | What It Does |
|---------|--------------|
| `loaf build` | Build all targets after modifying skills/agents |
| `loaf install` | Install to detected AI tools |
| `loaf check` | Run enforcement hooks manually |
| `loaf task` | Manage project tasks (list, show, update, archive) |
| `loaf spec` | Manage spec lifecycle |
| `loaf kb` | Knowledge base management |
| `loaf session` | Session journal management (list, start, end, log) |
| `loaf housekeeping` | Review and archive agent artifacts |
| `loaf release` / `loaf ship` | Orchestrate release ritual |

## Profiles

Loaf uses three functional profiles defined by mechanically enforced tool boundaries — not role titles, not domain labels. What an agent *can do* is fixed by its profile. What it *knows* comes from skills loaded at spawn time.

| Profile | Role | Tool Access | What It Does |
|---------|------|-------------|--------------|
| **Smith** | Implementer | Full write | Forges code, tests, config, and docs. Speciality determined by skills. |
| **Sentinel** | Reviewer | Read-only | Watches, guards, and verifies. Cannot modify what it reviews — by design. |
| **Ranger** | Researcher | Read + web | Scouts far, gathers intelligence, reports structured findings. |

The main session is the **Warden** — it coordinates and delegates but never implements directly. See [SOUL.md](SOUL.md) for the full fellowship identity.

## Skills

### Workflow

Skills you invoke directly to drive work forward.

| Skill | Activates When |
|-------|----------------|
| `shape` | Shaping ideas into bounded specs |
| `breakdown` | Decomposing specs into atomic tasks |
| `implement` | Starting task or spec implementation |
| `release` | Orchestrating the release ritual (version bump, squash merge) |
| `brainstorm` | Deep exploration of a problem space |
| `research` | Investigating questions, comparing options |
| `strategy` | Discovering or updating strategic direction |
| `architecture` | Creating Architecture Decision Records |
| `idea` | Quick capture of ideas for later evaluation |
| `triage` | Review and process intake queue (sparks + raw ideas) |
| `reflect` | Integrating learnings into strategic docs |
| `housekeeping` | Reviewing and archiving agent artifacts |
| `bootstrap` | Bootstrapping new or existing projects |
| `wrap` | End-of-session summary: shipped, pending, next |

### Orchestration & Knowledge

Background skills that activate automatically during agent coordination and project management.

| Skill | Activates When |
|-------|----------------|
| `orchestration` | Managing sessions, delegating agents, Linear integration |
| `council` | Multi-perspective deliberation during complex decisions |
| `knowledge-base` | Managing project knowledge files |
| `cli-reference` | Looking up which CLI command to use |

### Engineering Standards

Background knowledge that activates automatically to enforce quality.

| Skill | Activates When |
|-------|----------------|
| `foundations` | Writing code — style, naming, TDD, verification, code review |
| `git-workflow` | Branching, commits, PRs, squash merges |
| `debugging` | Diagnosing failures, tracking hypotheses, flaky tests |
| `security-compliance` | Threat modeling, secrets management, compliance checks |
| `documentation-standards` | ADRs, API docs, changelogs, Mermaid diagrams |

### Language & Domain

Domain expertise that loads based on project context.

| Skill | Activates When |
|-------|----------------|
| `typescript-development` | TypeScript, React, Next.js, Tailwind, Vitest |
| `python-development` | FastAPI, Pydantic, pytest, async patterns |
| `ruby-development` | Rails 8, Hotwire, Minitest |
| `go-development` | Go services, concurrency, testing |
| `interface-design` | UI/UX, accessibility (WCAG 2.1), design systems |
| `database-design` | Schema design, migrations, query optimization |
| `infrastructure-management` | Docker, Kubernetes, CI/CD, Terraform |
| `power-systems-modeling` | Thermal rating models, conductor physics |

## Multi-Target Support

Build once, deploy everywhere. Skills are the universal layer; profiles and hooks adapt per target.

| Target | Profiles | Skills | Hooks | Status |
|--------|:--------:|:------:|:-----:|--------|
| Claude Code | ✓ | ✓ | ✓ | Primary |
| OpenCode | ✓ | ✓ | ✓ | Full support |
| Cursor | ✓ | ✓ | ✓ | Full support |
| Codex | — | ✓ | ✓ | Skills + hooks |
| Gemini | — | ✓ | — | Skills only |

*Note: `council-session` skill renamed to `council` for consistency. Removed skills: `resume-session`, `reference-session`.*

## Getting Started

### Claude Code

```bash
/plugin marketplace add levifig/loaf
```

Updates happen automatically via plugin marketplace. Commands are scoped under `loaf:` (e.g., `/loaf:implement`).

### OpenCode, Cursor, Codex, Gemini

```bash
npx github:levifig/loaf install
```

Detects installed tools, lets you select targets, and installs pre-built distributions. Re-run with `--upgrade` to update.

**Install locations:**

| Target | Location |
|--------|----------|
| OpenCode | `~/.config/opencode/` or `~/.opencode/` |
| Cursor | `~/.cursor/` |
| Codex | `$CODEX_HOME/skills/` or `~/.codex/skills/` |
| Gemini | `~/.gemini/skills/` |

## Integrations

*Claude Code only.*

**Recommended MCP Servers:** Linear (issue tracking). **Optional:** Serena (semantic editing for large codebases — most code intelligence is now built into Claude Code's native LSP). Not bundled — `loaf install` will detect and recommend missing MCPs.

**LSP Servers:** gopls, pyright, typescript-language-server, solargraph

## Development

```bash
git clone https://github.com/levifig/loaf.git
cd loaf
npm install
npm run build
```

See [AGENTS.md](.agents/AGENTS.md) for development guidelines.

```bash
npm run typecheck    # Type check
npm run test         # Run tests
loaf build           # Build all targets (after initial npm run build)
loaf install --to all  # Install to detected tools
```

**Testing locally:**

- Claude Code: `/plugin marketplace add /path/to/loaf`
- Others: `loaf install --to all` (after `npm link`)

## License

MIT

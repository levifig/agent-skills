# 🍞 Loaf

> "Why have just a slice when you can get the whole loaf?"

Loaf is *my* opinionated agentic framework (aka *"Levi's Opinionated Agentic Framework"*), optimized for Claude Code but tightly couple with OpenCode and Cursor, and with mild integration with Codex and Gemini (Skills only). With Claude Code, it leverage's its full capabilities, using the Plugin marketplace, which allows automated updates and multi-agent orchestration, MCP servers, LSP integration, pre/post-tool hooks, and session management.

Loaf uses a scripted build system, allowing us to bundle native tooling for OpenCode and Cursor as well, closely following their own guidelines and protocols for optimal usage of Subagents, Commands, Hooks, and Skills. OpenCode even has the added benefit of allowing us to install our orchestrator/coordinator agent (aka `PM`) as a main Agent, which you can choose in addition to the built-in `Plan` and `Build`.

Given their own (current) limitations, distribution to Codex and Gemini consists of Skills only. I'm monitoring these tool's development and will add support for more features as they are made available.

## Installation

### Claude Code (easiest)

```
/plugin marketplace add levifig/loaf
```

Then install the `loaf` plugin via `/plugin`. No local setup needed—Claude Code fetches from GitHub automatically.

### OpenCode, Cursor, Codex, Gemini, etc

#### Install

OpenCode, Cursor, Codex, and Gemini can use the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh | bash
```

The installer will:

1. Detect which tools you have installed
2. Let you select which targets to install
3. Download pre-built distributions from GitHub
4. Cache to `~/.local/share/loaf/`
5. Install (rsync) to each selected target's config location (symlinks were problematic with some clients)

**What gets installed:**

| Target | Features | Location |
|--------|----------|----------|
| OpenCode | Skills, agents, commands, hooks | `~/.config/opencode/` |
| Cursor | Skills, agents, commands, hooks | `~/.cursor/` |
| Codex | Skills only | `$CODEX_HOME/skills` or `~/.codex/skills/` |
| Gemini | Skills only | `~/.gemini/skills/` |

Note: I tried, where possible, to support XDG-compliant or custom install directories. OpenCode supports `~/.config/opencode` so the installer will prefer that if it exists, defaulting to `~/.opencode` otherwise. Codex supports customizing config directory via env vars (aka `CODEX_HOME`), so we'll use that if set, otherwise fallback to `~/.codex`. Cursor and Gemini don't current support XDG-compliant or custom install directories.

### Update

Run the installer again:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh | bash
```

**OR** (recommended)

For unattended upgrades (CI, scripts, etc.), use `--upgrade` to skip prompts and only update already-installed targets:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh | bash -s -- --upgrade
```

For Claude Code, updates happen automatically when you use `/plugin`.

## How It Works

The build system transforms canonical source (`src/`) into target-specific formats:

| Target | Output | Features |
|--------|--------|----------|
| Claude Code | `plugins/` | Full (agents, skills, commands, hooks, MCP, LSP) |
| OpenCode | `dist/opencode/` | Full (agents, skills, commands, hooks) |
| Cursor | `dist/cursor/` | Full (agents, skills, commands, hooks) |
| Codex | `dist/codex/` | Skills only |
| Gemini | `dist/gemini/` | Skills only |

GitHub Actions automatically builds on every push to main.

## Agents

*Claude Code, OpenCode, and Cursor only.*

**Orchestrator:**

| Agent | Use For |
|-------|---------|
| `pm` | Orchestrating work, managing sessions, delegating |

**Explanation:** Use this as the orchestrator, like you'd use the native `Plan` feature. In Claude Code, it will call the `Plan` agent if it needs to. This agent coordinates the entire process.

**Implementation agents** (write code):

| Agent | Use For |
|-------|---------|
| `backend-dev` | Python, Ruby/Rails, Go, or TypeScript services |
| `frontend-dev` | React, Next.js, UI components |
| `devops` | Docker, Kubernetes, CI/CD |

**Explanation:** These will be called by the PM agent or you can call them directly for implementation work.

**Advisory agents** (provide expertise in councils, delegate implementation):

| Agent | Use For |
|-------|---------|
| `dba` | Schema design, migrations, queries |
| `qa` | Testing, code review, security |
| `design` | UI/UX, accessibility |
| `power-systems` | Grid design, power electronics, renewable integration |

**Explanation:** While these may be called directly, they are more often called as part of a "council", to get feedback and guidance for a specialized subject.

## Commands

*Claude Code, OpenCode, and Cursor only.*

Commands are the primary way to interact with Loaf. They orchestrate work sessions, manage state, and coordinate agents.

| Command | Purpose |
|---------|---------|
| `/start-session` | Start a new orchestrated work session |
| `/resume-session` | Resume an existing session |
| `/council-session` | Convene agents for multi-perspective deliberation |
| `/review-sessions` | Review and manage session artifacts |

### `/start-session`

Start a new orchestrated work session. The PM agent will:

1. Understand the task (from your input or a Linear issue)
2. Create a session file in `.agents/sessions/`
3. Break down work and delegate to specialized agents
4. Track progress and coordinate handoffs

```bash
/start-session Add user authentication with OAuth
/start-session LIN-123  # From Linear issue
```

### `/resume-session`

Resume an existing session after context loss (new conversation, compaction, etc.). Reads the session file and continues where you left off.

```bash
/resume-session 20250123-143022-auth-feature
```

### `/council-session`

Convene a council of specialized agents for complex decisions that benefit from multiple perspectives. The PM agent facilitates, agents provide input, and you make the final decision.

```bash
/council-session Should we use PostgreSQL or MongoDB for this project?
/council-session Review the authentication architecture before implementation
```

### `/review-sessions`

Review all session artifacts in `.agents/` and get hygiene recommendations. Identifies stale sessions, incomplete archives, and cleanup opportunities.

```bash
/review-sessions
```

## Skills

| Skill | Coverage |
|-------|----------|
| `orchestration` | Sessions, councils, Linear integration, Shape Up methodology |
| `foundations` | Code style, docs, security, commits |
| `python` | FastAPI, Pydantic, pytest, async |
| `typescript` | TypeScript/JavaScript (.ts/.tsx/.js/.jsx), React, Next.js, type safety |
| `ruby` | Ruby idioms, Rails 8, Hotwire, Minitest, gem dev, CLI tools, DHH/37signals philosophy |
| `go` | Go services, concurrency, testing |
| `database` | Schema design, migrations, query optimization |
| `infrastructure` | Docker, K8s, Terraform |
| `design` | Accessibility, design systems |
| `power-systems` | Grid design, power electronics, renewable integration |

## Plugin Scoping

*Claude Code only.*

Commands and agents are scoped to avoid conflicts:

```bash
/loaf:start-session          # Start a work session
/loaf:council-session        # Run a council deliberation
@loaf:backend-dev            # Reference an agent (like @files)
```

## Integrations

*Claude Code only.*

**MCP Servers** (external tool integrations):

- **Serena** - Code intelligence and semantic search
- **Sequential Thinking** - Structured reasoning and problem decomposition
- **Linear** - Issue tracking and project management

**LSP Servers** (language intelligence):

- **gopls** - Go language server
- **pyright** - Python type checking and completion
- **typescript-language-server** - TypeScript/JavaScript
- **solargraph** - Ruby language server

## Development

```bash
git clone https://github.com/levifig/loaf.git
cd loaf
npm install
npm run build
```

See [AGENTS.md](AGENTS.md) for the full development guide: structure, adding skills/agents/hooks, and quality checklist.

**Testing locally:**

- Claude Code: `/plugin marketplace add /path/to/loaf`
- Others: `./install.sh` (detects dev mode, builds first)

## License

MIT

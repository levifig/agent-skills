# Loaf

> "Why have just a slice when you can get the whole loaf?"

Loaf is *my* opinionated agentic framework (aka Levi's Opinionated Agentic Framework), optimized for Claude Code, but with support for OpenCode, Cursor, Codex, and Gemini. For Claude Code, it leverage's its full capabilities: multi-agent orchestration, MCP servers, LSP integration, pre/post-tool hooks, and session management.

Other tools (OpenCode, Cursor, Codex, Gemini) receive optimized builds of all supported features, but Claude Code is the primary target where Loaf's full feature set is available (OpenCode and Cursor are fairly close).

## Installation

### Claude Code

```
/plugin marketplace add levifig/loaf
```

Then install the `loaf` plugin via `/plugin`. No local setup needed—Claude Code fetches from GitHub automatically.

### Other Tools

OpenCode, Cursor, Codex, and Gemini receive exports via the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh | bash
```

The installer will:

1. Detect which tools you have installed
2. Let you select which targets to install
3. Download pre-built distributions from GitHub
4. Cache to `~/.local/share/loaf/`
5. Install to each selected target's config location (via individual symlinks)

**What gets installed:**

| Target | Features | Location |
|--------|----------|----------|
| OpenCode | Skills, agents, commands, hooks | `~/.config/opencode/` |
| Cursor | Skills, agents, commands, hooks | `~/.cursor/` |
| Codex | Skills only | `~/.codex/skills/` |
| Gemini | Skills only | `~/.gemini/skills/` |

Note: Only Claude Code supports the full feature set (agents, hooks, MCP/LSP servers, commands). OpenCode and Cursor are close seconds.

### Update

Run the installer again:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh | bash
```

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

**Implementation agents** (write code):

| Agent | Use For |
|-------|---------|
| `backend-dev` | Python, Ruby/Rails, Go, or TypeScript services |
| `frontend-dev` | React, Next.js, UI components |
| `devops` | Docker, Kubernetes, CI/CD |

**Advisory agents** (provide expertise in councils, delegate implementation):

| Agent | Use For |
|-------|---------|
| `dba` | Schema design, migrations, queries |
| `qa` | Testing, code review, security |
| `design` | UI/UX, accessibility |
| `power-systems` | Grid design, power electronics, renewable integration |

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

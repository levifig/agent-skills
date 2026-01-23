# Loaf - Levi's Opinionated Agentic Framework

> "Why have just a slice when you can get the whole loaf?"

An opinionated agentic framework **built for Claude Code**. Leverages Claude Code's full capabilities: multi-agent orchestration, MCP servers, LSP integration, pre/post-tool hooks, and session management.

Other tools (OpenCode, Cursor, Copilot, Codex, Gemini) receive best-effort skill exports, but Claude Code is the primary target where Loaf's full feature set is available.

## Installation

```
/plugin marketplace add levifig/loaf
```

Then install the `loaf` plugin via `/plugin`. No local setup needed—Claude Code fetches from GitHub automatically.

### Other Tools (Best-Effort)

OpenCode, Copilot, Codex, and Gemini receive skill exports via the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh | bash
```

The installer will:
1. Detect which tools you have installed
2. Let you select which targets to install
3. Download pre-built distributions from GitHub
4. Cache to `~/.local/share/loaf/`
5. Install to each selected target's config location

**What gets installed:**

| Target | Features | Location |
|--------|----------|----------|
| OpenCode | Skills, agents, commands, hooks | `~/.config/opencode/` |
| Codex | Skills only | `~/.codex/skills/` |
| Copilot | Skills only | `~/.copilot/skills/` |

Note: Only Claude Code supports the full feature set (agents, hooks, MCP/LSP servers, commands). The installer uses `~/.config/{tool}/` (XDG) if the directory exists AND the tool's config env var is set (e.g., `CODEX_HOME`).

**Cursor**: Install skills directly via Cursor's native GitHub integration—see [Cursor docs](https://cursor.com/docs/context/skills#installing-skills-from-github).

### Update

Run the installer again:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh | bash
```

For Claude Code, updates happen automatically when you use `/plugin`.

## How It Works

Loaf is designed around Claude Code's plugin architecture. The build system transforms canonical source into:

1. **Claude Code** (primary): Full plugin with agents, skills, commands, hooks, MCP servers, and LSP configs
2. **OpenCode**: Adapted format with most features
3. **Agent Skills**: Simplified skill-only exports for other tools

```
Source (src/)
    │
    ├──► Claude Code: plugins/loaf/ (full feature set)
    │
    └──► Others: dist/ (degraded feature set)
```

GitHub Actions automatically builds on every push to main.

## Agents

*Claude Code only. Other tools receive skills but not multi-agent orchestration.*

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
Task(loaf:backend-dev)       # Spawn backend developer agent
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

For contributors working on the skills themselves:

```bash
git clone https://github.com/levifig/loaf.git
cd loaf
npm install
```

### Building

```bash
npm run build              # Build all targets
npm run build:claude-code  # Claude Code plugins
npm run build:opencode     # OpenCode format
npm run build:agentskills  # Generic format (Codex, Cursor, Copilot, Gemini)
```

### Testing Your Changes

**Option 1: Use the installer (recommended)**

```bash
./install.sh
```

When run from a local clone, the installer:
1. Shows "DEVELOPMENT MODE" banner
2. Builds all targets from source
3. Outputs Claude Code plugins to repo root (`plugins/`)
4. Syncs other distributions to `~/.local/share/loaf/`
5. Shows development-specific instructions

**Option 2: Manual testing**

After `npm run build`:

- **Claude Code**: Add local marketplace
  ```
  /plugin marketplace add /path/to/loaf
  ```
- **OpenCode/Codex/Cursor/Copilot**: Copy from `dist/` to target locations

### Workflow

1. Make changes in `src/`
2. Run `npm run build`
3. Test with target tool
4. Repeat until satisfied
5. Commit and push (CI will rebuild and commit outputs)

See [AGENTS.md](AGENTS.md) for the full maintenance guide.

## Repository Structure

```
loaf/
├── src/                     # Source files
│   ├── skills/              # Domain knowledge (canonical)
│   ├── agents/              # Thin routing agents (7 total)
│   ├── commands/            # Portable commands
│   ├── hooks/               # Pre/post tool hooks
│   └── config/              # Build configuration (hooks.yaml, targets.yaml)
├── build/                   # Build system
│   ├── lib/                 # Shared utilities (sidecar.js)
│   └── targets/             # Target-specific transformers
├── plugins/                 # Claude Code marketplace (at root, committed by CI)
├── .claude-plugin/          # Claude Code marketplace manifest (at root)
├── dist/                    # Other distributions (committed by CI)
│   ├── opencode/            # OpenCode skills, agents, plugins
│   └── agentskills/         # Generic format (Codex, Cursor, Copilot, Gemini)
├── AGENTS.md                # Universal agent instructions
├── CLAUDE.md -> AGENTS.md   # Symlink for Claude Code
└── install.sh               # Installer for non-Claude Code targets
```

## License

MIT

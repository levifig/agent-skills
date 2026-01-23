# APT - Agentic Product Team

Your personal agentic product team for AI coding assistants. One source, multiple targets: Claude Code, OpenCode, and Agent Skills (Codex, Cursor, Copilot, Gemini).

**Version:** 1.7.0

## Installation

### Claude Code

Add the marketplace directly in Claude Code:

```
/plugin marketplace add levifig/agent-skills
```

Then browse and install the `apt` plugin via `/plugin`.

No local installation needed - Claude Code fetches from GitHub and handles caching automatically.

### OpenCode, Agent Skills (Codex, Cursor, Copilot, Gemini)

Run the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/agent-skills/main/install.sh | bash
```

The installer will:
1. Detect which tools you have installed
2. Let you select which targets to install
3. Download pre-built distributions from GitHub
4. Cache to `~/.local/share/agent-skills/`
5. Install to each selected target's config location

**What gets installed:**

| Target | Installation Location |
|--------|----------------------|
| OpenCode | `~/.config/opencode/{skill,agent,command,plugin}/` |
| Codex | `~/.codex/skills/` |
| Cursor | Project `.cursor/rules/` |
| Copilot | Repo `.github/copilot-instructions.md` |

### Update

Run the installer again:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/agent-skills/main/install.sh | bash
```

For Claude Code, updates happen automatically when you use `/plugin`.

## How It Works

```
Source (src/skills/, src/agents/, src/hooks/)
         │
         ▼
    npm run build
         │
         ▼
   Claude Code: plugins/, .claude-plugin/ (at repo root)
   Others: dist/ (for OpenCode, Agent Skills)
         │
         ├──► Claude Code: fetches plugins/ directly from GitHub
         │
         └──► Others: installer downloads dist/ to local cache, then installs
```

GitHub Actions automatically builds and commits `plugins/` and `dist/` on every push to main.

## Agents

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

Commands and agents are scoped to avoid conflicts:

```bash
/apt:start-session          # Start a work session
/apt:council-session        # Run a council deliberation
Task(apt:backend-dev)       # Spawn backend developer agent
```

## Integrations

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
git clone https://github.com/levifig/agent-skills.git
cd agent-skills
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
4. Syncs other distributions to `~/.local/share/agent-skills/`
5. Shows development-specific instructions

**Option 2: Manual testing**

After `npm run build`:

- **Claude Code**: Add local marketplace
  ```
  /plugin marketplace add /path/to/agent-skills
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
agent-skills/
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

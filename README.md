# Universal Agent Skills

Universal skills for AI coding assistants. One source, multiple targets: Claude Code, OpenCode, Cursor, Copilot.

## Installation

### Claude Code

Add the marketplace directly in Claude Code:

```
/plugin marketplace add github:levifig/agent-skills/dist/claude-code
```

Then browse and install plugins via `/plugin`.

No local installation needed - Claude Code fetches from GitHub and handles caching automatically.

### OpenCode, Cursor, Copilot

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
| Cursor | Instructions to copy `.cursor/rules/` to your project |
| Copilot | Instructions to copy `.github/copilot-instructions.md` to your repo |

### Update

Run the installer again:

```bash
curl -fsSL https://raw.githubusercontent.com/levifig/agent-skills/main/install.sh | bash
```

For Claude Code, updates happen automatically when you use `/plugin`.

## How It Works

```
Source (skills/, agents/, hooks/)
         │
         ▼
    npm run build
         │
         ▼
   dist/ (committed by CI)
         │
         ├──► Claude Code: fetches dist/claude-code/ directly from GitHub
         │
         └──► Others: installer downloads dist/ to local cache, then installs
```

GitHub Actions automatically builds and commits `dist/` on every push to main.

## Agents

| Agent | Use For |
|-------|---------|
| `pm` | Orchestrating work, managing sessions, delegating |
| `backend-dev` | Python, Rails, or TypeScript services |
| `frontend-dev` | React, Next.js, UI components |
| `dba` | Schema design, migrations, queries |
| `devops` | Docker, Kubernetes, CI/CD |
| `qa` | Testing, code review, security |
| `design` | UI/UX, accessibility |

## Skills

| Skill | Coverage |
|-------|----------|
| `orchestration` | Sessions, councils, Linear integration |
| `foundations` | Code style, docs, security, commits |
| `python` | FastAPI, Pydantic, pytest, async |
| `typescript` | React, Next.js, type safety |
| `rails` | Rails 8, Hotwire, Minitest |
| `infrastructure` | Docker, K8s, Terraform |
| `design` | Accessibility, design systems |

## Development

For contributors working on the skills themselves:

```bash
git clone https://github.com/levifig/agent-skills.git
cd agent-skills
npm install
npm run build        # Build all targets
npm run build:claude-code  # Build specific target
```

Running `./install.sh` from a local clone will:
1. Build all targets in the local repo
2. Sync `dist/` to `~/.local/share/agent-skills/`
3. Install to selected targets

This allows testing local changes before pushing.

See [AGENTS.md](AGENTS.md) for the full maintenance guide.

## Repository Structure

```
skills/          # Domain knowledge (canonical source)
agents/          # Thin routing agents (7 total)
commands/        # Portable commands
hooks/           # Pre/post tool hooks
config/          # Build configuration
build/           # Build system
dist/            # Built distributions (committed by CI)
  ├── claude-code/   # Claude Code marketplace plugins
  ├── opencode/      # OpenCode skills, agents, plugins
  ├── cursor/        # Cursor rules (.mdc files)
  └── copilot/       # Copilot instructions
```

## License

MIT

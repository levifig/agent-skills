# Universal Agent Skills

Universal skills for AI coding assistants. One source, multiple targets: Claude Code, OpenCode, Cursor, Copilot.

## Installation

### Claude Code

Add the marketplace directly in Claude Code:

```
/plugin marketplace add levifig/agent-skills
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
Source (src/skills/, src/agents/, src/hooks/)
         │
         ▼
    npm run build
         │
         ▼
   Claude Code: plugins/, .claude-plugin/ (at repo root)
   Others: dist/ (for OpenCode, Cursor, Copilot)
         │
         ├──► Claude Code: fetches plugins/ directly from GitHub
         │
         └──► Others: installer downloads dist/ to local cache, then installs
```

GitHub Actions automatically builds and commits `plugins/` and `dist/` on every push to main.

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
```

### Building

```bash
npm run build              # Build all targets
npm run build:claude-code  # Build specific target
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
- **OpenCode/Cursor/Copilot**: Copy from `dist/` to target locations

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
│   └── config/              # Build configuration
├── build/                   # Build system
├── plugins/                 # Claude Code marketplace (at root, committed by CI)
├── .claude-plugin/          # Claude Code marketplace manifest (at root)
├── dist/                    # Other distributions (committed by CI)
│   ├── opencode/            # OpenCode skills, agents, plugins
│   ├── cursor/              # Cursor rules (.mdc files)
│   └── copilot/             # Copilot instructions
├── AGENTS.md                # Universal agent instructions
├── CLAUDE.md -> AGENTS.md   # Symlink for Claude Code
└── install.sh               # Installer for non-Claude Code targets
```

## License

MIT

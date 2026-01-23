# Loaf Development

See [README.md](README.md) for what Loaf is and how to install it.

## Quick Start

```bash
npm install && npm run build
```

## Structure

```
src/
├── skills/{name}/SKILL.md      # Domain knowledge + references/
├── agents/{name}.md            # Thin routers (frontmatter: model, skills, tools)
├── commands/{name}.md          # Portable workflows
├── hooks/{pre,post}-tool/      # Hook scripts
└── config/
    ├── hooks.yaml              # Hook definitions, plugin-groups
    └── targets.yaml            # Target defaults, sidecars
build/targets/{target}.js       # Target transformers
```

**Output:** `plugins/` (Claude Code), `dist/{target}/` (others)

## Core Concepts

| Concept | Location | Purpose |
|---------|----------|---------|
| Skill | `src/skills/{name}/SKILL.md` | Domain knowledge, patterns, references |
| Agent | `src/agents/{name}.md` | Routes to skills, defines tools |
| Sidecar | `{name}.{target}.yaml` | Per-target frontmatter overrides |
| Hook | `src/hooks/` + `hooks.yaml` | Pre/post tool validation |
| Plugin Group | `hooks.yaml` | Bundles agents + skills + hooks |

## Common Tasks

**Add skill:** Create `src/skills/{name}/SKILL.md`, add to `plugin-groups` in `hooks.yaml`

**Add agent:** Create `src/agents/{name}.md`, add to `plugin-groups` in `hooks.yaml`

**Add hook:** Create script in `src/hooks/{pre,post}-tool/`, register in `hooks.yaml`

**Add target:** Create `build/targets/{target}.js`, add to `targets.yaml` and `build.js`

## Before Committing

- [ ] `npm run build` succeeds
- [ ] Frontmatter has required fields (name, description)
- [ ] New content registered in `hooks.yaml` plugin-groups
- [ ] No duplicate content with README

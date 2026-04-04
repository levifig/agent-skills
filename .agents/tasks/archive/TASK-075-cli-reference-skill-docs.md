---
id: TASK-075
title: CLI reference skill + update README.md / CLAUDE.md
spec: SPEC-020
status: done
priority: P2
created: '2026-04-04T16:41:22.295Z'
updated: '2026-04-04T19:34:19.992Z'
completed_at: '2026-04-04T19:34:19.991Z'
---

# TASK-075: CLI reference skill + update README.md / CLAUDE.md

Create the CLI reference skill and update project documentation.

## Scope

### CLI reference skill
Create `content/skills/cli-reference/SKILL.md`:
- Non-user-invocable reference skill (sidecar: `user-invocable: false`)
- Teaches agents when to use each `loaf` CLI command
- Uses `{{COMMAND}}` substitution for per-target command surfaces
- Covers: `loaf build`, `loaf install`, `loaf check`, `loaf session`, `loaf task`, `loaf spec`, `loaf kb`

### README.md updates
- Skill count (updated after renames/removals)
- Session journal model description
- Hooks description (enforcement vs skill instructions)
- Pipeline commands
- Multi-target table (add Amp column, Codex hooks row)
- Skill renames (`council-session` -> `council`)

### CLAUDE.md updates
- SKILL.md structural convention (Critical Rules -> Verification -> Quick Reference -> Topics)
- Verb/noun principle
- Two-tier descriptions
- Session journal vocabulary
- `loaf check` and `loaf session` commands
- Hook model changes (enforcement hooks vs skill instructions)

## Verification

- [ ] CLI reference skill exists with per-target command substitution
- [ ] Sidecar sets `user-invocable: false`
- [ ] README.md reflects all SPEC-020 changes
- [ ] CLAUDE.md reflects all SPEC-020 changes
- [ ] `loaf build` succeeds

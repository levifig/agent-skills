---
topics:
  - skills
  - agent-skills-standard
  - sidecars
  - references
  - templates
  - profiles
covers:
  - content/skills/**/*.md
  - content/skills/**/*.yaml
  - config/hooks.yaml
consumers:
  - implementer
  - reviewer
last_reviewed: '2026-04-10'
---

# Skill Architecture

Skills are the primary knowledge delivery mechanism. Each skill is a directory following the Agent Skills open standard.

## Key Rules

- **SKILL.md contains standard fields only.** `name`, `description`, `license`, `compatibility`, `metadata`. No tool-specific fields.
- **Sidecar files carry extensions.** `.claude-code.yaml` for Claude Code fields (`user-invocable`, `agent`, `context: fork`). `.opencode.yaml` for OpenCode commands. Target-specific sidecars are merged at build time.
- **Descriptions drive routing.** The model uses the description to choose from 100+ skills. Must start with action verb (third-person), include user-intent phrases, negative routing for confusable skills.
- **References are one level deep.** All references link from SKILL.md, never from other references. No nested chains.
- **Templates are structural artifacts.** `templates/` hold format templates (frontmatter schema, section headings). `references/` hold knowledge docs (conventions, patterns).
- **Command substitution.** `{{IMPLEMENT_CMD}}` and `{{ORCHESTRATE_CMD}}` placeholders are replaced per-target at build time.

## Skill Structure

```
content/skills/{name}/
├── SKILL.md                  # Standard frontmatter + content
├── SKILL.claude-code.yaml    # Claude Code extensions
├── SKILL.opencode.yaml       # OpenCode extensions (commands)
├── references/               # Knowledge docs (loaded on demand)
└── templates/                # Artifact templates (loaded on demand)
```

Skills compile to a shared intermediate at `dist/skills/` (base frontmatter, command substitution, shared templates merged), then each target reads from the intermediate.

## Session Journal Self-Logging

User-invocable workflow skills must log their invocation to the session journal as their first action. This creates an audit trail of which skills ran during a session:

```bash
loaf session log "skill(shape): shaping auth token rotation idea into spec"
loaf session log "skill(wrap): end-of-session summary"
```

The `/wrap` skill uses these entries to check whether housekeeping or other periodic skills were run.

## Shared Templates

`content/templates/` files are distributed to skills at build time via the `shared-templates` config in `targets.yaml`:

| Template | Distributed To |
|----------|---------------|
| `session.md` | implement, orchestration, housekeeping, bootstrap |
| `adr.md` | architecture, reflect |

Skill-specific templates live in `content/skills/{name}/templates/` and are not shared. SKILL.md references templates with links: `[templates/session.md](templates/session.md)`.

## Agent Profiles

SPEC-014 replaced 8 role-based agents with 3 functional profiles and 2 system profiles, defined in `SOUL.md`:

| Profile | Concept | Tool Access | Purpose |
|---------|---------|-------------|---------|
| **implementer** | Smith (Dwarf) | Full write | Code, tests, config, docs — specialty via skills |
| **reviewer** | Sentinel (Elf) | Read-only | Audits, reviews — mechanical independence |
| **researcher** | Ranger (Human) | Read + web | Research, comparison — structured reports |
| **librarian** | Librarian (Ent) | Read + Edit (.agents/) | Session lifecycle, state, wrap, pre-compaction preservation |
| **background-runner** | System | Read + Edit | Async non-blocking tasks |

Skills load into profiles at spawn time. What an agent *can do* is fixed by profile; what it *knows* comes from skills.

## Categories

| Category | `user-invocable` | Examples |
|----------|:-:|---------|
| Reference/Knowledge | `false` | python-development, database-design, foundations, git-workflow, orchestration |
| Workflow/Process | `true` (default) | implement, research, shape, breakdown, release, housekeeping, wrap |

31 skills total (as of dev.15): 15 workflow, 16 reference/knowledge.

## Cross-References

- [build-system.md](build-system.md) — how skills get distributed to targets
- [hook-system.md](hook-system.md) — how skills own hooks via `skill:` field in hooks.yaml

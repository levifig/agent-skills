# Knowledge Base

Loaf's domain knowledge — what agents need to understand about this project.

| File | Topics | Covers |
|------|--------|--------|
| [build-system.md](build-system.md) | build, targets, distribution | `cli/lib/build/**/*.ts`, `config/targets.yaml`, `config/hooks.yaml` |
| [glossary.md](glossary.md) | glossary | — |
| [hook-system.md](hook-system.md) | hooks, lifecycle, validation | `config/hooks.yaml`, `cli/commands/check.ts`, `content/hooks/**/*` |
| [knowledge-management-design.md](knowledge-management-design.md) | knowledge, staleness, qmd | `cli/lib/kb/*.ts`, `docs/knowledge/*.md` |
| [skill-architecture.md](skill-architecture.md) | skills, agent-skills-standard, sidecars | `content/skills/**/*.md`, `content/skills/**/*.yaml`, `config/hooks.yaml` |
| [task-system.md](task-system.md) | tasks, specs, shape-up, sessions | `.agents/specs/**/*.md`, `.agents/tasks/**/*.md`, `.agents/sessions/**/*.md`, `cli/commands/task.ts`, `cli/lib/session/*.ts`, `cli/lib/tasks/*.ts` |

See [../decisions/](../decisions/) for architecture decision records.

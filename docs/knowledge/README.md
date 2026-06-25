# Knowledge Base

Loaf's domain knowledge — what agents need to understand about this project.

| File | Topics | Covers |
|------|--------|--------|
| [build-system.md](build-system.md) | build, targets, distribution | `internal/cli/build*.go`, `config/targets.yaml`, `config/hooks.yaml` |
| [glossary.md](glossary.md) | glossary | — |
| [hook-system.md](hook-system.md) | hooks, lifecycle, validation | `config/hooks.yaml`, `internal/cli/check.go`, `content/hooks/**/*` |
| [knowledge-management-design.md](knowledge-management-design.md) | knowledge, staleness, qmd | `internal/cli/kb.go`, `docs/knowledge/*.md` |
| [skill-architecture.md](skill-architecture.md) | skills, agent-skills-standard, sidecars | `content/skills/**/*.md`, `content/skills/**/*.yaml`, `config/hooks.yaml` |
| [task-system.md](task-system.md) | tasks, specs, shape-up, sessions | `.agents/specs/**/*.md`, SQLite task/session state, `internal/cli/cli.go`, `internal/state/task_*.go` |

See [../decisions/](../decisions/) for architecture decision records.

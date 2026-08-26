# Knowledge Base

Loaf's domain knowledge — what agents need to understand about this project.

| File | Topics | Covers |
|------|--------|--------|
| [build-system.md](build-system.md) | build, targets, distribution | `internal/cli/build*.go`, `config/targets.yaml`, `config/hooks.yaml` |
| [glossary.md](glossary.md) | glossary | — |
| [hook-system.md](hook-system.md) | hooks, lifecycle, validation | `config/hooks.yaml`, `internal/cli/check.go`, `content/hooks/**/*` |
| [knowledge-management-design.md](knowledge-management-design.md) | knowledge, staleness, qmd | `internal/cli/kb.go`, `docs/knowledge/*.md` |
| [loaf-flow.md](loaf-flow.md) | workflow, pitch, both scales | `content/skills/{pitch,shape,triage,bootstrap,explore,brainstorm,implement,ship,release}/**` |
| [skill-architecture.md](skill-architecture.md) | skills, agent-skills-standard, sidecars | `content/skills/**/*.md`, `content/skills/**/*.yaml`, `config/hooks.yaml` |
| [task-system.md](task-system.md) | tasks, specs, changes, journal | `docs/changes/**/*.md`, `.agents/specs/**/*.md`, SQLite task state, `internal/cli/cli.go` |
| [work-model.md](work-model.md) | changes, cohorts, receipts, pipeline | `docs/changes/**/*`, `internal/cli/change_*.go`, pitch/shape/implement/triage skills |
| [cloud-attach-walkthrough.md](cloud-attach-walkthrough.md) | cloud attach, CI secrets, Cursor Cloud, Amp Orbs | `.cursor/loaf-cloud-*.sh`, `.agents/{setup,resume}`, `internal/auth/`, LOAF-67/78 |

See [../decisions/](../decisions/) for architecture decision records.

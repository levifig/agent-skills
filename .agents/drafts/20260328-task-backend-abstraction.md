---
title: "Task Backend Abstraction — .md-first filesystem, swappable backends"
source: conversation
created: 2026-03-28T22:00:00Z
status: draft
---

# Task Backend Abstraction

## Problem

Two related problems with the current task management model:

### 1. TASKS.json is a false source of truth

TASKS.json and .md frontmatter can be edited independently, creating sync conflicts. The current `loaf task sync --push` overwrites .md frontmatter from TASKS.json, which:
- Reverts direct .md edits (observed: spec status reset from `implementing` to `drafting`)
- Strips frontmatter fields TASKS.json doesn't model (observed: `files`, `depends_on`, `verify`, `done` lost)
- Forces an unintuitive workflow: edit .md → import → push, when editing .md should just work

The .md files contain everything: structured frontmatter + rich body content. TASKS.json is a derived index that claims to be the source of truth.

### 2. No backend abstraction

Every CLI command and workflow skill hardcodes TASKS.json reads/writes. Users who prefer Linear for task management must maintain both systems. There's no way to swap the backend.

## Desired Outcome

1. **.md files are canonical.** Task/spec .md files are the source of truth for the filesystem backend. TASKS.json becomes a derived cache — generated on demand, never authoritative.
2. **Backends are swappable.** A user can choose filesystem (.md files) or Linear (MCP-backed) as their task backend. CLI commands and skills work transparently against whichever is configured.

## Shape So Far

### Part 1: .md-first filesystem (the foundation)

Invert the authority model. The .md frontmatter is always the source of truth for the filesystem backend.

**What changes:**
- CLI commands (`loaf task update`, `create`, etc.) write .md frontmatter directly
- TASKS.json is rebuilt from .md files on demand (already works via `buildIndexFromFiles()`)
- `loaf task sync --push` goes away — nothing overwrites .md
- `loaf task sync` = rebuild index from files (what `--import` already does)
- `next_id` derived by scanning filenames for highest `TASK-NNN` prefix
- Unknown frontmatter fields preserved naturally (nobody overwrites them)

**TASKS.json becomes a build artifact:**
- Generated on demand for tools that want a single JSON payload (API responses, CI, web UIs)
- Never read as authoritative by CLI commands
- Can be gitignored or treated as a cache file

**Can a web kanban board work on .md files directly?** Yes. The frontmatter has everything: `status` (columns), `priority` (lanes), `spec` (grouping), `title`, `created`, `updated`. Dragging a card = writing one frontmatter field. A lightweight server parses with `gray-matter` and renders. No JSON intermediary needed.

### Part 2: Backend interface (the abstraction)

```typescript
interface TaskBackend {
  list(filter?: TaskFilter): Promise<TaskEntry[]>
  get(id: string): Promise<TaskEntry | null>
  create(task: NewTask): Promise<TaskEntry>
  update(id: string, changes: Partial<TaskEntry>): Promise<TaskEntry>
  archive(id: string): Promise<void>
}
```

Two implementations:
- `FilesystemBackend` — reads/writes .md frontmatter directly (Part 1)
- `LinearBackend` — wraps Linear MCP tool calls or API

### Configuration

Backend choice in `.agents/loaf.json` or similar project config:

```json
{
  "tasks": {
    "backend": "filesystem"
  }
}
```

### What Changes

| Component | Current | Part 1 (.md-first) | Part 2 (abstraction) |
|-----------|---------|---------------------|----------------------|
| `loaf task *` commands | Direct TASKS.json | Direct .md frontmatter | `TaskBackend` interface |
| `loaf task sync` | Bidirectional push/import | Rebuild cache from .md | N/A (backends handle their own storage) |
| TASKS.json | Source of truth | Derived cache | Backend-specific detail |
| `loaf spec archive` | Reads task index | Reads .md files | `TaskBackend.list()` |
| `loaf housekeeping` (SPEC-012) | Scans TASKS.json | Scans .md files | `TaskBackend.list()` |
| `/breakdown` skill | Writes TASKS.json | Writes .md files | Backend-agnostic |
| `/implement` skill | Reads from index | Reads .md files | Backend-agnostic |

### Open Questions

- Does the spec backend also need abstracting, or are specs always filesystem?
- How does `loaf task sync` work with Linear backend? (Becomes an import/export tool between backends?)
- What's the migration path for existing TASKS.json users? (Likely: one-time `--push` to ensure .md files are current, then stop reading TASKS.json)
- Does LinearBackend require MCP tools at CLI level, or does it use the Linear API directly?
- Performance: is parsing .md files on every `loaf task list` fast enough? (Likely yes at project scale — dozens of files, not thousands)

### Implementation Order

1. **Part 1 first** — invert to .md-first within the existing filesystem model. This fixes the immediate sync problems and is valuable standalone.
2. **Part 2 after** — extract the `TaskBackend` interface once Part 1 stabilizes the filesystem implementation.
3. SPEC-012's `loaf housekeeping` should be built against .md files directly where possible, TASKS.json as fallback until Part 1 lands.

### Dependencies

- Part 1 could ship independently as a small-medium spec
- Part 2 depends on Part 1 (the filesystem backend IS Part 1)
- Both are orthogonal to SPEC-012 and SPEC-014

## What This Is Not

- Not a rewrite of task management — the CLI surface stays the same
- Not adding new task features — just fixing the storage model
- Not deprecating .md files — they become MORE important
- Not removing TASKS.json — it becomes a derived cache, still useful for tooling

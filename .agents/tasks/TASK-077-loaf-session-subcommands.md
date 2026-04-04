---
id: TASK-077
title: Implement loaf session subcommands (start/end/log/archive)
spec: SPEC-020
status: todo
priority: P0
created: '2026-04-04T16:41:22.296Z'
updated: '2026-04-04T16:41:22.296Z'
---

# TASK-077: Implement `loaf session` subcommands (start/end/log/archive)

New CLI command group for the session journal model.

## Scope

Create `cli/commands/session.ts` with four subcommands:

### `loaf session start`
1. Detect current git branch
2. Find linked spec (branch name -> spec frontmatter `branch:` field)
3. Find/create session file in `.agents/sessions/`
4. Compute state: task completion, recent commits, branch status
5. Append `resume` entry with computed state
6. Output last 15-20 journal entries as context
7. Create ad-hoc session for branches without a linked spec

### `loaf session end`
1. Append `pause` entry with progress summary (commits, tasks completed)
2. Inject prompt for final `decide`/`conclude`/`todo` entries
3. Update `last_entry` timestamp in frontmatter

### `loaf session log`
1. Receive entry text as argument: `loaf session log "decide(hooks): remove bash wrappers"`
2. Validate `type(scope): description` format
3. Append timestamped entry to current branch's session file
4. `--from-hook` flag: parse stdin JSON, extract IDs (commit SHA, PR number)

### `loaf session archive`
1. Move session file to `.agents/sessions/archive/`
2. Set status to `archived` in frontmatter
3. Extract key decisions (`decide` entries)

## Constraints

- Session = branch scope. One branch = one session file
- Atomic appends (`>>` with single write) for concurrency safety
- Read-only agents CAN write journal entries via `loaf session log` (Bash command)
- Journal format matches spec: `## YYYY-MM-DD HH:MM` headers, `- type(scope): description` entries

## Verification

- [ ] `loaf session start` creates session file, appends resume, outputs context
- [ ] `loaf session start` links to spec when branch matches
- [ ] `loaf session start` creates ad-hoc session for unlinked branches
- [ ] `loaf session end` appends pause with progress
- [ ] `loaf session log "decide(hooks): test"` appends valid entry
- [ ] `loaf session log --from-hook` parses stdin JSON correctly
- [ ] `loaf session archive` moves to archive/, sets status
- [ ] `npm run typecheck` and `npm run test` pass

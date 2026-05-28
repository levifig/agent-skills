---
captured: 2026-05-01T17:50:00Z
status: raw
---

# `/loaf:breakdown` should auto-commit spec, cut branch, commit tasks

## Idea

Extend `/loaf:breakdown` so that, after task generation completes, it:

1. Commits spec frontmatter changes (status → `implementing`, `branch:` field, Open Questions resolutions) on `main` with a generated message like `chore: SPEC-NNN → implementing`.
2. Creates the feature branch declared in the spec frontmatter (`feat/<slug>`) from that commit.
3. Commits the new task files + `TASKS.json` delta on the branch with a generated message like `chore(SPEC-NNN): breakdown — N tasks`.
4. Returns the user to the branch, ready for `/loaf:implement TASK-NNN`.

## Context

Surfaced during SPEC-024 breakdown (2026-05-01). Memory rule `feedback_branch_at_breakdown` says branching happens at breakdown, not implementation, with spec on main and tasks on the branch. The skill currently writes the branch name into spec frontmatter but does not actually create the branch or split commits — that work falls on the orchestrator at the end of the breakdown session.

Doing it manually each time is fine but introduces avoidable bookkeeping at the start of every implementation session. Encoding it in the skill is mechanical and removes a class of "did I split the commits right?" errors.

## Open Questions

- Should commits run automatically, or behind a `--commit` flag (default off) for safety?
- What happens if the working tree is dirty with unrelated changes when breakdown is invoked? (Refuse and ask user to clean? Commit only the spec/task paths explicitly?)
- Should this respect `feedback_always_confirm_push` and stop at local commits, never pushing? (Yes — pushing remains a separate explicit step.)
- How does this interact with Linear-native mode where there are no local task files to commit on the branch?

## Related

- Memory: `feedback_branch_at_breakdown`, `feedback_always_confirm_push`, `feedback_branch_in_spec_frontmatter`
- Skill: `loaf:breakdown`
- Surfaced in: SPEC-024 breakdown session (`.agents/sessions/20260430-190531-session.md`)

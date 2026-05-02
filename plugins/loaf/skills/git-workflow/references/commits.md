# Commit Conventions

## Contents
- Commit Message Format
- Commit Body
- Linear Integration
- Branch Naming
- Pull Request Format
- Changelog Discipline
- Critical Rules
- Workflow Enforcement Hooks
- Semantic Versioning

Git commit, branch, and pull request standards.

## Commit Message Format

```
<type>: <description>

[optional body]

[optional footer]
```

### Types

| Type | Use For | Version Impact |
|------|---------|----------------|
| `feat` | New features | Minor bump |
| `fix` | Bug fixes | Patch bump |
| `refactor` | Code restructuring | None |
| `perf` | Performance improvements | Patch bump |
| `test` | Test additions/updates | None |
| `docs` | Documentation only | None |
| `chore` | Maintenance, deps, config | None |
| `ci` | CI/CD changes | None |
| `build` | Build system changes | None |

### Description Rules

- **Imperative mood**: "add feature" not "added feature"
- **Lowercase**: Start with lowercase after type
- **No period**: Don't end with a period
- **Short**: Under 72 characters
- **Focus on why**: The diff shows what

### Examples

```bash
# Good
feat: add thermal rating calculation
fix: prevent divide by zero in sag calculation
refactor: extract common validation logic

# Bad
feat: Added thermal rating calculation.  # Past tense, period
fix: Fixed the bug  # Vague, past tense
refactor: refactored code  # Redundant, past tense
```

## Commit Body

Add a body when:
- The "why" isn't obvious from title
- Trade-offs need documenting
- Implementation needs context

```
feat: add CIGRE TB 601 thermal model

Implement steady-state heat balance calculation per CIGRE TB 601.
Uses Newton-Raphson iteration for temperature convergence.

Key implementation notes:
- Natural convection below 0.5 m/s wind speed
- Film temperature for air property evaluation
- Tolerance: 0.1C for convergence
```

### What to Avoid in Body

- File lists (the diff shows this)
- Detailed code explanation
- Agent attribution
- Verbose descriptions

## Linear Integration

Use magic words in footer to link/close issues:

```
feat: add thermal rating API endpoint

Implement GET /api/towers/{id}/thermal-rating endpoint.

Closes BACK-123
```

### Keywords

| Keyword | Effect | Use For |
|---------|--------|---------|
| `Closes BACK-XXX` | Auto-closes on merge | Features, tasks |
| `Fixes BACK-XXX` | Auto-closes on merge | Bug fixes |
| `Resolves BACK-XXX` | Auto-closes on merge | Alternative |
| `Refs BACK-XXX` | Reference only | Related work |
| `Part of BACK-XXX` | Reference only | Partial work |

## Branch Naming

```
<type>/<description>
<type>/TASK-123-description
```

### Types

- `feat/` - New features (e.g., `feat/spec-010-task-management-cli`)
- `fix/` - Bug fixes
- `hotfix/` - Critical production fixes
- `release/` - Release preparation
- `chore/` - Maintenance, refactoring

### Rules

- Lowercase with hyphens (kebab-case)
- Short but descriptive (max 50 chars)
- Include spec or task slug when applicable (e.g., `feat/spec-010-task-management-cli`)

## Pull Request Format

### Title

Same format as commit messages (GitHub appends `(#N)` automatically on squash merge):

```
feat: add thermal rating calculation
```

### Description

Focus on **review context** — what changed, why, and how to test. Do not include squash merge commit text in the PR body.

```markdown
## Summary

Brief description of what this PR adds/changes and why.

- Bullet points covering key changes
- Focus on what a reviewer needs to know

## Test plan

- [ ] Unit tests added/updated
- [ ] Integration tests pass
- [ ] Manual testing performed

## Related Issues

Closes BACK-123
Refs BACK-124
```

### Merge Strategy

- **Prefer squash merge** unless explicitly told otherwise
- GitHub defaults the merge title to `PR title (#N)` — this is the desired format
- **Write a clean extended description** for the squash merge commit — a one-line summary followed by bullet points grouped by feature area
- **Never use the automatic squash description** that dumps all individual commit messages — it's noisy and unhelpful in git history
- Don't push or merge without explicit request
- The `/loaf:release` skill automates this workflow, including version bump on the feature branch before merge — use it when ready to squash merge a PR

## Changelog Discipline

`CHANGELOG.md` entries describe what shipped from a user's or operator's perspective — not how the work was tracked or organized internally. Curate the `[Unreleased]` section before bumping the version so the published release notes read as user-facing prose, not an internal worklog.

### Drop

Internal terms that have no meaning outside the team's working context:

- Spec IDs and task IDs (`SPEC-024`, `TASK-042`)
- Session, sprint, or branch references
- Internal terminology from skills/docs that isn't part of the user's mental model — e.g. `Q1`/`Q2`/`Q3` question numbers from a Triage Gate, internal gate-logic notation like `(Q1 OR Q2) AND Q3`, hook IDs that aren't user-facing
- "How the work got done" framing — interview steps, breakdown phases, review gates

### Keep

- **What changed** — commands, behaviors, file paths, config keys, hook names the user works with
- **Why it matters** — compatibility implications, migration notes, breaking-change call-outs
- **Public references** — `ADR-NNN` IDs, documented CLI flags, public file paths, named features

### Style

- Use backticks for all code references — file names, commands, config keys, hook names
- Write user-perspective prose: what shipped, not how it got built
- Group entries by category (`Added`, `Changed`, `Fixed`, `Removed`) per Keep a Changelog convention

### Auto-generated Entries

When `loaf release --pre-merge` auto-generates the `[Unreleased]` section from commit history, those entries inherit any internal terms present in the commit messages. Treat the generated output as a draft: rewrite it under the curated path before bumping. The release skill preserves curated content when it's already in `[Unreleased]` — curate first, bump second.

## Critical Rules

### Always

- Write atomic commits (one logical change)
- Commit complete units of work — finish the change, review it, then commit once
- Use imperative mood in messages
- Reference issue numbers when applicable
- Test before committing

### Never

- Commit partial or in-progress work — if feedback is likely, wait for it before committing
- Skip commit signing (wait for user if it fails)
- Push without explicit user confirmation
- Use scoped commits (`feat(scope):` - use `feat:` instead)
- Include file lists in message
- Add agent attribution
- Mix unrelated changes
- Commit secrets or sensitive data
- Put SPEC or TASK IDs in commit subject (use human-readable names)

### ID References

- **IDs belong in footer, not subject line**
  - Bad: `feat: implement SPEC-002 invisible sessions`
  - Good: `feat: implement invisible sessions and task board`
- Use descriptive names that are understandable without looking up IDs
- Linear issue IDs go in footer only (e.g., `Closes BACK-123`)

## Semantic Versioning

```
MAJOR.MINOR.PATCH[-PRERELEASE]

1.0.0 -> 1.0.1 (patch: bug fixes)
1.0.1 -> 1.1.0 (minor: new features)
1.1.0 -> 2.0.0 (major: breaking changes)
```

## Workflow Enforcement Hooks

Three hooks automatically enforce the conventions documented in this file:

| Hook | Phase | Behavior |
|------|-------|----------|
| `workflow-pre-pr` | Pre-tool (Bash) | Advisory: reminds about CHANGELOG [Unreleased] entries and PR format. Non-blocking. |
| `workflow-pre-push` | Pre-tool (Bash) | Advisory: reminders on `git push` — branch naming, uncommitted files, force-push safety. Non-blocking. |
| `workflow-post-merge` | Post-tool (Bash) | Advisory: injects housekeeping checklist after successful `gh pr merge`. Non-blocking. |

These hooks read instruction templates from `hooks/instructions/` and run automatically when the corresponding git/gh commands are invoked.

Breaking changes use `feat!:` or `fix!:` and include:

```
BREAKING CHANGE: Description of breaking change.
```

### Pre-Release Versions

When developing toward a major version, use pre-release suffixes to mark development milestones:

```
2.0.0-dev.0  →  dev.1  →  dev.2  →  ...  →  2.0.0
     ↑              ↑          ↑                 ↑
  start dev    milestone  milestone        stable release
```

| Suffix | Meaning | When to use |
|--------|---------|-------------|
| `-dev.N` | Development milestone | Active development toward a target version |
| `-alpha.N` | Alpha pre-release | Feature-complete but untested broadly |
| `-beta.N` | Beta pre-release | Testing with wider audience |
| `-rc.N` | Release candidate | Final validation before stable |

**Convention:**
- Set the target version with `-dev.0` when starting a major effort (e.g. `2.0.0-dev.0`)
- Bump the dev counter (`-dev.N` → `-dev.N+1`) when a meaningful batch of work ships — a spec completing, a group of related features landing
- Don't bump for every commit — that's what git history is for
- Strip the suffix (`-dev.N` → `2.0.0`) when all planned work is complete
- `loaf release` handles all bump types: `prerelease`, `release`, `major`, `minor`, `patch`

**Not required** — projects using simple `MAJOR.MINOR.PATCH` versioning can ignore pre-release suffixes entirely. This convention is for projects with multi-milestone development cycles.

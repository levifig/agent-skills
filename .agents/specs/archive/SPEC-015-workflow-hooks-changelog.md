---
id: SPEC-015
title: Workflow enforcement hooks + CHANGELOG management
source: direct
created: '2026-03-27T13:32:06.000Z'
status: complete
---

# SPEC-015: Workflow enforcement hooks + CHANGELOG management

## Problem Statement

Loaf's PR conventions, merge strategy, and post-merge housekeeping steps are documented in skills (foundations/references/commits.md, implement SKILL.md) and memory, but nothing enforces them at execution time. The agent may follow them, may not — it depends on whether the right reference was loaded. This has already caused missed steps in practice: version bumps forgotten, CHANGELOG not updated, PR descriptions not following format.

Additionally, there is no project-level CHANGELOG.md. Changes are tracked only via micro-changelogs at the bottom of individual docs, which don't provide a project-wide view of what shipped.

## Strategic Alignment

- **Vision:** "Hooks automate what skills teach" — this is a textbook application. The conventions exist in skills; hooks enforce them deterministically.
- **Architecture:** Script-based hooks that conditionally inject instructions are the right mechanism — they filter on specific commands and output checklists, matching existing hook patterns.
- **Principle:** "Maintenance as side effect" — CHANGELOG updates happen as part of the natural workflow, not as a separate chore.

## Solution Direction

### Workflow Hooks

Three new script-based hooks that conditionally inject instructions into the agent's context at critical workflow moments. Each hook consists of two files:

- **A bash script** — reads JSON stdin, checks if the command matches, outputs instruction text if relevant
- **A markdown file** — the instructions to inject (PR format, housekeeping checklist, etc.)

The script uses the hook library (`content/hooks/lib/json-parser.sh`) to parse the command from stdin. If the command doesn't match, the script exits silently. This matches how existing hooks like `validate-push` and `validate-commit` work.

**Exit code strategy varies by hook:**
- **Pre-PR:** Conditionally blocking (exit 2 if CHANGELOG entry missing, exit 0 with format instructions if present)
- **Post-merge:** Non-blocking (exit 0, advisory checklist)
- **Pre-push:** Non-blocking (exit 0, advisory warnings)

**Library extensions required:** The hook library (`json-parser.sh`) needs two new functions: `parse_command` (extracts `tool_input.command` from JSON) and `parse_exit_code` (extracts exit code from PostToolUse `tool_result`). These are small additions to the existing library.

#### 1. Pre-PR Hook (PreToolUse on Bash)

**Script matches:** Commands containing `gh pr create`

**Behavior:** Conditionally blocking. The script checks project state before deciding:

- **If CHANGELOG.md is missing or `[Unreleased]` has no entries:** Block (exit 2). Output the full checklist to stderr: "Before creating this PR, complete these steps: 1) Add CHANGELOG entry under [Unreleased], 2) Verify PR title format, 3) Verify PR body format. Then re-run `gh pr create`." The agent does the work and re-runs.
- **If CHANGELOG.md has `[Unreleased]` entries:** Pass through (exit 0). Output PR format instructions to stdout as a reminder (title format, body format, merge strategy). The PR proceeds.

**Instructions include:**
- PR title format (conventional commit style, <70 chars)
- PR body format (Summary + Test Plan, no squash commit text)
- Merge strategy reminder (squash merge, clean extended description)
- CHANGELOG entry format (Keep a Changelog categories, one line per change)
- If CHANGELOG.md doesn't exist: "Create it with the Keep a Changelog header first"
- Source: relevant sections from `foundations/references/commits.md`

**Why blocking:** PreToolUse hooks fire before the tool executes. With exit 0, the PR is created before the agent can act on the instructions — the CHANGELOG entry can't be added "before creating the PR" if the PR is already created. Blocking ensures the agent completes the checklist first.

#### 2. Post-Merge Hook (PostToolUse on Bash)

**Script matches:** Commands containing `gh pr merge`
**Condition:** Only outputs instructions if the merge command succeeded (checks exit code from tool result JSON). If the merge failed, exits silently — no housekeeping for a failed merge.

**Injects the housekeeping checklist:**
1. Switch to main, pull (handle rebase if needed)
2. Mark tasks done (`loaf task update TASK-XXX --status done`)
3. Mark spec done if all tasks complete
4. Finalize CHANGELOG entry — move items from `[Unreleased]` to versioned section if bumping
5. Bump version in `package.json` (patch for fix, minor for feat, or dev bump)
6. Rebuild all targets (`loaf build`)
7. Archive session file (status: complete, archived_at, archived_by)
8. Commit housekeeping (`chore: bump to X.Y.Z, close TASK-XXX session`)
9. Delete merged feature branch

#### 3. Pre-Push Hook (PreToolUse on Bash)

**Script matches:** Commands containing `git push`

**Injects:**
- Branch naming validation reminder (`<type>/<description>`)
- Force-push warning for main/master branches
- Reminder to check for uncommitted housekeeping files

Version bump is integrated into the post-merge checklist (step 5) rather than a separate hook. The post-merge instructions include guidance on which bump type to use based on the conventional commit type in the merge commit.

### CHANGELOG.md Management

**Format:** [Keep a Changelog](https://keepachangelog.com/)

**Location:** `CHANGELOG.md` at project root

**Agent workflow:**
1. **During PR creation** (triggered by pre-PR hook): Agent drafts an entry in the `[Unreleased]` section, categorized as Added/Changed/Fixed/Removed/Deprecated/Security. Entry is part of the PR diff and reviewed with the code.
2. **During post-merge housekeeping** (triggered by post-merge hook): If a version bump happens, agent moves `[Unreleased]` entries into a new versioned section with the date.

**Category mapping from conventional commits:**
| Commit Type | Changelog Category |
|-------------|-------------------|
| `feat` | Added |
| `fix` | Fixed |
| `refactor`, `perf` | Changed |
| `docs` | Changed (if user-facing) or skip |
| `chore`, `ci`, `build`, `test` | Skip (unless notable) |
| Breaking changes | header note: "BREAKING CHANGES" |

**Entry format:**
```markdown
## [Unreleased]

### Added
- /bootstrap skill for 0-to-1 project setup (SPEC-013)
- `loaf setup` CLI command wrapping init+build+install

### Fixed
- Setup command rejects non-directory paths before chdir
- Scaffold writes guarded against symlink escape outside project boundary
```

**Versioned section format:**
```markdown
## [2.0.0-dev.2] - 2026-03-27

### Added
- /bootstrap skill for 0-to-1 project setup (SPEC-013)
...
```

**Agent creates, human curates:** The agent drafts entries from the commit message/PR description. The human reviews and may edit for clarity, combine entries, or adjust categories. The agent should not produce verbose entries — one line per meaningful change, written for someone who wants to know what shipped.

### Hook Implementation

Script-based hooks using the existing Claude Code plugin hook system. Each hook has a bash script (command filter) and a markdown file (instruction content). The script uses `content/hooks/lib/json-parser.sh` for JSON parsing.

**Hook file structure:**
```
content/hooks/pre-tool/
  workflow-pre-pr.sh            # Conditional blocker: checks CHANGELOG, blocks or passes
  workflow-pre-push.sh          # Non-blocking: branch naming + push safety
content/hooks/post-tool/
  workflow-post-merge.sh        # Non-blocking: housekeeping checklist (success-gated)
content/hooks/instructions/
  pre-pr-checklist.md           # Full checklist (CHANGELOG + PR format) — shown when blocking
  pre-pr-format.md              # PR format reminder only — shown when passing through
  post-merge.md                 # Housekeeping checklist
  pre-push.md                   # Branch naming + push safety
```

**Pre-PR script pattern (conditional blocking):**
```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/json-parser.sh"

INPUT=$(cat)
COMMAND=$(parse_command "$INPUT")
INSTRUCTIONS="$(dirname "$0")/../instructions"

case "$COMMAND" in
  *"gh pr create"*) ;;
  *) exit 0 ;;
esac

# Check if CHANGELOG.md has [Unreleased] entries
if [[ -f "CHANGELOG.md" ]] && \
   sed -n '/^## \[Unreleased\]/,/^## \[/p' CHANGELOG.md | grep -q "^### "; then
  # Entries exist — pass through with format reminder
  cat "$INSTRUCTIONS/pre-pr-format.md"
  exit 0
else
  # Missing or empty — block until CHANGELOG is updated
  cat "$INSTRUCTIONS/pre-pr-checklist.md" >&2
  exit 2
fi
```

**Post-merge script pattern (non-blocking, success-gated):**
```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/json-parser.sh"

INPUT=$(cat)
COMMAND=$(parse_command "$INPUT")

case "$COMMAND" in
  *"gh pr merge"*) ;;
  *) exit 0 ;;
esac

EXIT_CODE=$(parse_exit_code "$INPUT")
[[ "$EXIT_CODE" != "0" ]] && exit 0   # merge failed, skip housekeeping

cat "$(dirname "$0")/../instructions/post-merge.md"
exit 0
```

**Pre-push script pattern (non-blocking):**
```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/json-parser.sh"

INPUT=$(cat)
COMMAND=$(parse_command "$INPUT")

case "$COMMAND" in
  *"git push"*) ;;
  *) exit 0 ;;
esac

# Detect branch and flags
BRANCH=$(git branch --show-current 2>/dev/null || echo "unknown")
FORCE_PUSH=false
[[ "$COMMAND" == *"--force"* || "$COMMAND" == *"-f"* ]] && FORCE_PUSH=true

cat "$(dirname "$0")/../instructions/pre-push.md"

# Extra warnings
if [[ "$BRANCH" == "main" || "$BRANCH" == "master" ]] && $FORCE_PUSH; then
  echo "WARNING: Force-pushing to $BRANCH. This rewrites shared history." >&2
fi

exit 0
```

**Registration in hooks.yaml:**
```yaml
pre-tool:
  - id: workflow-pre-pr
    skill: foundations
    script: hooks/pre-tool/workflow-pre-pr.sh
    matcher: "Bash"
    blocking: true              # Required — enables exit 2 for missing CHANGELOG
    timeout: 5000
    description: "Enforce PR format and CHANGELOG entry before gh pr create"

  - id: workflow-pre-push
    skill: foundations
    script: hooks/pre-tool/workflow-pre-push.sh
    matcher: "Bash"
    blocking: false
    timeout: 5000
    description: "Inject branch naming and push safety reminders before git push"

post-tool:
  - id: workflow-post-merge
    skill: foundations
    script: hooks/post-tool/workflow-post-merge.sh
    matcher: "Bash"
    timeout: 5000
    description: "Inject post-merge housekeeping checklist after successful gh pr merge"
```

**Note:** The `blocking: true` on `workflow-pre-pr` is critical — without it, exit 2 is treated as exit 0 and the CHANGELOG enforcement is bypassed.

### Distributable

These hooks ship with Loaf as part of the foundations plugin-group. Every Loaf-equipped project gets workflow enforcement. Projects can opt out by not including the foundations group.

## Scope

### In Scope
- Pre-PR workflow hook — conditional blocker (PR format, merge strategy, CHANGELOG enforcement)
- Post-merge workflow hook — advisory (full housekeeping checklist including version bump)
- Pre-push workflow hook — advisory (branch naming, force-push warning)
- CHANGELOG.md in Keep a Changelog format
- Agent-authored changelog entries at PR creation and post-merge
- Category mapping from conventional commit types
- Hook library extensions (`parse_command`, `parse_exit_code` in `json-parser.sh`)
- Hook registration in hooks.yaml
- Build integration for all targets
- Update foundations skill references to document the hooks

### Out of Scope
- Migrating existing validation hooks (check-secrets, validate-commit, validate-push) to the new workflow pattern
- Auto-generating CHANGELOG from git log (agent reads commits and writes entries, not a tool)
- Enforcing version bump rules (remind and guide, not block)
- CHANGELOG for non-Loaf projects (the format and hooks are distributable, but seeding the initial CHANGELOG.md is left to `/bootstrap` or manual creation)
- Micro-changelog format changes (per-doc changelogs continue as-is)

### Rabbit Holes
- Building a `loaf changelog` CLI command — the agent handles it, no CLI needed
- Complex command parsing in hooks beyond simple substring matching — keep the `case` patterns simple
- Trying to auto-determine version bump type from commit analysis — let the agent read the commit and decide, with the human confirming
- Supporting multiple CHANGELOG formats — pick one (Keep a Changelog) and commit

### No-Gos
- Don't block indefinitely — pre-PR blocks once for missing CHANGELOG, passes through when entries exist. Post-merge and pre-push never block.
- Don't auto-push or auto-merge — hooks inject instructions, the agent still needs human confirmation for destructive actions
- Don't replace existing validation hooks — new workflow hooks coexist with check-secrets, validate-commit, validate-push

## Dependencies

| Dependency | Type | Status | Notes |
|------------|------|--------|-------|
| Claude Code plugin hook system | Required | Available | PreToolUse / PostToolUse with Bash matcher |
| Hook library (`content/hooks/lib/`) | Required — needs extensions | Implemented | `json-parser.sh` needs `parse_command` and `parse_exit_code` functions |
| foundations/references/commits.md | Referenced | Implemented | PR format and merge strategy source material |
| implement SKILL.md AFTER phase | Referenced | Implemented | Housekeeping checklist source material |

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Hook scripts add latency to Bash calls | Low | Low | Scripts exit immediately on non-matching commands; only matching commands incur the `cat` overhead |
| Agent ignores injected instructions | Low | Medium | Instructions are explicit checklists; if consistently ignored, escalate to blocking script hooks |
| CHANGELOG entries become noisy or inconsistent | Medium | Low | "Agent creates, human curates" — review catches bad entries; conventions documented |
| Injected instruction text is too long, wastes context | Low | Medium | Keep instruction markdown concise; reference external files rather than embedding full docs |

## Open Questions

- [ ] Should the pre-PR hook also check that tests pass before allowing PR creation, or leave that to CI?
- [ ] Verify the exact JSON schema for PostToolUse hook input — specifically the field path for the Bash exit code (likely `tool_result.exit_code` or `tool_result.returncode` — confirm during implementation by inspecting a PostToolUse hook's stdin)
- [ ] Should the pre-PR CHANGELOG check be smarter (e.g., verify entries relate to current branch changes) or is "any entries under [Unreleased]" sufficient?
- [x] ~~Should CHANGELOG.md be seeded by `/bootstrap` for new projects?~~ Yes — as a future update to `/bootstrap` (SPEC-013). For now, the pre-PR hook handles missing CHANGELOG.md by blocking until it's created.
- [x] ~~Does the Claude Code plugin API support `type: prompt` hooks?~~ Resolved: use script-based hooks that conditionally output instruction markdown. Matches existing patterns.

## Test Conditions

### Pre-PR Hook
- [ ] Hook script only fires when Bash command contains `gh pr create`
- [ ] Hook script exits silently for non-matching commands (no context waste)
- [ ] When CHANGELOG.md is missing or `[Unreleased]` is empty: hook blocks (exit 2) with full checklist
- [ ] When CHANGELOG.md has `[Unreleased]` entries: hook passes (exit 0) with format reminder
- [ ] Agent adds CHANGELOG entry and re-runs `gh pr create` successfully after block
- [ ] If CHANGELOG.md doesn't exist, agent creates it with Keep a Changelog header before re-running

### Post-Merge Hook
- [ ] Hook script only outputs instructions when Bash command contains `gh pr merge`
- [ ] Hook script checks exit code — only injects on successful merge
- [ ] Agent follows the full housekeeping checklist (pull, mark done, changelog, version bump, rebuild, archive, commit)
- [ ] Version bump guidance matches the commit type (feat→minor, fix→patch)

### Pre-Push Hook
- [ ] Hook script only outputs instructions when Bash command contains `git push`
- [ ] Agent warns on force-push to main
- [ ] Agent validates branch naming convention

### CHANGELOG Management
- [ ] CHANGELOG.md follows Keep a Changelog format
- [ ] Entries are categorized correctly (Added/Changed/Fixed/etc.)
- [ ] `[Unreleased]` section accumulates entries across PRs
- [ ] Version bump moves `[Unreleased]` entries to a versioned section with date
- [ ] Agent writes concise, one-line-per-change entries (not verbose)

### Distribution
- [ ] Hooks build for Claude Code target
- [ ] Hooks appear in built plugin output
- [ ] Projects using foundations plugin-group get the workflow hooks

## Circuit Breaker

**At 50% appetite:** Ship pre-PR and post-merge hooks only (the two most valuable). Skip pre-push. CHANGELOG.md management as part of post-merge hook, not a separate concern.

**At 75% appetite:** Add pre-push hook and polish CHANGELOG entry quality. Skip distribution testing for non-Claude Code targets.

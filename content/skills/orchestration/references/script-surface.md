# Script Surface

## Contents
- Current Efficiency
- CLI Migration Rule
- Candidate Commands
- Keep as Skill-Local Scripts

Orchestration still carries useful helper scripts, but several now duplicate
behavior that has become core Loaf runtime behavior. Prefer CLI commands when
the operation is shared across skills, appears in hooks, mutates `.agents/`
state, or needs stable tests and error messages.

## Current Efficiency

The orchestration skill is split into a compact `SKILL.md` plus references,
which is directionally efficient for routing. The inefficient part is the
script surface:

- The source currently has 13 orchestration scripts out of 25 skill-local
  scripts overall.
- Several scripts overlap existing `loaf session`, `loaf task`, `loaf check`,
  and Linear-aware behavior.
- Shell/Python helpers are harder to discover than `loaf <noun> <verb>` and
  are not consistently covered by CLI tests.

Keep scripts only when they are narrow examples or skill-local adapters.

## CLI Migration Rule

Move a script into the Loaf CLI when at least two are true:

- It creates, validates, archives, or mutates `.agents/` state.
- It is referenced by hooks or another skill.
- It needs stable JSON output or machine-readable exit codes.
- It duplicates logic already present in `cli/`.
- Users would reasonably try `loaf <noun> <verb>` before locating a script.

Leave a script in the skill when it is one-off glue, an example transcript
transform, or depends on harness-only context unavailable to the CLI.

## Candidate Commands

| Current Script | Candidate CLI | Priority | Rationale |
|----------------|---------------|----------|-----------|
| `validate-session.py` | `loaf session validate <file>` | High | Session schema is core state, already managed by `loaf session`. |
| `new-session.sh` | `loaf session start` / `loaf session create` | High | Session creation already belongs to the CLI; avoid duplicate templates. |
| `validate-council.py` | `loaf council validate <file>` | High | Council lifecycle should not depend on direct script execution. |
| `new-council.sh` | `loaf council create` | High | Creates first-class `.agents/councils/` artifacts. |
| `check-linear-format.py` | `loaf linear check-format` | Medium | Linear hygiene should be reusable outside orchestration. |
| `format-progress.sh` | `loaf linear format-progress` | Medium | Useful user-facing formatter with simple inputs. |
| `extract-magic-words.sh` | `loaf linear refs` | Medium | Git-to-Linear reference extraction is runtime behavior. |
| `git-context-summary.sh` | `loaf session context git` | Medium | Similar context already powers session hooks. |
| `get-config.py` | `loaf config get` | Medium | Configuration lookup is cross-skill. |
| `suggest-team.py` | `loaf linear suggest-team` | Low | Needs clearer Linear-native contract before promotion. |

## Keep as Skill-Local Scripts

| Script | Reason |
|--------|--------|
| `extract-decisions.py` | Transitional Serena-memory helper; not a core Loaf storage contract. |
| `list-session-decisions.sh` | Serena-specific discovery helper; MCP access is the real interface. |
| `validate-roadmap.py` | Planning-reference utility; promote only if roadmap artifacts become first-class. |

## Before creating this PR, complete these steps:

### 1. CHANGELOG entry

Add an entry under `## [Unreleased]` in `CHANGELOG.md`, categorized by Common Changelog impact:

| Commit type | Category |
|-------------|----------|
| `feat` | Added |
| `fix` | Fixed |
| `refactor`, `perf` | Changed |
| `docs` (user-facing) | Changed |
| `chore`, `ci`, `test` | Skip unless notable |

One line per meaningful change. Write imperative, self-describing,
release-facing prose; include public references when available, and do not
include internal spec/task IDs.

```markdown
### Added

- Add bootstrap guidance for 0-to-1 project setup
```

If `CHANGELOG.md` does not exist, create it first:

```markdown
# Changelog

This project follows [Common Changelog](https://common-changelog.org/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). `## [Unreleased]`
is a workflow staging section for curated entries that land with PRs before a later release.

## [Unreleased]

### Added
- Your entry here
```

### 2. PR title

Conventional commit format, under 70 characters:

```
feat: add thermal rating calculation
fix: prevent divide by zero in sag calculation
```

No scope prefixes. No SPEC/TASK IDs in the title.

### 3. PR body

Build the body from the live canonical tracker record and the observed branch diff. Through the selected `project-management/v1` provider skill and harness-native connection, re-read the native reference, problem, definition of done, exclusions, relationships, and current status. Include the native reference, a concise change summary, criterion-by-criterion evidence, verification actually run, and remaining risk.

Do not render or synchronize a Loaf-local work record. If the tracker connection or a required read capability is unavailable, stop and report the connection-specific gap rather than inventing or reconstructing canonical work state. After creating the PR, read the PR body back and confirm that it matches the live work contract and observed diff.

### 4. Merge strategy

Squash merge. Write a clean extended description (2-4 lines summarizing the branch). Never use the auto-generated squash description.

---

Complete these steps, then re-run `gh pr create` with the prepared body.

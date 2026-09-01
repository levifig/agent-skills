## Before creating this PR, complete these steps:

### 1. Shipping unit

Confirm that the branch represents exactly one shippable root. Related stacked children must have verified ancestry and be assembled into the root with `git merge --ff-only`; independent shippable roots need separate PRs unless an explicit human decision records why one atomic landing is safer.

### 2. CHANGELOG entry

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

### 3. PR title

Conventional commit format, under 70 characters:

```
feat: add thermal rating calculation
fix: prevent divide by zero in sag calculation
```

No scope prefixes. No SPEC/TASK IDs in the title.

### 4. PR body

The body is `loaf issue render <ref>` output. No project headers, no hand-edited summary. Checkboxes stay unchecked until `loaf issue status <ref> done`.

```
gh pr create --title "type: summary" --body "$(loaf issue render <ref>)"
```

### 5. Merge strategy

Squash merge this one shippable unit. Write a clean extended description (2-4 lines summarizing the outcome). Never use the auto-generated squash description or create a merge commit merely to preserve feature-branch topology.

---

Complete these steps, then re-run `gh pr create`.

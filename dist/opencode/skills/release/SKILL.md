---
name: release
description: >-
  Cuts a retroactive release from already-landed issues: loaf release suggest
  reports the range, loaf release cut records the version. Use when the user
  says "cut a release," "publish a version," "release from main," or asks what
  landed since the last tag. Produces a recorded release row and members as
  facts. Not for reviewing or merging a PR (use ship).
version: 0.3.0
---

# Release

Cut a version from work that has already landed.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Process
- Attribution
- Bump derivation
- Must-contain convention
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

1. **Log invocation first** — `loaf journal log "skill(release): <what is being cut or suggested>"` before doing anything else.
2. **Release is not merge** — do not review, approve, or land a PR here. Verification authority is the ship workflow (PR review and CI at merge). If the user is asking to merge, stop and route to ship.
3. **A release is cut from what landed** — the surface is `loaf release suggest` and `loaf release cut`. Do not run unsubcommmanded `loaf release`, `--pre-merge`, or `--post-merge`; this skill does not own that path.
4. **Suggest writes nothing** — it reads `baseline-tag..HEAD` (or `--base <ref>..HEAD`), attributes commits to issues, rolls up through parents, reports partially-landed parents and unattributed commits as information, derives the bump, reports the advisory bucket delta, and drafts notes.
5. **Cut records facts** — it applies the version, prepends the drafted notes into `CHANGELOG.md`, tags, records the release row plus members, then attempts a GitHub Release draft. A `gh` failure degrades to a warning with a paste-ready retry command; the recorded row stays.
6. **No forward version stamp** — do not bind an issue to a future version. Members are what already landed. Buckets (`loaf issue bucket`) are advisory labels; planned-vs-landed is information only.
7. **No suite, no re-record, no publication stop in this skill** — ship already verified the merged work. Cut's operational refusals (dirty worktree, disagreeing version files, missing version, `--no-tag` without an existing tag) are command errors, not a substitute for ship.
8. **Confirm before cut** — present the suggest report (or `cut --dry-run`) first. Ask one question at a time, with a recommendation, using your harness's structured question tool if it has one. `--dry-run` previews everything and writes nothing.
9. **Log the outcome** — after a successful cut, `loaf journal log "decision(release): vX.Y.Z recorded from <base> with <summary>"`.

---

## Verification

- Journal contains the `skill(release)` invocation (and a `decision(release)` entry after a real cut)
- The work in the range already landed through ship (PR review and CI at merge); this skill did not re-verify or re-merge it
- `loaf release suggest` (or `cut --dry-run`) was shown: landed issues, partially-landed parents, unattributed commits, advisory buckets, derived bump, drafted notes
- Partially-landed parents, unattributed commits, and bucket drift were reported as information — not treated as a cut refusal
- Mutating `loaf release cut` updated version files, wrote the notes into `CHANGELOG.md`, created or reused tag `v<version>`, and recorded the release row with issue members (plus `--includes` release members when given)
- `cut --dry-run` left version files, changelog, tags, HEAD, and release rows untouched
- GitHub Release is a draft, was skipped with `--no-gh`, or failed with a warning plus a paste-ready `gh release create …` retry — never a silent rollback of the recorded row
- No issue was stamped with a future version

---

## Quick Reference

| Harness | Invoke skill |
|---------|----------------|
| Claude Code (plugin) | `/loaf:release` |
| OpenCode, Cursor, Codex, Amp | `/release` |

### Commands

```text
loaf release suggest [--base <ref>] [--json]
loaf release cut [--base <ref>] [--bump <type>] [--includes <version|tag>] [--no-tag] [--no-gh] [--dry-run]
loaf issue bucket <ref> now|next|later|none [--json]
loaf issue link <from> blocks|relates-to <to> [--json]
```

Both commands need initialized SQLite state (`loaf state init`, or `loaf state migrate markdown --apply`). They are not a git repository's optional extra — without SQLite they refuse.

| Command | Writes? | What it does |
|---------|---------|----------------|
| `loaf release suggest` | No | Report landed work since the last version tag |
| `loaf release cut` | Yes (unless `--dry-run`) | Cut the retroactive release and record members as facts |
| `loaf release cut --dry-run` | No | Print the plan, including `--includes` rows, and write nothing |

### `suggest` flags

| Flag | Meaning |
|------|---------|
| `--base <ref>` | Commits since `<ref>` instead of the last tag |
| `--json` | Machine-readable suggestion |
| `-h`, `--help` | Help |

`suggest` rejects `--dry-run` (it is already read-only) and rejects cut-only flags (`--bump`, `--includes`, `--no-tag`, `--no-gh`).

### `cut` flags

| Flag | Meaning |
|------|---------|
| `--base <ref>` | Commits since `<ref>` instead of the last tag |
| `--bump <type>` | Override the derived bump: `major`, `minor`, `patch`, `prerelease`, `release` |
| `--includes <version\|tag>` | Record a prior release as a member (repeatable). Use this to hang prerelease references on a stable |
| `--no-tag` | Do not create a git tag; tag `v<version>` must already exist |
| `--no-gh` | Skip the GitHub Release draft |
| `--dry-run` | Print the plan and write nothing |
| `-h`, `--help` | Help |

`cut` rejects `--json`. `--bump prerelease` and `--bump release` only produce a version when the current version already has a prerelease suffix; otherwise cut fails with `could not compute a version to cut`. `--no-tag` is checked before `--dry-run`: the tag must already exist even for a preview.

### Cut sequence (mutating)

1. Recompute the same suggestion as `suggest` (then apply `--bump` if given, and redraft notes)
2. Resolve each `--includes` ref to an existing release
3. Require a clean worktree
4. Apply the version to detected version files (they must exist and agree)
5. Prepend drafted notes into `CHANGELOG.md` (after `[Unreleased]`, ahead of prior versions; creates the file if missing)
6. Commit `chore: release vX.Y.Z`
7. Unless `--no-tag`: create annotated tag `vX.Y.Z` (`git tag -a`). Signing follows git config (`tag.gpgSign`); cut never passes `-s` or `--no-sign`
8. Record the release row, issue members, and `--includes` members as facts
9. Unless `--no-gh`: `gh release create <tag> --draft --title <tag> --notes <notes>` (adds `--prerelease` when the version is a prerelease). Switches to the configured GitHub account first. On `gh` missing, account failure, or create failure: print `warning:` plus a POSIX-quoted `retry:` command; do not fail the cut

Cut does not push the commit or the tag.

---

## Topics

| Topic | Use When |
|-------|----------|
| [Process](#process) | Running suggest then cut in this conversation |
| [Attribution](#attribution) | How commits become issue members |
| [Bump derivation](#bump-derivation) | Why suggest picked major, minor, or patch |
| [Must-contain convention](#must-contain-convention) | The rare promise that named issues must land first |

---

## Process

Parse `$ARGUMENTS` for a base, bump, version, `--includes`, `--no-tag`, `--no-gh`, or `--dry-run`. Default baseline is the last version tag; `--base` overrides. With neither a last tag nor `--base`, the range is all of `HEAD`.

### Step 1: Log and route

```bash
loaf journal log "skill(release): <what is being cut or suggested>"
```

If the user wants a PR reviewed or merged, stop and use ship. If the work is still on a feature branch, explain that a release is cut from landed `HEAD` since the baseline, and offer ship.

### Step 2: Suggest

```bash
loaf release suggest
# or
loaf release suggest --base <ref>
loaf release suggest --json
```

Present the report as-is: base, suggested bump and version, bump evidence, landed issues with commits, partially-landed parents (missing children), unattributed commits, advisory buckets (planned landed / planned not landed / unplanned landed), drafted notes.

Do not hide partial parents or unattributed commits, and do not refuse the cut because of them unless the operator is using the [must-contain convention](#must-contain-convention) and wants to wait.

### Step 3: Confirm

Show the exact `loaf release cut …` you would run. Recommend cutting the derived version when the landed set matches what the operator asked for. Use `--bump` only when they override. Use `--dry-run` when they want a preview:

```bash
loaf release cut --dry-run
loaf release cut --dry-run --no-gh
loaf release cut --dry-run --includes <version|tag>
```

### Step 4: Cut

```bash
loaf release cut
loaf release cut --bump minor
loaf release cut --includes v1.1.0-alpha.1
loaf release cut --no-tag --no-gh
```

On success, report version files updated, changelog written, tag created or reused, release recorded (member count), and GitHub draft created / skipped / warned. If stderr has `retry:`, paste that command; the row is already recorded.

```bash
loaf journal log "decision(release): vX.Y.Z recorded from <base> with <summary>"
```

### Step 5: After

Suggest reflect when the cut produced durable product or workflow learnings, and housekeeping when temporary artifacts need cleanup. Capture leftover discoveries as issues or sparks — not as extra changelog lines.

---

## Attribution

`suggest` (and `cut`, which recomputes the same suggestion) attributes each commit in the range to zero or more issues, then rolls up through parents.

**Commit → issue**, first match wins:

1. Issue alias (`PREFIX-N`, e.g. `LOAF-42`) in the subject or body (prefix case-sensitive). URLs and code spans are stripped first.
2. Else the merge/branch rung: aliases in a `Merge …` subject, plus any alias anywhere in the body (case-insensitive). Squash subjects like `feat: add auth (#42)` often carry the alias only in the body. No network, no `gh`. An alias that lived only on a deleted branch name is unattributable.
3. Else a unique journal `commit(<hash>)` row whose message contains an alias (scope uniquely matching that commit).

Resolved aliases become **landed** issues (with the commits that named them). Commits that match nothing are **unattributed** — listed, and included under drafted notes as `### Unattributed`. They do not block the cut.

**Parent rollup** (information):

- For each landed issue that has a parent, if any sibling child is not `done`, the parent is **partially landed** and the missing children are listed.
- A parent is not auto-added to landed unless a commit attributed to it.

Drafted notes are `## [version] - YYYY-MM-DD`, then one `### ALIAS — title` section per landed issue with commit subjects, then unattributed.

---

## Bump derivation

Derived from the range, in order:

| Condition | Bump |
|-----------|------|
| Breaking marker (`type!:` in the subject, or `BREAKING CHANGE:` / `BREAKING-CHANGE:` in the body) | `major` |
| A **done** parent with **two or more** children, every child `done` and landed, and the parent's done timestamp **after** the baseline tag's committer time | `minor` (closed multi-child parent fully landed) |
| Else a conventional `feat` commit | `minor` |
| Else | `patch` (`fix` / other) |

`--bump` on `cut` replaces the derived bump and redrafts notes; the evidence string becomes `overridden by --bump <type>`.

Current version comes from agreeing version files, else from a semver last tag. Cut still requires version files on the mutating path.

---

## Must-contain convention

Rare. When the operator needs a named set of issues to land before a cut, create a **release-prep** issue and express `blocked_by` edges with issue mechanics:

```bash
loaf issue new "Release prep for vX.Y.Z" --body "Must contain LOAF-12 and LOAF-15. Out of scope: the cut itself."
loaf issue link LOAF-12 blocks LOAF-99
loaf issue link LOAF-15 blocks LOAF-99
```

Stored write types are `blocks` and `relates_to`. `loaf issue link <must-land> blocks <release-prep>` is how you record that the prep issue is blocked by those that must land. `loaf issue frontier` and implement honor `blocks`. **`loaf release suggest` and `cut` do not read these edges** — convention, not schema. If the operator wants to wait, wait; if they cut anyway, cut records whatever actually landed.

Buckets stay labels:

```bash
loaf issue bucket LOAF-12 now
loaf issue bucket LOAF-12 none
```

`suggest` prints `bucket:<name> ALIAS — title (landed|not landed)` and `unplanned ALIAS — title (landed)`. Never treat that delta as a constraint.

---

## Related Skills

- **ship** — Reviews, verifies, and lands a PR. That merge is the verification authority for what this skill may later cut
- **git-workflow** — Branching, PRs, and any later push of the local tag (cut does not push)
- **documentation-standards** — Changelog prose if a human edits notes after the cut
- **reflect** — Durable learnings after a cut
- **housekeeping** — Cleanup of temporary artifacts

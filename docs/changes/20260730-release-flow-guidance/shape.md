<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Release Flow Guidance — the CLI Names the Sanctioned Door

## Problem

The v2.0.0-alpha.16 cut (2026-07-30) veered from the project's release convention because nothing at the decision point named it. `loaf release` supports the sanctioned two-phase flow (`--pre-merge` on a release branch → PR squash into one `chore: release vX (#PR)` commit → `--post-merge` on the default branch), but it equally permits single-shot `--bump <type> --yes` directly on the default branch, and the release skill presents the PR flow as conditional on branch protection. A model operator on an unprotected `main` takes the direct door and every subsequent guardrail is a symptom of the wrong mode: the clean-worktree refusal forces changelog curation into a stray `docs:` commit, the GitHub-release step fails because the tag was created before any push, `--post-merge` then refuses with `guardrail 7 failed: tag … already exists locally — run git tag -d … and rerun` (advice that is destructive once the tag is pushed), and the capability-evidence canary surfaces only in tag CI because nothing ran the suite against the prepared tree. Journal: `decision(release)` and `spark(release)` entries 2026-07-30.

## Hypothesis

When the CLI itself names the sanctioned flow at the two decision points that matter — a mutating invocation on the default branch, and each guardrail refusal — a model operator self-corrects mid-ceremony, and making the skill's release-PR flow unconditional removes the wrong door from the instructions entirely.

## Scope

**In**

- Contextual flow advisory: mutating `loaf release` modes (interactive or `--bump`) invoked on the repository default branch print a model-facing advisory before acting — naming the release-branch + `--pre-merge` → PR squash → `--post-merge` sequence and the reason (PR CI runs the suite against the prepared tree, so evidence canaries surface before tagging). Advisory only, never blocking; suppressed when running `--pre-merge`, `--post-merge`, or any read-only mode (`--dry-run`, help).
- Guardrail remediation text: the clean-worktree refusal states that changelog curation belongs on the release branch in the `--pre-merge` flow; the `--post-merge` tag-exists guardrail distinguishes an unpushed local tag (safe to `git tag -d`) from a pushed tag or existing GitHub release (name the non-destructive repair instead of advising deletion).
- Release skill: Step 5 rewritten so the release-PR flow is the default for every release regardless of branch protection; direct `--bump` on the default branch demoted to a named exception used only on explicit user request; Step 4 reframed to route through the PR flow.
- Rebuilt distribution targets committed with the source change.

**Out** (deferred, not rejected)

- Any blocking enforcement of the PR flow — a config knob or hard refusal is a later decision if the advisory proves insufficient.
- Decoupling capability evidence from binary rebuilds (`INTENT-20260719-decouple-installed-smoke-evidence-from-binary-rebuilds`).
- Running the Go suite inside `loaf release` after artifact refresh — candidate follow-up recorded as `spark(release)` 2026-07-30; interacts with release latency and belongs with the promotion-model ceremony work.
- Promotion-model surfaces (`docs/changes/20260728-release-promotion-model/`, PR #143).

## Observable Workflow

```
# on main, direct mutating invocation:
loaf release --bump prerelease
# advisory: releases are prepared on a release branch: loaf release --pre-merge there,
# squash-merge the release PR, then loaf release --post-merge here; PR CI verifies the
# prepared tree so evidence canaries surface before tagging. Proceeding directly skips that.

# post-merge with a pushed tag already present:
loaf release --post-merge
# guardrail: tag v2.0.0-alpha.16 already exists and is pushed — do not delete a published
# tag; if the GitHub release is missing assets, re-run the Release workflow or recreate the
# release from the existing tag instead.
```

## Rabbit Holes and No-Gos

- **Do not block the direct mode.** Consumers of Loaf may legitimately release single-shot; the advisory informs, the operator decides.
- **Do not detect the default branch via the network.** Resolve from local git state (`refs/remotes/origin/HEAD`, falling back to `main`/`master` heuristics); a release ceremony must not add a network dependency to print advice.
- **Do not reword unrelated guardrails** — only the two named surfaces change; the guardrail numbering and semantics stay.

## Decisions

1. **Advisory, not enforcement.** The incident was a routing error under ambiguous instructions, not a malicious bypass; naming the door is the proportionate fix, and the skill rewrite removes the ambiguity for skill-following operators.
2. **The CLI carries the convention, the skill defers to it.** The two-phase flags already encode the flow; the skill's job is routing, so its Step 5 stops conditioning the PR flow on branch protection.
3. **Tag-exists advice must be state-aware.** `git tag -d` on a pushed tag is destructive advice; the guardrail checks whether the tag exists on the remote before prescribing deletion.

## Planning Contract

Advisory emission: a helper resolves the repository default branch from local refs and reports whether the current invocation is a mutating release mode on that branch outside the two-phase flags; the release run prints the advisory block to the command's writer before the analysis phase when it is. Guardrail text: the clean-worktree refusal and the post-merge tag-exists guardrail gain the wordings pinned in Observable Workflow, with the tag-exists branch consulting the remote tag state guarded so a network failure degrades to the current wording, never an error.

## Implementation Units

- **TASK-001 — CLI advisory and guardrail remediation.** Default-branch resolution helper, advisory emission in mutating modes, the two guardrail wordings, unit tests.
- **TASK-002 — Skill rewrite and distribution rebuild.** Release skill Steps 4–5 reframed around the PR-flow default, direct mode as named exception; `loaf build`; rebuilt `dist/` and `plugins/` committed.

## Verification Contract

- **V1.** Advisory prints for mutating modes on the default branch and never in `--pre-merge`, `--post-merge`, or `--dry-run`. Command: `go test ./internal/cli -run 'TestReleaseFlowAdvisory' -count=1`. Expect: exit 0.
- **V2.** Guardrail wordings: clean-worktree names the pre-merge flow; tag-exists distinguishes pushed from unpushed tags and never advises deleting a pushed tag. Command: `go test ./internal/cli -run 'TestReleaseGuardrailRemediation' -count=1`. Expect: exit 0.
- **V3.** Distribution targets rebuild cleanly with the skill change. Command: `bin/loaf build`. Expect: exit 0.
- **V4.** The full suite is green. Command: `go test ./...`. Expect: exit 0.

<!-- Human review (H-tier): review material, never gate input. -->

- **H1.** A transcript of `loaf release --bump prerelease` on main reads as one clear advisory naming the two-phase sequence, not a wall of text.

## Definition of Done

- All V-entries green at HEAD via `loaf change verify` with the receipt committed.
- `loaf change check` reports zero violations and the change derives executable.
- The release skill contains no branch-protection conditional around the release-PR flow.

## Durable Outputs

- Release skill Steps 4–5 as the lasting statement of the release flow.
- Journal `decision(release-flow-guidance)` entries for the three decisions above.

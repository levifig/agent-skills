---
change: release-flow-guidance
id: TASK-001
title: CLI advisory and guardrail remediation
blocks:
  - TASK-002
---

# TASK-001 — CLI advisory and guardrail remediation

## Objective

Mutating `loaf release` invocations (interactive or `--bump`) on the repository default branch print a model-facing advisory before analysis: releases are prepared on a release branch with `loaf release --pre-merge`, squash-merged via a release PR, then finalized with `loaf release --post-merge` — because PR CI runs the suite against the prepared tree, so evidence canaries surface before tagging. The advisory never prints for `--pre-merge`, `--post-merge`, `--dry-run`, or help. Two guardrails gain state-aware remediation: the clean-worktree refusal names the pre-merge flow as where changelog curation belongs, and the post-merge tag-exists guardrail distinguishes an unpushed local tag (safe to delete) from a pushed tag (never advise deletion; name the non-destructive repair).

## Scope boundaries

**In:** Release command surface in `internal/cli` (advisory emission, default-branch resolution from local refs only, the two guardrail wordings), unit tests.

**Out:** Any blocking behavior; guardrail renumbering; skill prose (TASK-002); network-dependent default-branch detection.

## Context pointers

- Contract: `shape.md` — Scope → In; Decisions 1–3; Planning Contract; Observable Workflow (pinned wordings).
- Incident: journal `decision(release)` / `spark(release)` 2026-07-30; the alpha.16 release transcript.
- Default-branch resolution: local `refs/remotes/origin/HEAD`, falling back to `main` then `master`; no network calls.
- Remote-tag state for the guardrail: consult the remote guardedly — on failure degrade to the current wording, never error.

## Steps

- [ ] Add the default-branch resolution helper (local refs only) and the invocation-context predicate (mutating mode ∧ default branch ∧ not pre/post-merge).
- [ ] Emit the advisory block before analysis in matching invocations; keep it one short paragraph.
- [ ] Reword the clean-worktree refusal to name the pre-merge flow.
- [ ] Make the post-merge tag-exists guardrail state-aware (pushed vs unpushed) with the pinned wordings.
- [ ] Tests: `TestReleaseFlowAdvisory` (prints on default-branch mutating runs; absent in --pre-merge/--post-merge/--dry-run) and `TestReleaseGuardrailRemediation` (both wordings; pushed tag never gets deletion advice).

## Verification

- `go test ./internal/cli -run 'TestReleaseFlowAdvisory' -count=1` green.
- `go test ./internal/cli -run 'TestReleaseGuardrailRemediation' -count=1` green.

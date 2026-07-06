---
change: hook-provenance-scoping
created: 2026-07-06
branch: hook-provenance-scoping
---

<!-- Frontmatter must open the file at byte one — parsers depend on it. No status-like frontmatter (readiness/status/state): readiness is derived — a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Hook Provenance Scoping

## Problem

PR #91 made the Change workflow dogfoodable, but the generated Claude plugin hook
surface still trusts the host to honor `if: "Bash(git push:*)"` on blocking
pre-tool hooks. If Claude Code ignores that condition, push-only checks such as
`render-drift` and `ephemeral-provenance` can run on unrelated Bash commands and
block normal agent work.

## Hypothesis

Push-only checks should self-scope inside `loaf check`, not only in generated
hook metadata. If the host fires a hook too broadly, Loaf should inspect the
incoming hook payload and pass unless the tool call is a Bash `git push`.

## Scope

**In**

- Native self-scoping for `render-drift` and `ephemeral-provenance`.
- Tests proving non-push Bash hook invocations pass even when the repository
  contains data that would fail the push check.
- Verification that manual `loaf check --hook ...` still runs the checks without
  requiring a hook payload.

**Out** (deferred, not rejected)

- Full hook-surface re-evaluation remains a later Change.
- Shape-skill rewrite and spec-guidance sweep remain separate Changes.

**Cut** (explicitly rejected)

- No new hook condition language.
- No dependency on Claude Code fixing or documenting `if` behavior.
- No broad rewrite of generated hook JSON.

## Observable Workflow

If a generated Claude hook invokes:

```bash
loaf check --hook ephemeral-provenance
```

while the actual Bash command is `git status`, the check passes immediately. The
same check still runs and blocks when invoked manually without hook context, or
when the hook context is a Bash `git push`.

## Rabbit Holes and No-Gos

- Do not make every hook self-scope in this Change. `validate-push`,
  `workflow-pre-pr`, and `validate-commit` already inspect command context.
- Do not change advisory/blocking semantics.
- Do not treat SQLite as an evidence archive; this Change has no research
  artifact worth committing.

## Decisions

Provenance: discovered while preparing `v2.0.0-alpha.4` dogfooding after PR #91
landed; anchored by the project journal's `hook-provenance-scoping` next-step
entry and inspection of generated `plugins/loaf/hooks/hooks.json`.

1. **Self-scope only the push-only checks that currently lack self-scoping.**
   `render-drift` and `ephemeral-provenance` run because of a push condition in
   hook metadata but do not inspect the hook payload themselves. The fix belongs
   in their check functions.
2. **Manual checks still run.** Empty hook context means a human or script called
   `loaf check --hook ...` directly; that path remains authoritative.

## Planning Contract

### Approach

Add a shared helper that says whether a push-scoped check should run:

- Empty context: run, preserving manual checks.
- Non-Bash context: pass.
- Bash context whose command is not `git push`: pass.
- Bash context whose command starts with `git push`: run the check.

Wire that helper into `runNativeRenderDrift` and
`runNativeEphemeralProvenance`. Leave generated hook metadata unchanged for now;
the host condition remains an optimization, not the safety boundary.

## Implementation Units

<!-- In-document work packets — commit-boundary guides and review anchors, not tracked entities. -->

- **U1 - Check self-scoping.** Add hook-context gating to the two push-only
  checks that lacked it.
- **U2 - Regression tests.** Prove non-push Bash context passes despite failing
  repository state, and manual empty-context checks still block.
- **U3 - Release readiness.** Run native tests/build, regenerate distributable
  outputs if needed, ship this Change, then cut `v2.0.0-alpha.4`.

## Verification Contract

<!-- Executable (machine-checkable): -->

- **V1.** `go test ./internal/cli` passes, including regressions for non-push
  hook context and manual checks.
- **V2.** `go test ./...` and `go vet ./...` pass.
- **V3.** `go build -o /tmp/loaf-scratch ./cmd/loaf` passes.
- **V4.** `/tmp/loaf-scratch change check --require-executable
  docs/changes/20260706-hook-provenance-scoping` passes.

<!-- Human review: -->

- **H1.** The fix does not weaken push-time enforcement; it only prevents
  push-only checks from running on non-push hook invocations.

## Definition of Done

- Native check behavior is scoped correctly.
- The Change package checks clean.
- PR is merged before the alpha.4 release is cut.

## Durable Outputs

None. This is a small product fix and dogfood slice, not a durable model update.

## Open Questions

- Should generated Claude plugin hooks omit unsupported `if` fields entirely?
  Leave for the full hook-surface re-evaluation.

## Source Inputs

- Journal entry, 2026-07-06 16:38: `Next: loaf change init
  hook-provenance-scoping (Change #2, H3 proof, alpha.4 vehicle)`.
- Generated Claude plugin hook surface:
  `plugins/loaf/hooks/hooks.json`.
- Prior Change: `docs/changes/20260704-shape-first-change-workflow/`.

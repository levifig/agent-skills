---
change: install-upgrade-split
id: TASK-005
title: Harness drift surfacing (doctor + session start)
blocked-by:
  - TASK-002
---

# TASK-005 — Harness drift surfacing (doctor + session start)

Cuttable unit (shape.md Decision 4): sever if the core split runs long. Cutting is not silent — the Definition of Done requires a named follow-up (Intent or brief) captured before merge, and H2 plus the drift DoD line transfer to it.

## Objective

Stale harness content becomes visible without anyone remembering to look: a doctor check compares each installed harness's `.loaf-version` marker against the binary version, and the SessionStart context nudge lands in the shared emitter behind the target-specific `--from-hook` dispatch variants — Claude, Cursor, and Codex each invoke their own form (see `journal.go` dispatch and the `build_claude_code.go` / `build_cursor.go` / `build_codex.go` hook commands), and each maps to its own config dir through the same target table install stamps markers with. Marker semantics follow shape.md › Planning Contract › Drift marker semantics: equal → silent; older → nudge/flag; missing or unparseable → SessionStart silent, doctor reports unknown; newer than the binary → doctor flags the binary as the stale side and points at the channel command.

## Scope boundaries

**In:** One new check in `doctor.go` (report-only; its remediation string names `loaf upgrade`), the nudge line in the `--from-hook` context output path, tests for stale/current/missing-marker cases.

**Out:** Auto-fixing drift from doctor (remediation is running `loaf upgrade` yourself), any new hook registration — the SessionStart hook already exists, and subagent invocations stay silent.

## Context pointers

- Contract: `shape.md` — Decision 4, Observable Workflow (final paragraph).
- Existing surfaces: `doctor.go` check table (`doctorChecks()`), journal context hook output (`journal.go` / the `--from-hook` path), `.loaf-version` stamping in `install_target.go:897`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — harness drift surfacing"
# Read: internal/cli/doctor.go (checkFencedContent as the pattern), the journal context --from-hook emitter
```

## Steps

- [x] Doctor check: enumerate installed harness config dirs, compare markers to the binary version per the marker-semantics contract, report stale ones with the `loaf upgrade` remediation and newer-than-binary ones with the channel command.
- [x] Nudge line: resolve the invoking harness's config dir from the dispatch variant, apply the same comparison, append the one-liner in the shared emitter so all three harness variants carry it; equal/missing/unparseable stay silent at session start.
- [x] Tests: stale, current, missing, unparseable, and newer-marker cases for doctor; per-variant (claude/cursor/codex) output tests for the SessionStart nudge; no-installed-harness case for both surfaces.

## Verification

- Hand-edit a harness `.loaf-version` to an older version: `loaf doctor` flags it; `loaf journal context --from-hook` carries the nudge; restoring the marker silences both.
- `go test ./...` green.

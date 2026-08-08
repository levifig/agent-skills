---
change: state-dedupe
id: TASK-003
title: Doctor alias-parity diagnostic
blocks:
  - TASK-004
---

# TASK-003 — Doctor alias-parity diagnostic

## Objective

`loaf state doctor` reports alias parity: for every project and each of the six entity tables, raw row counts vs alias-reachable counts, plus dangling-alias counts — so a future identity fork is detected the day it happens instead of discovered by accident at housekeeping.

## Scope boundaries

**In:** The `loaf state doctor` surface (`internal/cli/cli.go:2368-2450` and the state-layer inspection it calls); tests.

**Out:** Any repair behavior — the diagnostic is read-only and never gains a `--fix` for this damage class (the migration from TASK-001 is the repair); `loaf doctor` (harness content files, unrelated); housekeeping scanner internals.

## Context pointers

- Contract: `shape.md` — Planning Contract (Approach), Decision 2
- Divergence mechanics: `internal/state/housekeeping.go:104-132` (raw count query) vs `internal/state/task_list.go:63-85` (alias INNER JOIN) — parity means these two shapes agree

## Acquisition

```bash
loaf journal log "skill(implement): TASK-003 — doctor alias-parity diagnostic"
export LOAF_DB="$(mktemp -d)/loaf.sqlite"
```

## Steps

- [x] Add an alias-parity section to `loaf state doctor` output (human and JSON): per project, per entity table — raw count, alias-reachable count, orphan delta, dangling aliases
- [x] Green state is exact parity and zero dangling aliases; any delta renders as a finding that names `loaf state migrate alias-orphans` as the repair
- [x] Tests (`TestStateDoctorAliasParity*`): parity on a clean fixture; orphan and dangling-alias fixtures produce the finding with correct counts; diagnostic performs no writes

## Verification

- `go test ./... -run 'AliasParity' -count=1` exits 0
- `go test ./...` exits 0

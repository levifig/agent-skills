---
change: change-work-model
id: TASK-026
title: Receipts record every atom even on conflict
---

# TASK-026 — Receipts record every atom even on conflict

## Objective

Close C5-4: the `ExitConflict` branch in `evaluateChangeExpectation` returns only the conflict check, so declared `contains` atoms vanish from the receipt. After this task the conflict is recorded alongside, never instead of, every other declared atom's result.

## Scope boundaries

**In:** `evaluateChangeExpectation` in `internal/cli/change_verify.go`; fixtures in `internal/cli/change_verify_test.go`.

**Out:** The conflict-fails-the-criterion rule (settled, TASK-023). Grammar, digest, advisory handling (unchanged). The do-not-touch list: `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`.

## Context pointers

- Round-5 board C5-4; TASK-019's receipt contract: "the receipt records each atom with its outcome."

## Acquisition

```bash
loaf journal log "skill(implement): TASK-026 — receipts record every atom"
```

## Steps

- [x] On conflict: the `exit-conflict` check (ok false) is recorded together with every `contains` atom's actual result; the criterion's `ok` stays false regardless of the other atoms.
- [x] Fixture: `Expect: exit 1 and contains \`sentinel\` and exit 0` against output containing the sentinel records the conflict false, the contains true, criterion false; the sentinel-absent variant records contains false.

## Verification

- `go test ./internal/cli/ -run 'ChangeVerify|ChangeExpect' -count=1` green including both conflict-plus-contains fixtures.

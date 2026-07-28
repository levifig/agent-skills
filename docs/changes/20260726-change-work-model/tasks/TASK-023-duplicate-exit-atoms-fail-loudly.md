---
change: change-work-model
id: TASK-023
title: Duplicate exit atoms fail loudly
---

# TASK-023 — Duplicate exit atoms fail loudly

## Objective

Close C4-4: `parseChangeExpectation` overwrites `ExitCode` on each `exit` atom, so `Expect: exit 1 and exit 0` silently last-wins and the receipt records a single check — contradicting the grammar's own "nothing silent in either direction." After this task a contradictory declaration fails the criterion loudly, naming both values.

## Scope boundaries

**In:** `parseChangeExpectation` / `evaluateChangeExpectation` in `internal/cli/change_verify.go`; the grammar sentence in `docs/knowledge/work-model.md` and the template comments in `internal/cli/change_shape_template.md` + `content/skills/shape/templates/shape.md` (one clause: a second `exit` atom is a contradiction and fails the criterion); fixtures in `internal/cli/change_verify_test.go`.

**Out:** The rest of the grammar (settled — `contains` stays repeatable; advisory fallthrough unchanged). `dist/`/`plugins/` rebuilds (integration owns them). Do not touch `bin/`, `config/target-capabilities.json`, `package.json`, or any gate/provenance/state file.

## Context pointers

- Round-4 board finding C4-4; TASK-019's doctrine this completes: "nothing silent in either direction."

## Acquisition

```bash
loaf journal log "skill(implement): TASK-023 — duplicate exit atoms fail loudly"
```

## Steps

- [x] A second `exit` atom in one Expect fails the criterion at evaluation: the receipt records a failed check naming both declared values (suggested kind `exit-conflict`), `ok: false`, and verify prints a plain failure line naming the contradiction.
- [x] Repeatable `contains` atoms stay unaffected; a single `exit` atom behaves exactly as today.
- [x] The grammar docs (work-model.md + both template comments) gain the one-clause rule; byte-identical template copies.
- [x] Fixtures: `exit 1 and exit 0` fails with both values recorded; `exit 0 and contains \`ok\`` unchanged; the criteria digest is unaffected by any of this (digest covers Expect text, not parse results).

## Verification

- `go test ./internal/cli/ -run 'ChangeVerify|ChangeExpect'` green including the contradiction fixtures.

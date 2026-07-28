---
change: change-work-model
id: TASK-019
title: Enforce Expect with a minimal grammar
blocked-by:
  - TASK-017
---

# TASK-019 — Enforce Expect with a minimal grammar

## Objective

Close C3-5: `Expect:` is parsed, digested, and taught by the template, but success is exit-code-zero only — a command exiting 0 with wrong output gets `ok: true` and its receipt opens the gate. Owner decision (2026-07-28): **enforce**, with a deliberately minimal grammar. After this task, a declared expectation is checked, an unenforceable one warns loudly, and the docs teach exactly what is enforced.

## Scope boundaries

**In:** criterion evaluation in `internal/cli/change_verify.go` (~:88-108); the receipt's per-criterion record (additive fields; `schema_version` stays 1 — Go unmarshal tolerates additions and no receipts exist in the wild beyond fixtures); `internal/cli/change_shape_template.md` + `content/skills/shape/templates/shape.md` teaching the grammar; the criteria sentences in `content/skills/shape/references/cli-boundary.md`, `content/skills/shape/references/decomposition.md`, and `docs/knowledge/work-model.md`; fixtures in `internal/cli/change_verify_test.go` **only**.

**Out:** The criteria digest (Expect text is already digested — enforcement changes no digests). Receipt freshness/HEAD-read (TASK-017, lands first in this lane). Rebuilding `dist/`/`plugins/` (integration does it once). Do not touch `change_release_gate*.go`, `change_provenance*.go`, `bin/`, `config/target-capabilities.json`, or `package.json`.

## Context pointers

- Round-3 board finding C3-5; ADR-023 (receipt records per-criterion evidence); the template's current promise: "Expect: exit 0 and/or specific output."

## Grammar (settled — implement, don't redesign)

Atoms joined by ` and `: `exit <N>` (required exit code; absent Expect or absent exit atom → exit 0, preserving today's semantics) and `contains \`text\`` (combined stdout+stderr contains the backtick-delimited text; repeatable). Any other clause is unenforceable: `verify` prints a plain warning naming the criterion and clause, records it on the criterion as advisory, and the clause plays no part in `ok`. Nothing silent in either direction.

## Steps

- [ ] Parse Expect into atoms; `ok` = command ran ∧ exit matches ∧ every `contains` matches; per-criterion receipt record gains the enforced expectation and per-atom results (additive fields).
- [ ] Unenforceable clauses warn at `verify` with criterion ID and clause text, and are recorded as advisory — never contributing to `ok`, never silently dropped.
- [ ] Templates and the four doc surfaces teach the grammar with one example per atom and the advisory rule; the "and/or specific output" promise is replaced by the real contract.
- [ ] Fixtures: exit-atom mismatch fails; `contains` match passes and mismatch fails; multi-atom `and`; absent Expect keeps exit-0; unenforceable clause warns and stays advisory; receipt records the atoms.

## Verification

- `go test ./internal/cli/ -run 'ChangeVerify'` green including the grammar fixtures.
- In a fixture repo: a criterion `Command: \`echo ok\`. Expect: contains \`ok\`.` verifies green; changing to `contains \`nope\`` fails the criterion and (with TASK-017) the committed failing receipt blocks the gate.

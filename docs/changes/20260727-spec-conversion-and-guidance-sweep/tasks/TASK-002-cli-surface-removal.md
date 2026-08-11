---
change: spec-conversion-and-guidance-sweep
id: TASK-002
title: CLI surface removal
blocked-by:
  - TASK-001
blocks:
  - TASK-003
---

# TASK-002 — CLI surface removal

## Objective

`loaf spec` and `loaf task` are unknown commands; the markdown-compat machinery and the `loaf migrate markdown` spec/task importers are gone; refusal is tested.

## Scope boundaries

**In:** `runSpec`/`runTask` dispatch + implementations + help writers + arg parsers + status helpers in `internal/cli/cli.go` (~3,600 lines); markdown-compat spec/task machinery; `loaf migrate markdown` spec/task import paths; `loaf state export spec`; root-help and `agent_help.go` rows; `cli_reference.go` generator entries; `cli_test.go` remediation (replace legacy coverage with refusal tests — `TestLegacyWorkSurfaceRemoved`); `cmd/loaf/main_test.go` root-help assertion.

**Out:** State-layer internals (TASK-003). Hook catalog and install surfaces (TASK-004). Generated skill content lands with TASK-005's regeneration; keep the reference-contract test green by pairing generator + generated output in this commit if required by `TestCLIReferenceSourceMatchesGeneratedContract`. Watch: this task is heavyweight — split if writing the packet's first slice reveals more than one coherent commit (sanctioned).

## Context pointers

- Contract: `shape.md` — Decision 1, Rabbit Holes ("The 792-line test file")
- Inventory anchors: `cli.go:4369-6330`, `cli.go:7765-8960`, `cli.go:11798-13040`, `cli_reference.go:405-479`

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — CLI surface removal"
```

## Steps

- [ ] Remove dispatch, implementations, parsers, helpers, and help surfaces for `spec` and `task`
- [ ] Remove markdown-compat machinery and `migrate markdown` spec/task importers; `state export spec` gone
- [ ] Regenerate the CLI reference; pair generator + output so the contract test stays green
- [ ] `TestLegacyWorkSurfaceRemoved`: both commands refuse with unknown-command errors
- [ ] Remediate `cli_test.go`: legacy coverage deleted, surviving suite untouched structurally

## Verification

- `go test ./internal/cli -run TestLegacyWorkSurfaceRemoved -v` green
- `go test ./...` green; `grep -r "runSpec\|runTask" internal/cli` returns nothing

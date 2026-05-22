---
id: SPEC-037
title: Mutable spec work definitions
source: direct
created: '2026-05-22T10:17:33Z'
status: drafting
related_specs:
  - SPEC-038
  - SPEC-039
---

# SPEC-037: Mutable spec work definitions

## Problem Statement

Loaf currently treats `SPEC-*` files as the central shaping artifact, but the surrounding guidance blurs two different roles:

1. **Work definition:** a mutable internal artifact that helps agents shape, decompose, coordinate, verify, and hand off work while the work is happening.
2. **Durable truth:** long-lived knowledge that future humans and agents should trust after implementation ships.

That ambiguity creates bad incentives. Specs become over-formal before evidence exists, while shipped behavior can remain trapped in `.agents/` process files instead of being extracted into code, tests, docs, project knowledge, changelog entries, and rare ADRs.

## Strategic Alignment

- **Vision:** Loaf should reduce coordination friction for agentic work without turning planning artifacts into permanent scripture.
- **Personas:** Solo developers and team leads need enough structure to make agent work reliable, but not external dead links to disposable local files.
- **Architecture:** Specs stay Loaf-internal. Durable knowledge moves to the appropriate public or project knowledge surface after implementation.

## Solution Direction

Keep the name `SPEC-*`, but redefine it precisely:

> A Loaf spec is a mutable internal work definition. It is not a permanent product contract, not an external reference target, and not a release artifact. When the work ships, durable truth is extracted into code, tests, docs, PKB, changelog/release notes, and rare ADRs; the spec becomes archive/process history.

The workflow becomes:

```text
idea / spark / brainstorm
  -> /shape
  -> mutable SPEC work definition
  -> /breakdown
  -> executable tasks
  -> implementation
  -> reflection / extraction
  -> code, tests, docs, PKB, changelog, rare ADR
  -> spec archived as process history
```

Specs may be rewritten, narrowed, split, abandoned, superseded, or archived. That is expected behavior, not lifecycle failure.

## Relationship To Other Specs

- **SPEC-038** enforces the boundary that `SPEC-*` and other `.agents/` identifiers never leak into external artifacts.
- **SPEC-039** builds the backend-neutral work ledger and Linear CLI adapter that lets specs map to external work systems without exposing internal IDs there.

This spec owns the semantics. SPEC-038 owns enforcement. SPEC-039 owns backend mechanics.

## Scope

### In Scope

- Redefine specs as mutable internal work definitions.
- Update `/shape` guidance and templates to describe provisional, editable work definitions.
- Update `/breakdown` guidance so tasks are generated from mutable specs but need not preserve spec wording after work starts.
- Update `/implement` closeout guidance so durable truth is extracted after shipping instead of leaving specs as the trusted record.
- Update `/reflect` guidance to treat completed specs as evidence sources, not canonical knowledge.
- Update architecture/product-workflow docs to describe spec lifecycle and extraction.
- Clarify that ADRs are rare, high-impact durable decisions, not the default place to preserve implementation learnings.

### Out of Scope

- Renaming `SPEC-*` to `BRIEF-*`, `PLAN-*`, or another artifact.
- Introducing PRDs, epics, or product-planning artifacts as Loaf primitives.
- Implementing SQLite or a new durable work ledger. That belongs to SPEC-039.
- Enforcing public artifact leakage rules. That belongs to SPEC-038.
- Rewriting all historical archived specs.
- Rebuilding generated `dist/` or `plugins/` artifacts before source guidance is stable.

### Rabbit Holes

- Trying to make specs perfectly accurate after implementation. Resist: extract truth elsewhere and archive the spec.
- Turning every shipped implementation into an ADR. Resist: ADRs are for critical, high-reversal-cost decisions.
- Encoding external system references directly into spec prose. Resist: SPEC-039 should own structured backend mappings.

### No-Gos

- Specs must not be described as immutable.
- Specs must not be treated as public or external reference targets.
- Specs must not become the only place where shipped behavior is documented.
- ADRs must not become the default archive for ordinary implementation decisions.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Agents treat "mutable" as permission for vague specs | Medium | Medium | Keep required fields: problem, scope, non-goals, verification, extraction targets |
| Durable knowledge extraction becomes optional and forgotten | Medium | High | Add explicit `/implement` and `/reflect` closeout checks |
| Historical specs confuse future readers | Medium | Medium | Archive specs with clear status and "process history, not canonical truth" language |
| ADR threshold becomes unclear | Medium | Medium | Add explicit ADR threshold language: high reversal cost, cross-cutting architecture, long-lived policy |

## Open Questions

- [ ] Should spec status include `shipped` before `archived`, or is `complete` sufficient?
- [ ] Should spec frontmatter record `extracted_to` links for PKB/docs/changelog/ADR outputs?
- [ ] Should `/reflect SPEC-XXX` become the canonical extraction workflow before archiving?
- [ ] Should specs support an explicit `superseded_by` relation for abandoned or reshaped work?

## Test Conditions

- [ ] `/shape` documentation defines specs as mutable internal work definitions.
- [ ] Spec template states that specs are not canonical after implementation and includes extraction targets.
- [ ] `/breakdown` documentation accepts specs as work definitions without implying immutable contract status.
- [ ] `/implement` closeout requires durable truth extraction into code/tests/docs/PKB/changelog/rare ADR as appropriate.
- [ ] `/reflect` treats completed specs as evidence sources, not canonical knowledge.
- [ ] Architecture/product workflow docs describe the new lifecycle.
- [ ] Existing source examples that imply specs are permanent truth are removed or rewritten.

## Priority Order

1. **Track A - Core semantics.** Update shape/spec template language and architecture docs. Go/no-go: every primary description of `SPEC-*` says mutable internal work definition.
2. **Track B - Workflow closeout.** Update breakdown, implement, and reflect guidance. Go/no-go: closeout requires extraction to durable surfaces where appropriate.
3. **Track C - Historical cleanup guidance.** Add minimal guidance for reading archived specs as process history. Go/no-go: no archived-spec rewrite required.

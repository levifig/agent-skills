---
id: SPEC-025
title: Remove appetite and replace circuit breaker with go/no-go gates
source: direct
created: '2026-04-06T17:04:15.000Z'
status: complete
---

# SPEC-025: Remove Appetite and Replace Circuit Breaker with Go/No-Go Gates

## Problem Statement

Two Shape Up concepts don't translate to agentic development:

**Appetite** — "fixed time, variable scope" assumes human time budgets. Agent timelines are fundamentally different — a "1-2 day" appetite can be consumed in minutes. The field creates ceremony without enforcement, adding friction to every spec without delivering value.

**Circuit breaker** — answers "what do we ship if the team runs out of steam at 75%?" Agents don't run out of steam. The scarce resource is scope clarity and context window, not motivation. The concept maps poorly to agentic workflows where the real question is "is this approach working?" not "are we running out of time?"

## Strategic Alignment

- **Vision:** Loaf optimizes for agent workflows. Removing a human-team mechanic that doesn't translate aligns with "agent-creates, human-curates."
- **Personas:** Reduces friction for the spec author (human or agent) — one less field to fill, one less concept to explain.
- **Architecture:** Simplifies the `SpecEntry` type and spec parsing. No new complexity introduced.

## Solution Direction

**Remove appetite** from frontmatter schema, TypeScript types, CLI display, templates, and all skill references.

**Replace circuit breaker with two concepts:**
- **Priority ordering** — spec tracks ship in explicit order (A → B → C → D). If scope needs cutting, drop from the end.
- **Go/no-go gates** — binary checks between priority tracks. Test conditions (already defined in specs) serve as the gate criteria. Example: "Verify Part A test conditions pass before starting Part B."

This eliminates the `## Circuit Breaker` section from spec/plan templates and replaces it with `## Priority Order` — a ranked list of tracks with go/no-go gates between them. Specs with a single track need no priority section.

## Scope

### In Scope
- Remove `appetite` field from `SpecEntry`/`SpecFrontmatter` types and parser/migrate/display code
- Replace `## Circuit Breaker` with `## Priority Order` in spec template
- Delete `content/templates/plan.md` entirely (dead concept — no plan files have ever been created)
- Remove plan file references from implement skill (`SKILL.md` and `references/session-management.md`)
- Remove `plan.md` from `shared-templates` in `targets.yaml`
- Delete empty `.agents/plans/` directory
- Update shape skill (remove appetite from interview, replace circuit breaker guidance with priority ordering)
- Update orchestration, breakdown, and bootstrap skills
- Update docs (ARCHITECTURE.md, task-system.md)
- Update CLAUDE.md references to circuit breaker
- Clean up existing active spec frontmatter and TASKS.json
- Leave the software pattern "circuit breaker" references untouched (foundations, production-readiness)
- Update tests
- Rebuild all targets

### Out of Scope
- Replacing appetite with a different sizing mechanism
- Redesigning the spec splitting workflow
- Modifying archived specs (historical record)

### Rabbit Holes
- **Sizing replacement** — Don't introduce a new sizing field (S/M/L complexity). If needed later, that's a separate spec.
- **Spec splitting redesign** — Replace "too big for appetite" with complexity-based language but don't redesign the splitting workflow itself.
- **Complex gate logic** — Go/no-go gates are binary checks against existing test conditions. Don't build a gate execution framework.

### No-Gos
- Don't modify archived spec files in `.agents/specs/archive/`
- Don't touch "circuit breaker" references that mean the software resilience pattern (not Shape Up)

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Existing specs break on missing field | Low | Low | Parser already handles optional fields; remove from type, parser ignores extra frontmatter |
| Loss of useful sizing signal | Low | Low | Scope boundaries and splitting guidance remain; add sizing later if needed |

## Open Questions

None — scope is well-defined.

## Test Conditions

- [ ] `grep -r "appetite" content/ cli/ docs/` returns zero matches
- [ ] `grep -r "Circuit Breaker" content/skills/ content/templates/` returns zero matches (excluding software pattern refs)
- [ ] Spec template has `## Priority Order` section instead of `## Circuit Breaker`
- [ ] `content/templates/plan.md` does not exist
- [ ] No plan file references in implement skill
- [ ] `.agents/plans/` directory removed
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes
- [ ] `loaf build` succeeds
- [ ] Existing active specs parse correctly after field removal

## Implementation Notes

- Implement AFTER SPEC-027 on the same branch — SPEC-027 changes session/implement skills that SPEC-025 also touches
- Ship both specs in one PR
- If TASKS.json migration is complex, leave existing `appetite` values as dead data (parser ignores unknown fields)

## Files to Modify

| Area | Files |
|------|-------|
| Types/CLI | `cli/lib/tasks/types.ts`, `parser.ts`, `migrate.ts`, `cli/commands/spec.ts` |
| Tests | `parser.test.ts`, `migrate.test.ts`, `archive.test.ts`, `scanner.test.ts` |
| Templates | `content/skills/shape/templates/spec.md`, `content/templates/plan.md` (DELETE) |
| Implement | `content/skills/implement/SKILL.md`, `content/skills/implement/references/session-management.md` (remove plan file refs) |
| Config | `config/targets.yaml` (remove plan.md from shared-templates) |
| Cleanup | `.agents/plans/` (DELETE empty directory) |
| Skills | shape `SKILL.md`, orchestration `SKILL.md` + `refs/planning.md` + `refs/specs.md` + `refs/product-development.md`, breakdown `SKILL.md`, bootstrap `SKILL.md` + `refs/interview-guide.md` |
| CLAUDE.md | `.claude/CLAUDE.md` (circuit breaker references in spec template docs) |
| Docs | `docs/ARCHITECTURE.md`, `docs/knowledge/task-system.md` |
| Data | `.agents/specs/SPEC-*.md` (active only), `.agents/TASKS.json` |
| Ideas | Archive `20260328-replace-circuit-breaker-with-go-nogo.md` (absorbed into this spec) |

---
id: SPEC-025
title: "Remove appetite from spec system"
source: "direct"
created: 2026-04-06T17:04:15Z
status: drafting
---

# SPEC-025: Remove Appetite from Spec System

## Problem Statement

The `appetite` concept (Shape Up's "fixed time, variable scope") assumes human time budgets. In AI-assisted development, agent timelines are fundamentally different — a "1-2 day" appetite can be consumed in minutes. The field creates ceremony without enforcement (nothing tracks or enforces it), adding friction to every spec without delivering value.

## Strategic Alignment

- **Vision:** Loaf optimizes for agent workflows. Removing a human-team mechanic that doesn't translate aligns with "agent-creates, human-curates."
- **Personas:** Reduces friction for the spec author (human or agent) — one less field to fill, one less concept to explain.
- **Architecture:** Simplifies the `SpecEntry` type and spec parsing. No new complexity introduced.

## Solution Direction

Remove `appetite` from frontmatter schema, TypeScript types, CLI display, templates, and all skill references. Keep circuit breakers but reframe them as scope-based decision points rather than time percentages (e.g., "If core approach isn't working" instead of "At 50% appetite").

## Scope

### In Scope
- Remove `appetite` field from `SpecEntry`/`SpecFrontmatter` types and parser/migrate/display code
- Update spec template and plan template (remove frontmatter field, reframe circuit breakers)
- Update shape, orchestration, breakdown, and bootstrap skills
- Update docs (ARCHITECTURE.md, task-system.md)
- Clean up existing active spec frontmatter and TASKS.json
- Update tests
- Rebuild all targets

### Out of Scope
- Replacing appetite with a different sizing mechanism
- Redesigning the spec splitting workflow
- Modifying archived specs (historical record)

### Rabbit Holes
- **Sizing replacement** — Don't introduce a new sizing field (S/M/L complexity). If needed later, that's a separate spec.
- **Spec splitting redesign** — Replace "too big for appetite" with complexity-based language but don't redesign the splitting workflow itself.

### No-Gos
- Don't modify archived spec files in `.agents/specs/archive/`
- Don't remove the circuit breaker section from spec/plan templates — reframe it

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Existing specs break on missing field | Low | Low | Parser already handles optional fields; remove from type, parser ignores extra frontmatter |
| Loss of useful sizing signal | Low | Low | Scope boundaries and splitting guidance remain; add sizing later if needed |

## Open Questions

None — scope is well-defined.

## Test Conditions

- [ ] `grep -r "appetite" content/ cli/ docs/` returns zero matches
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes
- [ ] `loaf build` succeeds
- [ ] Existing active specs parse correctly after appetite field removal

## Circuit Breaker

If updating the bootstrap interview guide proves too entangled with appetite philosophy, simplify to removing explicit appetite references without rewriting interview questions.

If TASKS.json migration is complex, leave existing `appetite` values as dead data (parser ignores unknown fields).

## Files to Modify

| Area | Files |
|------|-------|
| Types/CLI | `cli/lib/tasks/types.ts`, `parser.ts`, `migrate.ts`, `cli/commands/spec.ts` |
| Tests | `parser.test.ts`, `migrate.test.ts`, `archive.test.ts`, `scanner.test.ts` |
| Templates | `content/skills/shape/templates/spec.md`, `content/templates/plan.md` |
| Skills | shape `SKILL.md`, orchestration `SKILL.md` + `refs/planning.md` + `refs/specs.md` + `refs/product-development.md`, breakdown `SKILL.md`, bootstrap `SKILL.md` + `refs/interview-guide.md` |
| Docs | `docs/ARCHITECTURE.md`, `docs/knowledge/task-system.md` |
| Data | `.agents/specs/SPEC-*.md` (active only), `.agents/TASKS.json` |

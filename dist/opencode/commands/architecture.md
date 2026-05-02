---
description: >-
  Creates Architecture Decision Records through structured interviews,
  stabilizing canonical vocabulary in the domain glossary as load-bearing terms
  surface. Use when making technical decisions or when the user asks "should we
  use X or Y?" Produces ADRs and updates ARCHITECTURE.md with technical
  constraints. Not for strategic direction (use strategy) or multi-perspective
  deliberation (use council).
subtask: false
version: 2.0.0-dev.37
---

# Architecture

Guides technical decision-making through structured interviews, options analysis, and ADR creation. Stabilizes canonical vocabulary in `docs/knowledge/glossary.md` when load-bearing terms surface mid-interview — additive to ADR creation, never a gate on it.

## Critical Rules

**Always**
- Interview first to understand constraints, options already considered, and tradeoffs
- Read existing VISION.md, ARCHITECTURE.md, and ADRs before proposing changes
- Read `docs/knowledge/glossary.md` at interview start (via `loaf kb glossary list`); use canonical terms throughout
- When fuzzy/drifted language surfaces, challenge inline; if a load-bearing term emerges, offer `loaf kb glossary upsert` or `stabilize`
- Follow the shared interview protocol in [templates/grilling.md](../skills/architecture/templates/grilling.md)
- Present multiple options with pros/cons and "fits when" context
- Wait for explicit user decision before proceeding with documentation
- Log decision to session journal: `loaf session log "decision(architecture): ADR-NNN adopted for X"`

**Never**
- Make architectural decisions without user input
- Contradict existing decisions without explicitly superseding them
- Skip the interview phase for complex decisions
- Create ADRs without user approval
- Block ADR creation on glossary state — glossary mutations are additive and opt-in
- Call `loaf kb glossary propose` (reserved for upstream ambiguity-resolving skills)

## Verification

After work completes, verify:
- ADR created using template at [templates/adr.md](../skills/architecture/templates/adr.md)
- ARCHITECTURE.md updated with new constraints and ADR reference
- ADR number assigned sequentially (ls docs/decisions/ADR-*.md for next number)
- Council convened if decision affects multiple domains
- Glossary read at interview start; any load-bearing term surfaced was offered for `stabilize` or `upsert`

## Quick Reference

### ADR Numbering

```bash
ls docs/decisions/ADR-*.md 2>/dev/null | \
  grep -oE 'ADR-[0-9]+' | sort -t- -k2 -n | tail -1 | awk -F- '{print $2 + 1}'
```

Start with `ADR-001` if none exist.

### Council Triggers

Convene when: multiple domains affected, conflicting team opinions, high reversal cost, novel problem, or user requests deliberation.

### Evaluation Criteria

For each option: alignment with VISION/ARCHITECTURE, complexity added, reversibility, team capability, maintenance cost.

### Glossary Mutation Policy

This skill *stabilizes* terms — promote a previously-proposed candidate, or write a canonical term directly when one emerges mid-interview.

| Verb | When |
|------|------|
| `loaf kb glossary list` | At interview start (via grilling protocol) |
| `loaf kb glossary check <term>` | When a term's status is in question during the interview |
| `loaf kb glossary stabilize <term>` | A previously-proposed candidate has firmed up into a load-bearing decision |
| `loaf kb glossary upsert <term> --definition <d> --avoid <list>` | A load-bearing term emerges fresh and is canonical from the outset |

`propose` is reserved for upstream skills that resolve ambiguity (e.g., a future `/shape` evolution) — do not call it here.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| ADR Template | [templates/adr.md](../skills/architecture/templates/adr.md) | Creating new architecture decision records |
| Grilling Protocol | [templates/grilling.md](../skills/architecture/templates/grilling.md) | Running the structured interview, including glossary discipline |
| Council Workflow | `orchestration/references/councils.md` | Multi-agent deliberation for complex decisions |
| Documentation | `documentation-standards/references/documentation.md` | ADR formatting and standards |

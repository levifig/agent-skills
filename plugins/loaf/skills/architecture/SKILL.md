---
name: architecture
description: >-
  Creates Architecture Decision Records through structured evaluation. Use when
  making technical decisions or when the user asks "should we use X or Y?"
  Produces ADRs and updates ARCHITECTURE.md with technical constraints. Not for
  strategic directio...
user-invocable: true
argument-hint: '[topic or decision]'
version: 2.0.0-dev.13
---

# Architecture

Guides technical decision-making through structured interviews, options analysis, and ADR creation.

## Critical Rules

**Always**
- Interview first to understand constraints, options already considered, and tradeoffs
- Read existing VISION.md, ARCHITECTURE.md, and ADRs before proposing changes
- Present multiple options with pros/cons and "fits when" context
- Wait for explicit user decision before proceeding with documentation

**Never**
- Make architectural decisions without user input
- Contradict existing decisions without explicitly superseding them
- Skip the interview phase for complex decisions
- Create ADRs without user approval

## Verification

After work completes, verify:
- ADR created using template at [templates/adr.md](templates/adr.md)
- ARCHITECTURE.md updated with new constraints and ADR reference
- ADR number assigned sequentially (ls docs/decisions/ADR-*.md for next number)
- Council convened if decision affects multiple domains

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

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| ADR Template | [templates/adr.md](templates/adr.md) | Creating new architecture decision records |
| Council Workflow | `orchestration/references/councils.md` | Multi-agent deliberation for complex decisions |
| Documentation | `documentation-standards/references/documentation.md` | ADR formatting and standards |

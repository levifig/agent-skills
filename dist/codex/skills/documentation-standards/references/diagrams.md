# Architecture Diagrams

## Contents
- When to Create Diagrams
- Mermaid Quick Reference
- Storage Patterns
- Diagram Rules

Guidelines for creating Mermaid diagrams in project documentation.

## When to Create Diagrams

| Scenario | Diagram Type |
|----------|--------------|
| Multi-service changes | Component/flowchart |
| Data flow changes | Sequence/flowchart |
| Schema modifications | ERD |
| API design | Sequence |
| State machine logic | State diagram |
| Complex conditionals | Flowchart |

**Skip diagrams for:** single-file changes, bug fixes with no architectural impact, configuration-only changes.

## Mermaid Quick Reference

**Direction:** `TD` (top-down), `LR` (left-right), `BT` (bottom-top), `RL` (right-left)

**Node shapes:** `[Rectangle]` process, `{Diamond}` decision, `([Stadium])` start/end, `[(Database)]` database, `((Circle))` connector

**Sequence arrows:** `->>` sync call, `-->>` response, `--)` async message

**ERD relations:** `||` one required, `o|` zero-or-one, `}|` one-or-many, `}o` zero-or-many

## Storage Patterns

### Inline in the current work

For diagrams specific to current work or temporary visualizations. Include purpose, files involved, and limitations alongside the diagram.

### Stored in `.agents/diagrams/`

For reusable diagrams shared across conversations:

```
.agents/diagrams/
├── system-architecture.md
├── auth-flow.md
└── data-pipeline.md
```

**Diagram file format:**

```markdown
---
created: 2025-01-23T14:00:00Z
last_updated: 2025-01-23T14:00:00Z
tags: [auth, api, security]
---

# Diagram Title

## Overview
[Purpose of this diagram]

## Diagram
[mermaid block]

## Related Files
- [list of source files this diagram represents]
```

| Store in `.agents/diagrams/` | Keep Inline |
|------------------------------|-------------|
| Referenced by multiple sessions | One-time visualization |
| Documents stable architecture | Work-in-progress |
| Shared across team members | Personal understanding aid |

## Diagram Rules

1. **Use actual names from code** in nodes (service names, table names), not generic placeholders
2. **One concern per diagram** -- don't combine auth, billing, notifications into one
3. **Every diagram needs:** purpose, files involved, limitations
4. **Update when code changes** -- update diagram + `last_updated` timestamp
5. **Create during planning**, not after code is written

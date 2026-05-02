---
topics:
  - knowledge
  - staleness
  - qmd
  - covers
  - growth-loops
covers:
  - cli/lib/kb/**/*.ts
  - docs/knowledge/**/*.md
last_reviewed: '2026-04-29'
---

# Knowledge Management Design

The knowledge management feature: `loaf kb` commands + knowledge-base skill + QMD integration + lifecycle hooks.

## Naming Conventions

Storage uses full words: `docs/knowledge/` and `docs/decisions/` for directories, `{repo-folder}-knowledge` and `{repo-folder}-decisions` for QMD collections. The two names are similar in length and scan well together — `kb` versus `decisions` would create visual asymmetry, and `decisions` is more accessible to non-engineers than `adrs`.

The CLI shorthand is `kb`: `loaf kb check`, not `loaf knowledge check`. The full word is for storage (durable, read by humans and tools); the abbreviation is for typing (ergonomic, used dozens of times a day). ADR files keep the `ADR-XXX` prefix — that's the record format, not the directory name.

## Knowledge File Schema

```yaml
---
topics: [engine-registry, strategy-pattern]    # Required (min 1)
last_reviewed: 2026-03-14                       # Required
covers:                                         # Recommended (enables staleness)
  - "src/pipeline/registry.py"
  - "src/models/engine_*.py"
consumers: [backend, power-systems]             # Optional (agent routing)
depends_on: [thermal-physics.md]                # Optional (cross-references)
implementation_status: in-progress              # Optional
---
```

## Staleness Detection (Core Innovation)

No AI coding tool does this today. The `covers:` field links knowledge to code paths:
1. Parse `covers:` globs → expand to matching files
2. Query `git log --since={last_reviewed}` for those files
3. If commits exist → knowledge is potentially stale
4. Surface to agent (advisory, not blocking)

`covers:` is recommended, not required. Files without it can't use automated staleness but are valid knowledge files.

## Growth Loops

**Loop 1: Staleness → Review → Update.** Code edited → `covers:` match → agent nudged → reviews/updates → resets `last_reviewed`.

**Loop 2: Session → Consolidation → Knowledge.** Session ends → hook asks "learned anything?" → agent creates knowledge file → human reviews.

### Hook Scope (Extended Beyond Knowledge)

**SessionStart hook** surfaces both knowledge health AND spark status:
- "3 knowledge files relevant. 1 stale."
- "5 unprocessed sparks across 2 brainstorm documents."

This reminds the agent that exploration artifacts exist and may need processing.

**SessionEnd hook** prompts for both knowledge consolidation AND spark capture:
- "You modified 3 paths covered by knowledge files. Any updates needed?"
- "This session involved exploration. Any sparks worth noting?"

If the session produced a brainstorm document, reminds about the `## Sparks` section.

## The Original Problem (5 Overlapping Surfaces)

Knowledge accumulates across 5+ surfaces that don't coordinate:
1. `docs/knowledge/` — structured domain knowledge
2. `docs/decisions/` — immutable ADRs
3. Claude Code `MEMORY.md` — cross-session AI context (drifts)
4. Serena `.serena/memories/` — separate AI memory (goes stale)
5. `.agents/sessions/` + draft docs — temporary but become permanent

Same facts recorded in 2-3 places. Nothing triggers updates when code changes. Memory layers drift silently. The knowledge management system consolidates these into clear ownership (see Memory Surface Policy below).

## GridSight as Reference Implementation

The knowledge system was first prototyped in GridSight (`gridsight-core-gds`). Key patterns that carried forward:
- Knowledge files with YAML frontmatter (topics, consumers, depends_on, implementation_status, last_reviewed)
- README.md as index with dependency graph
- CLAUDE.md routing table mapping agent tasks → knowledge files
- Cross-references section in each file
- Categories (product/, technical/) as optional organization

What GridSight lacked (and Loaf adds): `covers:` field for staleness automation, QMD for retrieval, growth loops via hooks, CLI for management.

## Key Insight: Search Instead of Sync

Cross-project knowledge sharing doesn't require copying or vendoring. Index knowledge where it lives and make it searchable. QMD collections are pointers, not copies. One source of truth per file, many consumers, zero drift. Like how Google solved "information spread across the web" — index it, don't centralize it.

## QMD Integration

QMD handles retrieval (collections, search, MCP). Loaf wraps QMD for setup + adds lifecycle:
- `loaf kb init` → `qmd collection add` + context
- `loaf kb import` → `qmd collection add` for external repo + `.agents/loaf.json` update
- Collection naming: `{repo-folder}-knowledge`, `{repo-folder}-decisions`

## Three Knowledge Lifetimes

```
PERSISTENT (this person)     — crosses all projects, grows over career
  └→ DURABLE (this project)  — lives in git, shared with team
      └→ EPHEMERAL (this session) — lives in context window, dies at session end
```

| Lifetime | What | Where |
|----------|------|-------|
| **Persistent** | User preferences, expertise, patterns | `~/.config/claude/` (CLAUDE.md, PROFILE.md, memory/) |
| **Durable** | Domain knowledge, conventions, decisions | `docs/knowledge/` + `docs/decisions/` + CLAUDE.md |
| **Ephemeral** | Active task context, working hypotheses | `.agents/sessions/` + conversation |

Flow: persistent informs durable, durable informs ephemeral. Upward: session insights consolidate into durable knowledge, durable patterns graduate to persistent memory.

## Knowledge as Extended Agent Memory

The knowledge base is primarily for agents — it's their extended memory. This includes not just domain rules and conventions, but also roadmap context, future implementation plans, and strategic direction. Agents use this to make informed decisions aligned with project goals.

## Memory Surface Policy

| Surface | Contains |
|---------|----------|
| `docs/knowledge/` | Domain rules, contracts, architectural patterns, roadmap context |
| `docs/decisions/` | Immutable ADRs |
| `MEMORY.md` | User preferences, session pointers |
| `.agents/sessions/` | Ephemeral work context |
| Serena memories | **Deprecated for knowledge** — code analysis state only |

**Serena:** Keep code intelligence tools (find_symbol, get_symbols_overview). Deprecate memory system for knowledge persistence — with knowledge files + MEMORY.md, Serena memories become a redundant third layer.

## Growth Loop 3: Personal Knowledge Graduation

Personal knowledge grows through correction, not capture. When you say "don't use bare except clauses," that's a personal preference that applies everywhere — not just this project.

The graduation path:
```
Per-project correction → repeated across N projects → pattern recognized →
suggested promotion to global CLAUDE.md → user approves → permanent
```

v1: convention + documentation only (no automation for cross-project pattern detection). The skill documents what belongs in global CLAUDE.md vs per-project memory. Future: cross-project pattern detection (Mem0 territory), personal knowledge Zettelkasten, expertise graph.

## The Boundary Test: What Goes Where

| Surface | Contains | Decision Test |
|---------|----------|---------------|
| **Code** (docstrings, types) | What the code does | Is it self-documenting? → stays in code |
| **Knowledge files** | Domain rules, cross-cutting context | Requires context beyond the code? → knowledge file |
| **ADRs** | Why we chose this approach | Is it an architectural decision? → ADR |
| **CLAUDE.md** | Agent instructions, conventions | Is it about how agents should behave? → CLAUDE.md |
| **MEMORY.md** | User preferences, session pointers | Is it personal/ephemeral? → MEMORY.md |

CLAUDE.md references knowledge files but never duplicates their content.

## Staleness Detection Mitigations

| Risk | Mitigation |
|------|-----------|
| Alert fatigue (too many nudges) | Cooldown: max 1 nudge per knowledge file per session |
| Broad globs = constant nudging | Threshold: only nudge if >N days since review OR >N commits |
| Agent updates incorrectly | Advisory: "Consider reviewing" not "I've updated it" |
| Configurable thresholds | `staleness_threshold_days` in `.agents/loaf.json` |

## Cross-Project: `local-covers`

When importing knowledge from another repo, the knowledge file's `covers:` globs are relative to its origin repo. For staleness detection in the importing repo, use `local-covers` in the import config:

```json
{
  "knowledge": {
    "imports": [
      {
        "name": "gridsight-core-gds",
        "local-covers": {
          "thermal-physics": ["src/thermal_adapter.py"],
          "engine-registry": ["src/adapters/engine_*.py"]
        }
      }
    ]
  }
}
```

Each consuming repo declares which of ITS code paths relate to imported knowledge. The harness uses `local-covers` for PostToolUse nudges. The knowledge file itself doesn't need to know about every consumer.

## Cross-Project: Tightly-Coupled Repos

For tightly-coupled project families (e.g., GDS core + modules), the simplest approach is symlinks or workspace linking — work from the core repo with modules linked in. The agent sees `docs/knowledge/` directly. No import mechanism needed.

QMD collections + `loaf kb import` are for loosely-coupled sharing where symlinks aren't practical.

## QMD Lightweight Mode

QMD works without models (BM25-only):
- `qmd collection add/remove`, `qmd search`, `qmd get`, `qmd update` — no models needed
- `qmd embed` (embeddings) and `qmd query` (hybrid+reranking) — need ~2GB of GGUF models
- Loaf can depend on QMD but only require lightweight mode. Semantic search is opt-in.

## Cross-References

- [agent-harness-vision.md](agent-harness-vision.md) — how agents use the knowledge system
- [cli-design.md](cli-design.md) — the `loaf kb` commands
- [../decisions/ADR-003-qmd-as-retrieval-backend.md](../decisions/ADR-003-qmd-as-retrieval-backend.md) — QMD decision
- [../ARCHITECTURE.md#authorship-model--agents-create-humans-curate](../ARCHITECTURE.md#authorship-model--agents-create-humans-curate) — authoring model
- Full brainstorm: `.agents/drafts/brainstorm-loaf-cli-knowledge-harness.md`

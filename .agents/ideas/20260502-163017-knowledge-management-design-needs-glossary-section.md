---
title: "knowledge-management-design.md needs a glossary KB convention section"
captured: 2026-05-02T16:30:17Z
status: raw
tags: [knowledge-base, docs, glossary, kb-coverage]
related:
  - SPEC-034-refactor-deepen-grilling-glossary
---

# `knowledge-management-design.md` needs a glossary KB convention section

## Nugget

`docs/knowledge/knowledge-management-design.md` covers `cli/lib/kb/**/*.ts` and `docs/knowledge/**/*.md`. SPEC-034 expanded both: new `cli/lib/kb/glossary.ts` (parser, serializer, lazy creation, fence-aware splitting) and new `docs/knowledge/glossary.md` (a new KB *type*, with `type: glossary` frontmatter and four required sections). The design doc currently does not mention "glossary" anywhere. It's structurally stale — `loaf kb check` flags it, but a review-stamp would cover only the mechanical part; the actual content gap remains.

## Problem/Opportunity

`knowledge-management-design.md` is the canonical reference for "how Loaf's `loaf kb` surface works." A new KB type with its own data layer, CLI verbs, and lifecycle (lazy creation, Linear-native fail-fast) is a first-class addition that the design doc should describe.

The doc should grow:

1. **A "Glossary type" subsection** under the existing "Knowledge File Schema" section, documenting:
   - `type: glossary` frontmatter discriminator
   - Four required sections (`## Canonical Terms`, `## Candidates`, `## Relationships`, `## Flagged ambiguities`)
   - Term entry format (H3 heading + definition + optional `_Avoid_:` line)
   - Why it's structurally distinct from regular knowledge files (no `covers:`, write-once-at-a-time mutation, CLI-only writes)

2. **A "CLI mutation surface" subsection** describing the `loaf kb glossary` verbs and the design rationale (mutation policy in CLI verbs, not skill prose).

3. **An "Adapter pattern: lazy creation" subsection** — the glossary file is created only on first successful write, never on read. Same pattern is now relevant to PLAN artifacts (`.agents/plans/`) too. Worth elevating to a general convention.

## Initial Context

- **Surface area:** ~50-100 lines of additions to `docs/knowledge/knowledge-management-design.md`
- **Doesn't touch code:** purely a documentation gap
- **Adjacent:** `docs/knowledge/skill-architecture.md` was updated for the +1 skill count but doesn't cover glossary mechanics either. Should at minimum cross-reference the design doc.

## Sequencing

Low priority. Doc lag is normal post-feature; the spec, the skill SKILL.md, the references, and the CHANGELOG entry collectively cover the same ground for any reader. This idea promotes the convention from "discoverable across multiple artifacts" to "documented in one canonical place."

Worth picking up on the next docs/reflect cycle.

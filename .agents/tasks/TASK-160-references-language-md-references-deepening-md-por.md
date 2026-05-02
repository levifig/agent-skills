---
id: TASK-160
title: references/language.md + references/deepening.md (port + Loaf mapping)
status: done
priority: P2
created: '2026-05-02T01:25:49.035Z'
updated: '2026-05-02T01:25:49.035Z'
completed_at: '2026-05-02T03:35:00.000Z'
spec: SPEC-034
depends_on:
  - TASK-159
---

# TASK-160: references/language.md + references/deepening.md (port + Loaf mapping)

## Description

Verbatim ports of Matt Pocock's LANGUAGE.md and DEEPENING.md into `content/skills/refactor-deepen/references/`. Add Loaf-context mapping notes where Loaf vocabulary differs slightly (e.g., note that "module" here is the source's module concept — distinct from a Loaf "skill"). Both files >100 lines need `## Contents` TOC per Loaf convention.

## File Hints

- Source: https://github.com/mattpocock/skills/tree/main/skills/engineering/improve-codebase-architecture/LANGUAGE.md
- Source: https://github.com/mattpocock/skills/tree/main/skills/engineering/improve-codebase-architecture/DEEPENING.md
- Target: `content/skills/refactor-deepen/references/language.md` (new)
- Target: `content/skills/refactor-deepen/references/deepening.md` (new)

## Acceptance Criteria

- [ ] `references/language.md` exists with all 8 terms (Module, Interface, Implementation, Depth, Seam, Adapter, Leverage, Locality), Principles, Relationships, Rejected framings sections — all from source
- [ ] `references/deepening.md` exists with 4 dependency categories (in-process, local-substitutable, ports-and-adapters, true-external), Seam discipline, Testing strategy — all from source
- [ ] If either file >100 lines, has `## Contents` TOC after title (per Loaf reference rules)
- [ ] Loaf-context mapping notes added where vocabulary intersects with Loaf concepts (callout boxes or footnotes — not inline rewriting of source content)
- [ ] Source attribution in file header (link to mattpocock/skills)
- [ ] `loaf build` distributes both files to `plugins/loaf/skills/refactor-deepen/references/`

## Verification

```bash
loaf build
ls plugins/loaf/skills/refactor-deepen/references/{language.md,deepening.md}
wc -l content/skills/refactor-deepen/references/{language.md,deepening.md}
```

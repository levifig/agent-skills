---
id: TASK-161
title: 'references/interface-design.md (original authoring, 3-agent unprimed default)'
status: done
priority: P2
created: '2026-05-02T01:25:49.088Z'
updated: '2026-05-02T01:25:49.088Z'
completed_at: '2026-05-02T03:35:00.000Z'
spec: SPEC-034
depends_on:
  - TASK-159
---

# TASK-161: references/interface-design.md (original authoring, 3-agent unprimed default)

## Description

**Original authoring — NOT a verbatim port.** Matt's INTERFACE-DESIGN.md assumes domain-agnostic agents and primes each with opposing constraints (Agent 1: minimal, Agent 2: flexibility-max, Agent 3: common-caller). Loaf's design (per shape decision) is **3 agents with identical briefs** — diversity from sampling, not from manufactured opposition. Loaf has typed agent profiles (implementer/reviewer/researcher/librarian); document which profile to use for the design sub-agents (likely `researcher`). Document escalation rule (more agents or specific lenses opt-in only), cost note (3 × deep exploration per invocation), and fallback if accidental convergence (more agents or rerun, NOT priming).

## File Hints

- `content/skills/refactor-deepen/references/interface-design.md` (new — original work)
- Reference Matt's source for context only: https://github.com/mattpocock/skills/tree/main/skills/engineering/improve-codebase-architecture/INTERFACE-DESIGN.md

## Acceptance Criteria

- [ ] File exists with `## Contents` TOC (will be >100 lines)
- [ ] Default rule documented: spawn exactly 3 sub-agents with identical briefs (no opposing-constraint priming)
- [ ] Loaf agent profile mapping section: which profile to use for design sub-agents, why
- [ ] Escalation rule: more agents or specific design lenses are opt-in via explicit user request only
- [ ] Cost note: token usage approximately 3 × deep exploration per invocation; pattern is opt-in inside grilling loop, not auto-triggered
- [ ] Fallback section: if convergence observed in practice, response is more agents or rerun with different seeds — NOT priming
- [ ] Cross-references `references/language.md` for vocabulary
- [ ] Source attribution: notes that this is Loaf-specific, inspired by Matt's source but with the priming choice deliberately reversed (link to relevant ideas/SPEC-034)

## Verification

```bash
loaf build
ls plugins/loaf/skills/refactor-deepen/references/interface-design.md
grep -q "exactly 3 sub-agents" content/skills/refactor-deepen/references/interface-design.md
grep -q "identical briefs" content/skills/refactor-deepen/references/interface-design.md
```

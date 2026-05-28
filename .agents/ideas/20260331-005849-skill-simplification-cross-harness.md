---
title: Skill simplification and cross-harness portability
status: absorbed
absorbed-by: specs/SPEC-020-target-convergence-amp.md
created: 2026-03-31T00:58:49Z
tags: [skills, descriptions, cross-harness, plugin, portability]
related:
  - specs/SPEC-014-skill-activation-redesign.md
  - specs/SPEC-020-target-convergence-amp.md
---

# Skill Simplification & Cross-Harness Portability

## Core Idea

Thin out and simplify skills with more assertive ("pushy") descriptions that give the model confidence in routing decisions. Ship all skills as cross-harness compatible through the plugin/packaging system.

## Context

Emerged from the SPEC-014 description audit session where we discovered:
- All 30 skill descriptions exceeded Claude Code's 250-char truncation budget
- Narrow, focused skills with temporal proximity outperform broad catch-all skills
- The description is the *only* routing signal for non-invocable skills
- Skills are always loaded (no lazy loading) — every description competes for attention

## Key Tensions

- **Specificity vs. brevity**: More trigger phrases improve routing but eat the 250-char budget
- **Per-harness vs. universal**: Each harness (Claude Code, Cursor, Codex, Gemini) has different skill loading mechanics — descriptions need to work across all of them
- **Skill count vs. focus**: Fewer skills = less routing ambiguity, but merging loses temporal proximity advantage

## Next Steps

- Shape into spec if pursuing: `/loaf:shape`
- Connects to SPEC-020 (target convergence) for the cross-harness packaging angle

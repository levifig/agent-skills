---
id: TASK-041
title: 'Bootstrap skill — Core SKILL.md (detection, brief, interview, doc population)'
spec: SPEC-013
status: done
priority: P1
created: '2026-03-27T02:59:45.942Z'
updated: '2026-03-27T11:02:16.932Z'
completed_at: '2026-03-27T11:02:16.931Z'
---

# TASK-041: Bootstrap skill — Core SKILL.md (detection, brief, interview, doc population)

## Description

Write the core `/bootstrap` SKILL.md — the main deliverable of SPEC-013. This is the intelligent layer that interviews users and populates project documents.

The interview guide reference file already exists at `content/skills/bootstrap/references/interview-guide.md`.

**Files:** `content/skills/bootstrap/SKILL.md`, `content/skills/bootstrap/SKILL.claude-code.yaml`

**SKILL.md covers:**
- Frontmatter: name, description with user-intent phrases ("how do I start a new project?", "set up Loaf") and negative routing ("Not for feature specs — use /shape")
- State detection: auto-classify brownfield / greenfield+brief / greenfield+empty using signal table
- Brief intake: four forms (text in prompt, file path, folder path, no input)
- Brief-as-artifact: persist/synthesize to `docs/BRIEF.md` with source frontmatter
- Interview flow: three depths referencing `references/interview-guide.md`
- Document population: VISION.md (always), STRATEGY.md + ARCHITECTURE.md (conditional), AGENTS.md (incremental with detected stack + recommended Loaf skills)
- Structured review rounds with AskUserQuestion

**Sidecar (`SKILL.claude-code.yaml`):**
- `user-invocable: true`
- `argument-hint: "[brief or path]"`
- `allowed-tools: Read, Write, Edit, Bash, Glob, Grep, AskUserQuestion`

## Acceptance Criteria

- [ ] SKILL.md has valid Agent Skills frontmatter (name, description)
- [ ] Description starts with action verb, includes user-intent phrases and negative routing
- [ ] State detection section with signal table for all three modes
- [ ] Brief intake handles all four input forms
- [ ] Brief-as-artifact section with `docs/BRIEF.md` persistence and frontmatter schema
- [ ] Interview flow references `references/interview-guide.md` for all three depths
- [ ] Document population covers VISION (always), STRATEGY/ARCHITECTURE (conditional), AGENTS.md (incremental)
- [ ] Structured review rounds described with AskUserQuestion
- [ ] Sidecar has correct fields
- [ ] `loaf build --target claude-code` succeeds and skill appears in output

## Verification

```bash
loaf build --target claude-code
ls plugins/loaf/skills/bootstrap/
```

## Context
See SPEC-013 — full spec. Interview guide: `content/skills/bootstrap/references/interview-guide.md`.

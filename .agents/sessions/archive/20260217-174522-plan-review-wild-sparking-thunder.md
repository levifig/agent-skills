---
session:
  title: "review implementation plan (wild-sparking-thunder)"
  status: archived
  created: "2026-02-17T00:00:00Z"
  archived_at: "2026-03-27T23:06:02Z"
  archived_by: cleanup
---

# Session: review implementation plan (wild-sparking-thunder)

**Date:** 2026-02-17
**Owner:** orchestrating PM

## User request
- Review current implementation plan and interview for clarifications as needed.

## Notes
- Original repo-relative path was not found; user supplied absolute path.
- Authoritative plan reviewed at `/Users/levifig/.claude/plans/wild-sparking-thunder.md`.
- Plan is strong but has unresolved architectural decisions (OpenCode routing contract, template ownership, cutover strategy).
- Multi-agent review complete (explore/backend/qa) with consensus: feasible via phased rollout, high risk if big-bang migration.
- User preference: no backward-compatibility period required.
- Open question from user: whether Cursor/OpenCode should continue emitting commands as adapters over skills.
- User decision: generate commands from skills for OpenCode only; Cursor should be skills-only.
- User concern: build-time copied templates are acceptable if single source exists; asked about long-term conflict risks.
- User requested a pros/cons comparison for OpenCode command-derivation metadata placement (`SKILL.opencode.yaml` vs `targets.yaml` vs hybrid).
- Decision: user accepted recommendation to use `SKILL.opencode.yaml` as canonical OpenCode command-derivation metadata.
- Reviewed external reviewer plan: `.claude/plans/enumerated-moseying-eich.md` (v3).
- PM verdict: strong direction but not implementation-ready without edits (skill/command coexistence collision risk, docs scope gaps, Cursor stale command cleanup gap, and a few verification inaccuracies).
- Applied requested in-place revisions; reviewer plan is now `v3.1` with sequencing and verification fixes.
- Added final installer hygiene requirement to v3.1 (`install.sh` removes stale Cursor command files on upgrade).

## Plan references
- .agents/plans/20260217-174818-wild-sparking-thunder-closeout-review.md (pending user confirmation)
- .agents/plans/20260217-175455-wild-sparking-thunder-review.md (pending user confirmation)
- .agents/plans/20260217-210745-migrate-commands-to-skills-v2.md (pending user approval)
- .agents/plans/20260217-212637-migrate-commands-to-skills-v2-1.md (pending user approval; supersedes v2)

# H1 — Pitch interview dogfood

**Date:** 2026-07-30  
**Scale:** project (greenfield)  
**Candidate commit:** `f275f7bc`  
**Method:** user-invoked checkpoint on a real new greenfield project (not a scratch exercise). The candidate plugin was loaded from this repository's own working tree at commit `f275f7bc` — the user's Claude config targets `plugins/loaf` in this repo directly, so no `--plugin-dir` was needed; the candidate is pinned by construction. The produced `docs/BRIEF.md` remains in the user's project and is not copied here.

## Outcome

- The interview ran end-to-end.
- `docs/BRIEF.md` was created with the shared problem-space skeleton (`source: pitch`).
- User verdict: **"the pitch went well."**

## User reaction (drives revisions)

The ceremony ends abruptly — no closing summary, no explanation of what was authored, no clear instructions for what comes next (the `/bootstrap` handoff). Separately, the interview needs more depth.

No project names, BRIEF body content, or other sensitive material from the dogfood project are recorded here. This note records the run, the reaction, and the revision list only.

## Revisions produced

1. **Closing ceremonies at both scales** (`content/skills/pitch/SKILL.md`):
   - **Project scale:** end with a section-by-section gist of the BRIEF, the artifact path (`docs/BRIEF.md`), and an explicit handoff — next, run `/bootstrap`: it reads this BRIEF as discovery-done (`source: pitch`), interviews only on gaps, populates the operating documents, and closes by proposing the initial arc of captured changes. Announce completion; never trail off.
   - **Change scale:** end with a recap of the brief and the change folder created, then restate the landing option actually taken (shape now / park targeted / park untargeted) and what each means next. Announce completion.

2. **Interview depth pass** (`content/skills/pitch/references/interview-guide.md`) — five imports, Loaf voice (concepts, not wholesale copies):
   - **Destination pinning** — open by pinning a one-or-two-line end state before dimension grilling; project scale feeds VISION success criteria; change scale sharpens the eventual Hypothesis.
   - **Specifiability test for Open Questions** — a precise entry only if the question can be *stated* precisely now (not answered now); vaguer items stay coarse notes.
   - **HITL/AFK tags** — each precise open question marked needs-human-live vs agent-runnable; tags land in the brief's Open Questions section.
   - **Scenario stress-testing** — probe with concrete lived scenarios and invented edge cases to force boundary precision.
   - **Challenge stance** — demand specificity, probe rationale, push back on vagueness; flag terms that conflict with `docs/knowledge/glossary.md` and sharpen fuzzy terms to a canonical choice mid-interview.

## Related seam (TASK-003, same finalization)

Bootstrap gap mode's session-start opening now acknowledges the pitched BRIEF (summarize what pitch captured; state what bootstrap will do next) so the handoff reads as one continuous flow. Series-prep mint granularity adopts the same **specifiability** phrasing: a concept mints only if its problem can be stated precisely now.

## H4

H4 evidence will be appended later.

---
title: "Report: Loaf Skills Deep Audit"
type: audit
created: 2026-06-20T21:44:48Z
status: final
source: manual
report_alias: report-loaf-skills-deep-audit
tags:
  - skills
  - harnessing
  - sqlite-state
  - build-targets
  - taxonomy
---

# Report: Loaf Skills Deep Audit

**Question:** What improvements are needed across all Loaf-shipped skills to tighten focus, improve harnessing, reduce clutter, and make the shipped experience more reliable?

## Summary

Loaf's skill system is directionally strong: the verb-led workflow skills and noun-led reference skills are the right foundation. The current shipped surface is not failing because it has too many skills in principle; it is failing because several high-impact skills and target harnesses have drifted from the current SQLite-native runtime, cross-target build behavior, and the intended progressive-disclosure model.

The highest-priority work is not prose polish. It is harness correctness, session-model convergence, and taxonomy visibility. Amp currently ships a plugin file that is not valid JavaScript. Codex hook generation appears to harden advisory workflow hooks into fail-closed hooks. OpenCode omits command files for several intended workflow skills. Major skills still teach markdown-era session file editing even though Loaf's operational state is SQLite-backed. After those are fixed, the next priority is reducing skill clutter by turning roots back into routers, hiding reference-only skills, refreshing routing evals, and cleaning stale cross-skill references.

Confidence: High. Findings are based on direct source inspection, generated artifact inspection, subagent audit reports, and local verification commands.

## Methodology

The audit used five parallel read-only review streams:

1. Workflow/process skills review.
2. Domain/reference skills review.
3. Harness, build, sidecar, hook, and target-output review.
4. Cross-skill taxonomy and discoverability review.
5. Templates, references, and link integrity review.

The coordinator also ran direct mechanical checks:

- Counted all source `SKILL.md` files and large markdown assets.
- Compared source skills, sidecars, generated `dist/*` outputs, and `plugins/loaf`.
- Inspected `config/hooks.yaml`, `config/targets.yaml`, native build code, and routing eval scripts.
- Verified selected high-risk findings locally.

Verified commands included:

```bash
find content/skills -mindepth 2 -maxdepth 2 -name 'SKILL.md' | sort | wc -l
find content/skills -mindepth 2 -maxdepth 2 -name 'SKILL.md' -print0 | xargs -0 wc -l
node --check dist/amp/plugins/loaf.js
python3 - <<'PY'
try:
 import yaml
 print('yaml import ok')
except Exception as e:
 print(type(e).__name__ + ': ' + str(e))
PY
test -f dist/opencode/commands/ship.md
test -f dist/opencode/commands/release.md
test -f dist/opencode/commands/bootstrap.md
```

## Key Findings

1. Amp target output is not runnable JavaScript.
   Confidence: High.

2. Codex hook generation turns advisory workflow hooks into fail-closed looking hooks.
   Confidence: High.

3. OpenCode command harnessing is incomplete for intended workflow skills.
   Confidence: High.

4. Session guidance is split between SQLite-native state and markdown-era session files.
   Confidence: High.

5. Transcript archival guidance conflicts with SPEC-040's redaction boundary.
   Confidence: High.

6. Workflow skills mostly log outcomes, not invocation, despite Loaf's self-logging rule.
   Confidence: High.

7. Progressive disclosure is breaking down: several roots are too long, too broad, or too policy-heavy.
   Confidence: High.

8. Skill taxonomy and routing evals are stale.
   Confidence: High.

9. Script guidance is split between native `loaf check`, hooks, and unwired skill-local helpers.
   Confidence: High.

10. Several concrete contradictions and stale references should be fixed before broad polish.
   Confidence: High.

## Detailed Analysis

### 1. Harness Correctness

Amp is the most urgent harness issue. The installed Amp plugin path is generated as `dist/amp/plugins/loaf.js`, but it contains TypeScript syntax:

- `dist/amp/plugins/loaf.js:38` contains `interface HookResult`.
- `node --check dist/amp/plugins/loaf.js` fails with `SyntaxError: Unexpected strict mode reserved word`.
- `dist/amp/plugins/loaf.js:383` references `call.toolName` inside a handler whose parameter is named `input`.

This is a release-blocking target issue because it means Loaf ships an Amp plugin that cannot even parse as JavaScript.

Codex hook generation is also high risk. In `config/hooks.yaml`, push and PR workflow hooks are explicitly advisory:

- `config/hooks.yaml:37-39` says these hooks are safety nets, not gates.
- `config/hooks.yaml:40-45` sets `validate-push` as `blocking: false`.
- `config/hooks.yaml:48-52` starts `workflow-pre-pr` as advisory.

Generated Codex output does not preserve that nuance:

- `dist/codex/.codex/hooks.json:21-35` emits `validate-push` and `workflow-pre-pr` with `failClosed: true`.
- There is no generated `if` condition in that hook object.

The result is semantic drift between source hook policy and target behavior. Even if Codex happens to interpret the hook layer leniently, the shipped artifact communicates the wrong contract.

OpenCode command generation is incomplete. Commands are only generated for skills with `SKILL.opencode.yaml`, but several Claude-invocable workflow skills do not have OpenCode command files:

- Missing `dist/opencode/commands/ship.md`.
- Missing `dist/opencode/commands/release.md`.
- Missing `dist/opencode/commands/bootstrap.md`.
- Missing `dist/opencode/commands/refactor-deepen.md`.

Recommendation: make workflow command exposure target-explicit. Either every intended workflow has target sidecars, or there is a canonical command allowlist that build tests enforce.

### 2. SQLite-Native Session Model Drift

Loaf's current operational model is one global SQLite database, with markdown as compatibility/source/export material. However, several skills still teach agents to directly create, edit, enrich, archive, or rely on `.agents/sessions/*.md` files as the primary state surface.

High-risk examples:

- `content/skills/implement/SKILL.md:249-273` mandates creating and continuously updating a session file before work.
- `content/skills/orchestration/SKILL.md:27-32` instructs session-file creation and archive behavior.
- `content/templates/session.md:1-68` still defines the canonical-looking session file template.
- `content/skills/orchestration/references/context-management.md` still orients around `## Current State` and old compaction behavior.

`wrap` is the closest current model. It already uses `loaf session list --json`, `loaf session show <session-ref> --json`, and `loaf session end --wrap`, and explicitly says not to invent missing active markdown session files in SQLite mode.

Recommendation: make `wrap` plus SPEC-040 the canonical session model, then rewrite all session-related skill references around `loaf session start/list/show/log/end --wrap`.

### 3. Transcript Archival Risk

`content/skills/implement/references/session-management.md:220-234` instructs agents to copy full transcripts into `.agents/transcripts/`. This conflicts with SPEC-040's decision that raw transcript capture remains harness-native and out of scope until redaction controls exist:

- `.agents/specs/SPEC-040-sqlite-backed-loaf-operational-state.md:456`
- `.agents/specs/SPEC-040-sqlite-backed-loaf-operational-state.md:458`

Recommendation: remove or quarantine transcript archival guidance immediately. Reintroduce it only through an explicit design that covers redaction, storage location, gitignore policy, and report export boundaries.

### 4. Workflow Skill Invocation Logging

Project guidance says user-invocable workflow skills must log invocation as their first action. Current skill coverage is inconsistent.

Good examples:

- `handoff` logs invocation first.
- `refactor-deepen` logs invocation first.
- `wrap` logs invocation first.

Many others log only completion, decisions, or outcomes:

- `architecture`
- `bootstrap`
- `brainstorm`
- `breakdown`
- `council`
- `idea`
- `research`
- `shape`
- `ship`
- `release`
- `strategy`
- `reflect`
- `triage`

Recommendation: add a standard first-action rule to every user-invocable workflow skill:

```bash
loaf session log "skill(<name>): <intent>"
```

Where the workflow may run before session initialization, the skill should say to attempt logging when session state is available and continue without fabricating state if not.

### 5. Progressive Disclosure and Root Skill Size

The corpus has 35 source skills. Top-level `SKILL.md` files total roughly 6.3k lines, and skill markdown/scripts total roughly 22k lines. That is manageable only if root files behave as routers.

Outliers:

- `content/skills/cli-reference/SKILL.md` is 908 lines.
- `content/skills/bootstrap/SKILL.md` is 434 lines.
- `content/skills/implement/SKILL.md` is 361 lines.
- `content/skills/breakdown/SKILL.md` is 329 lines.
- `content/skills/refactor-deepen/SKILL.md` is 322 lines.
- `content/skills/release/SKILL.md` is 289 lines.

The worst case is `cli-reference`: it is generated, hidden for Claude, and useful, but it violates the intended root overview pattern and should either be split into references or documented as a deliberate generated exception.

Recommendation: root files should contain only:

- Trigger and non-trigger rules.
- Critical rules.
- Verification.
- Quick reference.
- Topic routing table.
- A few workflow steps when necessary.

Long policy blocks, examples, and command inventories should live under `references/` or generated command-specific files.

### 6. Taxonomy and Visibility

The taxonomy is basically right, but the visible surface needs tiers.

Recommended tiers:

Core workflows:

- `bootstrap`
- `idea`
- `brainstorm`
- `triage`
- `shape`
- `breakdown`
- `implement`
- `ship`
- `release`
- `wrap`

Advanced workflows:

- `architecture`
- `council`
- `research`
- `strategy`
- `reflect`
- `handoff`
- `housekeeping`
- `refactor-deepen`

Reference packs:

- `foundations`
- `git-workflow`
- `documentation-standards`
- `security-compliance`
- `debugging`
- language skills
- domain skills
- `cli-reference`
- `orchestration`

Candidate changes:

- Hide `debugging` unless it becomes a full workflow.
- Rename or absorb `thermo-nuclear-code-quality-review`; it currently reads like an imported personal skill and lacks Loaf structure/sidecars.
- Keep `orchestration` hidden as an internal coordination reference unless Loaf intentionally exposes it as a user command.
- Consider install profiles or opt-in packs for language/domain reference skills so every Loaf install does not carry every domain by default.

### 7. Routing Eval Drift

`cli/scripts/eval-skill-routing.mjs` is stale:

- It references non-existent skills such as `council-session`, `cleanup`, `resume-session`, and `reference-session`.
- It expects some prompts to route to `idea` even though `triage` owns queue processing.
- It does not sufficiently test conflicts such as `ship` vs `release`.

Recommendation: refresh routing evals around current skill names and add conflict prompts for:

- `idea` vs `triage`
- `research` vs `brainstorm`
- `strategy` vs `reflect`
- `ship` vs `release`
- `architecture` vs `shape`
- `debugging` vs language skills

### 8. Cross-Target Language Leakage

Generated target outputs still contain harness-specific assumptions.

Examples reported and spot-checked:

- Amp bootstrap says the skill is designed for Claude Code.
- OpenCode implement includes Claude Code session naming guidance.
- Codex ships Claude-oriented session resume guidance.
- Non-Claude target outputs include phrases like `AskUserQuestion`, `Task tool`, and `TodoWrite`.

Recommendation: add cross-target content lint for non-Claude outputs:

```bash
rg -n 'Claude Code|CLAUDE|AskUserQuestion|Task tool|TodoWrite|\\.claude' dist/{opencode,codex,gemi...
```

Then either transform those phrases per target or isolate them into Claude-only sidecars/references.

### 9. Script and Validation Policy Drift

Loaf currently has three kinds of validation logic:

- Native `loaf check` hook IDs.
- Hook registrations in `config/hooks.yaml`.
- Skill-local scripts in `content/skills/*/scripts`.

The problem is not that helper scripts exist. The problem is that some are presented as authoritative while not being wired into tests or native checks.

Examples:

- `loaf check` recognizes only five hook IDs in the current native check runner.
- `foundations` has seven scripts but only lists a subset.
- `infrastructure-management/scripts/validate-k8s-manifest.py` imports `yaml`, but there is no declared Python dependency and local `import yaml` fails.
- `power-systems-modeling` sidecar allows only `Bash(python:*)` while the skill exposes a shell script.

Recommendation: classify every script as one of:

- CLI-owned: promoted to native `loaf check` or another native command and covered by Go tests.
- Hook-owned: invoked from `config/hooks.yaml` and covered by build/hook tests.
- Skill-local helper: optional, documented as a helper, not an enforcement mechanism.
- Retired: removed from distributed outputs.

### 10. Concrete Content Contradictions

Fix these before broad prose cleanup:

- `git-workflow` says commit format is `type(scope): description`, while `commits.md` and native check code ban scoped commits.
- `knowledge-base` routes agent instructions to `CLAUDE.md` instead of the current AGENTS entrypoint.
- `database-design` refers to nonexistent `infrastructure`.
- `power-systems-modeling` refers to nonexistent `database-patterns`.
- `foundations/references/code-style.md` refers to `python`, `typescript`, and `rails` instead of current skill names.
- `triage` has a stray closing code fence after the deferral text.
- `shape/templates/spec.md` lacks `source_sessions`.
- `documentation-standards` points an example ADR link through a path that is wrong relative to the reference file.
- `refactor-deepen` links to an active `.agents/specs/SPEC-034...` path even though the file lives in archive, and distributed skill content should not rely on local `.agents` history.

## Recommended Work Plan

### Track 0: Stop Shipping Broken Harnesses

Goal: target outputs are executable and hook semantics match source policy.

Tasks:

1. Fix Amp plugin generation.
   - Remove TypeScript syntax from emitted `.js`, or ship/transpile as TypeScript intentionally.
   - Replace the undefined `call` variable with the actual hook input shape.
   - Add `node --check dist/amp/plugins/loaf.js` to tests or build verification.

2. Fix Codex hook generation.
   - Preserve `if`, `blocking`, and `failClosed`.
   - Generate only real enforcement hooks as fail-closed.
   - Ensure advisory hooks cannot block unexpectedly.

3. Fix OpenCode command exposure.
   - Add sidecars for all intended OpenCode commands, or define and test a command allowlist.
   - Decide explicitly whether `debugging` is visible.

4. Implement or correct target sidecar support.
   - Either merge `.cursor.yaml`, `.codex.yaml`, `.gemini.yaml`, and `.amp.yaml` from `content/skills`, or remove the documentation claims from `targets.yaml`.

Validation:

```bash
node --check dist/amp/plugins/loaf.js
go test ./internal/cli -run 'TestRunnerBuildTarget(Codex|Amp|OpenCode|Cursor|ClaudeCode)'
npm run build
git diff --exit-code -- dist plugins .claude-plugin
```

### Track 1: Canonicalize SQLite-Native Session Guidance

Goal: all skills use current Loaf state surfaces and treat markdown files as compatibility/source/export artifacts.

Tasks:

1. Make `wrap` the model for session shutdown and resumption.
2. Rewrite:
   - `implement`
   - `orchestration/SKILL.md`
   - `orchestration/references/sessions.md`
   - `orchestration/references/context-management.md`
   - `bootstrap`
   - `handoff`
   - `council`
   - `research`
   - `reflect`
3. Update or retire `content/templates/session.md`.
4. Remove transcript archival guidance until redaction/storage policy is designed.
5. Add `source_sessions` to spec provenance in `shape/templates/spec.md`.

Validation:

```bash
rg -n 'Create session file|active session file|Update session file|session frontmatter|\\.agents/sessions' content/skills
rg -n 'transcripts|Transcript Archival|cp /path/to/transcript' content/skills
loaf session list --json
loaf session show <active-or-recent-session> --json
```

### Track 2: Normalize Skill Structure

Goal: every source skill follows the Loaf skill contract or has an explicit generated exception.

Tasks:

1. Add missing standard sections:
   - `## Contents`
   - `## Critical Rules`
   - `## Verification`
   - `## Quick Reference`
   - `## Topics`
2. Prioritize:
   - `thermo-nuclear-code-quality-review`
   - `shape`
   - `strategy`
   - `bootstrap`
   - `council`
   - `brainstorm`
   - `housekeeping`
   - `triage`
   - `wrap`
3. Add missing sidecars, especially for `thermo-nuclear-code-quality-review`.
4. Add first-action self-logging to all user-invocable workflow skills.
5. Decide whether `cli-reference` is split or marked as a generated exception.

Validation:

```bash
python3 scripts-or-new-check skill-structure-lint
find content/skills -mindepth 2 -maxdepth 2 -name 'SKILL.md' -print0 | xargs -0 wc -l | sort -nr
```

### Track 3: Restore Progressive Disclosure

Goal: roots route; references explain.

Tasks:

1. Move long policy sections from root `SKILL.md` files into `references/`.
2. Split generated CLI reference by command family or generated references.
3. Move repeated quick-reference tables out of roots when duplicated.
4. Convert code-span topic references into markdown links where link checking matters.
5. Extract wrap report skeleton into `templates/`.

Validation:

```bash
find content/skills -mindepth 2 -maxdepth 2 -name 'SKILL.md' -print0 | xargs -0 wc -l | sort -nr
rg -n '\\| .*`[^`]+\\.md`|`[^`]+/references/' content/skills/*/SKILL.md
```

### Track 4: Clean Taxonomy and Routing

Goal: skill discovery is predictable and visible commands are intentional.

Tasks:

1. Update `docs/knowledge/skill-architecture.md` to current counts and tiers.
2. Hide or demote `debugging`.
3. Rename or integrate `thermo-nuclear-code-quality-review`.
4. Make `research` investigation-only.
5. Make `brainstorm` divergent-thinking-only.
6. Make `idea` capture-only.
7. Make `triage` intake processing-only.
8. Keep `strategy` pre-implementation and `reflect` post-shipping.
9. Update routing evals to current skill names and conflict prompts.

Validation:

```bash
npm run eval:routing -- --model <chosen-model>
rg -n 'council-session|cleanup|resume-session|reference-session' cli/scripts content docs
```

### Track 5: Script and Hook Policy

Goal: no authoritative-but-unwired scripts.

Tasks:

1. Inventory every `content/skills/*/scripts/*`.
2. Classify each as CLI-owned, hook-owned, skill-local helper, or retired.
3. Promote important checks to native `loaf check` or target-specific hooks.
4. Remove dependencies that are not bundled or declared.
5. Add script syntax/dependency smoke checks.

Validation:

```bash
find content/skills -path '*/scripts/*' -type f -maxdepth 4
go test ./...
python3 -m py_compile content/skills/*/scripts/*.py
shellcheck content/skills/*/scripts/*.sh
```

If `shellcheck` is not required for contributors, replace with POSIX/bash syntax checks already available in the development environment.

### Track 6: Fix Concrete Contradictions

Goal: remove the known sharp edges before larger edits create churn.

Tasks:

1. Align git commit format with native check behavior.
2. Replace stale skill names:
   - `infrastructure` -> `infrastructure-management`
   - `database-patterns` -> `database-design` or another real skill
   - `python` -> `python-development`
   - `typescript` -> `typescript-development`
   - `rails` -> `ruby-development`
3. Replace `CLAUDE.md` guidance with `.agents/AGENTS.md` / AGENTS entrypoint guidance.
4. Fix `triage` stray fence.
5. Fix `shape` provenance.
6. Fix `refactor-deepen` SPEC-034 link by moving the decision summary into a stable reference file.
7. Fix Python `uv:latest` example or explicitly mark it as a placeholder that must be pinned.
8. Fix `power-systems-modeling` conversion claims and sidecar tool permissions.

Validation:

```bash
rg -n 'CLAUDE.md|database-patterns|`infrastructure`|`python` skill|`typescript` skill|`rails` skill' content/skills docs
rg -n 'type\\(scope\\): description|scoped commits' content/skills internal/cli
```

## Suggested Sequencing

Do not try to clean every skill in one PR. This should be a small series:

1. PR 1: Harness correctness.
   - Amp parse fix.
   - Codex hook semantics.
   - OpenCode command exposure.
   - Cross-target lint.

2. PR 2: SQLite session guidance convergence.
   - Rewrite session guidance.
   - Remove transcript archival guidance.
   - Add `source_sessions`.

3. PR 3: Skill structure and logging.
   - Standard sections.
   - Invocation self-log rules.
   - `thermo` sidecar/rename decision.

4. PR 4: Taxonomy and routing.
   - Update architecture docs.
   - Routing eval refresh.
   - Visibility changes.

5. PR 5: Script policy and concrete contradictions.
   - Native/script classification.
   - Stale names.
   - Commit-format contradiction.
   - PyYAML/script dependency issue.

6. PR 6: Progressive disclosure cleanup.
   - Split long roots.
   - Fix links.
   - Extract templates.

## Sources

- `content/skills/*/SKILL.md` and sidecars.
- `content/skills/*/references/*`.
- `content/skills/*/templates/*`.
- `content/templates/*`.
- `config/hooks.yaml`.
- `config/targets.yaml`.
- `internal/cli/build_*.go`.
- `internal/cli/check.go`.
- `internal/cli/cli_reference.go`.
- `internal/cli/install_target.go`.
- `dist/*/skills`.
- `dist/opencode/commands`.
- `dist/codex/.codex/hooks.json`.
- `dist/amp/plugins/loaf.js`.
- `.agents/specs/SPEC-040-sqlite-backed-loaf-operational-state.md`.
- `docs/knowledge/skill-architecture.md`.
- `cli/scripts/eval-skill-routing.mjs`.

## Open Questions

1. Should Loaf continue shipping every reference pack to every target by default, or should install profiles select language/domain packs?
2. Should `orchestration` remain hidden as a reference skill, or should it become an explicit user workflow?
3. Should `thermo-nuclear-code-quality-review` be renamed to a Loaf-native `maintainability-review`, merged into `foundations`, or kept as a sharp optional review skill?
4. Should raw source links be required to resolve before build, or is built-output link validity the only supported contract for shared templates?
5. Should `cli-reference` remain a single generated hidden skill, or should it become command-family reference files generated under `references/`?

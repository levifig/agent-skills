---
traceability:
  requirement: >-
    Close gaps between feat/target-convergence (Phase 3 behaviors) and
    opencode/hidden-wolf (Phase 4 + convergence wins)
  architecture:
    - Hook Consolidation
    - Session Journal Model
    - Target Convergence
  decisions: []
plans:
  - 20260331-222122-spec-020-consolidation-sweep.md
transcripts: []
orchestration:
  current_task: COMPLETE - SPEC-020 Consolidation Sweep finished
  spawned_agents:
    - task: 'Wave 0: Comparison checklist'
      status: completed
      note: Detailed comparison checklist saved to session file
    - task: 'Wave 1: Fix hooks.yaml'
      status: completed
      note: Enforcement semantics restored - 11 edits applied
    - task: 'Wave 2: Fix check.ts'
      status: completed
      note: Bash command secret scanning added to checkSecrets()
    - task: 'Wave 3: Journal auto-entry hooks'
      status: completed
      note: >-
        Already completed in Wave 1 (journal-post-commit, journal-post-pr,
        journal-post-merge added)
    - task: 'Wave 4: Align session.ts'
      status: completed
      note: >-
        Ported timestamped filenames, flat frontmatter, 19 entry types,
        branch-based lookup
    - task: 'Wave 5: Claude binary path'
      status: completed
      note: >-
        BINARY_PATH_HOOKS set added session/journal hooks - all paths now
        consistent
    - task: 'Wave 6: Clean stale docs'
      status: completed
      note: >-
        Removed resume-session/reference-session references from README.md,
        AGENTS.md, content/agents/, content/skills/orchestration/
    - task: 'Wave 7: Amp output naming'
      status: completed
      note: >-
        DECISION: Keep loaf.js - Amp uses JavaScript, spec .ts was documentation
        error
    - task: Full verification
      status: completed
      note: 'TypeScript passes, 440 tests pass, all 7 targets build successfully'
    - task: 'Wave 7: Amp output naming'
      status: completed
      note: >-
        DECISION: Keep loaf.js - Amp uses JavaScript, spec .ts was documentation
        error
title: >-
  SPEC-020 Consolidation Sweep: Port Phase 3 behaviors from
  feat/target-convergence
status: archived
created: '2026-03-31T22:23:41Z'
last_updated: '2026-04-04T17:27:51.184Z'
branch: opencode/hidden-wolf
spec: SPEC-020
archived_at: '2026-04-04T17:27:51.184Z'
---

# SPEC-020 Consolidation Sweep Session

## Context

This session executes a **consolidation sweep** to bring `opencode/hidden-wolf` closer to SPEC-020 completeness. The goal is to:

1. **Preserve current branch wins** from Phase 4 and convergence:
   - Amp target with runtime plugin generator
   - Fenced install logic
   - `cli-reference` skill
   - Direct `loaf check` architecture (no shell wrappers)
   - Removal of deprecated `resume-session` and `reference-session` skills

2. **Port only the stronger Phase 3 behaviors** from `feat/target-convergence`:
   - Enforcement semantics in `config/hooks.yaml`
   - Bash command text scanning in `loaf check`
   - Session journal auto-entry hooks for commit/PR/merge
   - Branch-scoped, append-only, spec-linked journal model

**Source of truth:** `.agents/plans/20260331-222122-spec-020-consolidation-sweep.md`

## Verification Results

### Commands Run
- ✅ `npm run typecheck` - TypeScript passes
- ✅ `npm run test` - 440 tests pass
- ✅ `npm run build` - All 7 targets build successfully

### Generated Outputs Verified

**Claude Code (`plugins/loaf/.claude-plugin/plugin.json`):**
- ✅ `check-secrets` matcher: `"Edit|Write|Bash"` (includes Bash commands)
- ✅ `security-audit`: `blocking: true` via `failClosed: true`
- ✅ `validate-commit`: `blocking: true` via `failClosed: true`
- ✅ `session start/end`: Uses `"${CLAUDE_PLUGIN_ROOT}/bin/loaf"` binary path
- ✅ Journal auto-entry hooks: All 3 use `"${CLAUDE_PLUGIN_ROOT}/bin/loaf"`

**Cursor (`dist/cursor/hooks.json`):**
- ✅ Uses bare `loaf` commands (no plugin root path)
- ✅ `check-secrets` matcher: `"Edit|Write|Bash"`
- ✅ Journal auto-entry hooks present

**Amp (`dist/amp/plugins/`):**
- ✅ Output: `loaf.js` (correct - Amp uses JavaScript)
- ✅ Plugin binary accessible at `plugins/loaf/bin/loaf`

### Documentation Cleaned
- ✅ README.md - Removed stale skill references
- ✅ AGENTS.md - Updated shared-templates
- ✅ content/agents/context-archiver.md - Updated references
- ✅ content/skills/orchestration/references/*.md - Updated references

### Preserved Phase 4 Work
- ✅ Amp target and runtime plugin generator
- ✅ Fenced install logic
- ✅ `cli-reference` skill
- ✅ Direct `loaf check` architecture
- ✅ Session skill removal (resume-session/reference-session stay deleted)

## Key Decisions

1. **Hook Structure (Wave 1):** Removed explicit `type: command` and `command` fields from enforcement hooks in hooks.yaml - build generator adds them automatically per SPEC-020 Phase 3.

2. **Bash Command Scanning (Wave 2):** Added command text scanning to `checkSecrets()` - secrets in Bash commands like `curl -H "Authorization: Bearer..."` are now detected.

3. **Session Journal Model (Wave 4):** Ported timestamped filenames (`YYYYMMDD-HHMMSS-slug.md`), flat frontmatter structure, 19 entry types, and branch-based session lookup.

4. **Amp Output Naming (Wave 7):** **DECISION** - Keep `dist/amp/plugins/loaf.js`. Amp uses JavaScript plugins; spec showing `.ts` was documentation error.

## Current State

- **SPEC-020 Consolidation Sweep: COMPLETE**
- All 8 waves executed
- All verifications pass
- Phase 3 behaviors ported from feat/target-convergence
- Phase 4 work preserved

## Next Steps

- [x] All waves completed
- [x] All verifications passed
- [ ] Archive this session file
- [ ] Consider running `/reflect` to update strategic docs with learnings

## Resumption Prompt

Read this session file. **SPEC-020 Consolidation Sweep is COMPLETE**. All Phase 3 behaviors have been selectively ported from feat/target-convergence while preserving all Phase 4 work. All builds pass, all tests pass, all documentation cleaned. The framework is ready for use.

## Implementation Waves

Per the plan file:

1. **Wave 0:** Reconfirm spec and create comparison checklist
2. **Wave 1:** Restore enforcement-hook correctness (hooks.yaml)
3. **Wave 2:** Restore Bash secret scanning in `loaf check`
4. **Wave 3:** Restore journal auto-entry hooks
5. **Wave 4:** Align `loaf session` with SPEC journal model
6. **Wave 5:** Fix Claude plugin binary-path consistency
7. **Wave 6:** Clean stale documentation references
8. **Wave 7:** Resolve Amp output naming deliberately

## Approval Conditions

User approved with the following guardrails:

1. **Behavior reference only:** Treat `feat/target-convergence` as behavior reference only. Do NOT cherry-pick or restore its old validation shell-hook stack, shared hook libs, or deprecated session skills.

2. **Expanded Wave 6:** Beyond top-level docs, also grep for stale `resume-session` / `reference-session` mentions in:
   - `content/agents/`
   - `content/skills/orchestration/`
   - Generated outputs if they're tracked

3. **Wave 7 as decision checkpoint:** Default to keeping `dist/amp/plugins/loaf.js` unless runtime actually requires `loaf.ts`. Do not assume implementation - verify requirement first.

## Key Decisions

*(none yet - will capture as we proceed)*

## Wave 0 Comparison Checklist

*Research completed by Ranger agent - 2026-03-31*

---

### Section 1: Must Preserve from Current Branch

These files/features exist in the current branch and MUST NOT be changed or removed during consolidation:

| # | Feature | File Path | Status | Notes |
|---|---------|-----------|--------|-------|
| 1 | **Amp target** | `cli/lib/build/targets/amp.ts` | ✅ EXISTS | Full implementation with `copySkills()` and `generateRuntimePlugin(AmpPlatform)` |
| 2 | **Runtime plugin generator** | `cli/lib/build/lib/hooks/runtime-plugin.ts` | ✅ EXISTS | 493-line unified generator with `AmpPlatform` and `OpenCodePlatform` adapters |
| 3 | **Fenced install logic** | `cli/lib/install/fenced-section.ts` | ✅ EXISTS | Complete implementation with `installFencedSection()`, `TARGET_FILES` mapping for all 5 targets |
| 4 | **cli-reference skill** | `content/skills/cli-reference/SKILL.md` | ✅ EXISTS | 286-line reference skill with command substitution table |
| 5 | **Direct loaf check architecture** | `cli/commands/check.ts` | ✅ EXISTS | No shell wrappers — direct CLI commands in hook configs |
| 6 | **Session skill removal** | N/A | ✅ VERIFIED | `resume-session` and `reference-session` skills DO NOT exist in `content/skills/` — already deleted as required |

**Critical Preservation Notes:**

1. **Amp target output naming**: Current `amp.ts` generates `dist/amp/plugins/loaf.js` (line 92 in runtime-plugin.ts). The spec mentions `loaf.ts` in Phase 4 but this is NOT a discrepancy — the runtime-plugin correctly generates `.js` for Amp (line 92: `outputFile = platform.platform === "opencode" ? "hooks.ts" : "loaf.js"`). ✅ **No action needed.**

2. **Runtime plugin architecture**: The current `runtime-plugin.ts` has the complete `RuntimePlatform` interface with `AmpPlatform` and `OpenCodePlatform` adapters. This is MORE complete than what the spec describes. ✅ **Do not modify.**

3. **Fenced section**: Already implements full `TARGET_FILES` mapping for all targets including Amp. ✅ **Do not modify.**

---

### Section 2: Behaviors to Port from feat/target-convergence

#### 2.1 config/hooks.yaml Differences

| Hook | Current (main) | feat/target-convergence | Action Required |
|------|----------------|-------------------------|-----------------|
| **check-secrets** | `matcher: "Edit\|Write"` | `matcher: "Edit\|Write\|Bash"` | ❓ **VERIFY**: Should Bash be added? Current check.ts scans Bash command text already (line 191 in check.ts), so the matcher should probably include "Bash" |
| **check-secrets blocking** | `blocking: true` | No `blocking` field (enforcement hooks default to blocking via generator) | ⚠️ **Decision needed**: Current has explicit `blocking: true`, feat version removes `command` and `blocking` fields — let generator handle it |
| **security-audit blocking** | `blocking: false` | `blocking: true` | 🔴 **PORT**: Change to `blocking: true` per spec |
| **validate-commit blocking** | `blocking: false` | `blocking: true` | 🔴 **PORT**: Change to `blocking: true` per spec |
| **Enforcement hook structure** | Has `type: command` + `command: 'loaf check...'` | No `type` or `command` fields | 🔴 **PORT**: Remove `type: command` and `command` from enforcement hooks — let build system generate direct commands |
| **Journal auto-entry hooks** | Missing from current | `journal-post-commit`, `journal-post-pr`, `journal-post-merge` in `post-tool` | 🔴 **PORT**: Add auto-entry hooks for git commit/PR/merge |
| **Session hooks** | `session-start-loaf`, `session-end-loaf` (type: command) | `journal-session-start`, `journal-session-end` (no type — generator adds) | 🟡 **Consider**: Rename and remove `type: command` to match spec pattern |
| **failClosed enforcement hooks** | All have `failClosed: true` | All have `failClosed: true` | ✅ **No change needed** |

**Key YAML Structure Difference:**

```diff
# Current (main) - Enforcement hook with explicit command
  - id: check-secrets
    skill: security-compliance
    type: command
    command: 'loaf check --hook check-secrets'
    matcher: "Edit|Write"
    blocking: true

# feat/target-convergence - Enforcement hook (generator adds command)
  - id: check-secrets
    skill: security-compliance
    matcher: "Edit|Write|Bash"
    blocking: true
    failClosed: true
```

#### 2.2 cli/commands/check.ts Differences

| Aspect | Current (main) | feat/target-convergence | Action Required |
|--------|----------------|-------------------------|-----------------|
| **Result structure** | `CheckResult { passed, blocked, warnings, errors }` | `CheckResult { status: "pass"\|"warn"\|"block", hook, messages }` | 🔴 **PORT**: Simpler status enum pattern |
| **Secret scanning scope** | Scans `content` (Edit/Write) and `filePath` | Scans `content`, `new_string`, AND `command` (Bash) | ✅ **Already in current**: Current check.ts line 191-194 checks `getContent()` which includes file content, and there's logic for Bash — but verify it actually scans Bash commands |
| **Secret patterns** | 12 detailed patterns with names | 9 simpler regex patterns | 🟡 **Consider**: Port simplified patterns from feat version |
| **security-audit result** | `blocking: false` (returns warnings) | `status: "block"` when dangerous patterns found | 🔴 **PORT**: Must block on dangerous commands |
| **validate-commit result** | `blocking: false` (non-blocking by default) | `status: "block"` on invalid format | 🔴 **PORT**: Must block on invalid commits |
| **validate-push result** | `blocking: true` with multiple checks | `status: "warn"` on force push/main push | 🟡 **Consider**: Current is more thorough — verify if feat version is sufficient |
| **Hook context parsing** | Manual field extraction | `parseHookContext()` normalizes flat/nested | 🔴 **PORT**: Use normalized context parsing |

**Bash Command Scanning Verification:**

Current check.ts does NOT appear to scan Bash command content for secrets:
```typescript
// Current (lines 189-195)
const filePath = getFilePath(context);
const content = getContent(context);  // Gets Edit/Write content, NOT Bash command
```

feat/target-convergence version explicitly scans commands:
```typescript
// feat version - checkSecrets()
if (ctx.toolInput.command) {
  contentToScan.push(ctx.toolInput.command as string);
}
```

**🔴 ACTION REQUIRED**: Update current `checkSecrets()` to scan `command` field when tool is Bash.

#### 2.3 cli/commands/session.ts Differences

| Aspect | Current (main) | feat/target-convergence | Action Required |
|--------|----------------|-------------------------|-----------------|
| **Filename format** | `{sanitized-branch}.md` (e.g., `feat-target-convergence.md`) | `{YYYYMMDD}-{HHMMSS}-{slug}.md` (e.g., `20260331-222341-target-convergence.md`) | 🔴 **PORT**: Timestamped+slugged format per spec |
| **Frontmatter structure** | Nested `session: { title, status, created, ... }` | Flat `branch, status, created, spec` | 🔴 **PORT**: Simpler flat frontmatter |
| **Journal entry types** | 15 types (resume, pause, commit, pr, merge, decide, discover, conclude, block, unblock, spark, todo, assume, progress) | 19 types (adds: branch, task, linear, hypothesis, try, reject, compact) | 🔴 **PORT**: Add missing entry types to `VALID_ENTRY_TYPES` |
| **Auto-entry hook support** | `loaf session log --from-hook` exists | More robust parsing for commit/PR/merge | 🟡 **Consider**: Port improved `--from-hook` parsing |
| **Spec task progress** | `countTasksInSession()` counts checkboxes | `getTaskProgress()` queries `.agents/tasks/` | 🔴 **PORT**: Link to spec task files, not just session checkboxes |
| **Entry count on output** | Shows 15 recent entries | Same pattern | ✅ **No change needed** |
| **Session frontmatter fields** | Has `traceability`, `plans`, `transcripts`, `orchestration` | Minimal: `branch, status, created, spec` | 🔴 **PORT**: Remove unused complex frontmatter |

**Critical Filename Change:**

```diff
# Current (line 181-183)
function getSessionFilePath(agentsDir: string, branch: string): string {
  const sanitized = branch.replace(/[^a-zA-Z0-9-_]/g, "-").replace(/-+/g, "-");
  return join(agentsDir, "sessions", `${sanitized}.md`);
}

# feat/target-convergence
function createSessionFile(sessionsDir, branch, spec) {
  const slug = slugify(branch.replace(/^feat\//, ""));
  const datePrefix = now.slice(0, 10).replace(/-/g, "");
  const timePrefix = now.slice(11, 16).replace(":", "");
  const filename = `${datePrefix}-${timePrefix}-${slug}.md`;
}
```

**🔴 ACTION REQUIRED**: Port timestamped filename format to prevent collisions and enable chronological sorting.

**Journal Auto-Entry Hooks (Missing from Current):**

The feat/target-convergence `hooks.yaml` has these auto-entry hooks in `post-tool`:
```yaml
- id: journal-post-commit
  skill: orchestration
  command: loaf session log --from-hook
  matcher: "Bash"
  if: "Bash(git commit:*)"

- id: journal-post-pr
  skill: orchestration
  command: loaf session log --from-hook
  matcher: "Bash"
  if: "Bash(gh pr create:*)"

- id: journal-post-merge
  skill: orchestration
  command: loaf session log --from-hook
  matcher: "Bash"
  if: "Bash(gh pr merge:*)"
```

**🔴 ACTION REQUIRED**: Add auto-entry hooks to current `hooks.yaml`.

---

### Section 3: Current Branch Cleanup Needed

#### 3.1 Claude Plugin Path Consistency

**Issue**: Inconsistent binary path references in hook commands.

| Location | Current Path | Should Be |
|----------|--------------|-----------|
| `hooks.yaml` session hooks (lines 217, 224) | `loaf session start` | `"${CLAUDE_PLUGIN_ROOT}/bin/loaf" session start` |
| `hooks.yaml` enforcement hooks (e.g., line 24) | `loaf check --hook check-secrets` | `"${CLAUDE_PLUGIN_ROOT}/bin/loaf" check --hook check-secrets` |

**Spec Reference** (SPEC-020 Phase 3, line 622-626):
```jsonc
// Claude Code (plugin.json)
"SessionStart": [{ "command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" session start" }]
// Cursor (hooks.json)
"sessionStart": [{ "command": "loaf session start" }]
```

Claude Code needs `${CLAUDE_PLUGIN_ROOT}/bin/loaf` because it's plugin-bundled. Cursor/Codex use PATH-based `loaf`.

**🔴 ACTION REQUIRED**: 
1. Keep `hooks.yaml` using bare `loaf` commands (source of truth)
2. Build system should substitute `${CLAUDE_PLUGIN_ROOT}/bin/loaf` for Claude Code target only
3. Verify build system handles this substitution

#### 3.2 Stale Documentation References

**Found in README.md:**
- Line 38: `/resume-session after context loss`
- Line 69: `/resume-session` in command table
- Line 79: `/reference-session` in command table  
- Line 126: `reference-session` in skill categories table
- Line 127: `resume-session` in skill categories table
- Line 181: Note mentions "Deprecated skills: `resume-session`, `reference-session`"

**Found in AGENTS.md:**
- Line 217: `session.md: [implement, resume, orchestration, reference-session, review-sessions]`

**🔴 ACTION REQUIRED**:
1. Remove `/resume-session` and `/reference-session` references from README.md lines 38, 69, 79, 126, 127
2. Keep line 181 (deprecation note) but update to say "removed" not "deprecated"
3. Update AGENTS.md line 217 to remove `resume-session` and `reference-session` from shared-templates

#### 3.3 Amp Output Naming Discrepancy

**Spec says** (line 755): `dist/amp/plugins/loaf.ts`
**Current generates**: `dist/amp/plugins/loaf.js`

**Analysis**: 
- Spec shows `.ts` but this is likely a documentation oversight
- `runtime-plugin.ts` line 92 correctly generates `.js` for Amp (non-OpenCode platforms)
- Amp is JavaScript-based, not TypeScript
- The generated file IS JavaScript (uses `export default { ... }` not TypeScript syntax)

**✅ VERDICT**: Current behavior is CORRECT. The spec has a minor documentation error showing `.ts` instead of `.js`. Amp uses JavaScript plugins, so `.js` is correct.

#### 3.4 Session Journal Auto-Entry Hooks (Missing)

Current `hooks.yaml` is missing the auto-entry hooks for git workflow events:
- `journal-post-commit` — auto-log after `git commit`
- `journal-post-pr` — auto-log after `gh pr create`
- `journal-post-merge` — auto-log after `gh pr merge`

These are in feat/target-convergence but missing from current.

**🔴 ACTION REQUIRED**: Add to `hooks.yaml` post-tool section.

#### 3.5 check.ts Bash Command Scanning Gap

Current `checkSecrets()` only scans file content (`getContent()`) but NOT Bash command text. The `check-secrets` hook has `matcher: "Edit|Write"` but NOT "Bash".

However, secrets CAN appear in Bash commands:
```bash
curl -H "Authorization: Bearer sk-abc123..." https://api.example.com
```

**🔴 ACTION REQUIRED**:
1. Add "Bash" to `check-secrets` matcher in hooks.yaml
2. Update `checkSecrets()` to scan `tool_input.command` when present

---

## Summary: Critical Actions Required

### High Priority (Must Fix Before Consolidation)

1. **hooks.yaml enforcement hook structure**: Remove `type: command` and `command` fields — let build generator add them
2. **hooks.yaml blocking flags**: Change `security-audit` and `validate-commit` to `blocking: true`
3. **hooks.yaml check-secrets matcher**: Add "Bash" to matcher
4. **cli/commands/check.ts security-audit**: Make blocking (exit 2 on dangerous patterns)
5. **cli/commands/check.ts validate-commit**: Make blocking (exit 2 on invalid format)
6. **cli/commands/check.ts checkSecrets**: Add Bash command scanning
7. **cli/commands/session.ts**: Port timestamped filename format
8. **cli/commands/session.ts**: Port simplified flat frontmatter
9. **README.md**: Remove stale resume-session/reference-session references
10. **AGENTS.md**: Update shared-templates line to remove deleted skills

### Medium Priority (Should Fix)

11. **hooks.yaml**: Add journal auto-entry hooks (journal-post-commit, journal-post-pr, journal-post-merge)
12. **cli/commands/session.ts**: Add missing entry types (branch, task, linear, hypothesis, try, reject, compact)
13. **cli/commands/session.ts**: Port `getTaskProgress()` to link spec tasks
14. **cli/commands/check.ts**: Consider simplified secret patterns from feat version

### Low Priority (Nice to Have)

15. **check.ts**: Port `parseHookContext()` normalizer for robust context handling
16. **spec documentation**: Fix line 755 to show `.js` instead of `.ts` for Amp output

---

## Files to Modify (Consolidation Target List)

| File | Changes |
|------|---------|
| `config/hooks.yaml` | Remove command fields, fix blocking flags, add Bash to check-secrets, add auto-entry hooks |
| `cli/commands/check.ts` | Make security-audit and validate-commit blocking, add Bash command scanning |
| `cli/commands/session.ts` | Port timestamped filenames, flat frontmatter, missing entry types, task progress linking |
| `README.md` | Remove stale skill references |
| `AGENTS.md` | Update shared-templates line |

---

## Files to Preserve (Do Not Touch)

| File | Reason |
|------|--------|
| `cli/lib/build/targets/amp.ts` | Already complete per spec |
| `cli/lib/build/lib/hooks/runtime-plugin.ts` | Already complete, more featureful than spec |
| `cli/lib/install/fenced-section.ts` | Already complete per spec |
| `content/skills/cli-reference/SKILL.md` | Already complete |

---

*End of Wave 0 Comparison Checklist*

## Original Session Content



Read this session file. Current state: Session and plan created, awaiting user approval to begin SPEC-020 consolidation sweep. Next: Get approval, then spawn Wave 0 agent to reconfirm spec and build comparison checklist.

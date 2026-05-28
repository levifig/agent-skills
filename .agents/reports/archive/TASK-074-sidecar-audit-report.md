---
title: 'Report: TASK-074 Sidecar Audit (SPEC-020)'
type: audit
created: '2025-03-31T00:00:00Z'
source: TASK-074
status: archived
finalized_at: '2025-03-31T00:00:00Z'
archived_at: '2026-04-24T22:21:19.833Z'
archived_by: loaf housekeeping
---
# TASK-074: Sidecar Audit Report

**Date**: 2025-03-31  
**Task**: TASK-074 for SPEC-020  
**Scope**: Audit sidecars, agent profiles, and verify description truncation

---

## Summary

✅ **All verification checks passed**

- Description truncation implemented and working for Claude Code target
- Sidecar fields verified as target-specific (no universal promotion needed)
- Agent profiles validated with proper invocability settings
- Missing `disable-model-invocation` fields added to Cursor read-only agents

---

## 1. Sidecar Audit

### Current Inventory

| Target | Skill Sidecars | Agent Sidecars | Notes |
|--------|---------------|----------------|-------|
| **Claude Code** | 31 SKILL.claude-code.yaml | 5 | Full feature support |
| **OpenCode** | 13 SKILL.opencode.yaml | 5 | Workflow skills only |
| **Cursor** | 0 | 5 | Updated: added `disable-model-invocation` |
| **Codex** | 0 | 0 | Skills only, no sidecars needed |
| **Gemini** | 0 | 0 | Skills only, no sidecars needed |

### Field Analysis: Target-Specific vs Universal

#### SKILL.md (Universal - in all targets)
- `name` - Skill identifier
- `description` - Full description (Claude Code truncates at 250 chars)

#### Claude Code Extensions (SKILL.claude-code.yaml)
```yaml
user-invocable: true|false        # CC-specific: controls /command visibility
allowed-tools: "Read, Write..."   # CC-specific: tool permissions
argument-hint: "[topic]"          # CC-specific: UI autocomplete
```

✅ **Verified**: All fields are genuinely CC-specific

#### OpenCode Extensions (SKILL.opencode.yaml)
```yaml
subtask: false                    # OC-specific: command behavior
```

✅ **Verified**: Only present for workflow skills that generate commands

#### Cursor Agent Extensions (*.cursor.yaml)
```yaml
disable-model-invocation: true    # NEW: Prevents accidental agent invocation
tools: { Read: true, ... }        # Cursor format: object with boolean values
```

✅ **Updated**: Added `disable-model-invocation: true` to reviewer and researcher

### Invocability Matrix

| Skill | CC user-invocable | CC Context | Cursor disable-model-invocation | Notes |
|-------|------------------|------------|--------------------------------|-------|
| **Knowledge Skills** (ts-dev, python-dev, db-design, etc.) | `false` | Reference | N/A | Hidden from / menu |
| **Workflow Skills** (implement, shape, research, etc.) | `true` | User-invoked | N/A | Explicit /command |
| **Implementer** (Agent) | N/A | Spawned | `false` (default) | Full write access |
| **Reviewer** (Agent) | N/A | Spawned | **`true`** ← UPDATED | Read-only, prevents accidental invocation |
| **Researcher** (Agent) | N/A | Spawned | **`true`** ← UPDATED | Read-only + web, prevents accidental invocation |

---

## 2. Agent Profile Audit

### Implementer (Smith)

| Target | Status | Tools |
|--------|--------|-------|
| Claude Code | ✅ Valid | `[Read, Write, Edit, Bash, Glob, Grep]` (list format) |
| Cursor | ✅ Valid | `{ Read: true, Write: true, ... }` (object format) |
| OpenCode | ✅ Valid | Full write access |

### Reviewer (Sentinel)

| Target | Status | Tools | disable-model-invocation |
|--------|--------|-------|--------------------------|
| Claude Code | ✅ Valid | `[Read, Glob, Grep]` | Not applicable |
| Cursor | ✅ Updated | `{ Read: true, Glob: true, Grep: true }` | **`true`** ← ADDED |
| OpenCode | ✅ Valid | Read-only | Not applicable |

### Researcher (Ranger)

| Target | Status | Tools | disable-model-invocation |
|--------|--------|-------|--------------------------|
| Claude Code | ✅ Valid | `[Read, Glob, Grep, WebSearch, WebFetch]` | Not applicable |
| Cursor | ✅ Updated | `{ Read: true, Glob: true, Grep: true, WebFetch: true, WebSearch: true }` | **`true`** ← ADDED |
| OpenCode | ✅ Valid | Read + web access | Not applicable |

---

## 3. Description Truncation Verification

### Implementation Location
File: `cli/lib/build/targets/claude-code.ts` (lines 209-212)

```typescript
// Truncate description at 250 chars for Claude Code
if (merged.description && merged.description.length > 250) {
  merged.description = merged.description.substring(0, 250) + "...";
}
```

### Test Results

| Target | Status | Example Output |
|--------|--------|--------------|
| **Claude Code** | ✅ Truncated at 250 chars | `...database schema (use dat...` |
| **Cursor** | ✅ Full preserved | `...database schema (use database-design)...` |
| **Codex** | ✅ Full preserved | `...database schema (use database-design)...` |
| **Gemini** | ✅ Full preserved | `...database schema (use database-design)...` |

---

## 4. Build Verification

### TypeScript Type Checking
```
✅ npm run typecheck
> tsc --noEmit
(no errors)
```

### Test Suite
```
✅ npm test
 Test Files 19 passed (19)
      Tests 375 passed (375)
   Duration 2.05s
```

### Build Output
```
✅ npm run build
  ✓ shared skills intermediate (0.06s)
  ✓ claude-code (0.08s)
  ✓ opencode (0.06s)
  ✓ cursor (0.06s)
  ✓ codex (0.02s)
  ✓ gemini (0.02s)
Build complete (0.32s)
```

---

## 5. Files Modified

### Agent Sidecars Updated
1. `content/agents/reviewer.cursor.yaml` - Added `disable-model-invocation: true`
2. `content/agents/researcher.cursor.yaml` - Added `disable-model-invocation: true`, added web tools

### Verification: All Target Outputs
- `plugins/loaf/` - Claude Code plugin with truncated descriptions
- `dist/cursor/` - Cursor distribution with `disable-model-invocation` agents
- `dist/codex/` - Codex distribution with full descriptions
- `dist/gemini/` - Gemini distribution with full descriptions
- `dist/opencode/` - OpenCode distribution

---

## 6. Recommendations

### No Action Required
- **Knowledge skill sidecars for Cursor/Codex/Gemini**: Currently inherit all fields from SKILL.md. Sidecars only needed for target-specific overrides.
- **Universal field promotion**: All sidecar fields are correctly target-specific. No fields need to move to SKILL.md.

### Future Enhancements (Optional)
1. **Codex Enhancement Tracking**: Document Codex-specific enhancements when the platform adds new features
2. **Sidecar Field Documentation**: Consider documenting the sidecar schema in AGENTS.md or a new REFERENCE.md

---

## Conclusion

✅ **Sidecar audit complete** - 31 CC sidecars + 13 OC sidecars verified  
✅ **Agent profiles validated** - 3 agents × 3 targets = 9 configurations verified  
✅ **Description truncation working** - Claude Code truncates at 250 chars, others preserve full  
✅ **All checks pass** - typecheck, tests, and build all green

**Status**: READY FOR REVIEW

#!/bin/bash
# Hook: Validate CHANGELOG.md entries (INFORMATIONAL)
# PreToolUse hook - reads JSON from stdin per Claude Code hooks API
#
# Triggers on Write|Edit to CHANGELOG.md
# Displays guidance about product-focused entries (non-blocking)

# Read and parse stdin JSON
INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" 2>/dev/null || echo "")

# Only validate CHANGELOG.md files
if [[ "$FILE_PATH" != *"CHANGELOG.md" ]]; then
  exit 0
fi

cat << 'EOF'
# CHANGELOG Validation Reminder

## Anti-Patterns to Avoid

The following patterns indicate **file-level entries** (NOT allowed):

### File Path Patterns
- `backend/` or `frontend/` paths
- `.py`, `.ts`, `.md` file extensions in entries
- `docs/decisions/ADR-` file references
- Specific file names like `executor.py`, `mqtt_service.py`

### Implementation Detail Patterns
- "Enhanced N Python files with..."
- "Updated `filename`"
- "Refactored X to Y" without user benefit
- Lists of file paths

## Required Format

Each entry MUST be:
- **User-focused**: What value does this provide?
- **File-path free**: No paths or extensions
- **Concise**: One line, under 80 characters preferred
- **Aggregated**: Related changes combined

## Examples

**AVOID:**
```markdown
- `docs/decisions/ADR-012-rate-limit-circuit-breaker.md` - Circuit breaker
- Enhanced 9 Python files with docstrings
- Removed `docs/future/`, `docs/digital-twin/`
```

**USE:**
```markdown
- Circuit breaker pattern for API rate limiting
- Improved pipeline documentation with architecture patterns
- Consolidated documentation (redundant design docs removed)
```

## Version Protection

**CRITICAL**: Version numbers (## [X.Y.Z]) should NEVER be modified without explicit user approval.

If you're adding a new version header, ensure user has approved the version bump.

## Reference

See Skill: `documentation-standards/micro-changelog.md` for complete format rules.

EOF

exit 0

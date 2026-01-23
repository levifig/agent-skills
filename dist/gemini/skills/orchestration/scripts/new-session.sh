#!/bin/bash
# Generate a new session file with correct format
# Usage: new-session.sh <description> [linear-issue]
# Example: new-session.sh implement-auth-system BACK-123

set -e

DESCRIPTION="${1:?Usage: new-session.sh <description> [linear-issue]}"
LINEAR_ISSUE="${2:-}"

# Generate timestamps
TIMESTAMP=$(date -u +"%Y%m%d-%H%M%S")
ISO_TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Read Linear workspace from config (fallback to placeholder)
CONFIG_FILE=".agents/config.json"
if [[ -f "$CONFIG_FILE" ]] && command -v jq &> /dev/null; then
    LINEAR_WORKSPACE=$(jq -r '.linear.workspace // empty' "$CONFIG_FILE" 2>/dev/null)
fi
LINEAR_WORKSPACE="${LINEAR_WORKSPACE:-{{your-org}}}"

# Validate description format (kebab-case)
if [[ ! "$DESCRIPTION" =~ ^[a-z0-9-]+$ ]]; then
    echo "Error: Description must be kebab-case (lowercase letters, numbers, hyphens)" >&2
    exit 1
fi

# Build filename
FILENAME="${TIMESTAMP}-${DESCRIPTION}.md"
FILEPATH=".agents/sessions/${FILENAME}"

# Check if file already exists
if [[ -f "$FILEPATH" ]]; then
    echo "Error: Session file already exists: $FILEPATH" >&2
    exit 1
fi

# Build Linear fields if provided
LINEAR_FIELDS=""
if [[ -n "$LINEAR_ISSUE" ]]; then
    LINEAR_FIELDS="  linear_issue: \"${LINEAR_ISSUE}\"
  linear_url: \"https://linear.app/${LINEAR_WORKSPACE}/issue/${LINEAR_ISSUE}\""
fi

# Generate session file
cat > "$FILEPATH" << EOF
---
session:
  title: "${DESCRIPTION//-/ }"
  status: in_progress
  created: "${ISO_TIMESTAMP}"
  last_updated: "${ISO_TIMESTAMP}"
${LINEAR_FIELDS}

orchestration:
  current_task: "Initial setup"
  spawned_agents: []
---

# Session: ${DESCRIPTION//-/ }

## Context

Background for anyone picking up this work.
What problem are we solving? Why now?

## Current State

Where we are right now. What just happened.
**This section should ALWAYS be handoff-ready.**

## Next Steps

1. First action
2. Second action

## Progress

- [ ] First task

## Files Modified

- None yet

---

## Session Log

### ${ISO_TIMESTAMP} - PM
Session created.
EOF

echo "Created: $FILEPATH"

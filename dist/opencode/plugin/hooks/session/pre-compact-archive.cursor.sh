#!/bin/bash
# Hook: Pre-compact for Cursor
# PreCompact hook - returns JSON per Cursor hooks API
#
# Cursor expects: { "continue": true, "additional_context": "..." }

# Use project dir or current directory
SESSIONS_DIR="${PWD}/.agents/sessions"

# Find sessions modified in last 60 minutes
RECENT_THRESHOLD=60
CONTEXT=""

if [ -d "$SESSIONS_DIR" ]; then
  RECENT_COUNT=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f -mmin -${RECENT_THRESHOLD} 2>/dev/null | wc -l | tr -d ' ')

  if [ "$RECENT_COUNT" -gt 0 ]; then
    CONTEXT="IMPORTANT: $RECENT_COUNT session file(s) modified recently. Before compaction, update session files with current work state for seamless resumption."
  fi
fi

# Output valid JSON
if [ -n "$CONTEXT" ]; then
  ESCAPED_CONTEXT=$(echo "$CONTEXT" | sed 's/"/\\"/g' | tr '\n' ' ')
  echo "{\"continue\": true, \"additional_context\": \"$ESCAPED_CONTEXT\"}"
else
  echo "{\"continue\": true}"
fi

exit 0

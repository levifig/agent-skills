#!/bin/bash
# Hook: Agent-aware session startup for Cursor
# SessionStart hook - returns JSON per Cursor hooks API
#
# Cursor expects: { "continue": true, "additional_context": "..." }

# Use project dir or current directory
SESSIONS_DIR="${PWD}/.agents/sessions"

# Count session files (excluding archive subdirectory)
SESSION_COUNT=0
CONTEXT=""

if [ -d "$SESSIONS_DIR" ]; then
  SESSION_COUNT=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | wc -l | tr -d ' ')
fi

if [ "$SESSION_COUNT" -gt 0 ]; then
  CONTEXT="Active Sessions: $SESSION_COUNT session file(s) in .agents/sessions/. "
  CONTEXT+="Use /review-sessions to see them or /resume-session to continue one."
fi

# Output valid JSON for Cursor
if [ -n "$CONTEXT" ]; then
  # Escape quotes and newlines for JSON
  ESCAPED_CONTEXT=$(echo "$CONTEXT" | sed 's/"/\\"/g' | tr '\n' ' ')
  echo "{\"continue\": true, \"additional_context\": \"$ESCAPED_CONTEXT\"}"
else
  echo "{\"continue\": true}"
fi

exit 0

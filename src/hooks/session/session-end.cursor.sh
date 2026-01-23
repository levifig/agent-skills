#!/bin/bash
# Hook: Session end for Cursor
# SessionEnd hook - returns JSON per Cursor hooks API
#
# Cursor expects: { "continue": true }

# Use project dir or current directory
SESSIONS_DIR="${PWD}/.agents/sessions"

# Count session files
SESSION_COUNT=0
if [ -d "$SESSIONS_DIR" ]; then
  SESSION_COUNT=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | wc -l | tr -d ' ')
fi

# Output valid JSON - sessionEnd doesn't block, just acknowledge
if [ "$SESSION_COUNT" -gt 0 ]; then
  echo "{\"continue\": true, \"additional_context\": \"$SESSION_COUNT active session(s). Remember to update session files with outcomes.\"}"
else
  echo "{\"continue\": true}"
fi

exit 0

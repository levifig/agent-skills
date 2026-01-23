#!/bin/bash
# Hook: Check for active sessions on startup (INFORMATIONAL)
# SessionStart hook - receives JSON from stdin per Claude Code hooks API
#
# Triggers on session start/resume
# Displays count and list of active session files

# Use CLAUDE_PROJECT_DIR if available, otherwise current directory
SESSIONS_DIR="${CLAUDE_PROJECT_DIR:-.}/.agents/sessions"

if [ -d "$SESSIONS_DIR" ]; then
  # Count session files (excluding archive subdirectory)
  SESSION_COUNT=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | wc -l | tr -d ' ')

  if [ "$SESSION_COUNT" -gt 0 ]; then
    echo "# Active Sessions Found"
    echo ""
    echo "There are **$SESSION_COUNT** session file(s) in \`.agents/sessions/\`:"
    echo ""

    # List session files with metadata
    find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | while read -r session; do
      FILENAME=$(basename "$session")
      TITLE=$(grep -E "^\s+title:" "$session" 2>/dev/null | head -1 | sed 's/.*title:\s*"\?\([^"]*\)"\?/\1/')
      UPDATED=$(grep -E "^\s+last_updated:" "$session" 2>/dev/null | head -1 | sed 's/.*last_updated:\s*"\?\([^"]*\)"\?/\1/')
      LINEAR=$(grep -E "^\s+linear_issue:" "$session" 2>/dev/null | head -1 | sed 's/.*linear_issue:\s*"\?\([^"]*\)"\?/\1/')

      echo "- **$FILENAME**"
      [ -n "$TITLE" ] && echo "  - Title: $TITLE"
      [ -n "$UPDATED" ] && echo "  - Updated: $UPDATED"
      [ -n "$LINEAR" ] && echo "  - Linear: $LINEAR"
      echo ""
    done

    echo "Consider reviewing with \`/review-sessions\` or resuming with \`/resume-session\`"
  fi
fi

exit 0

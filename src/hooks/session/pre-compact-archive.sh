#!/bin/bash
# Hook: Pre-compact session preservation reminder (INFORMATIONAL)
# PreCompact hook - warns about recent session activity before context compaction
#
# Triggers before conversation context is compacted
# Identifies sessions modified recently that may need preservation

# Use CLAUDE_PROJECT_DIR if available, otherwise current directory
SESSIONS_DIR="${CLAUDE_PROJECT_DIR:-.}/.agents/sessions"
COUNCILS_DIR="${CLAUDE_PROJECT_DIR:-.}/.agents/councils"

# Find sessions modified in last 60 minutes
RECENT_THRESHOLD=60

if [ ! -d "$SESSIONS_DIR" ]; then
  exit 0
fi

echo "# Pre-Compact Session Check"
echo ""

# Find recently modified sessions
RECENT_SESSIONS=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f -mmin -${RECENT_THRESHOLD} 2>/dev/null)

if [ -n "$RECENT_SESSIONS" ]; then
  echo "**Warning**: Sessions modified in the last ${RECENT_THRESHOLD} minutes:"
  echo ""

  echo "$RECENT_SESSIONS" | while read -r session; do
    FILENAME=$(basename "$session")
    TITLE=$(grep -E "^\s+title:" "$session" 2>/dev/null | head -1 | sed 's/.*title:\s*"\?\([^"]*\)"\?/\1/')
    STATUS=$(grep -E "^\s+status:" "$session" 2>/dev/null | head -1 | sed 's/.*status:\s*"\?\([^"]*\)"\?/\1/')

    echo "- **$FILENAME**"
    [ -n "$TITLE" ] && echo "  - Title: $TITLE"
    [ -n "$STATUS" ] && echo "  - Status: $STATUS"
    echo ""
  done

  echo "**Recommendations**:"
  echo "- Ensure \`## Current State\` reflects latest progress"
  echo "- Update session status if work phase changed"
  echo "- Consider memory snapshots for complex decisions"
  echo ""
fi

# Check for recent council files
if [ -d "$COUNCILS_DIR" ]; then
  RECENT_COUNCILS=$(find "$COUNCILS_DIR" -maxdepth 1 -name "*.md" -type f -mmin -${RECENT_THRESHOLD} 2>/dev/null)

  if [ -n "$RECENT_COUNCILS" ]; then
    echo "**Council files** modified recently:"
    echo ""

    echo "$RECENT_COUNCILS" | while read -r council; do
      FILENAME=$(basename "$council")
      echo "- **$FILENAME**"
    done

    echo ""
    echo "**Tip**: Ensure council decisions are captured and user-approved."
    echo ""
  fi
fi

# Count all active sessions
SESSION_COUNT=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | wc -l | tr -d ' ')

if [ "$SESSION_COUNT" -gt 0 ]; then
  echo "---"
  echo ""
  echo "**Total Active Sessions**: $SESSION_COUNT"
  echo "**Reminder**: Sessions with significant decisions should be memorialized before deletion."
fi

exit 0

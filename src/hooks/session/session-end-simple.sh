#!/bin/bash
# Hook: Session end cleanup reminder (INFORMATIONAL)
# SessionEnd hook - reminds about session hygiene before closing
#
# Triggers when Claude Code session ends
# Displays checklist for session file maintenance

# Use CLAUDE_PROJECT_DIR if available, otherwise current directory
SESSIONS_DIR="${CLAUDE_PROJECT_DIR:-.}/.agents/sessions"

if [ -d "$SESSIONS_DIR" ]; then
  # Count session files (excluding archive subdirectory)
  SESSION_COUNT=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | wc -l | tr -d ' ')

  if [ "$SESSION_COUNT" -gt 0 ]; then
    echo "# Session End Checklist"
    echo ""
    echo "**$SESSION_COUNT** active session file(s) found. Before closing:"
    echo ""
    echo "- [ ] Update session status if work is complete"
    echo "- [ ] Ensure \`## Current State\` is handoff-ready"
    echo "- [ ] Sync Linear issue status with actual progress"
    echo "- [ ] Archive/delete completed sessions via \`/review-sessions\`"
    echo ""
    echo "**Tip**: Session files persist across conversations. Keep them current!"
  fi
fi

exit 0

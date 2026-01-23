#!/bin/bash
# Hook: Pre-compact context preservation with agent archival
# PreCompact hook - identifies sessions requiring preservation and provides
# instructions for spawning context-archiver agent
#
# Triggers before conversation context is compacted
# Outputs structured instructions for Claude to spawn context-archiver

# Use CLAUDE_PROJECT_DIR if available, otherwise current directory
SESSIONS_DIR="${CLAUDE_PROJECT_DIR:-.}/.agents/sessions"
COUNCILS_DIR="${CLAUDE_PROJECT_DIR:-.}/.agents/councils"

# Find sessions modified in last 60 minutes
RECENT_THRESHOLD=60

if [ ! -d "$SESSIONS_DIR" ]; then
  exit 0
fi

# Find recently modified sessions
RECENT_SESSIONS=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f -mmin -${RECENT_THRESHOLD} 2>/dev/null)

# Count all active sessions (not in archive)
SESSION_COUNT=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | wc -l | tr -d ' ')

# Build session list for agent prompt
SESSION_LIST=""
if [ -n "$RECENT_SESSIONS" ]; then
  while read -r session; do
    [ -z "$session" ] && continue
    FILENAME=$(basename "$session")
    TITLE=$(grep -E "^\s+title:" "$session" 2>/dev/null | head -1 | sed 's/.*title:\s*"\?\([^"]*\)"\?/\1/')
    STATUS=$(grep -E "^\s+status:" "$session" 2>/dev/null | head -1 | sed 's/.*status:\s*"\?\([^"]*\)"\?/\1/')
    LINEAR=$(grep -E "^\s+linear_issue:" "$session" 2>/dev/null | head -1 | sed 's/.*linear_issue:\s*"\?\([^"]*\)"\?/\1/')

    SESSION_LIST="${SESSION_LIST}- **${FILENAME}**"
    [ -n "$TITLE" ] && SESSION_LIST="${SESSION_LIST} - ${TITLE}"
    [ -n "$LINEAR" ] && SESSION_LIST="${SESSION_LIST} (${LINEAR})"
    SESSION_LIST="${SESSION_LIST}\n"
  done <<< "$RECENT_SESSIONS"
fi

# Output based on whether we have active sessions
if [ -n "$RECENT_SESSIONS" ]; then
  echo "# CRITICAL: Context Preservation Required"
  echo ""
  echo "**Action Required**: Before compaction proceeds, spawn the context-archiver agent"
  echo "to preserve current work state for seamless resumption."
  echo ""
  echo "## Sessions Requiring Archive"
  echo ""
  echo -e "$SESSION_LIST"
  echo ""
  echo "## Spawn Command"
  echo ""
  echo "Use the Task tool to spawn the context-archiver agent:"
  echo ""
  echo '```'
  echo 'Task(loaf:context-archiver, "Preserve session state before compaction.'
  echo ''
  echo 'Sessions to archive:'
  echo -e "$SESSION_LIST"
  echo ''
  echo 'Current work context:'
  echo '- What task/issue is being worked on: [fill in]'
  echo '- Last completed action: [fill in]'
  echo '- Immediate next step planned: [fill in]'
  echo '- Key decisions made: [fill in or None]'
  echo '- Current blockers: [fill in or None]'
  echo '")'
  echo '```'
  echo ""
  echo "**Include in prompt**:"
  echo "- What task/issue was being worked on"
  echo "- Last completed action"
  echo "- Immediate next step that was planned"
  echo "- Any key decisions or blockers"
  echo ""
  echo "---"
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
    echo "**Tip**: Ensure council decisions are captured in session before compaction."
    echo ""
  fi
fi

# Check for running background agents
RUNNING_BG_COUNT=0
RUNNING_BG_LIST=""
if [ -n "$RECENT_SESSIONS" ]; then
  while read -r session; do
    [ -z "$session" ] && continue
    # Extract running background agents
    BG_RUNNING=$(awk '
      /^---$/ { in_fm = !in_fm; next }
      in_fm && /^background_agents:/ { in_bg = 1; next }
      in_fm && in_bg && /^[a-z_]+:/ && !/^[[:space:]]/ { in_bg = 0 }
      in_fm && in_bg && /^[[:space:]]*- id:/ {
        gsub(/^[[:space:]]*- id:[[:space:]]*"?|"?$/, "")
        current_id = $0
      }
      in_fm && in_bg && /^[[:space:]]*status:/ {
        gsub(/^[[:space:]]*status:[[:space:]]*"?|"?$/, "")
        if ($0 == "running" && current_id != "") {
          print current_id
        }
      }
    ' "$session" 2>/dev/null)

    if [ -n "$BG_RUNNING" ]; then
      FILENAME=$(basename "$session")
      while read -r bg_id; do
        [ -z "$bg_id" ] && continue
        RUNNING_BG_COUNT=$((RUNNING_BG_COUNT + 1))
        RUNNING_BG_LIST="${RUNNING_BG_LIST}- **${bg_id}** (in ${FILENAME})\n"
      done <<< "$BG_RUNNING"
    fi
  done <<< "$RECENT_SESSIONS"
fi

if [ "$RUNNING_BG_COUNT" -gt 0 ]; then
  echo "## Background Agents Running"
  echo ""
  echo "**$RUNNING_BG_COUNT** background agent(s) still running:"
  echo ""
  echo -e "$RUNNING_BG_LIST"
  echo ""
  echo "**Note**: Background agent state will be preserved. Include in context-archiver prompt:"
  echo "- Background agent IDs and their tasks"
  echo "- Expected completion status"
  echo ""
fi

# Summary
if [ "$SESSION_COUNT" -gt 0 ]; then
  echo "---"
  echo ""
  echo "**Total Active Sessions**: $SESSION_COUNT"
  if [ "$RUNNING_BG_COUNT" -gt 0 ]; then
    echo "**Running Background Agents**: $RUNNING_BG_COUNT"
  fi
  if [ -n "$RECENT_SESSIONS" ]; then
    echo ""
    echo "**Reminder**: The context-archiver agent will generate a Resumption Prompt section"
    echo "in each session file. After compaction, read the session file to continue seamlessly."
  else
    echo "**Reminder**: Sessions with significant decisions should be memorialized before deletion."
  fi
fi

exit 0

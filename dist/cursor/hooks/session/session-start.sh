#!/bin/bash
# Hook: Agent-aware session startup (INFORMATIONAL)
# SessionStart hook - receives JSON from stdin per Claude Code hooks API
#
# Triggers on session start/resume
# Displays agent-specific guidance based on AGENT_TYPE environment variable

# Use CLAUDE_PROJECT_DIR if available, otherwise current directory
SESSIONS_DIR="${CLAUDE_PROJECT_DIR:-.}/.agents/sessions"

# Detect agent type from environment
AGENT_TYPE="${AGENT_TYPE:-unknown}"

# Count session files (excluding archive subdirectory)
SESSION_COUNT=0
if [ -d "$SESSIONS_DIR" ]; then
  SESSION_COUNT=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | wc -l | tr -d ' ')
fi

# Agent-specific messaging
case "$AGENT_TYPE" in
  pm-orchestrator|product)
    # PM agents: Show active sessions, session creation reminder
    echo "# PM Agent Session Context"
    echo ""
    if [ "$SESSION_COUNT" -gt 0 ]; then
      echo "**$SESSION_COUNT** active session file(s) found in \`.agents/sessions/\`:"
      echo ""

      find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | while read -r session; do
        FILENAME=$(basename "$session")
        TITLE=$(grep -E "^\s+title:" "$session" 2>/dev/null | head -1 | sed 's/.*title:\s*"\?\([^"]*\)"\?/\1/')
        STATUS=$(grep -E "^\s+status:" "$session" 2>/dev/null | head -1 | sed 's/.*status:\s*"\?\([^"]*\)"\?/\1/')
        LINEAR=$(grep -E "^\s+linear_issue:" "$session" 2>/dev/null | head -1 | sed 's/.*linear_issue:\s*"\?\([^"]*\)"\?/\1/')

        echo "- **$FILENAME**"
        [ -n "$TITLE" ] && echo "  - Title: $TITLE"
        [ -n "$STATUS" ] && echo "  - Status: $STATUS"
        [ -n "$LINEAR" ] && echo "  - Linear: $LINEAR"
        echo ""
      done

      echo "**Tip**: Review with \`/review-sessions\` or resume with \`/resume-session\`"
      echo ""
      echo "Remember to create a session file before starting new work."
    else
      echo "No active sessions found."
      echo ""
      echo "**Reminder**: Create a session file with \`/start-session\` before coordinating work."
    fi
    ;;

  backend-dev|frontend-dev|rails-dev|dba|devops)
    # Implementation agents: Show session context, active task
    echo "# Implementation Agent Context"
    echo ""
    if [ "$SESSION_COUNT" -gt 0 ]; then
      echo "**$SESSION_COUNT** active session file(s) exist:"
      echo ""

      find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | while read -r session; do
        FILENAME=$(basename "$session")
        TITLE=$(grep -E "^\s+title:" "$session" 2>/dev/null | head -1 | sed 's/.*title:\s*"\?\([^"]*\)"\?/\1/')
        CURRENT_TASK=$(grep -E "^\s+current_task:" "$session" 2>/dev/null | head -1 | sed 's/.*current_task:\s*"\?\([^"]*\)"\?/\1/')

        echo "- **$FILENAME**"
        [ -n "$TITLE" ] && echo "  - Context: $TITLE"
        [ -n "$CURRENT_TASK" ] && echo "  - Current Task: $CURRENT_TASK"
        echo ""
      done

      echo "**Tip**: Check session file for implementation context and requirements."
    else
      echo "No active sessions found."
      echo ""
      echo "**Tip**: If this is coordinated work, ask PM to create a session file."
    fi
    ;;

  code-reviewer|security|testing-qa)
    # Quality agents: Show scope focus
    echo "# Quality Agent Context"
    echo ""
    if [ "$SESSION_COUNT" -gt 0 ]; then
      echo "**$SESSION_COUNT** active session file(s) found:"
      echo ""

      find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | while read -r session; do
        FILENAME=$(basename "$session")
        TITLE=$(grep -E "^\s+title:" "$session" 2>/dev/null | head -1 | sed 's/.*title:\s*"\?\([^"]*\)"\?/\1/')

        echo "- **$FILENAME**: $TITLE"
      done
      echo ""
      echo "**Focus**: Ensure quality criteria align with session objectives."
    else
      echo "No active sessions found."
      echo ""
      echo "**Tip**: Review work in context of overall project goals."
    fi
    ;;

  design)
    # Design agent: UI/UX focus
    echo "# Design Agent Context"
    echo ""
    if [ "$SESSION_COUNT" -gt 0 ]; then
      echo "**$SESSION_COUNT** active session file(s) found:"
      echo ""

      find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | while read -r session; do
        FILENAME=$(basename "$session")
        TITLE=$(grep -E "^\s+title:" "$session" 2>/dev/null | head -1 | sed 's/.*title:\s*"\?\([^"]*\)"\?/\1/')

        echo "- **$FILENAME**: $TITLE"
      done
      echo ""
      echo "**Focus**: Ensure UI/UX decisions align with session requirements and user experience goals."
    else
      echo "No active sessions found."
      echo ""
      echo "**Tip**: Check for design specifications in project documentation."
    fi
    ;;

  *)
    # Unknown or no agent type: Generic message
    if [ "$SESSION_COUNT" -gt 0 ]; then
      echo "# Active Sessions"
      echo ""
      echo "There are **$SESSION_COUNT** session file(s) in \`.agents/sessions/\`:"
      echo ""

      find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | while read -r session; do
        FILENAME=$(basename "$session")
        TITLE=$(grep -E "^\s+title:" "$session" 2>/dev/null | head -1 | sed 's/.*title:\s*"\?\([^"]*\)"\?/\1/')

        echo "- **$FILENAME**"
        [ -n "$TITLE" ] && echo "  - Title: $TITLE"
        echo ""
      done
    fi
    ;;
esac

exit 0

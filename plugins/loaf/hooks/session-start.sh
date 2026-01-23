#!/bin/bash
# Hook: Agent-aware session startup (INFORMATIONAL)
# SessionStart hook - receives JSON from stdin per Claude Code hooks API
#
# Triggers on session start/resume
# Displays agent-specific guidance based on AGENT_TYPE environment variable
# Includes git branch detection and session branch validation

# Use CLAUDE_PROJECT_DIR if available, otherwise current directory
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
SESSIONS_DIR="$PROJECT_DIR/.agents/sessions"

# Detect agent type from environment
AGENT_TYPE="${AGENT_TYPE:-unknown}"

# Extract YAML frontmatter value (portable across macOS/Linux)
# Usage: extract_yaml_value <file> <key>
extract_yaml_value() {
  local file="$1"
  local key="$2"
  # Match lines like "  key: value" or "  key: "value"" in YAML frontmatter
  awk -v key="$key" '
    /^---$/ { in_fm = !in_fm; next }
    in_fm && $1 == key":" {
      sub(/^[[:space:]]*[^:]+:[[:space:]]*/, "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  ' "$file" 2>/dev/null
}

# Extract completed background agents from session frontmatter
# Returns lines of: id|agent|task|result_location
extract_completed_background_agents() {
  local file="$1"
  awk '
    /^---$/ { in_fm = !in_fm; next }
    in_fm && /^background_agents:/ { in_bg = 1; next }
    in_fm && in_bg && /^[a-z_]+:/ && !/^[[:space:]]/ { in_bg = 0 }
    in_fm && in_bg && /^[[:space:]]*- id:/ {
      gsub(/^[[:space:]]*- id:[[:space:]]*"?|"?$/, "")
      current_id = $0
    }
    in_fm && in_bg && /^[[:space:]]*agent:/ {
      gsub(/^[[:space:]]*agent:[[:space:]]*"?|"?$/, "")
      current_agent = $0
    }
    in_fm && in_bg && /^[[:space:]]*task:/ {
      gsub(/^[[:space:]]*task:[[:space:]]*"?|"?$/, "")
      current_task = $0
    }
    in_fm && in_bg && /^[[:space:]]*status:/ {
      gsub(/^[[:space:]]*status:[[:space:]]*"?|"?$/, "")
      current_status = $0
    }
    in_fm && in_bg && /^[[:space:]]*result_location:/ {
      gsub(/^[[:space:]]*result_location:[[:space:]]*"?|"?$/, "")
      current_result = $0
      if (current_status == "completed" && current_id != "") {
        print current_id "|" current_agent "|" current_task "|" current_result
      }
    }
  ' "$file" 2>/dev/null
}

# Git context detection
CURRENT_BRANCH=""
UNCOMMITTED_COUNT=0
IN_GIT_REPO=false

if git -C "$PROJECT_DIR" rev-parse --is-inside-work-tree &>/dev/null; then
  IN_GIT_REPO=true
  CURRENT_BRANCH=$(git -C "$PROJECT_DIR" branch --show-current 2>/dev/null || echo "")
  if [ -z "$CURRENT_BRANCH" ]; then
    # Detached HEAD state
    CURRENT_BRANCH="(detached HEAD)"
  fi
  UNCOMMITTED_COUNT=$(git -C "$PROJECT_DIR" status --porcelain 2>/dev/null | wc -l | tr -d ' ')
fi

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
        TITLE=$(extract_yaml_value "$session" "title")
        STATUS=$(extract_yaml_value "$session" "status")
        LINEAR=$(extract_yaml_value "$session" "linear_issue")
        SESSION_BRANCH=$(extract_yaml_value "$session" "branch")

        echo "- **$FILENAME**"
        [ -n "$TITLE" ] && echo "  - Title: $TITLE"
        [ -n "$STATUS" ] && echo "  - Status: $STATUS"
        [ -n "$LINEAR" ] && echo "  - Linear: $LINEAR"
        [ -n "$SESSION_BRANCH" ] && echo "  - Branch: \`$SESSION_BRANCH\`"
        # Warn on branch mismatch
        if [ -n "$SESSION_BRANCH" ] && [ -n "$CURRENT_BRANCH" ] && [ "$SESSION_BRANCH" != "$CURRENT_BRANCH" ]; then
          echo "  - **WARNING**: Session expects branch \`$SESSION_BRANCH\`, currently on \`$CURRENT_BRANCH\`"
        fi
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
        TITLE=$(extract_yaml_value "$session" "title")
        SESS_TASK=$(extract_yaml_value "$session" "current_task")
        SESSION_BRANCH=$(extract_yaml_value "$session" "branch")

        echo "- **$FILENAME**"
        [ -n "$TITLE" ] && echo "  - Context: $TITLE"
        [ -n "$SESS_TASK" ] && echo "  - Current Task: $SESS_TASK"
        [ -n "$SESSION_BRANCH" ] && echo "  - Branch: \`$SESSION_BRANCH\`"
        # Warn on branch mismatch
        if [ -n "$SESSION_BRANCH" ] && [ -n "$CURRENT_BRANCH" ] && [ "$SESSION_BRANCH" != "$CURRENT_BRANCH" ]; then
          echo "  - **WARNING**: Session expects branch \`$SESSION_BRANCH\`, currently on \`$CURRENT_BRANCH\`"
        fi
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
        TITLE=$(extract_yaml_value "$session" "title")
        SESSION_BRANCH=$(extract_yaml_value "$session" "branch")

        echo "- **$FILENAME**: $TITLE"
        # Warn on branch mismatch
        if [ -n "$SESSION_BRANCH" ] && [ -n "$CURRENT_BRANCH" ] && [ "$SESSION_BRANCH" != "$CURRENT_BRANCH" ]; then
          echo "  - **WARNING**: Session expects branch \`$SESSION_BRANCH\`, currently on \`$CURRENT_BRANCH\`"
        fi
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
        TITLE=$(extract_yaml_value "$session" "title")
        SESSION_BRANCH=$(extract_yaml_value "$session" "branch")

        echo "- **$FILENAME**: $TITLE"
        # Warn on branch mismatch
        if [ -n "$SESSION_BRANCH" ] && [ -n "$CURRENT_BRANCH" ] && [ "$SESSION_BRANCH" != "$CURRENT_BRANCH" ]; then
          echo "  - **WARNING**: Session expects branch \`$SESSION_BRANCH\`, currently on \`$CURRENT_BRANCH\`"
        fi
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
        TITLE=$(extract_yaml_value "$session" "title")
        SESSION_BRANCH=$(extract_yaml_value "$session" "branch")

        echo "- **$FILENAME**"
        [ -n "$TITLE" ] && echo "  - Title: $TITLE"
        # Warn on branch mismatch
        if [ -n "$SESSION_BRANCH" ] && [ -n "$CURRENT_BRANCH" ] && [ "$SESSION_BRANCH" != "$CURRENT_BRANCH" ]; then
          echo "  - **WARNING**: Session expects branch \`$SESSION_BRANCH\`, currently on \`$CURRENT_BRANCH\`"
        fi
        echo ""
      done
    fi
    ;;
esac

# Check for completed background agents across all sessions
COMPLETED_BG_AGENTS=""
if [ -d "$SESSIONS_DIR" ]; then
  while read -r session; do
    [ -z "$session" ] && continue
    BG_AGENTS=$(extract_completed_background_agents "$session")
    if [ -n "$BG_AGENTS" ]; then
      COMPLETED_BG_AGENTS="${COMPLETED_BG_AGENTS}${BG_AGENTS}\n"
    fi
  done < <(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null)
fi

if [ -n "$COMPLETED_BG_AGENTS" ]; then
  echo ""
  echo "## Background Work Completed"
  echo ""
  echo "The following background agents have completed:"
  echo ""

  echo -e "$COMPLETED_BG_AGENTS" | while IFS='|' read -r bg_id bg_agent bg_task bg_result; do
    [ -z "$bg_id" ] && continue
    echo "- **$bg_id** ($bg_agent)"
    echo "  - Task: $bg_task"
    if [ -n "$bg_result" ] && [ "$bg_result" != "null" ]; then
      echo "  - Result: \`$bg_result\`"
    fi
    echo ""
  done

  echo "**Action**: Review reports and update session frontmatter after processing."
fi

# Git context summary (shown for all agents when in a git repo)
if [ "$IN_GIT_REPO" = true ]; then
  echo ""
  echo "## Git Context"
  echo "- **Branch**: \`$CURRENT_BRANCH\`"
  echo "- **Uncommitted changes**: $UNCOMMITTED_COUNT file(s)"
fi

exit 0

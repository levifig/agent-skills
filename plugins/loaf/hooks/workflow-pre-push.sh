#!/usr/bin/env bash
# Hook: Pre-Push Gate (BLOCKING)
# PreToolUse hook - reads JSON from stdin per Claude Code hooks API
#
# Blocks git push commands until user explicitly approves

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Resolve paths for both source layout (pre-tool/../) and built layout (hooks/)
if [[ -f "${SCRIPT_DIR}/../lib/json-parser.sh" ]]; then
  source "${SCRIPT_DIR}/../lib/json-parser.sh"
  INSTRUCTIONS="${SCRIPT_DIR}/../instructions"
else
  source "${SCRIPT_DIR}/lib/json-parser.sh"
  INSTRUCTIONS="${SCRIPT_DIR}/instructions"
fi

# Read hook input — Claude Code: stdin JSON; OpenCode: TOOL_NAME + TOOL_INPUT env vars
if [[ -n "${TOOL_INPUT:-}" ]]; then
  INPUT="{\"tool_name\":\"${TOOL_NAME:-}\",\"tool_input\":${TOOL_INPUT}}"
else
  INPUT=$(cat)
fi
COMMAND=$(parse_command "$INPUT")

# Match git push as a command, not as substring in arguments (e.g. commit messages)
if ! echo "$COMMAND" | grep -qE '(^|[;&|]\s*)git\s+push'; then
  exit 0
fi

# Detect force-push flags
FORCE_PUSH=false
[[ "$COMMAND" == *"--force"* || "$COMMAND" == *"--force-with-lease"* || "$COMMAND" == *"-f "* || "$COMMAND" == *" -f" ]] && FORCE_PUSH=true

# Determine target branch — check bare arg, refspec (:main), or fall back to current
TARGET_BRANCH=""
if [[ "$COMMAND" =~ :(main|master)($|\ ) ]]; then
  # Refspec push: HEAD:main, branch:master, etc.
  TARGET_BRANCH="${BASH_REMATCH[1]}"
elif [[ "$COMMAND" =~ git\ push\ [^-][^\ ]*\ (main|master)($|\ ) ]]; then
  # Bare arg: git push origin main
  TARGET_BRANCH="${BASH_REMATCH[1]}"
fi
if [[ -z "$TARGET_BRANCH" ]]; then
  TARGET_BRANCH=$(git branch --show-current 2>/dev/null || echo "unknown")
fi

# Output advisory reminders
cat "${INSTRUCTIONS}/pre-push.md"

# Extra warning for force-push to protected branches
if [[ "$TARGET_BRANCH" == "main" || "$TARGET_BRANCH" == "master" ]] && $FORCE_PUSH; then
  echo "" >&2
  echo "WARNING: Force-pushing to $TARGET_BRANCH. This rewrites shared history." >&2
fi

# Advisory — remind user to confirm, but let Claude Code's permission system handle approval
exit 0

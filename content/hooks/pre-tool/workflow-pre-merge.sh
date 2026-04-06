#!/usr/bin/env bash
# Hook: Pre-Merge Conventions (ADVISORY)
# PreToolUse hook - reads JSON from stdin per Claude Code hooks API
#
# Reminds about squash merge conventions before gh pr merge

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

# Only match gh pr merge
case "$COMMAND" in
  *"gh pr merge"*) ;;
  *) exit 0 ;;
esac

# Output merge convention reminder
cat "$INSTRUCTIONS/pre-merge.md"

# Advisory — not blocking
exit 0

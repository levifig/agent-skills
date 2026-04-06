#!/usr/bin/env bash
# Post-merge housekeeping checklist
# Injects advisory checklist after a successful `gh pr merge`
# Post-tool hook for Bash (non-blocking, informational only)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Resolve paths for both source layout (post-tool/../) and built layout (hooks/)
if [[ -f "${SCRIPT_DIR}/../lib/json-parser.sh" ]]; then
  source "${SCRIPT_DIR}/../lib/json-parser.sh"
  INSTRUCTIONS="${SCRIPT_DIR}/../instructions"
else
  source "${SCRIPT_DIR}/lib/json-parser.sh"
  INSTRUCTIONS="${SCRIPT_DIR}/instructions"
fi

# Read hook input — Claude Code: stdin JSON; OpenCode: TOOL_NAME + TOOL_INPUT env vars
if [[ -n "${TOOL_INPUT:-}" ]]; then
  HOOK_INPUT="{\"tool_name\":\"${TOOL_NAME:-}\",\"tool_input\":${TOOL_INPUT}}"
else
  HOOK_INPUT=$(cat)
fi
COMMAND=$(parse_command "${HOOK_INPUT}")

# Only match gh pr merge commands
case "$COMMAND" in
  *"gh pr merge"*) ;;
  *) exit 0 ;;
esac

# Skip on confirmed failure — unknown exit code (OpenCode, empty) still shows checklist
EXIT_CODE=$(parse_exit_code "${HOOK_INPUT}")
[[ -n "$EXIT_CODE" && "$EXIT_CODE" != "0" ]] && exit 0

# Output the housekeeping checklist
cat "${INSTRUCTIONS}/post-merge.md"

exit 0

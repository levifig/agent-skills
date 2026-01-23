#!/bin/bash
# Ruff Linting for Python
# Fast linting with auto-fix suggestions
# Exit 0 (informational)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"

# Check if ruff linting is enabled
if ! is_hook_enabled "ruff-lint"; then
    exit 0
fi

# Check if ruff is available
if ! command -v ruff >/dev/null 2>&1; then
    exit 0
fi

# Read hook input from stdin
HOOK_INPUT=$(cat)
FILE_PATH=$(parse_file_path "${HOOK_INPUT}")
TOOL_NAME=$(parse_tool_name "${HOOK_INPUT}")

# Only run on Write/Edit
if [[ "${TOOL_NAME}" != "Write" ]] && [[ "${TOOL_NAME}" != "Edit" ]]; then
    exit 0
fi

# Only check Python files
if [[ ! "${FILE_PATH}" =~ \.py$ ]]; then
    exit 0
fi

echo ""
echo "🔎 Running Ruff Lint..."
echo ""

# Run ruff check
if ruff check "${FILE_PATH}" 2>&1 | tee /tmp/ruff-check.log; then
    echo "   ✅ No linting issues"
else
    ISSUE_COUNT=$(grep -c "^${FILE_PATH}:" /tmp/ruff-check.log || true)

    if [[ ${ISSUE_COUNT} -gt 0 ]]; then
        echo "   ⚠️  ${ISSUE_COUNT} linting issue(s) found"
        echo ""
        cat /tmp/ruff-check.log
        echo ""
        echo "💡 To fix automatically:"
        echo "   ruff check --fix ${FILE_PATH}"
        echo ""
        echo "💡 To format code:"
        echo "   ruff format ${FILE_PATH}"
    fi
fi

echo ""

exit 0

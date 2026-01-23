#!/bin/bash
# ESLint Validation
# Shows linting issues and fix commands
# Exit 0 (informational)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"

# Check if eslint check is enabled
if ! is_hook_enabled "eslint-check"; then
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

# Only check TypeScript/JavaScript files
if [[ ! "${FILE_PATH}" =~ \.(ts|tsx|js|jsx)$ ]]; then
    exit 0
fi

PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

# Check if eslint is available
if ! command -v eslint >/dev/null 2>&1 && ! npx eslint --version >/dev/null 2>&1; then
    exit 0
fi

echo ""
echo "🔍 Running ESLint..."
echo ""

# Run eslint
if npx eslint "${FILE_PATH}" 2>&1 | tee /tmp/eslint-check.log; then
    echo "   ✅ No linting issues"
else
    ERROR_COUNT=$(grep -c "error" /tmp/eslint-check.log || true)
    WARNING_COUNT=$(grep -c "warning" /tmp/eslint-check.log || true)

    if [[ ${ERROR_COUNT} -gt 0 ]] || [[ ${WARNING_COUNT} -gt 0 ]]; then
        echo "   ⚠️  Issues found:"
        echo "      Errors: ${ERROR_COUNT}"
        echo "      Warnings: ${WARNING_COUNT}"
        echo ""
        cat /tmp/eslint-check.log
        echo ""
        echo "💡 To fix automatically:"
        echo "   npx eslint --fix ${FILE_PATH}"
    fi
fi

echo ""

exit 0

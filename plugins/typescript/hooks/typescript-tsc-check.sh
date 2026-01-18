#!/bin/bash
# TypeScript Compilation Check
# Runs tsc --noEmit to validate types
# Exit 0 (informational)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/timeout-manager.sh"

# Start timeout tracker
start_timeout_tracker 120

# Check if tsc check is enabled
if ! is_hook_enabled "tsc-check"; then
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

# Check if tsconfig.json exists
if [[ ! -f "${PROJECT_ROOT}/tsconfig.json" ]]; then
    exit 0
fi

# Check if tsc is available
if ! command -v tsc >/dev/null 2>&1 && ! npx tsc --version >/dev/null 2>&1; then
    exit 0
fi

echo ""
echo "📘 Running TypeScript Compilation Check..."
echo ""

cd "${PROJECT_ROOT}"

# Run tsc --noEmit
if npx tsc --noEmit 2>&1 | tee /tmp/tsc-check.log; then
    echo "   ✅ No type errors"
else
    ERROR_COUNT=$(grep -c "error TS" /tmp/tsc-check.log || true)

    if [[ ${ERROR_COUNT} -gt 0 ]]; then
        echo "   ⚠️  ${ERROR_COUNT} type error(s) found"
        echo ""
        cat /tmp/tsc-check.log | head -30
        echo ""
        echo "💡 Common fixes:"
        echo "   • Add proper type annotations"
        echo "   • Use union types for nullable values"
        echo "   • Import types from @types/* packages"
        echo "   • Use 'as const' for literal types"
    fi
fi

echo ""

exit 0

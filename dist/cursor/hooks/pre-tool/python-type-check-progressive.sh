#!/bin/bash
# Progressive Python Type Checking
# Tier 1: Quick check (single file)
# Tier 2: Strict mode (if configured)
# Exit 0 (informational)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/timeout-manager.sh"

# Start timeout tracker
start_timeout_tracker 120

# Check if type checking is enabled
if ! is_hook_enabled "type-check"; then
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

# Skip test files (they often have looser types)
if [[ "${FILE_PATH}" =~ test_.*\.py$ ]] || [[ "${FILE_PATH}" =~ _test\.py$ ]]; then
    exit 0
fi

PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

echo ""
echo "🔍 Running Python Type Check..."
echo ""

# Determine type checker (prefer mypy, fall back to pyright)
TYPE_CHECKER=""
if command -v mypy >/dev/null 2>&1; then
    TYPE_CHECKER="mypy"
elif command -v pyright >/dev/null 2>&1; then
    TYPE_CHECKER="pyright"
fi

if [[ -z "${TYPE_CHECKER}" ]]; then
    echo "   ⚠️  No type checker available (install mypy or pyright)"
    exit 0
fi

# Get validation level
VALIDATION_LEVEL=$(get_validation_level)
STRICT_MODE=$(get_config_value "hooks.type-check.strict" || echo "false")

echo "   → Using ${TYPE_CHECKER} for type checking..."

# Tier 1: Quick check on single file
if [[ "${TYPE_CHECKER}" == "mypy" ]]; then
    if mypy "${FILE_PATH}" 2>&1 | tee /tmp/mypy-quick.log; then
        echo "     ✅ No type errors"
    else
        ERROR_COUNT=$(grep -c "error:" /tmp/mypy-quick.log || true)
        echo "     ⚠️  ${ERROR_COUNT} type error(s) found"
        echo ""
        cat /tmp/mypy-quick.log
        echo ""
        echo "💡 To fix type issues:"
        echo "   1. Add type annotations where missing"
        echo "   2. Use proper return types"
        echo "   3. Consider using 'typing' module for complex types"
    fi
else
    # pyright
    if pyright "${FILE_PATH}" 2>&1 | tee /tmp/pyright-quick.log; then
        echo "     ✅ No type errors"
    else
        ERROR_COUNT=$(grep -c "error:" /tmp/pyright-quick.log || true)
        echo "     ⚠️  ${ERROR_COUNT} type error(s) found"
        echo ""
        cat /tmp/pyright-quick.log
    fi
fi

# Tier 2: Strict mode (if enabled and time permits)
if [[ "${STRICT_MODE}" == "true" ]] || [[ "${VALIDATION_LEVEL}" == "thorough" ]]; then
    if check_remaining_time 60; then
        echo ""
        echo "   → Running strict type check..."

        cd "${PROJECT_ROOT}"

        if [[ "${TYPE_CHECKER}" == "mypy" ]]; then
            if mypy --strict "${FILE_PATH}" 2>&1 | tee /tmp/mypy-strict.log | head -20; then
                echo "     ✅ Strict mode passed"
            else
                echo "     ⚠️  Strict mode issues detected"
                echo "     Review: cat /tmp/mypy-strict.log"
            fi
        fi
    fi
fi

echo ""
echo "✅ Type checking complete"
echo ""

exit 0

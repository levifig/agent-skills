#!/bin/bash
# Multi-Language Format Check
# Validates code formatting across Python, TypeScript/JavaScript, and Ruby
# Exit 0 (informational only - shows fix commands)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"

# Check if format-check is enabled
if ! is_hook_enabled "format-check"; then
    exit 0
fi

# Read hook input from stdin
HOOK_INPUT=$(cat)
FILE_PATH=$(parse_file_path "${HOOK_INPUT}")
TOOL_NAME=$(parse_tool_name "${HOOK_INPUT}")

# Only run on Write and Edit operations
if [[ "${TOOL_NAME}" != "Write" ]] && [[ "${TOOL_NAME}" != "Edit" ]]; then
    exit 0
fi

# Skip if file doesn't exist
if [[ ! -f "${FILE_PATH}" ]]; then
    exit 0
fi

# Get file extension
FILE_EXT="${FILE_PATH##*.}"

# Track formatting issues
ISSUES=()
FIX_COMMANDS=()

# Python files - check with black
if [[ "${FILE_EXT}" == "py" ]] && command -v black >/dev/null 2>&1; then
    if ! black --check --quiet "${FILE_PATH}" 2>/dev/null; then
        ISSUES+=("Python: Code formatting doesn't match Black style")
        FIX_COMMANDS+=("black ${FILE_PATH}")
    fi
fi

# JavaScript/TypeScript files - check with prettier
if [[ "${FILE_EXT}" =~ ^(js|jsx|ts|tsx)$ ]] && command -v prettier >/dev/null 2>&1; then
    if ! prettier --check "${FILE_PATH}" 2>/dev/null; then
        ISSUES+=("JavaScript/TypeScript: Code formatting doesn't match Prettier style")
        FIX_COMMANDS+=("prettier --write ${FILE_PATH}")
    fi
fi

# Ruby files - check with standardrb or rubocop
if [[ "${FILE_EXT}" == "rb" ]]; then
    if command -v standardrb >/dev/null 2>&1; then
        if ! standardrb --format quiet "${FILE_PATH}" 2>/dev/null; then
            ISSUES+=("Ruby: Code formatting doesn't match Standard Ruby style")
            FIX_COMMANDS+=("standardrb --fix ${FILE_PATH}")
        fi
    elif command -v rubocop >/dev/null 2>&1; then
        if ! rubocop --format quiet "${FILE_PATH}" 2>/dev/null; then
            ISSUES+=("Ruby: Code formatting doesn't match RuboCop style")
            FIX_COMMANDS+=("rubocop -a ${FILE_PATH}")
        fi
    fi
fi

# If issues found, provide actionable feedback
if [[ ${#ISSUES[@]} -gt 0 ]]; then
    echo ""
    echo "📝 Formatting Issues Detected"
    echo ""
    echo "File: ${FILE_PATH}"
    echo ""
    echo "Issues:"
    for issue in "${ISSUES[@]}"; do
        echo "  • ${issue}"
    done
    echo ""
    echo "To fix, run:"
    for cmd in "${FIX_COMMANDS[@]}"; do
        echo "  ${cmd}"
    done
    echo ""
    echo "💡 Tip: Configure your editor to format on save for automatic compliance."
    echo ""
fi

exit 0

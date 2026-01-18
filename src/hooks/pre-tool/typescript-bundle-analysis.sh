#!/bin/bash
# Bundle Size Analysis
# Warns on large component sizes
# Exit 0 (informational)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"

# Check if bundle analysis is enabled
if ! is_hook_enabled "bundle-analysis"; then
    exit 0
fi

# Only run for frontend-dev agent
AGENT_TYPE=$(get_agent_type)
if [[ "${AGENT_TYPE}" != "frontend-dev" ]]; then
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

# Only check TypeScript/JavaScript component files
if [[ ! "${FILE_PATH}" =~ \.(ts|tsx|js|jsx)$ ]]; then
    exit 0
fi

# Skip test files
if [[ "${FILE_PATH}" =~ \.(test|spec)\.(ts|tsx|js|jsx)$ ]]; then
    exit 0
fi

echo ""
echo "📦 Analyzing Component Size..."
echo ""

# Get file size
FILE_SIZE=$(wc -c < "${FILE_PATH}" | tr -d ' ')
FILE_SIZE_KB=$((FILE_SIZE / 1024))

# Count lines of code (excluding comments and blank lines)
LOC=$(grep -v '^\s*$' "${FILE_PATH}" | grep -v '^\s*//' | wc -l | tr -d ' ')

# Count imports
IMPORT_COUNT=$(grep -c "^import " "${FILE_PATH}" || true)

echo "   File: $(basename ${FILE_PATH})"
echo "   Size: ${FILE_SIZE_KB} KB"
echo "   Lines of Code: ${LOC}"
echo "   Imports: ${IMPORT_COUNT}"

# Warnings
WARNINGS=()

if [[ ${FILE_SIZE_KB} -gt 100 ]]; then
    WARNINGS+=("⚠️  Large file size (${FILE_SIZE_KB} KB > 100 KB)")
fi

if [[ ${LOC} -gt 500 ]]; then
    WARNINGS+=("⚠️  Large component (${LOC} LOC > 500)")
fi

if [[ ${IMPORT_COUNT} -gt 20 ]]; then
    WARNINGS+=("⚠️  Many imports (${IMPORT_COUNT} > 20)")
fi

# Check for large dependencies
if command -v bundlephobia >/dev/null 2>&1; then
    LARGE_DEPS=$(grep "from ['\"]" "${FILE_PATH}" | grep -v "^//" | grep -v "^\s\+//" || true)
    if [[ -n "${LARGE_DEPS}" ]]; then
        echo ""
        echo "   Dependencies detected - check sizes at:"
        echo "   https://bundlephobia.com"
    fi
fi

if [[ ${#WARNINGS[@]} -gt 0 ]]; then
    echo ""
    echo "   Optimization suggestions:"
    for warning in "${WARNINGS[@]}"; do
        echo "   ${warning}"
    done
    echo ""
    echo "💡 Consider:"
    echo "   • Breaking into smaller components"
    echo "   • Using code splitting / lazy loading"
    echo "   • Tree-shaking unused imports"
    echo "   • Using lighter alternatives for heavy dependencies"
else
    echo "   ✅ Component size looks good"
fi

echo ""

exit 0

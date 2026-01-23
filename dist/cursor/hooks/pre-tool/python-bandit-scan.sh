#!/bin/bash
# Bandit Security Scan for Python
# Exit 0 (informational)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/timeout-manager.sh"

# Start timeout tracker
start_timeout_tracker 60

# Check if bandit scan is enabled
if ! is_hook_enabled "bandit-scan"; then
    exit 0
fi

# Only run for security agent or thorough validation
AGENT_TYPE=$(get_agent_type)
VALIDATION_LEVEL=$(get_validation_level)

if [[ "${AGENT_TYPE}" != "security" ]] && [[ "${VALIDATION_LEVEL}" != "thorough" ]]; then
    exit 0
fi

# Check if bandit is available
if ! command -v bandit >/dev/null 2>&1; then
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
echo "🔒 Running Bandit Security Scan..."
echo ""

# Run bandit on single file
if bandit -f txt "${FILE_PATH}" 2>&1 | tee /tmp/bandit-results.txt; then
    echo "   ✅ No security issues detected"
else
    # Parse results
    HIGH_SEVERITY=$(grep -c "Issue: \[B[0-9]*:.*\] Severity: High" /tmp/bandit-results.txt || true)
    MEDIUM_SEVERITY=$(grep -c "Issue: \[B[0-9]*:.*\] Severity: Medium" /tmp/bandit-results.txt || true)

    if [[ ${HIGH_SEVERITY} -gt 0 ]] || [[ ${MEDIUM_SEVERITY} -gt 0 ]]; then
        echo "   ⚠️  Security issues detected:"
        echo "      High Severity: ${HIGH_SEVERITY}"
        echo "      Medium Severity: ${MEDIUM_SEVERITY}"
        echo ""
        cat /tmp/bandit-results.txt
        echo ""
        echo "💡 Common fixes:"
        echo "   • Replace assert with proper error handling"
        echo "   • Use secrets module for random tokens"
        echo "   • Validate and sanitize user inputs"
        echo "   • Use parameterized queries for SQL"
    fi
fi

echo ""

exit 0

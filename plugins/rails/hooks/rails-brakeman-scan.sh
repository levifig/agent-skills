#!/bin/bash
# Brakeman Security Scan for Rails
# Exit 0 (informational only)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/timeout-manager.sh"

# Start timeout tracker
start_timeout_tracker 120

# Check if brakeman scan is enabled
if ! is_hook_enabled "brakeman-scan"; then
    exit 0
fi

# Only run for security agent or thorough validation
AGENT_TYPE=$(get_agent_type)
VALIDATION_LEVEL=$(get_validation_level)

if [[ "${AGENT_TYPE}" != "security" ]] && [[ "${VALIDATION_LEVEL}" != "thorough" ]]; then
    exit 0
fi

# Check if brakeman is available
if ! command -v brakeman >/dev/null 2>&1 && ! bundle exec brakeman --version >/dev/null 2>&1; then
    exit 0
fi

PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

# Check if this is a Rails project
if [[ ! -f "${PROJECT_ROOT}/config/application.rb" ]]; then
    exit 0
fi

echo ""
echo "🔒 Running Brakeman Security Scan..."
echo ""

cd "${PROJECT_ROOT}"

# Run brakeman
if bundle exec brakeman --quiet --format text --output /tmp/brakeman-results.txt 2>&1; then
    echo "   ✅ No security issues detected by Brakeman"
else
    # Parse results
    HIGH_CONFIDENCE=$(grep -c "Confidence: High" /tmp/brakeman-results.txt || true)
    MEDIUM_CONFIDENCE=$(grep -c "Confidence: Medium" /tmp/brakeman-results.txt || true)
    TOTAL_WARNINGS=$(grep -c "Warning" /tmp/brakeman-results.txt || true)

    if [[ ${TOTAL_WARNINGS} -gt 0 ]]; then
        echo "   ⚠️  Security warnings detected:"
        echo "      High Confidence: ${HIGH_CONFIDENCE}"
        echo "      Medium Confidence: ${MEDIUM_CONFIDENCE}"
        echo ""
        echo "   Review detailed results:"
        echo "      cat /tmp/brakeman-results.txt"
        echo ""
        echo "   Top issues:"
        head -30 /tmp/brakeman-results.txt
    fi
fi

echo ""

exit 0

#!/bin/bash
# Kubernetes Dry-Run Server-Side Validation
# Validates K8s manifests with server-side dry-run or client-side fallback
# Exit 0 (informational)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/timeout-manager.sh"

# Start timeout tracker
start_timeout_tracker 120

# Check if k8s validation is enabled
if ! is_hook_enabled "k8s-dry-run"; then
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

# Only check K8s manifest files
if [[ ! "${FILE_PATH}" =~ \.(yaml|yml)$ ]]; then
    exit 0
fi

# Quick check if this looks like a K8s manifest
if ! grep -q "apiVersion:" "${FILE_PATH}" 2>/dev/null; then
    exit 0
fi

echo ""
echo "☸️  Validating Kubernetes Manifest..."
echo ""

VALIDATION_PASSED=false

# Try server-side dry-run first (requires cluster access)
if command -v kubectl >/dev/null 2>&1; then
    echo "   → Attempting server-side dry-run validation..."

    if kubectl apply --dry-run=server -f "${FILE_PATH}" 2>&1 | tee /tmp/k8s-server-dry-run.log; then
        echo "     ✅ Server-side validation passed"
        VALIDATION_PASSED=true
    else
        echo "     ⚠️  Server-side validation failed (may need cluster access)"

        # Fall back to client-side dry-run
        echo ""
        echo "   → Falling back to client-side validation..."

        if kubectl apply --dry-run=client -f "${FILE_PATH}" 2>&1 | tee /tmp/k8s-client-dry-run.log; then
            echo "     ✅ Client-side validation passed"
            VALIDATION_PASSED=true
        else
            echo "     ❌ Client-side validation failed"
            cat /tmp/k8s-client-dry-run.log
        fi
    fi
fi

# Run kubesec security scan if available
if command -v kubesec >/dev/null 2>&1 && check_remaining_time 30; then
    echo ""
    echo "   → Running kubesec security scan..."

    if kubesec scan "${FILE_PATH}" > /tmp/kubesec-results.json 2>&1; then
        SCORE=$(jq -r '.[0].score // 0' /tmp/kubesec-results.json)
        CRITICAL=$(jq -r '.[0].scoring.critical // []' /tmp/kubesec-results.json | jq 'length')

        echo "     Security Score: ${SCORE}"

        if [[ ${CRITICAL} -gt 0 ]]; then
            echo "     ⚠️  ${CRITICAL} critical security issue(s) detected"
            echo ""
            echo "     Review: cat /tmp/kubesec-results.json"
        else
            echo "     ✓ No critical security issues"
        fi
    fi
fi

echo ""

if [[ "${VALIDATION_PASSED}" == "true" ]]; then
    echo "✅ Kubernetes manifest validation complete"
else
    echo "⚠️  Validation incomplete - ensure kubectl is configured"
    echo ""
    echo "💡 To enable full validation:"
    echo "   1. Configure kubectl with cluster access"
    echo "   2. Use 'kubectl apply --dry-run=server -f <file>'"
fi

echo ""

exit 0

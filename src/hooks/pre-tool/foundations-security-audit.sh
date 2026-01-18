#!/bin/bash
# Comprehensive Security Audit
# Deep security scanning using multiple tools
# Only runs on commits or when agent=security
# Exit 2 if critical vulnerabilities, 0 otherwise

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/timeout-manager.sh"

# Start timeout tracker (10-minute budget)
start_timeout_tracker 600

# Check if security audit is enabled
if ! is_hook_enabled "security-audit"; then
    exit 0
fi

# Only run on specific triggers
AGENT_TYPE=$(get_agent_type)
VALIDATION_LEVEL=$(get_validation_level)

# Skip unless: main agent, security agent, or thorough validation
if [[ "${AGENT_TYPE}" != "main" ]] && \
   [[ "${AGENT_TYPE}" != "security" ]] && \
   [[ "${VALIDATION_LEVEL}" != "thorough" ]]; then
    exit 0
fi

# Read hook input from stdin
HOOK_INPUT=$(cat)
TOOL_NAME=$(parse_tool_name "${HOOK_INPUT}")

# Only run on Bash tool (likely commits) or when agent=security
if [[ "${TOOL_NAME}" != "Bash" ]] && [[ "${AGENT_TYPE}" != "security" ]]; then
    exit 0
fi

# Get project root
PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

echo ""
echo "🔒 Running Security Audit..."
echo ""

# Track critical findings
CRITICAL_FINDINGS=()
WARNING_FINDINGS=()

# 1. Trivy - Container and filesystem scanning
if command -v trivy >/dev/null 2>&1 && check_remaining_time 120; then
    echo "   → Scanning with Trivy..."

    if trivy fs --quiet --severity CRITICAL,HIGH "${PROJECT_ROOT}" > /tmp/trivy-results.txt 2>&1; then
        TRIVY_CRITICAL=$(grep -c "CRITICAL" /tmp/trivy-results.txt || true)
        TRIVY_HIGH=$(grep -c "HIGH" /tmp/trivy-results.txt || true)

        if [[ ${TRIVY_CRITICAL} -gt 0 ]]; then
            CRITICAL_FINDINGS+=("Trivy: ${TRIVY_CRITICAL} CRITICAL vulnerabilities")
        fi

        if [[ ${TRIVY_HIGH} -gt 0 ]]; then
            WARNING_FINDINGS+=("Trivy: ${TRIVY_HIGH} HIGH vulnerabilities")
        fi

        echo "     ✓ Trivy scan complete (CRITICAL: ${TRIVY_CRITICAL}, HIGH: ${TRIVY_HIGH})"
    fi
fi

# 2. Semgrep - Static analysis
if command -v semgrep >/dev/null 2>&1 && check_remaining_time 120; then
    echo "   → Scanning with Semgrep..."

    if semgrep scan --config=auto --severity ERROR --quiet "${PROJECT_ROOT}" > /tmp/semgrep-results.txt 2>&1; then
        SEMGREP_ERRORS=$(wc -l < /tmp/semgrep-results.txt | tr -d ' ')

        if [[ ${SEMGREP_ERRORS} -gt 0 ]]; then
            CRITICAL_FINDINGS+=("Semgrep: ${SEMGREP_ERRORS} security errors")
        fi

        echo "     ✓ Semgrep scan complete (Errors: ${SEMGREP_ERRORS})"
    fi
fi

# 3. Bundle Audit - Ruby dependencies
if [[ -f "${PROJECT_ROOT}/Gemfile.lock" ]] && command -v bundle-audit >/dev/null 2>&1 && check_remaining_time 60; then
    echo "   → Auditing Ruby dependencies..."

    cd "${PROJECT_ROOT}"
    if bundle-audit check --quiet > /tmp/bundle-audit-results.txt 2>&1; then
        echo "     ✓ No Ruby vulnerabilities found"
    else
        RUBY_VULNS=$(grep -c "Advisory" /tmp/bundle-audit-results.txt || true)
        if [[ ${RUBY_VULNS} -gt 0 ]]; then
            CRITICAL_FINDINGS+=("Bundle Audit: ${RUBY_VULNS} vulnerable gems")
        fi
        echo "     ⚠ Ruby vulnerabilities detected"
    fi
fi

# 4. NPM Audit - JavaScript dependencies
if [[ -f "${PROJECT_ROOT}/package.json" ]] && command -v npm >/dev/null 2>&1 && check_remaining_time 60; then
    echo "   → Auditing NPM dependencies..."

    cd "${PROJECT_ROOT}"
    if npm audit --audit-level=high --production > /tmp/npm-audit-results.txt 2>&1; then
        echo "     ✓ No critical NPM vulnerabilities found"
    else
        NPM_CRITICAL=$(grep -c "critical" /tmp/npm-audit-results.txt || true)
        NPM_HIGH=$(grep -c "high" /tmp/npm-audit-results.txt || true)

        if [[ ${NPM_CRITICAL} -gt 0 ]] || [[ ${NPM_HIGH} -gt 0 ]]; then
            CRITICAL_FINDINGS+=("NPM Audit: ${NPM_CRITICAL} critical, ${NPM_HIGH} high")
        fi
        echo "     ⚠ NPM vulnerabilities detected"
    fi
fi

# 5. Safety - Python dependencies
if [[ -f "${PROJECT_ROOT}/requirements.txt" ]] && command -v safety >/dev/null 2>&1 && check_remaining_time 60; then
    echo "   → Auditing Python dependencies..."

    if safety check --file="${PROJECT_ROOT}/requirements.txt" --output text > /tmp/safety-results.txt 2>&1; then
        echo "     ✓ No Python vulnerabilities found"
    else
        PY_VULNS=$(grep -c "vulnerability found" /tmp/safety-results.txt || true)
        if [[ ${PY_VULNS} -gt 0 ]]; then
            CRITICAL_FINDINGS+=("Safety: ${PY_VULNS} vulnerable Python packages")
        fi
        echo "     ⚠ Python vulnerabilities detected"
    fi
fi

echo ""

# Report findings
if [[ ${#CRITICAL_FINDINGS[@]} -gt 0 ]]; then
    echo "🚨 CRITICAL Security Issues Found:"
    echo ""
    for finding in "${CRITICAL_FINDINGS[@]}"; do
        echo "  • ${finding}"
    done
    echo ""
    echo "Review detailed results:"
    echo "  Trivy:   cat /tmp/trivy-results.txt"
    echo "  Semgrep: cat /tmp/semgrep-results.txt"
    echo ""
    echo "⚠️  Address critical issues before proceeding."
    echo ""
    exit 2
fi

if [[ ${#WARNING_FINDINGS[@]} -gt 0 ]]; then
    echo "⚠️  Security Warnings:"
    echo ""
    for finding in "${WARNING_FINDINGS[@]}"; do
        echo "  • ${finding}"
    done
    echo ""
    echo "💡 Review and address when possible."
    echo ""
fi

if [[ ${#CRITICAL_FINDINGS[@]} -eq 0 ]] && [[ ${#WARNING_FINDINGS[@]} -eq 0 ]]; then
    echo "✅ No critical security issues detected."
    echo ""
fi

exit 0

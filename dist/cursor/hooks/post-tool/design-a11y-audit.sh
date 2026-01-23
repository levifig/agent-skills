#!/bin/bash
# Accessibility Audit
# Validates accessibility compliance in frontend code
# Exit 0 (informational)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"

# Check if a11y audit is enabled
if ! is_hook_enabled "a11y-audit"; then
    exit 0
fi

# Only run for design or frontend-dev agent
AGENT_TYPE=$(get_agent_type)
if [[ "${AGENT_TYPE}" != "design" ]] && [[ "${AGENT_TYPE}" != "frontend-dev" ]]; then
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

# Only check component files
if [[ ! "${FILE_PATH}" =~ \.(tsx|jsx|vue|svelte)$ ]]; then
    exit 0
fi

echo ""
echo "♿ Running Accessibility Audit..."
echo ""

ISSUES=()

# 1. Check for missing alt attributes on images
if grep -q "<img[^>]*>" "${FILE_PATH}"; then
    if grep -q "<img[^>]*>" "${FILE_PATH}" | grep -v "alt=" ; then
        MISSING_ALT=$(grep -c "<img[^>]*>" "${FILE_PATH}" || true)
        if [[ ${MISSING_ALT} -gt 0 ]]; then
            ISSUES+=("⚠️  Images without alt attribute detected (${MISSING_ALT})")
        fi
    fi
fi

# 2. Check for button elements without accessible labels
if grep -qE "<button[^>]*>" "${FILE_PATH}"; then
    # Look for buttons without text content or aria-label
    if grep -E "<button[^>]*></button>" "${FILE_PATH}"; then
        ISSUES+=("⚠️  Empty button elements (need text or aria-label)")
    fi
fi

# 3. Check for form inputs without labels
if grep -qE "<input[^>]*>" "${FILE_PATH}"; then
    INPUT_COUNT=$(grep -cE "<input[^>]*>" "${FILE_PATH}" || true)
    LABEL_COUNT=$(grep -cE "<label[^>]*>" "${FILE_PATH}" || true)

    if [[ ${INPUT_COUNT} -gt ${LABEL_COUNT} ]]; then
        ISSUES+=("⚠️  More inputs than labels - ensure all inputs are labeled")
    fi
fi

# 4. Check for missing language attribute (if HTML root)
if grep -qE "<html[^>]*>" "${FILE_PATH}"; then
    if ! grep -qE "<html[^>]*lang=" "${FILE_PATH}"; then
        ISSUES+=("⚠️  Missing lang attribute on html element")
    fi
fi

# 5. Check for proper heading hierarchy
H1_COUNT=$(grep -cE "<h1[^>]*>" "${FILE_PATH}" || true)
if [[ ${H1_COUNT} -gt 1 ]]; then
    ISSUES+=("⚠️  Multiple h1 elements (should be only one per page)")
fi

# 6. Check for color contrast hints (hardcoded colors)
if grep -qE "(color:|background:).*(#[0-9a-fA-F]{3,6}|rgb|rgba)" "${FILE_PATH}"; then
    ISSUES+=("💡 Hardcoded colors detected - verify contrast ratio (WCAG 4.5:1)")
fi

# 7. ESLint jsx-a11y rules (if available)
PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

if command -v eslint >/dev/null 2>&1 || npx eslint --version >/dev/null 2>&1; then
    # Check if jsx-a11y plugin is configured
    if grep -q "eslint-plugin-jsx-a11y" "${PROJECT_ROOT}/package.json" 2>/dev/null; then
        echo "   → Running ESLint jsx-a11y rules..."

        if npx eslint --no-eslintrc --plugin jsx-a11y --rule 'jsx-a11y/*: error' "${FILE_PATH}" 2>&1 | tee /tmp/a11y-eslint.log; then
            echo "     ✅ No accessibility violations"
        else
            A11Y_ERRORS=$(grep -c "jsx-a11y/" /tmp/a11y-eslint.log || true)
            if [[ ${A11Y_ERRORS} -gt 0 ]]; then
                ISSUES+=("⚠️  ${A11Y_ERRORS} accessibility rule violation(s)")
                echo ""
                cat /tmp/a11y-eslint.log | head -20
            fi
        fi
    fi
fi

# Report findings
if [[ ${#ISSUES[@]} -gt 0 ]]; then
    echo ""
    echo "   Accessibility Issues:"
    for issue in "${ISSUES[@]}"; do
        echo "   ${issue}"
    done
    echo ""
    echo "💡 Accessibility Guidelines:"
    echo "   • All images must have meaningful alt text"
    echo "   • All interactive elements must be keyboard accessible"
    echo "   • Color contrast must meet WCAG AA standards (4.5:1)"
    echo "   • All form inputs must have associated labels"
    echo "   • Use semantic HTML (header, nav, main, footer)"
    echo "   • ARIA attributes when semantic HTML isn't enough"
    echo ""
    echo "   Resources:"
    echo "   • WCAG Guidelines: https://www.w3.org/WAI/WCAG21/quickref/"
    echo "   • Contrast Checker: https://webaim.org/resources/contrastchecker/"
else
    echo "   ✅ No obvious accessibility issues detected"
    echo ""
    echo "💡 Remember to test with:"
    echo "   • Screen readers (NVDA, JAWS, VoiceOver)"
    echo "   • Keyboard-only navigation"
    echo "   • axe DevTools browser extension"
fi

echo ""

exit 0

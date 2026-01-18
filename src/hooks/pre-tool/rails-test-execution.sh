#!/bin/bash
# Rails Test Execution
# Progressive test execution: single file → related tests → full suite
# Exit 0 always (informational)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/timeout-manager.sh"

# Start timeout tracker (8 minutes)
start_timeout_tracker 480

# Check if test execution is enabled
if ! is_hook_enabled "test-execution"; then
    exit 0
fi

# Only run for testing-qa agent or thorough validation
AGENT_TYPE=$(get_agent_type)
VALIDATION_LEVEL=$(get_validation_level)

if [[ "${AGENT_TYPE}" != "testing-qa" ]] && [[ "${VALIDATION_LEVEL}" != "thorough" ]]; then
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

# Only for Ruby files
if [[ ! "${FILE_PATH}" =~ \.rb$ ]]; then
    exit 0
fi

PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

# Check if Rails test suite exists
if [[ ! -d "${PROJECT_ROOT}/test" ]] && [[ ! -d "${PROJECT_ROOT}/spec" ]]; then
    exit 0
fi

echo ""
echo "🧪 Running Rails Tests..."
echo ""

# Determine test framework
TEST_FRAMEWORK="minitest"
if [[ -d "${PROJECT_ROOT}/spec" ]]; then
    TEST_FRAMEWORK="rspec"
fi

# Try to find corresponding test file
TEST_FILE=""
if [[ "${FILE_PATH}" =~ app/(models|controllers|helpers|mailers)/(.*)\.rb$ ]]; then
    COMPONENT_TYPE="${BASH_REMATCH[1]}"
    COMPONENT_NAME="${BASH_REMATCH[2]}"

    if [[ "${TEST_FRAMEWORK}" == "rspec" ]]; then
        TEST_FILE="${PROJECT_ROOT}/spec/${COMPONENT_TYPE}/${COMPONENT_NAME}_spec.rb"
    else
        TEST_FILE="${PROJECT_ROOT}/test/${COMPONENT_TYPE}/${COMPONENT_NAME}_test.rb"
    fi
fi

cd "${PROJECT_ROOT}"

# Progressive execution
if [[ -f "${TEST_FILE}" ]] && check_remaining_time 120; then
    echo "   → Running specific test: ${TEST_FILE#${PROJECT_ROOT}/}"

    if [[ "${TEST_FRAMEWORK}" == "rspec" ]]; then
        if bundle exec rspec "${TEST_FILE}" 2>&1 | tee /tmp/rails-test.log; then
            echo "     ✅ Tests passed"
        else
            echo "     ❌ Tests failed"
            echo ""
            echo "Review test output: cat /tmp/rails-test.log"
        fi
    else
        if bundle exec rails test "${TEST_FILE}" 2>&1 | tee /tmp/rails-test.log; then
            echo "     ✅ Tests passed"
        else
            echo "     ❌ Tests failed"
            echo ""
            echo "Review test output: cat /tmp/rails-test.log"
        fi
    fi
elif check_remaining_time 240; then
    echo "   → Running related tests..."

    # Determine test scope
    if [[ "${FILE_PATH}" =~ app/models/ ]]; then
        TEST_SCOPE="test/models/"
        if [[ "${TEST_FRAMEWORK}" == "rspec" ]]; then
            TEST_SCOPE="spec/models/"
        fi
    elif [[ "${FILE_PATH}" =~ app/controllers/ ]]; then
        TEST_SCOPE="test/controllers/"
        if [[ "${TEST_FRAMEWORK}" == "rspec" ]]; then
            TEST_SCOPE="spec/controllers/"
        fi
    else
        TEST_SCOPE=""
    fi

    if [[ -n "${TEST_SCOPE}" ]] && [[ -d "${PROJECT_ROOT}/${TEST_SCOPE}" ]]; then
        if [[ "${TEST_FRAMEWORK}" == "rspec" ]]; then
            bundle exec rspec "${TEST_SCOPE}" 2>&1 | tee /tmp/rails-test.log | tail -20
        else
            bundle exec rails test "${TEST_SCOPE}" 2>&1 | tee /tmp/rails-test.log | tail -20
        fi
    fi
else
    echo "   ⏱️  Insufficient time for test execution"
fi

ELAPSED=$(get_elapsed_time)
ELAPSED_FMT=$(format_duration ${ELAPSED})

echo ""
echo "✅ Test execution complete (${ELAPSED_FMT})"
echo ""

exit 0

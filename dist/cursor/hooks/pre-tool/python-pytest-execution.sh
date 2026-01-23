#!/bin/bash
# Progressive Pytest Execution
# Single file → module → full suite
# Exit 0 (informational)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/timeout-manager.sh"

# Start timeout tracker (8 minutes)
start_timeout_tracker 480

# Check if test execution is enabled
if ! is_hook_enabled "pytest-execution"; then
    exit 0
fi

# Only run for testing-qa agent or thorough validation
AGENT_TYPE=$(get_agent_type)
VALIDATION_LEVEL=$(get_validation_level)

if [[ "${AGENT_TYPE}" != "testing-qa" ]] && [[ "${VALIDATION_LEVEL}" != "thorough" ]]; then
    exit 0
fi

# Check if pytest is available
if ! command -v pytest >/dev/null 2>&1; then
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

# Only for Python files
if [[ ! "${FILE_PATH}" =~ \.py$ ]]; then
    exit 0
fi

PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

# Check if tests directory exists
if [[ ! -d "${PROJECT_ROOT}/tests" ]] && [[ ! -d "${PROJECT_ROOT}/test" ]]; then
    exit 0
fi

echo ""
echo "🧪 Running Pytest..."
echo ""

cd "${PROJECT_ROOT}"

# Determine test strategy based on file type
TEST_FILE=""
TEST_MODULE=""

# If this is a test file itself
if [[ "${FILE_PATH}" =~ test_.*\.py$ ]] || [[ "${FILE_PATH}" =~ _test\.py$ ]]; then
    TEST_FILE="${FILE_PATH}"
# If this is source code, find corresponding test
elif [[ "${FILE_PATH}" =~ /src/.*\.py$ ]] || [[ "${FILE_PATH}" =~ /app/.*\.py$ ]]; then
    BASENAME=$(basename "${FILE_PATH}" .py)

    # Try to find test file
    POSSIBLE_TEST_FILES=(
        "tests/test_${BASENAME}.py"
        "tests/${BASENAME}_test.py"
        "test/test_${BASENAME}.py"
        "test/${BASENAME}_test.py"
    )

    for test_file in "${POSSIBLE_TEST_FILES[@]}"; do
        if [[ -f "${PROJECT_ROOT}/${test_file}" ]]; then
            TEST_FILE="${PROJECT_ROOT}/${test_file}"
            break
        fi
    done

    # Determine module for broader tests
    if [[ "${FILE_PATH}" =~ /src/([^/]+)/ ]] || [[ "${FILE_PATH}" =~ /app/([^/]+)/ ]]; then
        MODULE="${BASH_REMATCH[1]}"
        TEST_MODULE="tests/test_${MODULE}/"
        if [[ ! -d "${PROJECT_ROOT}/${TEST_MODULE}" ]]; then
            TEST_MODULE="tests/"
        fi
    fi
fi

# Progressive execution
if [[ -f "${TEST_FILE}" ]] && check_remaining_time 120; then
    echo "   → Running specific test: ${TEST_FILE#${PROJECT_ROOT}/}"

    if pytest "${TEST_FILE}" -v --tb=short 2>&1 | tee /tmp/pytest.log | tail -30; then
        echo "     ✅ Tests passed"
    else
        echo "     ❌ Tests failed"
        echo ""
        echo "Review full output: cat /tmp/pytest.log"
    fi
elif [[ -n "${TEST_MODULE}" ]] && check_remaining_time 240; then
    echo "   → Running module tests: ${TEST_MODULE}"

    if pytest "${TEST_MODULE}" -v --tb=short 2>&1 | tee /tmp/pytest.log | tail -30; then
        echo "     ✅ Module tests passed"
    else
        echo "     ❌ Module tests failed"
    fi
elif check_remaining_time 300; then
    echo "   → Running full test suite..."

    if pytest -v --tb=short 2>&1 | tee /tmp/pytest.log | tail -50; then
        echo "     ✅ All tests passed"
    else
        echo "     ❌ Some tests failed"
    fi
else
    echo "   ⏱️  Insufficient time for test execution"
fi

# Show coverage if available
if command -v pytest-cov >/dev/null 2>&1 && check_remaining_time 60; then
    echo ""
    echo "   → Coverage report:"
    pytest --cov --cov-report=term-missing 2>&1 | tail -20 || true
fi

ELAPSED=$(get_elapsed_time)
ELAPSED_FMT=$(format_duration ${ELAPSED})

echo ""
echo "✅ Test execution complete (${ELAPSED_FMT})"
echo ""

exit 0

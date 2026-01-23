#!/bin/bash
# Python plugin hook: Validate test file structure
# Type: PreToolUse (informational, exit 0)
# Trigger: Before Edit/Write on test_*.py files

set -euo pipefail

# Read and parse stdin JSON
INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_name',''))" 2>/dev/null || echo "")
FILE_PATH=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" 2>/dev/null || echo "")
NEW_CONTENT=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('content','') or d.get('tool_input',{}).get('new_string',''))" 2>/dev/null || echo "")

# Only check test files
if [[ ! "$FILE_PATH" =~ test_.*\.py$ ]] && [[ ! "$FILE_PATH" =~ /tests/ ]]; then
    exit 0
fi

# Skip if not a test file
if [[ ! "$FILE_PATH" =~ \.py$ ]]; then
    exit 0
fi

WARNINGS=""

# Check for common test file issues
if [[ -n "$NEW_CONTENT" ]]; then
    # Check for pytest.mark.asyncio on async tests
    if echo "$NEW_CONTENT" | grep -q "^async def test_"; then
        if ! echo "$NEW_CONTENT" | grep -q "@pytest.mark.asyncio"; then
            WARNINGS="${WARNINGS}\n  - Async test functions should use @pytest.mark.asyncio decorator"
        fi
    fi
    
    # Check for proper test function naming
    if echo "$NEW_CONTENT" | grep -q "^def test" | grep -qv "^def test_"; then
        WARNINGS="${WARNINGS}\n  - Test function names should start with 'test_' (found 'test' without underscore)"
    fi
    
    # Check for bare asserts (should use assert with message or pytest assertions)
    BARE_ASSERTS=$(echo "$NEW_CONTENT" | grep -c "^\s*assert [^,]*$" || true)
    if [[ $BARE_ASSERTS -gt 3 ]]; then
        WARNINGS="${WARNINGS}\n  - Consider using pytest assertions or descriptive assert messages"
    fi
    
    # Check for missing pytest import
    if echo "$NEW_CONTENT" | grep -q "def test_"; then
        if ! echo "$NEW_CONTENT" | grep -q "import pytest"; then
            WARNINGS="${WARNINGS}\n  - Missing 'import pytest' in test file"
        fi
    fi
fi

if [[ -n "$WARNINGS" ]]; then
    echo "⚠️  Test file structure issues in $FILE_PATH:$WARNINGS"
    echo ""
    echo "Pytest best practices:"
    echo "  - Name test functions test_*"
    echo "  - Use @pytest.mark.asyncio for async tests"
    echo "  - Import pytest in test files"
    echo "  - Use fixtures for setup/teardown"
fi

# Always exit 0 (informational only)
exit 0

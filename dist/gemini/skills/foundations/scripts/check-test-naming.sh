#!/bin/bash
# Check test file naming conventions
# Usage: check-test-naming.sh [directory]
#
# Validates:
# - Test files follow test_*.py pattern
# - Test functions follow test_* pattern
# - Fixture naming conventions (*_perfect, *_degraded, *_chaos)

set -e

TARGET="${1:-.}"

echo "Checking test naming conventions in: $TARGET"
echo "================================================"

errors=0
warnings=0

# Check test file naming
echo ""
echo "Test File Naming:"
echo "-----------------"

# Find test files that don't follow convention
while IFS= read -r -d '' file; do
    filename=$(basename "$file")
    if [[ "$filename" != test_*.py ]] && [[ "$filename" != *_test.py ]] && [[ "$filename" != conftest.py ]]; then
        echo "⚠ $file"
        echo "   Should be test_*.py or *_test.py"
        warnings=$((warnings + 1))
    fi
done < <(find "$TARGET" -type f -name "*.py" -path "*/tests/*" -print0 2>/dev/null)

# Check test function naming
echo ""
echo "Test Function Naming:"
echo "---------------------"

while IFS= read -r -d '' file; do
    # Find function definitions that don't start with test_ in test files
    if [[ $(basename "$file") == test_*.py ]] || [[ $(basename "$file") == *_test.py ]]; then
        # Look for def statements that aren't test_ or helper functions
        grep -n "^def " "$file" 2>/dev/null | while read -r line; do
            func_name=$(echo "$line" | sed 's/.*def \([a-zA-Z_][a-zA-Z0-9_]*\).*/\1/')
            line_num=$(echo "$line" | cut -d: -f1)

            # Skip fixture functions and helper functions (starting with _)
            if [[ "$func_name" != test_* ]] && [[ "$func_name" != _* ]] && [[ "$func_name" != setup* ]] && [[ "$func_name" != teardown* ]]; then
                echo "⚠ $file:$line_num"
                echo "   Function '$func_name' doesn't start with 'test_'"
            fi
        done
    fi
done < <(find "$TARGET" -type f \( -name "test_*.py" -o -name "*_test.py" \) -print0 2>/dev/null)

# Check fixture naming patterns
echo ""
echo "Fixture Naming Patterns:"
echo "------------------------"

FIXTURE_PATTERNS="_perfect|_degraded|_chaos|_minimal|_empty|_invalid"

# Find fixture files
fixture_count=0
while IFS= read -r -d '' file; do
    fixture_count=$((fixture_count + 1))
    filename=$(basename "$file" .json)
    filename=$(basename "$filename" .yaml)
    filename=$(basename "$filename" .yml)

    # Check if follows scenario naming
    if ! echo "$filename" | grep -qE "($FIXTURE_PATTERNS)$"; then
        echo "⚠ $file"
        echo "   Consider scenario suffix: _perfect, _degraded, _chaos"
        warnings=$((warnings + 1))
    fi
done < <(find "$TARGET" -type f \( -name "*.json" -o -name "*.yaml" -o -name "*.yml" \) -path "*/fixtures/*" -print0 2>/dev/null)

if [[ $fixture_count -eq 0 ]]; then
    echo "No fixture files found in */fixtures/"
fi

# Summary
echo ""
echo "================================================"
if [[ $errors -eq 0 ]] && [[ $warnings -eq 0 ]]; then
    echo "✓ All test naming conventions followed"
    exit 0
else
    echo "Errors: $errors, Warnings: $warnings"
    exit 0  # Warnings don't fail the check
fi

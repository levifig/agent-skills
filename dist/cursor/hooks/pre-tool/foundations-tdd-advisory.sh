#!/bin/bash
# Hook: TDD Advisory (INFORMATIONAL)
# PreToolUse hook - reads JSON from stdin per Claude Code hooks API
#
# Triggers on Edit|Write to implementation files
# Warns when tests don't exist or don't fail first (non-blocking)
# Inspired by Cursor's TDD best practice

# Read and parse stdin JSON
INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" 2>/dev/null || echo "")

# Exit early if no file path
if [ -z "$FILE_PATH" ]; then
  exit 0
fi

# Skip if already a test file
if [[ "$FILE_PATH" == *"test"* ]] || [[ "$FILE_PATH" == *"spec"* ]] || [[ "$FILE_PATH" == *"_test."* ]] || [[ "$FILE_PATH" == *".test."* ]]; then
  exit 0
fi

# Skip non-code files
EXTENSION="${FILE_PATH##*.}"
case "$EXTENSION" in
  py|ts|tsx|js|jsx|rb|go|rs)
    # Continue checking
    ;;
  *)
    # Not a code file, skip
    exit 0
    ;;
esac

# Skip config and generated files
if [[ "$FILE_PATH" == *"config"* ]] || [[ "$FILE_PATH" == *"generated"* ]] || [[ "$FILE_PATH" == *"__init__"* ]]; then
  exit 0
fi

# Get the base name and directory for test file detection
BASENAME=$(basename "$FILE_PATH")
DIRNAME=$(dirname "$FILE_PATH")
FILENAME="${BASENAME%.*}"

# Build potential test file patterns based on language conventions
POTENTIAL_TESTS=()
case "$EXTENSION" in
  py)
    POTENTIAL_TESTS=(
      "${DIRNAME}/test_${FILENAME}.py"
      "${DIRNAME}/${FILENAME}_test.py"
      "tests/test_${FILENAME}.py"
      "tests/${FILENAME}_test.py"
    )
    ;;
  ts|tsx|js|jsx)
    POTENTIAL_TESTS=(
      "${DIRNAME}/${FILENAME}.test.${EXTENSION}"
      "${DIRNAME}/${FILENAME}.spec.${EXTENSION}"
      "${DIRNAME}/__tests__/${FILENAME}.test.${EXTENSION}"
      "__tests__/${FILENAME}.test.${EXTENSION}"
    )
    ;;
  rb)
    POTENTIAL_TESTS=(
      "spec/${FILENAME}_spec.rb"
      "${DIRNAME}/${FILENAME}_spec.rb"
      "test/${FILENAME}_test.rb"
    )
    ;;
  go)
    POTENTIAL_TESTS=(
      "${DIRNAME}/${FILENAME}_test.go"
    )
    ;;
esac

# Check if any test file exists
TEST_EXISTS=false
FOUND_TEST=""
for TEST_FILE in "${POTENTIAL_TESTS[@]}"; do
  if [ -f "$TEST_FILE" ]; then
    TEST_EXISTS=true
    FOUND_TEST="$TEST_FILE"
    break
  fi
done

# Output advisory (non-blocking)
if [ "$TEST_EXISTS" = false ]; then
  cat << EOF
# TDD Advisory

Editing implementation file: \`$FILE_PATH\`

**No corresponding test file found.** Consider Test-Driven Development:

1. Write a failing test first
2. Then implement the feature
3. Run tests until green

**Expected test locations:**
$(for t in "${POTENTIAL_TESTS[@]}"; do echo "- \`$t\`"; done)

**Why TDD?** Tests document expected behavior and prevent regressions.
Skip this advisory if you're:
- Doing a quick fix with existing test coverage
- Working in an area where tests aren't practical

EOF
fi

# Always exit 0 (non-blocking advisory)
exit 0

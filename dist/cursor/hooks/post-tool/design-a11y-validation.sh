#!/bin/bash
# Accessibility validation hook - runs after Edit/Write tool use
# Checks for common accessibility issues in code

set -e

# Colors for output
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get the file that was just edited/written
FILE_PATH="${TOOL_FILE_PATH:-}"

if [ -z "$FILE_PATH" ]; then
  exit 0
fi

# Only check relevant file types
if [[ ! "$FILE_PATH" =~ \.(tsx?|jsx?|html|vue|svelte)$ ]]; then
  exit 0
fi

echo -e "${BLUE}[A11y Check]${NC} Validating accessibility patterns..."

# Read file content
CONTENT=$(cat "$FILE_PATH" 2>/dev/null || echo "")

# Track if we found any issues
FOUND_ISSUES=0

# Check 1: Button-like divs without role
if echo "$CONTENT" | grep -qE '<div[^>]*onClick' && ! echo "$CONTENT" | grep -qE 'role="button"'; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found <div onClick> without role=\"button\""
  echo "  Consider using <button> instead, or add role=\"button\" and tabIndex={0}"
  FOUND_ISSUES=1
fi

# Check 2: Images without alt attribute
if echo "$CONTENT" | grep -qE '<img[^>]*src=' && echo "$CONTENT" | grep -vqE 'alt='; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found <img> without alt attribute"
  echo "  All images need alt=\"description\" or alt=\"\" for decorative images"
  FOUND_ISSUES=1
fi

# Check 3: Inputs without labels
if echo "$CONTENT" | grep -qE '<input[^>]*id=' && ! echo "$CONTENT" | grep -qE '(aria-label=|<label[^>]*for=)'; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found <input> without associated label"
  echo "  Add <label htmlFor=\"id\"> or aria-label attribute"
  FOUND_ISSUES=1
fi

# Check 4: Modals/dialogs without aria-modal
if echo "$CONTENT" | grep -qE '(role="dialog"|className.*modal)' && ! echo "$CONTENT" | grep -qE 'aria-modal'; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found dialog without aria-modal=\"true\""
  echo "  Add aria-modal=\"true\" and aria-labelledby attributes"
  FOUND_ISSUES=1
fi

# Check 5: Icon-only buttons without accessible label
if echo "$CONTENT" | grep -qE '<button[^>]*>[[:space:]]*<[A-Z][a-zA-Z]*Icon' && ! echo "$CONTENT" | grep -qE 'aria-label'; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found icon-only button without aria-label"
  echo "  Add aria-label=\"descriptive text\" to icon-only buttons"
  FOUND_ISSUES=1
fi

# Check 6: Missing focus-visible styles
if echo "$CONTENT" | grep -qE '(className|class)=' && ! echo "$CONTENT" | grep -qE '(focus-visible|focus:)'; then
  if echo "$CONTENT" | grep -qE '<(button|input|a|select|textarea)'; then
    echo -e "${YELLOW}⚠ Potential issue:${NC} Interactive elements may lack focus indicators"
    echo "  Consider adding focus-visible styles for keyboard navigation"
    FOUND_ISSUES=1
  fi
fi

if [ $FOUND_ISSUES -eq 0 ]; then
  echo -e "${BLUE}[A11y Check]${NC} No obvious accessibility issues detected ✓"
fi

# Exit 0 (informational only, don't block)
exit 0

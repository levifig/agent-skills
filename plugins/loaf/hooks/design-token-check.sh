#!/bin/bash
# Design token validation hook - runs after Edit/Write tool use
# Checks for hardcoded values that should use design tokens

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
if [[ ! "$FILE_PATH" =~ \.(tsx?|jsx?|css|scss|less|vue|svelte)$ ]]; then
  exit 0
fi

echo -e "${BLUE}[Token Check]${NC} Checking for hardcoded design values..."

# Read file content
CONTENT=$(cat "$FILE_PATH" 2>/dev/null || echo "")

# Track if we found any issues
FOUND_ISSUES=0

# Check 1: Hardcoded color values (hex, rgb, hsl)
if echo "$CONTENT" | grep -qE '(#[0-9a-fA-F]{3,8}|rgb\(|hsl\()' | grep -vE '(tokens\.|primitives\.|colors\.)'; then
  # Exclude common legitimate uses (transparent, currentColor, etc.)
  if echo "$CONTENT" | grep -E '(#[0-9a-fA-F]{3,8}|rgb\(|hsl\()' | grep -vE '(transparent|currentColor|inherit|tokens\.|primitives\.|colors\.)' > /dev/null 2>&1; then
    echo -e "${YELLOW}⚠ Potential issue:${NC} Found hardcoded color values"
    echo "  Consider using design tokens: tokens.color.* or primitives.color.*"
    echo "  Example: color: tokens.color.text.primary"
    FOUND_ISSUES=1
  fi
fi

# Check 2: Hardcoded pixel spacing values
if echo "$CONTENT" | grep -qE '(padding|margin|gap):[[:space:]]*[0-9]+px' | grep -vE '(spacing\[|spacing\.|tokens\.spacing)'; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found hardcoded spacing values (px)"
  echo "  Consider using spacing tokens: tokens.spacing[4] or spacing.component.padding.md"
  echo "  Example: padding: tokens.spacing[4] (16px)"
  FOUND_ISSUES=1
fi

# Check 3: Hardcoded font sizes
if echo "$CONTENT" | grep -qE 'font-size:[[:space:]]*[0-9]+px' | grep -vE '(fontSize\.|typography\.fontSize)'; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found hardcoded font-size values"
  echo "  Consider using typography tokens: typography.fontSize.base"
  echo "  Example: fontSize: typography.fontSize.lg"
  FOUND_ISSUES=1
fi

# Check 4: Hardcoded border radius
if echo "$CONTENT" | grep -qE 'border-radius:[[:space:]]*[0-9]+px' | grep -vE '(borderRadius\.|primitives\.borderRadius)'; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found hardcoded border-radius values"
  echo "  Consider using tokens: primitives.borderRadius.md"
  FOUND_ISSUES=1
fi

# Check 5: Hardcoded shadow values
if echo "$CONTENT" | grep -qE 'box-shadow:[[:space:]]*[0-9]' | grep -vE '(shadow\.|primitives\.shadow)'; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found hardcoded box-shadow values"
  echo "  Consider using tokens: primitives.shadow.md"
  FOUND_ISSUES=1
fi

# Check 6: Hardcoded transition durations
if echo "$CONTENT" | grep -qE 'transition:[^;]*[0-9]+ms' | grep -vE '(motion\.|timing\.)'; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found hardcoded transition durations"
  echo "  Consider using tokens: motion.duration.base"
  echo "  Example: transition: opacity \${motion.duration.fast} \${motion.easing.easeOut}"
  FOUND_ISSUES=1
fi

# Check 7: Inline styles in JSX (often a code smell for design tokens)
if echo "$CONTENT" | grep -qE 'style=\{\{[^}]*:(.*[0-9]+px|#[0-9a-fA-F]{3,8})' && [[ "$FILE_PATH" =~ \.(tsx?|jsx?)$ ]]; then
  echo -e "${YELLOW}⚠ Potential issue:${NC} Found inline styles with hardcoded values"
  echo "  Consider using className with token-based styles"
  FOUND_ISSUES=1
fi

if [ $FOUND_ISSUES -eq 0 ]; then
  echo -e "${BLUE}[Token Check]${NC} No hardcoded design values detected ✓"
else
  echo ""
  echo -e "${BLUE}ℹ Info:${NC} Using design tokens ensures consistency across your design system"
  echo "  See: design-core skill for token structure and naming conventions"
fi

# Exit 0 (informational only, don't block)
exit 0

#!/bin/bash
# Check for proper standard citations in Python code
# Usage: check-standard-refs.sh <file-or-directory>
# Verifies that physics code includes proper CIGRE/IEEE/IEC section references

set -e

TARGET="${1:-.}"

echo "Checking standard references in: $TARGET"
echo "================================================"

# Patterns to check for
PHYSICS_KEYWORDS="convection|radiation|thermal|catenary|sag|tension|conductor|heat_balance|skin_effect"
STANDARD_CITATION="CIGRE|IEEE|IEC|EN|ANSI"
SECTION_REF="[Ss]ection|[Cc]hapter|[Cc]lause|§"

# Find Python files with physics keywords but no standard citations
echo ""
echo "Files with physics code but NO standard references:"
echo "----------------------------------------------------"

found_issues=0

while IFS= read -r -d '' file; do
    # Check if file contains physics keywords
    if grep -qEi "$PHYSICS_KEYWORDS" "$file" 2>/dev/null; then
        # Check if file contains standard citations
        if ! grep -qE "$STANDARD_CITATION" "$file" 2>/dev/null; then
            echo "⚠ $file"
            echo "   Contains physics code but no CIGRE/IEEE/IEC references"
            found_issues=$((found_issues + 1))
        fi
    fi
done < <(find "$TARGET" -name "*.py" -type f -print0 2>/dev/null)

echo ""
echo "Files with standard references but missing section numbers:"
echo "------------------------------------------------------------"

while IFS= read -r -d '' file; do
    # Check if file mentions standards
    if grep -qE "$STANDARD_CITATION" "$file" 2>/dev/null; then
        # Check if section numbers are included
        if ! grep -qE "$SECTION_REF" "$file" 2>/dev/null; then
            echo "⚠ $file"
            echo "   References standard but no section/chapter numbers"
            # Show the lines with standard references
            grep -nE "$STANDARD_CITATION" "$file" | head -3 | sed 's/^/     /'
            found_issues=$((found_issues + 1))
        fi
    fi
done < <(find "$TARGET" -name "*.py" -type f -print0 2>/dev/null)

echo ""
echo "================================================"
if [[ $found_issues -eq 0 ]]; then
    echo "✓ All physics code has proper standard citations"
    exit 0
else
    echo "Found $found_issues file(s) needing attention"
    echo ""
    echo "Best practice: Include section references like:"
    echo '  # CIGRE TB 601, Section 4.2.3: Natural convection heat loss'
    echo '  # IEEE 738-2012, Equation 4-5: Solar heat gain'
    exit 1
fi

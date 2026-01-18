#!/usr/bin/env python3
"""
Check text for Linear compatibility.
Usage: check-linear-format.py <file>
       echo "text" | check-linear-format.py -

Validates:
- No emoji in progress lists
- Proper checkbox format
- No local file references
- No phase/stage terminology
"""

import re
import sys
from pathlib import Path


def check_linear_format(content: str) -> tuple[list[str], list[str]]:
    """Check content for Linear compatibility issues."""
    errors = []
    warnings = []

    lines = content.split('\n')

    for i, line in enumerate(lines, 1):
        # Check for emoji in progress lists (common Unicode emoji ranges)
        if re.match(r'^[-*]\s*\[.\]', line):
            # Check for emoji
            emoji_pattern = r'[\U0001F300-\U0001F9FF\U0001FA00-\U0001FA6F\U0001FA70-\U0001FAFF]'
            if re.search(emoji_pattern, line):
                errors.append(f"Line {i}: Emoji in progress list (use checkboxes only)")

        # Check for emoji bullets (not checkboxes)
        if re.match(r'^[\U0001F300-\U0001F9FF]', line):
            errors.append(f"Line {i}: Emoji bullet (use Markdown checkboxes)")

        # Check for local file references
        if '.agents/sessions/' in line or '.agents/councils/' in line:
            errors.append(f"Line {i}: Local file reference (remove before syncing to Linear)")

        if re.search(r'/Users/[^/]+/', line):
            errors.append(f"Line {i}: Absolute path (use relative paths)")

        # Check for phase/stage terminology
        phase_patterns = [
            r'\bPhase\s+\d',
            r'\bStage\s+\d',
            r'\bWeek\s+\d',
            r'\bSprint\s+\d',
        ]
        for pattern in phase_patterns:
            if re.search(pattern, line, re.IGNORECASE):
                warnings.append(f"Line {i}: Avoid phase/stage terminology for Linear")

        # Check for agent/council references
        agent_patterns = [
            r'\bspawned\s+(backend|frontend|dba|devops|testing)',
            r'\bcouncil\s+decision',
            r'\bsession\s+file',
        ]
        for pattern in agent_patterns:
            if re.search(pattern, line, re.IGNORECASE):
                warnings.append(f"Line {i}: Internal process reference (remove for Linear)")

        # Check for issue ID with full title
        if re.search(r'[A-Z]+-\d+\s+[A-Z]', line):
            # Might be ID followed by title
            warnings.append(f"Line {i}: Issue ID may include title (Linear auto-expands)")

    # Check for proper checkbox format
    checkbox_count = len(re.findall(r'^[-*]\s*\[[ x]\]', content, re.MULTILINE))
    bullet_count = len(re.findall(r'^[-*]\s+[^[\]]', content, re.MULTILINE))

    if bullet_count > 0 and checkbox_count == 0:
        warnings.append("No checkboxes found - consider using '- [ ]' for progress tracking")

    return errors, warnings


def main():
    # Read input
    if len(sys.argv) < 2:
        print("Usage: check-linear-format.py <file>")
        print("       echo 'text' | check-linear-format.py -")
        sys.exit(1)

    if sys.argv[1] == '-':
        content = sys.stdin.read()
        source = "stdin"
    else:
        filepath = Path(sys.argv[1])
        if not filepath.exists():
            print(f"Error: {filepath} not found")
            sys.exit(1)
        content = filepath.read_text()
        source = str(filepath)

    print(f"Checking Linear format: {source}")
    print("=" * 50)

    errors, warnings = check_linear_format(content)

    if errors:
        print("\nErrors (fix before syncing to Linear):")
        for e in errors:
            print(f"  ✗ {e}")

    if warnings:
        print("\nWarnings:")
        for w in warnings:
            print(f"  ⚠ {w}")

    print()
    if not errors:
        print("✓ Content is Linear-compatible" + (" (with warnings)" if warnings else ""))
        sys.exit(0)
    else:
        print(f"✗ Found {len(errors)} error(s)")
        sys.exit(1)


if __name__ == '__main__':
    main()

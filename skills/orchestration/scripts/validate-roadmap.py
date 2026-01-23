#!/usr/bin/env python3
"""Validate roadmap format for Now/Next/Later structure.

Usage: validate-roadmap.py <roadmap-file.md>
"""

import re
import sys
from pathlib import Path

# Patterns that indicate problematic content
VERSION_PATTERNS = [
    r"\bv\d+\.\d+",  # v1.0, v2.1
    r"\bversion\s+\d+",  # version 1
    r"\brelease\s+\d+",  # release 1
]

DATE_PATTERNS = [
    r"\b\d{4}-\d{2}-\d{2}\b",  # 2025-01-15
    r"\b(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{4}",  # Jan 2025
    r"\bQ[1-4]\s+\d{4}",  # Q1 2025
    r"\b\d{4}\s+Q[1-4]",  # 2025 Q1
]

TIME_ESTIMATE_PATTERNS = [
    r"\(\d+\s*(days?|weeks?|months?|hours?)\)",  # (2 weeks)
    r"\b\d+\s*(days?|weeks?|months?)\s*(from|after)",  # 2 weeks from
    r"\bby\s+(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)",  # by January
    r"\bdue\s+",  # due date
    r"\bdeadline",  # deadline
]

PHASE_PATTERNS = [
    r"\bphase\s+\d+",  # phase 1
    r"\bstage\s+\d+",  # stage 1
    r"\bsprint\s+\d+",  # sprint 1
    r"\biteration\s+\d+",  # iteration 1
]


def check_roadmap(content: str) -> list[str]:
    """Check roadmap content for format issues."""
    errors = []
    lines = content.split("\n")

    # Check for required sections
    has_now = bool(re.search(r"^##\s+Now", content, re.MULTILINE | re.IGNORECASE))
    has_next = bool(re.search(r"^##\s+Next", content, re.MULTILINE | re.IGNORECASE))
    has_later = bool(re.search(r"^##\s+Later", content, re.MULTILINE | re.IGNORECASE))

    if not (has_now or has_next or has_later):
        errors.append("Missing Now/Next/Later sections (need at least one)")

    # Check for version numbers
    for i, line in enumerate(lines, 1):
        for pattern in VERSION_PATTERNS:
            if re.search(pattern, line, re.IGNORECASE):
                errors.append(f"Line {i}: Version number found (use Now/Next/Later instead)")
                break

    # Check for date estimates
    for i, line in enumerate(lines, 1):
        for pattern in DATE_PATTERNS:
            if re.search(pattern, line):
                errors.append(f"Line {i}: Date found (avoid time-based planning)")
                break

    # Check for time estimates
    for i, line in enumerate(lines, 1):
        for pattern in TIME_ESTIMATE_PATTERNS:
            if re.search(pattern, line, re.IGNORECASE):
                errors.append(f"Line {i}: Time estimate found (avoid duration estimates)")
                break

    # Check for phase/sprint terminology
    for i, line in enumerate(lines, 1):
        for pattern in PHASE_PATTERNS:
            if re.search(pattern, line, re.IGNORECASE):
                errors.append(f"Line {i}: Phase/Sprint terminology (use Now/Next/Later)")
                break

    return errors


def main():
    if len(sys.argv) < 2:
        print("Usage: validate-roadmap.py <roadmap-file.md>")
        sys.exit(1)

    filepath = Path(sys.argv[1])
    if not filepath.exists():
        print(f"Error: File not found: {filepath}")
        sys.exit(1)

    content = filepath.read_text()
    errors = check_roadmap(content)

    print(f"Validating roadmap: {filepath}")
    print("=" * 50)

    if errors:
        for error in errors:
            print(f"⚠️  {error}")
        print("=" * 50)
        print(f"Found {len(errors)} issue(s)")
        print("\nRemember: Use Now/Next/Later buckets, not dates or versions")
        sys.exit(1)
    else:
        print("✓ Roadmap format looks good!")
        print("  - Uses Now/Next/Later structure")
        print("  - No version numbers")
        print("  - No date estimates")
        sys.exit(0)


if __name__ == "__main__":
    main()

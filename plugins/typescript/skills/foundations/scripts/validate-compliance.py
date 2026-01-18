#!/usr/bin/env python3
"""Validate security compliance checklist completion.

Usage: validate-compliance.py <checklist-file.md>

Checks that all checklist items are marked as complete.
"""

import re
import sys
from pathlib import Path


def parse_checklist(content: str) -> tuple[list[str], list[str]]:
    """Parse markdown checklist items.

    Returns:
        Tuple of (completed items, incomplete items)
    """
    completed = []
    incomplete = []

    for line in content.split("\n"):
        line = line.strip()

        # Match checklist items: - [ ] or - [x]
        match = re.match(r"-\s+\[([ xX])\]\s+(.+)", line)
        if match:
            checked = match.group(1).lower() == "x"
            item = match.group(2)

            if checked:
                completed.append(item)
            else:
                incomplete.append(item)

    return completed, incomplete


def main():
    if len(sys.argv) < 2:
        print("Usage: validate-compliance.py <checklist-file.md>")
        sys.exit(1)

    filepath = Path(sys.argv[1])
    if not filepath.exists():
        print(f"Error: File not found: {filepath}")
        sys.exit(1)

    content = filepath.read_text()
    completed, incomplete = parse_checklist(content)

    total = len(completed) + len(incomplete)
    if total == 0:
        print("No checklist items found in file")
        sys.exit(0)

    print(f"Compliance Checklist: {filepath.name}")
    print("=" * 50)
    print(f"Total items: {total}")
    print(f"Completed:   {len(completed)} ({len(completed)*100//total}%)")
    print(f"Incomplete:  {len(incomplete)} ({len(incomplete)*100//total}%)")
    print()

    if incomplete:
        print("Incomplete items:")
        for item in incomplete:
            print(f"  ❌ {item}")
        print()
        print("=" * 50)
        print(f"Compliance: INCOMPLETE ({len(incomplete)} items remaining)")
        sys.exit(1)
    else:
        print("=" * 50)
        print("✓ All compliance checks complete!")
        sys.exit(0)


if __name__ == "__main__":
    main()

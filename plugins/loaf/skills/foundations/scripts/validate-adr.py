#!/usr/bin/env python3
"""Validate Architecture Decision Record (ADR) format.

Usage: validate-adr.py <adr-file.md>
"""

import re
import sys
from pathlib import Path

VALID_STATUSES = {"Proposed", "Accepted", "Deprecated", "Superseded"}

REQUIRED_SECTIONS = [
    "Context",
    "Decision",
    "Consequences",
]


def validate_adr(filepath: Path) -> list[str]:
    """Validate ADR file and return list of errors."""
    errors = []
    content = filepath.read_text()
    lines = content.split("\n")

    # Check filename format
    filename = filepath.name
    if not re.match(r"ADR\d{3}-[\w-]+\.md", filename):
        errors.append(f"Filename should match ADRXXX-title.md pattern: {filename}")

    # Check for title
    if not lines or not lines[0].startswith("# ADR-"):
        errors.append("Missing ADR title (should start with '# ADR-XXX:')")

    # Check for decision date
    date_pattern = r"\*\*Decision Date\*\*:\s*\d{4}-\d{2}-\d{2}"
    if not re.search(date_pattern, content):
        errors.append("Missing Decision Date (format: **Decision Date**: YYYY-MM-DD)")

    # Check for status
    status_match = re.search(r"\*\*Status\*\*:\s*(\w+)", content)
    if not status_match:
        errors.append("Missing Status field")
    elif status_match.group(1) not in VALID_STATUSES:
        errors.append(
            f"Invalid status '{status_match.group(1)}'. "
            f"Must be one of: {', '.join(VALID_STATUSES)}"
        )

    # Check for required sections
    for section in REQUIRED_SECTIONS:
        pattern = rf"^##\s+{section}"
        if not re.search(pattern, content, re.MULTILINE):
            errors.append(f"Missing required section: ## {section}")

    # Check Consequences has subsections
    if "## Consequences" in content:
        if "### Positive" not in content:
            errors.append("Consequences section missing ### Positive subsection")
        if "### Negative" not in content:
            errors.append("Consequences section missing ### Negative subsection")

    # Check for Alternatives Considered
    if "## Alternatives Considered" not in content:
        errors.append("Missing ## Alternatives Considered section (even if brief)")

    return errors


def main():
    if len(sys.argv) < 2:
        print("Usage: validate-adr.py <adr-file.md>")
        sys.exit(1)

    filepath = Path(sys.argv[1])
    if not filepath.exists():
        print(f"Error: File not found: {filepath}")
        sys.exit(1)

    print(f"Validating: {filepath}")
    print("=" * 50)

    errors = validate_adr(filepath)

    if errors:
        for error in errors:
            print(f"❌ {error}")
        print("=" * 50)
        print(f"Found {len(errors)} issue(s)")
        sys.exit(1)
    else:
        print("✓ All checks passed!")
        sys.exit(0)


if __name__ == "__main__":
    main()

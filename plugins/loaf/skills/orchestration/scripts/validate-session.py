#!/usr/bin/env python3
"""
Validate session file format and required fields.
Usage: validate-session.py <session-file>
Exit codes: 0 = valid, 1 = invalid
"""

import sys
import re
import yaml
from pathlib import Path
from datetime import datetime


def parse_frontmatter(content: str) -> tuple[dict, str]:
    """Extract YAML frontmatter from markdown content."""
    if not content.startswith('---'):
        return {}, content

    parts = content.split('---', 2)
    if len(parts) < 3:
        return {}, content

    try:
        frontmatter = yaml.safe_load(parts[1])
        body = parts[2]
        return frontmatter or {}, body
    except yaml.YAMLError as e:
        print(f"Error parsing YAML frontmatter: {e}")
        return {}, content


def validate_filename(filepath: Path) -> list[str]:
    """Validate session filename format."""
    errors = []
    pattern = r'^\d{8}-\d{6}-[a-z0-9-]+\.md$'

    if not re.match(pattern, filepath.name):
        errors.append(f"Filename must match YYYYMMDD-HHMMSS-<description>.md, got: {filepath.name}")

    return errors


def validate_frontmatter(fm: dict) -> list[str]:
    """Validate required frontmatter fields."""
    errors = []

    # Check session block exists
    session = fm.get('session', {})
    if not session:
        errors.append("Missing 'session' block in frontmatter")
        return errors

    # Required session fields
    required_session = ['title', 'status', 'created', 'last_updated']
    for field in required_session:
        if field not in session:
            errors.append(f"Missing required field: session.{field}")
        elif not session[field]:
            errors.append(f"Empty required field: session.{field}")

    # Validate status values
    valid_statuses = ['in_progress', 'paused', 'completed', 'archived']
    if session.get('status') and session['status'] not in valid_statuses:
        errors.append(f"Invalid session.status: {session['status']} (must be one of: {', '.join(valid_statuses)})")

    # Validate ISO 8601 timestamps
    for field in ['created', 'last_updated']:
        value = session.get(field, '')
        if value:
            try:
                datetime.fromisoformat(value.replace('Z', '+00:00'))
            except ValueError:
                errors.append(f"Invalid ISO 8601 timestamp in session.{field}: {value}")

    # Validate branch field format if present (optional field)
    branch = session.get('branch', '')
    if branch:
        # Branch names should be non-empty strings without spaces
        # Common formats: feature/name, username/ticket-desc, bugfix/name, etc.
        if not isinstance(branch, str):
            errors.append(f"Invalid session.branch: must be a string, got {type(branch).__name__}")
        elif ' ' in branch:
            errors.append(f"Invalid session.branch: '{branch}' contains spaces (branch names cannot have spaces)")
        elif branch.startswith('/') or branch.endswith('/'):
            errors.append(f"Invalid session.branch: '{branch}' cannot start or end with '/'")
        elif '//' in branch:
            errors.append(f"Invalid session.branch: '{branch}' contains consecutive slashes")

    # Check orchestration block
    orch = fm.get('orchestration', {})
    if not orch:
        errors.append("Missing 'orchestration' block in frontmatter")
    elif not orch.get('current_task'):
        errors.append("Missing or empty: orchestration.current_task")

    return errors


def validate_sections(body: str) -> list[str]:
    """Validate required markdown sections."""
    errors = []
    required_sections = ['## Context', '## Current State', '## Next Steps']

    for section in required_sections:
        if section not in body:
            errors.append(f"Missing required section: {section}")

    return errors


def validate_linear_compat(body: str) -> list[str]:
    """Check for Linear-incompatible patterns."""
    warnings = []

    # Check for emoji in progress lists
    emoji_pattern = r'- \[.\] .*[\U0001F300-\U0001F9FF]'
    if re.search(emoji_pattern, body):
        warnings.append("Warning: Emoji found in progress lists (Linear uses checkboxes only)")

    # Check for local file references
    if '.agents/sessions/' in body or '.agents/councils/' in body:
        warnings.append("Warning: Local file references found (remove before syncing to Linear)")

    return warnings


def main():
    if len(sys.argv) != 2:
        print("Usage: validate-session.py <session-file>")
        sys.exit(1)

    filepath = Path(sys.argv[1])

    if not filepath.exists():
        print(f"Error: File not found: {filepath}")
        sys.exit(1)

    content = filepath.read_text()
    frontmatter, body = parse_frontmatter(content)

    errors = []
    warnings = []

    errors.extend(validate_filename(filepath))
    errors.extend(validate_frontmatter(frontmatter))
    errors.extend(validate_sections(body))
    warnings.extend(validate_linear_compat(body))

    # Output results
    if errors:
        print("VALIDATION FAILED")
        print("\nErrors:")
        for e in errors:
            print(f"  - {e}")

    if warnings:
        print("\nWarnings:")
        for w in warnings:
            print(f"  - {w}")

    if not errors:
        print("VALIDATION PASSED" + (" (with warnings)" if warnings else ""))

    sys.exit(1 if errors else 0)


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""
Validate council file format and required fields.
Usage: validate-council.py <council-file>
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
    """Validate council filename format."""
    errors = []
    pattern = r'^\d{8}-\d{6}-[a-z0-9-]+\.md$'

    if not re.match(pattern, filepath.name):
        errors.append(f"Filename must match YYYYMMDD-HHMMSS-<topic>.md, got: {filepath.name}")

    return errors


def validate_frontmatter(fm: dict) -> list[str]:
    """Validate required frontmatter fields."""
    errors = []

    # Check council block exists
    council = fm.get('council', {})
    if not council:
        errors.append("Missing 'council' block in frontmatter")
        return errors

    # Required council fields
    required_fields = ['topic', 'timestamp', 'status', 'session', 'participants', 'decision']
    for field in required_fields:
        if field not in council:
            errors.append(f"Missing required field: council.{field}")

    # Validate status values
    valid_statuses = ['pending', 'approved', 'rejected', 'deferred']
    if council.get('status') and council['status'] not in valid_statuses:
        errors.append(f"Invalid council.status: {council['status']} (must be one of: {', '.join(valid_statuses)})")

    # Validate ISO 8601 timestamp
    timestamp = council.get('timestamp', '')
    if timestamp:
        try:
            datetime.fromisoformat(timestamp.replace('Z', '+00:00'))
        except ValueError:
            errors.append(f"Invalid ISO 8601 timestamp: {timestamp}")

    # Validate participants
    participants = council.get('participants', [])
    if participants:
        if len(participants) < 5:
            errors.append(f"Council requires at least 5 participants (got {len(participants)})")
        if len(participants) % 2 == 0:
            errors.append(f"Council requires an ODD number of participants (got {len(participants)})")

    # Validate session link (file should exist, but we just check format here)
    session = council.get('session', '')
    if session:
        session_pattern = r'^\d{8}-\d{6}-[a-z0-9-]+$'
        if not re.match(session_pattern, session):
            errors.append(f"Invalid session reference format: {session}")

    return errors


def validate_sections(body: str) -> list[str]:
    """Validate required markdown sections."""
    errors = []
    required_sections = ['## Context', '## Decision', '## Rationale']

    for section in required_sections:
        if section not in body:
            errors.append(f"Missing required section: {section}")

    return errors


def main():
    if len(sys.argv) != 2:
        print("Usage: validate-council.py <council-file>")
        sys.exit(1)

    filepath = Path(sys.argv[1])

    if not filepath.exists():
        print(f"Error: File not found: {filepath}")
        sys.exit(1)

    content = filepath.read_text()
    frontmatter, body = parse_frontmatter(content)

    errors = []

    errors.extend(validate_filename(filepath))
    errors.extend(validate_frontmatter(frontmatter))
    errors.extend(validate_sections(body))

    # Output results
    if errors:
        print("VALIDATION FAILED")
        print("\nErrors:")
        for e in errors:
            print(f"  - {e}")
        sys.exit(1)
    else:
        print("VALIDATION PASSED")
        sys.exit(0)


if __name__ == '__main__':
    main()

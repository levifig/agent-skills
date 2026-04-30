#!/usr/bin/env python3
"""
Validate session file format for compact inline journal.
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
    if not content.startswith("---"):
        return {}, content

    parts = content.split("---", 2)
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
    warnings = []
    # Standard format: YYYYMMDD-HHMMSS-<description>.md
    # Also allow legacy 4-digit time: YYYYMMDD-HHMM-<description>.md
    standard_pattern = r"^\d{8}-\d{6}-[a-z0-9-]+\.md$"
    legacy_pattern = r"^\d{8}-\d{4}-[a-z0-9-]+\.md$"

    if re.match(standard_pattern, filepath.name):
        pass  # Valid standard format
    elif re.match(legacy_pattern, filepath.name):
        warnings.append(
            f"Filename uses legacy format (4-digit time): {filepath.name}. Consider renaming to YYYYMMDD-HHMMSS-<description>.md"
        )
    else:
        errors.append(
            f"Filename must match YYYYMMDD-HHMMSS-<description>.md, got: {filepath.name}"
        )

    return errors, warnings


def validate_frontmatter(fm: dict) -> list[str]:
    """Validate required frontmatter fields for compact format."""
    errors = []

    # Required fields (flat structure, not nested)
    required_fields = ["branch", "status", "created"]
    for field in required_fields:
        if field not in fm:
            errors.append(f"Missing required field: {field}")
        elif not fm[field]:
            errors.append(f"Empty required field: {field}")

    # Validate status values
    valid_statuses = ["active", "stopped", "done", "blocked", "archived"]
    if fm.get("status") and fm["status"] not in valid_statuses:
        errors.append(
            f"Invalid status: {fm['status']} (must be one of: {', '.join(valid_statuses)})"
        )

    # Validate ISO 8601 timestamps
    for field in ["created", "last_entry"]:
        value = fm.get(field, "")
        if value:
            try:
                datetime.fromisoformat(value.replace("Z", "+00:00"))
            except ValueError:
                errors.append(f"Invalid ISO 8601 timestamp in {field}: {value}")

    # Validate branch field
    branch = fm.get("branch", "")
    if branch:
        if not isinstance(branch, str):
            errors.append(
                f"Invalid branch: must be a string, got {type(branch).__name__}"
            )
        elif " " in branch:
            errors.append(f"Invalid branch: '{branch}' contains spaces")

    return errors


def validate_journal_entries(body: str) -> list[str]:
    """Validate compact journal entry format."""
    errors = []

    # Pattern for valid journal entries with scope (type(scope): desc)
    entry_pattern_with_scope = (
        r"^- (\d{4}-\d{2}-\d{2} \d{2}:\d{2}) (\w+)\(([^)]+)\): (.+)$"
    )
    # Pattern for entries without scope (type: desc) - for hypothesis, try, reject
    entry_pattern_without_scope = r"^- (\d{4}-\d{2}-\d{2} \d{2}:\d{2}) (\w+): (.+)$"

    # Entry types that don't require scope
    no_scope_types = ["hypothesis", "try", "reject"]

    lines = body.split("\n")
    for i, line in enumerate(lines, 1):
        line = line.strip()
        if not line or line.startswith("#") or line.startswith("---"):
            continue

        # Check if it looks like an entry (starts with - and has timestamp)
        if line.startswith("- ") and re.match(r"^- \d{4}-\d{2}-\d{2}", line):
            # Try with scope pattern first
            match = re.match(entry_pattern_with_scope, line)
            if match:
                timestamp_str, entry_type, scope, desc = match.groups()
            else:
                # Try without scope pattern
                match = re.match(entry_pattern_without_scope, line)
                if match:
                    timestamp_str, entry_type, desc = match.groups()
                    scope = None
                else:
                    errors.append(
                        f"Line {i}: Invalid entry format. Expected: `- YYYY-MM-DD HH:MM type(scope): description` or `- YYYY-MM-DD HH:MM type: description`"
                    )
                    continue

            # Validate timestamp format
            try:
                datetime.strptime(timestamp_str, "%Y-%m-%d %H:%M")
            except ValueError:
                errors.append(f"Line {i}: Invalid timestamp format: {timestamp_str}")

            # Validate entry type — keep in sync with cli/commands/session.ts EntryType
            valid_types = [
                "session",
                "start",
                "resume",
                "pause",
                "clear",
                "progress",
                "commit",
                "pr",
                "merge",
                "decision",
                "discover",
                "finding",
                "block",
                "unblock",
                "spark",
                "todo",
                "assume",
                "branch",
                "task",
                "linear",
                "hypothesis",
                "try",
                "reject",
                "compact",
                "skill",
                "wrap",
                "idea",
                "spec",
                "report",
                "council",
                "brainstorm",
                "plan",
                "draft",
            ]
            if entry_type not in valid_types:
                errors.append(f"Line {i}: Unknown entry type: {entry_type}")

            # Check if entry type requires scope but doesn't have it
            if scope is None and entry_type not in no_scope_types:
                errors.append(
                    f"Line {i}: Entry type '{entry_type}' requires scope. Expected: `{entry_type}(scope): description`"
                )

    return errors


def validate_pause_headers(body: str) -> list[str]:
    """Validate PAUSE header format."""
    errors = []

    # PAUSE header pattern
    pause_pattern = r"^--- PAUSE (\d{4}-\d{2}-\d{2} \d{2}:\d{2}) ---$"

    lines = body.split("\n")
    for i, line in enumerate(lines, 1):
        line = line.strip()
        if "PAUSE" in line:
            match = re.match(pause_pattern, line)
            if not match:
                errors.append(
                    f"Line {i}: Invalid PAUSE header. Expected: `--- PAUSE YYYY-MM-DD HH:MM ---`"
                )
            else:
                timestamp_str = match.group(1)
                try:
                    datetime.strptime(timestamp_str, "%Y-%m-%d %H:%M")
                except ValueError:
                    errors.append(
                        f"Line {i}: Invalid timestamp in PAUSE header: {timestamp_str}"
                    )

    return errors


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

    filename_errors, filename_warnings = validate_filename(filepath)
    errors.extend(filename_errors)
    warnings.extend(filename_warnings)
    errors.extend(validate_frontmatter(frontmatter))
    errors.extend(validate_journal_entries(body))
    errors.extend(validate_pause_headers(body))

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


if __name__ == "__main__":
    main()

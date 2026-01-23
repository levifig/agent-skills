#!/usr/bin/env python3
"""
Extract decisions from a session file and format as Serena memory.

Usage: extract-decisions.py <session-file>

Reads a session file, extracts the ## Decisions section, and outputs
formatted Serena memory content to stdout. The output can be piped to
Serena MCP's write_memory tool.

Exit codes:
  0 - Success (decisions extracted)
  1 - Error (file not found, parse error)
  2 - No decisions found
"""

import sys
import re
import yaml
from pathlib import Path
from datetime import datetime, timezone


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
        print(f"Warning: Could not parse frontmatter: {e}", file=sys.stderr)
        return {}, content


def extract_decisions_section(body: str) -> str | None:
    """Extract the ## Decisions section from markdown body."""
    # Match ## Decisions followed by content until next ## or end
    pattern = r'^## Decisions\s*\n(.*?)(?=^## |\Z)'
    match = re.search(pattern, body, re.MULTILINE | re.DOTALL)

    if match:
        content = match.group(1).strip()
        if content:
            return content
    return None


def parse_decisions(decisions_text: str) -> list[dict]:
    """Parse individual decisions from the decisions section."""
    decisions = []

    # Match ### Decision N: Title or ### Title patterns
    pattern = r'^###\s+(?:Decision\s+\d+:\s+)?(.+?)\n(.*?)(?=^### |\Z)'
    matches = re.findall(pattern, decisions_text, re.MULTILINE | re.DOTALL)

    for title, content in matches:
        decision = {
            'title': title.strip(),
            'decision': '',
            'rationale': '',
            'council': ''
        }

        # Extract **Decision**: value
        decision_match = re.search(
            r'\*\*Decision\*\*:\s*(.+?)(?=\n\*\*|\Z)',
            content, re.DOTALL
        )
        if decision_match:
            decision['decision'] = decision_match.group(1).strip()

        # Extract **Rationale**: value
        rationale_match = re.search(
            r'\*\*Rationale\*\*:\s*(.+?)(?=\n\*\*|\Z)',
            content, re.DOTALL
        )
        if rationale_match:
            decision['rationale'] = rationale_match.group(1).strip()

        # Extract **Council**: value (optional)
        council_match = re.search(
            r'\*\*Council\*\*:\s*(.+?)(?=\n\*\*|\Z)',
            content, re.DOTALL
        )
        if council_match:
            decision['council'] = council_match.group(1).strip()

        # Only add if we have at least a title and decision
        if decision['title'] and decision['decision']:
            decisions.append(decision)

    return decisions


def format_memory(
    session_file: str,
    frontmatter: dict,
    decisions: list[dict]
) -> str:
    """Format decisions as Serena memory markdown."""
    session = frontmatter.get('session', {})
    title = session.get('title', 'Unknown Session')
    linear_issue = session.get('linear_issue', 'N/A')
    archived_at = session.get('archived_at', datetime.now(timezone.utc).isoformat().replace('+00:00', 'Z'))

    # Generate slug from filename
    filename = Path(session_file).stem
    slug = re.sub(r'^\d{8}-\d{6}-', '', filename)

    lines = [
        f"# Memory: session-{slug}-decisions.md",
        "",
        "## Session Context",
        f"- **Session**: {Path(session_file).name}",
        f"- **Title**: {title}",
        f"- **Archived**: {archived_at}",
        f"- **Linear Issue**: {linear_issue}",
        "",
        "## Key Decisions",
        ""
    ]

    for i, d in enumerate(decisions, 1):
        lines.append(f"### Decision {i}: {d['title']}")
        lines.append(f"**Decision**: {d['decision']}")
        if d['rationale']:
            lines.append(f"**Rationale**: {d['rationale']}")
        if d['council']:
            lines.append(f"**Council**: {d['council']}")
        lines.append("")

    return '\n'.join(lines)


def generate_memory_name(session_file: str) -> str:
    """Generate Serena memory name from session filename."""
    filename = Path(session_file).stem
    slug = re.sub(r'^\d{8}-\d{6}-', '', filename)
    return f"session-{slug}-decisions.md"


def main():
    if len(sys.argv) != 2:
        print("Usage: extract-decisions.py <session-file>", file=sys.stderr)
        print("", file=sys.stderr)
        print("Extracts decisions from a session file and outputs Serena memory format.", file=sys.stderr)
        print("The memory name will be: session-<slug>-decisions.md", file=sys.stderr)
        sys.exit(1)

    session_file = sys.argv[1]
    filepath = Path(session_file)

    if not filepath.exists():
        print(f"Error: File not found: {session_file}", file=sys.stderr)
        sys.exit(1)

    content = filepath.read_text()
    frontmatter, body = parse_frontmatter(content)

    decisions_text = extract_decisions_section(body)
    if not decisions_text:
        print(f"No ## Decisions section found in: {session_file}", file=sys.stderr)
        sys.exit(2)

    decisions = parse_decisions(decisions_text)
    if not decisions:
        print(f"No parseable decisions found in: {session_file}", file=sys.stderr)
        sys.exit(2)

    # Output memory content to stdout
    memory_content = format_memory(session_file, frontmatter, decisions)
    print(memory_content)

    # Output memory name to stderr for scripting
    memory_name = generate_memory_name(session_file)
    print(f"\n# Memory name: {memory_name}", file=sys.stderr)
    print(f"# Decisions extracted: {len(decisions)}", file=sys.stderr)


if __name__ == '__main__':
    main()

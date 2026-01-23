#!/usr/bin/env python3
"""
PreToolUse hook: Validate commit messages against code-style conventions.

BLOCKING: Intercepts `git commit` commands and validates the message format.
Uses skills/code-style/scripts/check-commit-msg.sh for validation.

Exit codes:
  0 - Allow (validation passed or not applicable)
  2 - Block (validation failed, stderr contains reason)
"""
import json
import os
import re
import subprocess
import sys


def extract_commit_message(command: str) -> str | None:
    """Extract commit message from git commit command.

    Handles:
    - git commit -m "message"
    - git commit -m 'message'
    - git commit -m "$(cat <<'EOF'\nmessage\nEOF\n)"
    """
    # Match -m "message" or -m 'message' (non-greedy for nested quotes)
    # Handle escaped quotes and multi-word messages
    patterns = [
        r'-m\s+"([^"]+)"',      # Double quotes
        r"-m\s+'([^']+)'",      # Single quotes
        r'-m\s+"([^"]*(?:\\"[^"]*)*)"',  # Escaped double quotes
    ]

    for pattern in patterns:
        match = re.search(pattern, command)
        if match:
            return match.group(1)

    # Match HEREDOC: -m "$(cat <<'EOF' ... EOF )"
    heredoc_match = re.search(
        r"<<'?EOF'?\s*\n(.+?)\n\s*EOF",
        command,
        re.DOTALL
    )
    if heredoc_match:
        return heredoc_match.group(1).strip()

    return None


def find_validation_script() -> str | None:
    """Find the check-commit-msg.sh script.

    Searches in order:
    1. Plugin root (when running as plugin hook)
    2. Project .claude/scripts/ (when installed locally)
    """
    plugin_root = os.environ.get("CLAUDE_PLUGIN_ROOT", "")
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", ".")

    search_paths = [
        # Plugin location
        os.path.join(plugin_root, "skills/code-style/scripts/check-commit-msg.sh")
        if plugin_root else None,
        # Local project installation
        os.path.join(project_dir, ".claude/scripts/check-commit-msg.sh"),
    ]

    for path in search_paths:
        if path and os.path.exists(path):
            return path

    return None


def main():
    # Read hook input from stdin
    try:
        input_data = json.load(sys.stdin)
    except json.JSONDecodeError:
        # Can't parse input, allow
        sys.exit(0)

    tool_name = input_data.get("tool_name", "")
    tool_input = input_data.get("tool_input", {})
    command = tool_input.get("command", "")

    # Only check Bash commands that are git commits
    if tool_name != "Bash" or "git commit" not in command:
        sys.exit(0)

    # Skip if --amend without -m (uses existing message)
    if "--amend" in command and "-m" not in command:
        sys.exit(0)

    # Skip merge commits
    if "--no-edit" in command or "git merge" in command:
        sys.exit(0)

    # Extract commit message
    message = extract_commit_message(command)
    if not message:
        # Can't extract message (might be interactive), allow
        sys.exit(0)

    # Find validation script
    script_path = find_validation_script()
    if not script_path:
        # Script not found, allow but could warn
        sys.exit(0)

    try:
        result = subprocess.run(
            ["bash", script_path, "-"],
            input=message,
            capture_output=True,
            text=True,
            timeout=5
        )

        if result.returncode != 0:
            # Validation failed - BLOCK
            # Print validation output to stderr (shown to user as block reason)
            print(result.stdout.strip(), file=sys.stderr)
            sys.exit(2)  # Exit code 2 = block

    except subprocess.TimeoutExpired:
        # Allow on timeout to avoid blocking work
        pass
    except Exception as e:
        # Allow on error, could log for debugging
        pass

    sys.exit(0)


if __name__ == "__main__":
    main()

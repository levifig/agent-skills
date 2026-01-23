#!/usr/bin/env python3
"""
Detect Linear Magic Words in Commits
Suggests Linear status updates when magic words are detected in commit messages
Exit 0 (informational only)
"""

import json
import os
import re
import subprocess
import sys
from pathlib import Path


def get_project_root():
    """Get the project root directory."""
    try:
        result = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            capture_output=True,
            text=True,
            check=True
        )
        return Path(result.stdout.strip())
    except subprocess.CalledProcessError:
        return Path.cwd()


def load_config():
    """Load project configuration."""
    project_root = get_project_root()
    config_paths = [
        project_root / ".agents" / "config.json",
        project_root / ".claude" / "config.json",
        project_root / "config.json",
    ]

    for config_path in config_paths:
        if config_path.exists():
            with open(config_path) as f:
                return json.load(f)

    return {}


def is_hook_enabled():
    """Check if detect-linear-magic hook is enabled."""
    config = load_config()
    hook_config = config.get("hooks", {}).get("detect-linear-magic", {})
    return hook_config.get("enabled", True)


def get_recent_commits(count=5):
    """Get recent commit messages."""
    try:
        result = subprocess.run(
            ["git", "log", f"-{count}", "--pretty=format:%H%n%s%n%b%n---"],
            capture_output=True,
            text=True,
            check=True
        )

        commits = []
        commit_blocks = result.stdout.split("---\n")

        for block in commit_blocks:
            if not block.strip():
                continue

            lines = block.strip().split("\n")
            if len(lines) < 2:
                continue

            commit_hash = lines[0]
            subject = lines[1]
            body = "\n".join(lines[2:]) if len(lines) > 2 else ""

            commits.append({
                "hash": commit_hash[:8],
                "subject": subject,
                "body": body,
                "full_message": f"{subject}\n{body}".strip()
            })

        return commits

    except subprocess.CalledProcessError:
        return []


def detect_magic_words(text):
    """
    Detect Linear magic words in text.

    Magic words:
    - Fixes/Fixed/Fix <ISSUE-ID>
    - Closes/Closed/Close <ISSUE-ID>
    - Resolves/Resolved/Resolve <ISSUE-ID>
    """
    magic_patterns = [
        (r'\b(fixes?|fixed)\s+([A-Z]+-\d+)', "fix", "mark as Done"),
        (r'\b(closes?|closed)\s+([A-Z]+-\d+)', "close", "mark as Done"),
        (r'\b(resolves?|resolved)\s+([A-Z]+-\d+)', "resolve", "mark as Done"),
    ]

    detections = []
    text_lower = text.lower()

    for pattern, action, suggestion in magic_patterns:
        matches = re.finditer(pattern, text, re.IGNORECASE)
        for match in matches:
            issue_id = match.group(2)
            detections.append({
                "action": action,
                "issue_id": issue_id,
                "suggestion": suggestion,
                "text": match.group(0)
            })

    return detections


def main():
    """Main execution."""
    if not is_hook_enabled():
        sys.exit(0)

    # Get recent commits
    commits = get_recent_commits(count=3)

    if not commits:
        sys.exit(0)

    # Detect magic words
    all_detections = []
    for commit in commits:
        detections = detect_magic_words(commit["full_message"])
        if detections:
            all_detections.append({
                "commit": commit,
                "detections": detections
            })

    if not all_detections:
        sys.exit(0)

    # Print suggestions
    print("\n🔗 Linear Magic Words Detected!\n")

    for item in all_detections:
        commit = item["commit"]
        detections = item["detections"]

        print(f"   Commit: {commit['hash']} - {commit['subject']}")

        for detection in detections:
            print(f"   • Found: \"{detection['text']}\"")
            print(f"     → Suggestion: {detection['suggestion']} for {detection['issue_id']}")

        print()

    print("💡 Consider updating Linear issue status to match commit intent.")
    print("   Use the pm-orchestrator agent to sync status automatically.")
    print()

    sys.exit(0)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Suggest the best team for a task based on context.

Usage:
    suggest-team.py "task description"
    suggest-team.py --list-teams              # List all workspace teams
    suggest-team.py --add-known "Team Name"   # Add team to known_teams

Returns JSON with:
    - suggested_team: Team name and ID
    - confidence: high/medium/low
    - needs_confirmation: true if team is new to project
    - reason: Why this team was suggested
"""

import json
import re
import subprocess
import sys
from pathlib import Path


def find_config() -> Path:
    """Find config.json in .agents or .claude directory."""
    current = Path.cwd()
    for parent in [current] + list(current.parents):
        # New location: .agents/config.json
        config_path = parent / ".agents" / "config.json"
        if config_path.exists():
            return config_path
        # Legacy location: .claude/config.json
        legacy_path = parent / ".claude" / "config.json"
        if legacy_path.exists():
            return legacy_path
    raise FileNotFoundError("No .agents/config.json found")


def load_config() -> dict:
    """Load project configuration."""
    config_path = find_config()
    with open(config_path) as f:
        return json.load(f)


def save_config(config: dict) -> None:
    """Save project configuration."""
    config_path = find_config()
    with open(config_path, "w") as f:
        json.dump(config, f, indent=2)
        f.write("\n")


def get_workspace_teams() -> list[dict]:
    """Fetch all teams from Linear workspace via MCP.

    Note: This is a placeholder. In practice, Claude will use the
    Linear MCP tools directly. This script provides the logic.
    """
    # Return cached teams if available, or empty list
    # The actual team fetching happens via Linear MCP in Claude
    return []


def analyze_task(description: str, team_keywords: dict) -> list[tuple[str, int, str]]:
    """Analyze task description and score teams by keyword matches.

    Returns list of (team_name, score, matched_keywords) sorted by score.
    """
    description_lower = description.lower()
    scores = []

    for team, keywords in team_keywords.items():
        matched = []
        score = 0
        for keyword in keywords:
            # Check for keyword (word boundary matching)
            pattern = r'\b' + re.escape(keyword.lower()) + r'\b'
            if re.search(pattern, description_lower):
                matched.append(keyword)
                score += 1

        if score > 0:
            scores.append((team, score, ", ".join(matched)))

    # Sort by score descending
    scores.sort(key=lambda x: x[1], reverse=True)
    return scores


def is_team_known(team_name: str, config: dict) -> bool:
    """Check if team is already known to this project."""
    known_teams = config.get("linear", {}).get("known_teams", [])
    return any(t.get("name") == team_name for t in known_teams)


def add_known_team(team_name: str, team_id: str = "") -> dict:
    """Add a team to known_teams in config."""
    config = load_config()

    if "linear" not in config:
        config["linear"] = {}
    if "known_teams" not in config["linear"]:
        config["linear"]["known_teams"] = []

    # Check if already known
    if is_team_known(team_name, config):
        return {"status": "already_known", "team": team_name}

    # Add team
    config["linear"]["known_teams"].append({
        "name": team_name,
        "id": team_id
    })

    save_config(config)
    return {"status": "added", "team": team_name}


def suggest_team(description: str) -> dict:
    """Suggest the best team for a task."""
    try:
        config = load_config()
    except FileNotFoundError:
        return {
            "error": "No config.json found",
            "suggestion": "Run /init-config to set up project configuration"
        }

    linear_config = config.get("linear", {})
    team_keywords = linear_config.get("team_keywords", {})
    default_team = linear_config.get("default_team", "Engineering")

    if not team_keywords:
        return {
            "suggested_team": {"name": default_team},
            "confidence": "low",
            "needs_confirmation": not is_team_known(default_team, config),
            "reason": "No team_keywords configured, using default"
        }

    # Analyze task
    scores = analyze_task(description, team_keywords)

    if not scores:
        # No keyword matches, use default
        return {
            "suggested_team": {"name": default_team},
            "confidence": "low",
            "needs_confirmation": not is_team_known(default_team, config),
            "reason": f"No keyword matches, defaulting to {default_team}"
        }

    # Best match
    best_team, score, matched = scores[0]

    # Determine confidence
    if score >= 3:
        confidence = "high"
    elif score >= 2:
        confidence = "medium"
    else:
        confidence = "low"

    # Check if team is new to project
    needs_confirmation = not is_team_known(best_team, config)

    result = {
        "suggested_team": {"name": best_team},
        "confidence": confidence,
        "needs_confirmation": needs_confirmation,
        "reason": f"Matched keywords: {matched}",
        "alternatives": [
            {"name": t, "score": s, "keywords": k}
            for t, s, k in scores[1:3]  # Top 2 alternatives
        ]
    }

    if needs_confirmation:
        result["confirmation_message"] = (
            f"'{best_team}' hasn't been used in this project yet. "
            f"Assigning this issue will add {best_team} to the project. Proceed?"
        )

    return result


def main():
    if len(sys.argv) < 2:
        print("Usage: suggest-team.py <task description>", file=sys.stderr)
        print("       suggest-team.py --add-known <team-name> [team-id]", file=sys.stderr)
        sys.exit(1)

    if sys.argv[1] == "--add-known":
        if len(sys.argv) < 3:
            print("Usage: suggest-team.py --add-known <team-name> [team-id]", file=sys.stderr)
            sys.exit(1)
        team_name = sys.argv[2]
        team_id = sys.argv[3] if len(sys.argv) > 3 else ""
        result = add_known_team(team_name, team_id)
        print(json.dumps(result, indent=2))
    else:
        # Join all args as description
        description = " ".join(sys.argv[1:])
        result = suggest_team(description)
        print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()

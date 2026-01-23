#!/usr/bin/env python3
"""Read project configuration for Linear integration.

Usage:
    get-config.py                    # Print full Linear config
    get-config.py project.id         # Print specific value
    get-config.py default_teams      # Print default teams
"""

import json
import sys
from pathlib import Path


def find_config() -> Path:
    """Find config.json in .agents or .claude directory."""
    # Look in current directory and parents
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

    raise FileNotFoundError("No .agents/config.json found in project hierarchy")


def get_value(config: dict, key_path: str):
    """Get nested value from config using dot notation."""
    keys = key_path.split(".")
    value = config.get("linear", {})

    for key in keys:
        if isinstance(value, dict):
            value = value.get(key)
        else:
            return None

    return value


def main():
    try:
        config_path = find_config()
        with open(config_path) as f:
            config = json.load(f)

        linear_config = config.get("linear", {})

        if len(sys.argv) == 1:
            # Print full Linear config
            print(json.dumps(linear_config, indent=2))
        else:
            # Print specific value
            key_path = sys.argv[1]
            value = get_value(config, key_path)

            if value is None:
                print(f"Key not found: {key_path}", file=sys.stderr)
                sys.exit(1)

            if isinstance(value, (dict, list)):
                print(json.dumps(value, indent=2))
            else:
                print(value)

    except FileNotFoundError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
    except json.JSONDecodeError as e:
        print(f"Invalid JSON in config: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()

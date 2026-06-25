#!/bin/bash
# Compatibility shim. Sessions are SQLite-native after SPEC-045.

set -euo pipefail

echo "Error: new-session.sh no longer creates .agents/sessions/*.md files." >&2
echo "Use: loaf session start" >&2
echo "Then inspect with: loaf session show <session-ref>" >&2
exit 1

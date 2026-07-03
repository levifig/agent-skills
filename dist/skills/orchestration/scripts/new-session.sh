#!/bin/bash
# Retired shim. There is no session to create — journaling is continuous.

set -euo pipefail

echo "Error: new-session.sh is obsolete. There is no 'session' to start." >&2
echo "The project journal is the only session-related structure." >&2
echo "Log your first entry: loaf journal log \"skill(<name>): <purpose>\"" >&2
echo "Read continuity with: loaf journal context / loaf journal recent" >&2
exit 1

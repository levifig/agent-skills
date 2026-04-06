STOP. Before running gh pr merge, follow squash merge conventions:

1. TITLE: Let GitHub default it — "PR title (#N)" format.
2. BODY: Write a clean, concise summary of the branch's work (2-4 sentences).
   NEVER use the automatic squash description that dumps all individual commit messages.
3. Use --body flag with a HEREDOC to pass the clean description.

Example:
```
gh pr merge N --squash --body "$(cat <<'EOF'
Add loaf housekeeping CLI command that scans .agents/ directories and
recommends cleanup actions. Includes shared prompt helpers, scanner
engine, interactive/dry-run modes, and skill update.
EOF
)"
```

Rewrite your gh pr merge command now with a clean --body.

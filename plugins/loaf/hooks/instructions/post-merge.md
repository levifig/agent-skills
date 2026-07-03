**Note:** If you used `/ship`, these steps were already handled by the skill. This checklist is for manual merges.

# Pre-Merge Checklist

Complete these steps on the feature branch before creating the PR.

1. **Close out spec artifacts** (so they're included in the squash merge):
   ```
   loaf task update TASK-XXX --status done
   loaf task archive --spec SPEC-XXX
   loaf spec archive SPEC-XXX
   ```
   Write an optional `wrap(scope)` journal entry with `loaf journal log` if the work produced synthesis worth saving.

2. **Update CHANGELOG.md when the PR has release-facing impact:**
   Add curated entries under `[Unreleased]` describing what the PR lands. Do not move entries to a versioned section here; `/release` publishes the batch later.

3. **Rebuild all targets:**
   ```
   npx loaf build
   ```

4. **Commit and push** the changelog and generated artifacts to the PR branch.

5. **Create PR** with `gh pr create` — title + summary + test plan.

6. **Squash merge** with a clean commit body:
   - Let GitHub default the title: `PR title (#N)`
   - Write a concise 2-4 sentence summary as `--body` (use a HEREDOC)
   - **Never** use the automatic squash description that dumps all individual commit messages

---

# Post-Merge Housekeeping

Complete these steps on main after merging.

1. **Switch to main and pull:**
   ```
   git checkout main && git pull --rebase
   ```

2. **Delete merged feature branch:**
   ```
   git branch -d feat/xxx
   git push origin --delete feat/xxx
   ```

3. **Suggest reflection** if the session had key decisions or learnings.

4. **Suggest `/release` only when appropriate** — if this PR completes a coherent batch or release branch, publish from the base branch after the landed work is present there.

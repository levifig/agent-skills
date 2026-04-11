**Note:** If you used `/release`, these steps were already handled by the skill. This checklist is for manual merges.

# Pre-Merge Checklist

Complete these steps on the feature branch before creating the PR.

1. **Close out spec artifacts** (so they're included in the squash merge):
   ```
   loaf task update TASK-XXX --status done
   loaf task archive --spec SPEC-XXX
   loaf spec archive SPEC-XXX
   ```
   Archive session file (status: done, `archived_at`, `archived_by`, move to archive/).

2. **Update CHANGELOG.md:**
   Add entries under `[Unreleased]` describing what the PR ships.

3. **Bump version** in `package.json`:
   - `feat` commit -> minor bump (or dev bump if pre-release)
   - `fix` commit -> patch bump
   - Breaking change -> major bump

4. **Move `[Unreleased]` to versioned section:**
   `## [X.Y.Z] - YYYY-MM-DD`

5. **Rebuild all targets:**
   ```
   npx loaf build
   ```

6. **Commit and push** the changelog + version + archived artifacts to the PR branch.

7. **Create PR** with `gh pr create` — title + summary + test plan.

8. **Squash merge** with a clean commit body:
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

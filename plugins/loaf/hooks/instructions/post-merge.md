**Note:** If you used the ship workflow, these steps were already handled by the skill. This checklist is for manual merges.

# Post-Merge Housekeeping

Complete these steps after a successful squash merge. Leave the started worktree before removing it — do not run `loaf issue stop` from inside that worktree.

1. **Switch to the PR base and pull:**
   ```
   git checkout <baseRefName>
   git pull --ff-only origin <baseRefName>
   ```

2. **Mark the bound issue done** — this is what "done" means; `loaf issue stop` does not change status:
   ```
   loaf issue status <ref> done
   ```

3. **Stop the started worktree** if one exists (`loaf issue list --started`). `loaf issue stop` removes the worktree, clears `started_branch` / `started_worktree`, and keeps the branch:
   ```
   loaf issue stop <ref>
   ```
   If the issue was never started, the command errors with `issue <ref> is not started` — treat that as already clean and continue. Do not pass `--force` without user confirmation.

4. **Delete the local feature branch** when safe:
   ```
   git branch -d <headRefName>
   ```

5. **Log the landing:**
   ```
   loaf journal log "decision(ship): PR #N landed via squash merge; <ref> done"
   loaf journal log "commit(<hash>): <squash subject>"
   ```

6. **Suggest reflection** if the work produced key decisions or learnings.

7. **Suggest release only when appropriate** — if this PR completes a coherent batch, publish later with `loaf release suggest` / `loaf release cut`. The PR is landed, not released, until that cut.

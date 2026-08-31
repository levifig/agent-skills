**Note:** If you used the ship workflow, these steps were already handled by the skill. This checklist is for manual merges.

# Post-Merge Housekeeping

Complete these steps only after authoritative PR readback confirms the squash merge succeeded. Hook command matching alone does not prove success.

1. **Confirm the merge and capture identity.** Read the PR's state, merge commit, base branch, head branch, number, URL, and linked native tracker reference. If the PR is not observed as merged, stop without changing tracker or Git state.

2. **Switch to the PR base and pull:**
   ```
   git checkout <baseRefName>
   git pull --ff-only origin <baseRefName>
   ```

3. **Transition the canonical native tracker record.** Through the selected `project-management/v1` provider skill and harness-native connection, read the provider's valid statuses, perform the authorized completion transition, then read the same native record back. Report unsupported, failed, or indeterminate provider outcomes without creating a local fallback record.

4. **Clean up the Git worktree directly.** From outside the feature worktree, inspect registered worktrees and confirm the candidate is clean and no agent is using it:
   ```
   git worktree list
   git -C <worktree-path> status --short
   git worktree remove <worktree-path>
   ```
   If no linked worktree exists, continue. Never use `--force` without explicit user confirmation, and never remove the worktree while running inside it.

5. **Delete the local feature branch** when safe:
   ```
   git branch -d <headRefName>
   ```

6. **Log the landing:**
   ```
   loaf journal log "decision(ship): PR #N landed via squash merge; <native-ref> completed"
   loaf journal log "commit(<hash>): <squash subject>"
   ```

7. **Suggest reflection** if the work produced key decisions or learnings.

8. **Suggest release only when appropriate** — if this PR completes a coherent rideable batch, publish later with `loaf release suggest` / `loaf release cut`. The PR is landed, not released, until that cut.

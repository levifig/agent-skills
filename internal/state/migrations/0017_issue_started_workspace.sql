-- Started workspace: branch and worktree recorded on the issue row.
--
-- Worktrees were observed (journal, project identity, storage migration)
-- but never managed. This migration adds the two columns that bind an
-- issue to the workspace `loaf issue start` creates. started_branch and
-- started_worktree are nullable: an unstarted issue has neither. They are
-- written together when start records the workspace, and cleared together
-- when stop tears it down.
--
-- Status remains the events projection; these columns are workspace facts,
-- not a status. Stopping does not change status. There is no data
-- backfill: existing issues stay unstarted (NULL).
--
-- No new tables. ALTER TABLE ADD COLUMN is SQLite-safe.

ALTER TABLE issues ADD COLUMN started_branch TEXT;
ALTER TABLE issues ADD COLUMN started_worktree TEXT;

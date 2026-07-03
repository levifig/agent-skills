-- SPEC-056 journal-first: the project journal replaces the session entity.
-- Destructive and idempotent. Backfills harness_session_id onto journal rows,
-- purges lifecycle-noise entries, drops the session FK columns/tables, and
-- rebuilds the journal FTS index keyed on harness_session_id.
--
-- Ordering note: PRAGMA foreign_keys is a no-op inside a transaction, and
-- ApplyMigrations runs every migration inside one transaction. The rebuilds
-- below therefore drop the session FK columns from handoffs and journal_entries
-- BEFORE dropping the sessions table, so no live foreign key references the
-- sessions table at COMMIT. SQLite's DROP TABLE does not enforce child->parent
-- foreign keys, so intermediate states inside the transaction are safe.

-- (1) Backfill harness_session_id from the owning session record.
UPDATE journal_entries
SET harness_session_id = (
  SELECT s.harness_session_id FROM sessions s WHERE s.id = journal_entries.session_id
)
WHERE harness_session_id IS NULL
  AND session_id IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM sessions s
    WHERE s.id = journal_entries.session_id AND s.harness_session_id IS NOT NULL
  );

-- (2) Purge lifecycle-noise entries safely. entry_type='session' was the
-- lifecycle machinery's bookkeeping type; on this installation every such row is
-- a machine-generated marker or auto-summary. But an unconditional
-- `DELETE FROM journal_entries WHERE entry_type='session'` would also destroy any
-- user-authored session(...) row on some other installation. So instead of
-- trusting the type blindly, we enumerate the exact machine-generated shapes and
-- purge ONLY those; anything typed 'session' that does not match is preserved by
-- renaming its entry_type to 'legacy_session' (content untouched). The 'session'
-- type still dies; unknown human content survives and is re-indexed by the FTS
-- rebuild in step (8) like any other row.
--
-- The machine shapes were derived empirically from the production journal
-- (entry_type='session' rows). Each family is anchored on both scope and message
-- so a coincidental human message under a lifecycle scope is not swept up:
--   start   : "=== SESSION STARTED ==="  [+ optional " (session <hex>)"]
--   resume  : "=== SESSION RESUMED ==="   [+ optional " (session <hex>)"]
--   stop    : "=== SESSION STOPPED ===" | "=== SESSION COMPLETE ==="
--   clear   : "=== CONTEXT CLEARED ==="
--   end / conclude / wrap (machine auto-summaries, NOT the deliberate
--            entry_type='wrap' synthesis rows, which are a different type):
--            "at commit <hash>[, N commits][, N decisions][, N entries]"
--            | "session ended"
--            | "closed by new conversation"
--            | "session handed off, pending final status update"
--   context : "from commit <hash>"  arrival stamps
--   merge   : "consolidated from <file>.md"  housekeeping records
--   test    : "verify session type"  fixture
--   enrich  : "recorded native SQLite enrichment checkpoint"
--
-- Deliberate synthesis ("tried X, abandoned Y, next Z") lives in entry_type='wrap'
-- rows (SPEC-056 §Solution Direction) and is never touched: we only ever read or
-- rewrite entry_type='session'.

-- (2a) Preserve any entry_type='session' row that is NOT a known machine shape
-- by demoting it to 'legacy_session'. Runs before the purge so unknown human
-- content is already off the 'session' type when the DELETE fires.
--
-- NULL-safety: journal_entries.scope is NULLABLE, so a bare `scope = 'start'`
-- yields SQL NULL (not false) on a NULL-scope row; the whole OR-chain then
-- collapses to NULL and NOT(NULL) is NULL, so the row would NOT be demoted and
-- would be swept up by the DELETE in step (2b). Machine writers ALWAYS set
-- scope, so a NULL-scope row is not provably machine-generated and must be
-- preserved. COALESCE(scope,'') maps NULL to '' — which equals no family scope
-- literal and satisfies no IN(...) list — so the machine-shape match reliably
-- fails and the row is demoted to legacy_session. (message is NOT NULL in the
-- schema, so message comparisons never introduce the same NULL hazard.)
UPDATE journal_entries
SET entry_type = 'legacy_session'
WHERE entry_type = 'session'
  AND NOT (
       (COALESCE(scope, '') = 'start'  AND (message = '=== SESSION STARTED ==='
                              OR message GLOB '=== SESSION STARTED === (session *)'))
    OR (COALESCE(scope, '') = 'resume' AND (message = '=== SESSION RESUMED ==='
                              OR message GLOB '=== SESSION RESUMED === (session *)'))
    OR (COALESCE(scope, '') = 'stop'   AND message IN ('=== SESSION STOPPED ===', '=== SESSION COMPLETE ==='))
    OR (COALESCE(scope, '') = 'clear'  AND message = '=== CONTEXT CLEARED ===')
    OR (COALESCE(scope, '') IN ('end', 'conclude', 'wrap')
                         AND (message LIKE 'at commit %'
                              OR message IN ('session ended',
                                             'closed by new conversation',
                                             'session handed off, pending final status update')))
    OR (COALESCE(scope, '') = 'context' AND message LIKE 'from commit %')
    OR (COALESCE(scope, '') = 'merge'   AND message LIKE 'consolidated from %')
    OR (COALESCE(scope, '') = 'test'    AND message = 'verify session type')
    OR (COALESCE(scope, '') = 'enrich'  AND message = 'recorded native SQLite enrichment checkpoint')
  );

-- (2b) Purge the remaining entry_type='session' rows: now exactly the machine
-- shapes, since every unknown row was demoted to 'legacy_session' above.
DELETE FROM journal_entries WHERE entry_type = 'session';

-- (3) Rebuild handoffs: replace the session_id FK with a plain
-- harness_session_id provenance column. Backfill from the session record.
CREATE TABLE handoffs_new (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  harness_session_id TEXT,
  task_id TEXT,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (task_id) REFERENCES tasks(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

INSERT INTO handoffs_new (id, project_id, harness_session_id, task_id, title, status, body_source_id, created_at, updated_at)
SELECT
  h.id,
  h.project_id,
  (SELECT s.harness_session_id FROM sessions s WHERE s.id = h.session_id),
  h.task_id,
  h.title,
  h.status,
  h.body_source_id,
  h.created_at,
  h.updated_at
FROM handoffs h;

DROP TABLE handoffs;
ALTER TABLE handoffs_new RENAME TO handoffs;
CREATE INDEX IF NOT EXISTS idx_handoffs_context ON handoffs (project_id, harness_session_id, task_id);

-- (4) Delete session lifecycle rows from the event log and alias registry.
DELETE FROM events WHERE entity_kind = 'session';
DELETE FROM aliases WHERE entity_kind = 'session';

-- (5) Rebuild journal_entries without the session_id FK column.
CREATE TABLE journal_entries_new (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  entry_type TEXT NOT NULL,
  scope TEXT,
  message TEXT NOT NULL,
  observed_branch TEXT,
  observed_worktree TEXT,
  harness_session_id TEXT,
  spec_id TEXT,
  task_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (spec_id) REFERENCES specs(id),
  FOREIGN KEY (task_id) REFERENCES tasks(id)
);

INSERT INTO journal_entries_new (id, project_id, entry_type, scope, message, observed_branch, observed_worktree, harness_session_id, spec_id, task_id, created_at, updated_at)
SELECT id, project_id, entry_type, scope, message, observed_branch, observed_worktree, harness_session_id, spec_id, task_id, created_at, updated_at
FROM journal_entries;

DROP TABLE journal_entries;
ALTER TABLE journal_entries_new RENAME TO journal_entries;
CREATE INDEX IF NOT EXISTS idx_journal_context ON journal_entries (project_id, spec_id, task_id);
CREATE INDEX IF NOT EXISTS idx_journal_harness ON journal_entries (project_id, harness_session_id);

-- (6) Drop the empty per-session snapshot table.
DROP TABLE IF EXISTS session_state_snapshots;

-- (7) Drop the sessions table. Safe now that no live FK references it.
DROP TABLE IF EXISTS sessions;

-- (8) Rebuild the journal FTS index keyed on harness_session_id.
DROP TABLE IF EXISTS journal_search;
CREATE VIRTUAL TABLE journal_search USING fts5(
  project_id UNINDEXED,
  journal_entry_id UNINDEXED,
  harness_session_id UNINDEXED,
  entry_type,
  scope,
  message
);
INSERT INTO journal_search(rowid, project_id, journal_entry_id, harness_session_id, entry_type, scope, message)
SELECT rowid, project_id, id, COALESCE(harness_session_id, ''), entry_type, COALESCE(scope, ''), message
FROM journal_entries;

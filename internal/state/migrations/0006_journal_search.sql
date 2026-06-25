CREATE VIRTUAL TABLE IF NOT EXISTS journal_search USING fts5(
  project_id UNINDEXED,
  journal_entry_id UNINDEXED,
  session_id UNINDEXED,
  entry_type,
  scope,
  message
);

INSERT INTO journal_search(rowid, project_id, journal_entry_id, session_id, entry_type, scope, message)
SELECT rowid, project_id, id, COALESCE(session_id, ''), entry_type, COALESCE(scope, ''), message
FROM journal_entries
WHERE rowid NOT IN (SELECT rowid FROM journal_search);

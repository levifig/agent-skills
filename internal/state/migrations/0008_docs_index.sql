CREATE TABLE IF NOT EXISTS docs_index (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  path TEXT NOT NULL,
  content TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  indexed_ref TEXT,
  indexed_worktree TEXT NOT NULL,
  indexed_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, indexed_worktree, path)
);

CREATE VIRTUAL TABLE IF NOT EXISTS docs_search USING fts5(
  project_id UNINDEXED,
  id UNINDEXED,
  path,
  content,
  content='docs_index',
  content_rowid='rowid'
);

CREATE INDEX IF NOT EXISTS idx_docs_index_project_path ON docs_index (project_id, path);
CREATE INDEX IF NOT EXISTS idx_docs_index_worktree ON docs_index (project_id, indexed_worktree);

-- Client sync engine machine-local state: outbound queue and relay arrival cursors.

CREATE TABLE IF NOT EXISTS sync_outbound_queue (
  project_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  enqueued_at TEXT NOT NULL,
  pushed_at TEXT,
  PRIMARY KEY (project_id, fact_id),
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE INDEX IF NOT EXISTS idx_sync_outbound_queue_pending
  ON sync_outbound_queue (project_id, enqueued_at)
  WHERE pushed_at IS NULL;

CREATE TABLE IF NOT EXISTS sync_project_cursors (
  project_id TEXT PRIMARY KEY NOT NULL,
  arrival_cursor INTEGER NOT NULL DEFAULT 0 CHECK (arrival_cursor >= 0),
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

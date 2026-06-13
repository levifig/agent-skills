CREATE TABLE IF NOT EXISTS session_state_snapshots (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  content TEXT NOT NULL,
  observed_branch TEXT,
  observed_worktree TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (session_id) REFERENCES sessions(id),
  UNIQUE (project_id, session_id)
);

CREATE INDEX IF NOT EXISTS idx_session_state_snapshots_session ON session_state_snapshots (project_id, session_id);

ALTER TABLE projects ADD COLUMN friendly_name TEXT;
ALTER TABLE projects ADD COLUMN current_path TEXT;
ALTER TABLE projects ADD COLUMN last_seen_at TEXT;

CREATE TABLE IF NOT EXISTS project_paths (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  path TEXT NOT NULL,
  is_current INTEGER NOT NULL,
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (path)
);

CREATE INDEX IF NOT EXISTS idx_project_paths_project_current ON project_paths (project_id, is_current);

ALTER TABLE relationships ADD COLUMN origin TEXT;
ALTER TABLE relationships ADD COLUMN source_id TEXT;
ALTER TABLE relationships ADD COLUMN source_field TEXT;

CREATE INDEX IF NOT EXISTS idx_relationships_origin ON relationships (project_id, origin);

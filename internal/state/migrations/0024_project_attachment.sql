CREATE TABLE IF NOT EXISTS project_conf_labels (
  conf_id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS project_attachment_evidence (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  evidence_kind TEXT NOT NULL,
  evidence_value TEXT NOT NULL,
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, evidence_kind, evidence_value)
);

CREATE INDEX IF NOT EXISTS idx_project_attachment_evidence_lookup
  ON project_attachment_evidence (evidence_kind, evidence_value);

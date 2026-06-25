CREATE TABLE IF NOT EXISTS artifact_bodies (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  entity_kind TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  body_kind TEXT NOT NULL DEFAULT 'markdown',
  content TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (source_id) REFERENCES sources(id),
  UNIQUE (project_id, entity_kind, entity_id, body_kind)
);

CREATE VIRTUAL TABLE IF NOT EXISTS artifact_search USING fts5(
  project_id UNINDEXED,
  entity_kind,
  entity_id,
  body_kind,
  content,
  content='artifact_bodies',
  content_rowid='rowid'
);

CREATE TABLE IF NOT EXISTS plans (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  spec_id TEXT,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (spec_id) REFERENCES specs(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

CREATE TABLE IF NOT EXISTS handoffs (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  session_id TEXT,
  task_id TEXT,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (session_id) REFERENCES sessions(id),
  FOREIGN KEY (task_id) REFERENCES tasks(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

CREATE TABLE IF NOT EXISTS councils (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  spec_id TEXT,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (spec_id) REFERENCES specs(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

CREATE INDEX IF NOT EXISTS idx_artifact_bodies_entity ON artifact_bodies (project_id, entity_kind, entity_id);
CREATE INDEX IF NOT EXISTS idx_artifact_bodies_source ON artifact_bodies (project_id, source_id);
CREATE INDEX IF NOT EXISTS idx_plans_spec ON plans (project_id, spec_id);
CREATE INDEX IF NOT EXISTS idx_handoffs_context ON handoffs (project_id, session_id, task_id);
CREATE INDEX IF NOT EXISTS idx_councils_spec ON councils (project_id, spec_id);

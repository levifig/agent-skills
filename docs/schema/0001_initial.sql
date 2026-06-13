CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY NOT NULL,
  name TEXT NOT NULL,
  checksum TEXT NOT NULL,
  applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY NOT NULL,
  identity_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS aliases (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  entity_kind TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  namespace TEXT NOT NULL,
  alias TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, namespace, alias)
);

CREATE TABLE IF NOT EXISTS specs (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  spec_id TEXT,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  priority TEXT,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (spec_id) REFERENCES specs(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

CREATE TABLE IF NOT EXISTS ideas (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

CREATE TABLE IF NOT EXISTS sparks (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  scope TEXT,
  status TEXT NOT NULL,
  text TEXT NOT NULL,
  source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (source_id) REFERENCES sources(id)
);

CREATE TABLE IF NOT EXISTS brainstorms (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

CREATE TABLE IF NOT EXISTS shaping_drafts (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  harness_session_id TEXT,
  branch TEXT,
  status TEXT NOT NULL,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

CREATE TABLE IF NOT EXISTS reports (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  report_kind TEXT NOT NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  body_source_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (body_source_id) REFERENCES sources(id)
);

CREATE TABLE IF NOT EXISTS journal_entries (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  entry_type TEXT NOT NULL,
  scope TEXT,
  message TEXT NOT NULL,
  observed_branch TEXT,
  observed_worktree TEXT,
  harness_session_id TEXT,
  session_id TEXT,
  spec_id TEXT,
  task_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (session_id) REFERENCES sessions(id),
  FOREIGN KEY (spec_id) REFERENCES specs(id),
  FOREIGN KEY (task_id) REFERENCES tasks(id)
);

CREATE TABLE IF NOT EXISTS events (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  entity_kind TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  from_status TEXT,
  to_status TEXT,
  note TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS relationships (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  from_entity_kind TEXT NOT NULL,
  from_entity_id TEXT NOT NULL,
  to_entity_kind TEXT NOT NULL,
  to_entity_id TEXT NOT NULL,
  relationship_type TEXT NOT NULL,
  reason TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS tags (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, name)
);

CREATE TABLE IF NOT EXISTS entity_tags (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  tag_id TEXT NOT NULL,
  entity_kind TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (tag_id) REFERENCES tags(id),
  UNIQUE (project_id, tag_id, entity_kind, entity_id)
);

CREATE TABLE IF NOT EXISTS bundles (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  slug TEXT NOT NULL,
  title TEXT NOT NULL,
  tag_query TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, slug)
);

CREATE TABLE IF NOT EXISTS bundle_members (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  bundle_id TEXT NOT NULL,
  entity_kind TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (bundle_id) REFERENCES bundles(id),
  UNIQUE (project_id, bundle_id, entity_kind, entity_id)
);

CREATE TABLE IF NOT EXISTS sources (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  path TEXT NOT NULL,
  hash TEXT,
  line_start INTEGER,
  line_end INTEGER,
  imported_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS backend_mappings (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  backend TEXT NOT NULL,
  entity_kind TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  external_kind TEXT NOT NULL,
  external_id TEXT NOT NULL,
  external_url TEXT,
  sync_status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, backend, external_kind, external_id)
);

CREATE TABLE IF NOT EXISTS hook_events (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  hook_id TEXT NOT NULL,
  hook_phase TEXT NOT NULL,
  command TEXT,
  exit_code INTEGER,
  message TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS exports (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  export_kind TEXT NOT NULL,
  format TEXT NOT NULL,
  path TEXT NOT NULL,
  state_version INTEGER,
  source_entity_kind TEXT,
  source_entity_id TEXT,
  generated_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE INDEX IF NOT EXISTS idx_aliases_entity ON aliases (project_id, entity_kind, entity_id);
CREATE INDEX IF NOT EXISTS idx_tasks_spec ON tasks (project_id, spec_id);
CREATE INDEX IF NOT EXISTS idx_journal_context ON journal_entries (project_id, session_id, spec_id, task_id);
CREATE INDEX IF NOT EXISTS idx_events_entity ON events (project_id, entity_kind, entity_id);
CREATE INDEX IF NOT EXISTS idx_relationships_from ON relationships (project_id, from_entity_kind, from_entity_id);
CREATE INDEX IF NOT EXISTS idx_relationships_to ON relationships (project_id, to_entity_kind, to_entity_id);
CREATE INDEX IF NOT EXISTS idx_entity_tags_entity ON entity_tags (project_id, entity_kind, entity_id);
CREATE INDEX IF NOT EXISTS idx_sources_path ON sources (project_id, path);
CREATE INDEX IF NOT EXISTS idx_backend_mappings_entity ON backend_mappings (project_id, entity_kind, entity_id);
CREATE INDEX IF NOT EXISTS idx_hook_events_hook ON hook_events (project_id, hook_id, hook_phase);
CREATE INDEX IF NOT EXISTS idx_exports_source ON exports (project_id, source_entity_kind, source_entity_id);

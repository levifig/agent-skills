-- Issue schema and identity foundation.
--
-- Issues are the recursive work entity. This migration is additive: the
-- existing tasks and specs tables are left untouched and go inert. There is
-- no data migration and no compatibility shim.
--
-- Status on issues.status is a projection of the append-only events table
-- (entity_kind 'issue'). Writes go through events; a parity check proves the
-- column equals the latest event. Default status at creation is triage.
-- There is no blocked status (that is a relationship) and no review status
-- (a display name for active).
--
-- Title and body are fully mutable at every status. Body is a plain TEXT
-- column; the sources/body_source_id indirection is not reused. Fog holds
-- questions not yet sharp enough to be issues.
--
-- Human-readable IDs for local authority are minted from
-- issue_identity.next_number and stored in aliases (entity_kind and
-- namespace 'issue'). The counter is a stored value, never derived from
-- MAX() over aliases or issues. Minted numbers are permanent: removing or
-- hard-deleting an issue does not free its number. Tracker authorities
-- (linear, github) mint no local alias.
--
-- parent_id is a same-project self-reference. Cycle prevention is a
-- write-time guard in the Go API, not a schema trigger.
--
-- issue_criteria.command / expect use the same grammar loaf change verify
-- parses today (exit N, contains <text>). This migration only stores them.

CREATE TABLE IF NOT EXISTS issues (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  parent_id TEXT,
  kind TEXT NOT NULL CHECK (kind IN ('delivery', 'decision')),
  title TEXT NOT NULL CHECK (length(trim(title)) > 0),
  body TEXT NOT NULL DEFAULT '',
  fog TEXT,
  status TEXT NOT NULL CHECK (status IN ('triage', 'backlog', 'todo', 'active', 'done', 'cancelled', 'duplicate')),
  archived_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, parent_id) REFERENCES issues(project_id, id),
  UNIQUE (project_id, id)
);
CREATE INDEX IF NOT EXISTS idx_issues_project ON issues (project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_issues_parent ON issues (project_id, parent_id);
CREATE INDEX IF NOT EXISTS idx_issues_status ON issues (project_id, status);

CREATE TABLE IF NOT EXISTS issue_criteria (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  issue_id TEXT NOT NULL,
  position INTEGER NOT NULL CHECK (position >= 1),
  text TEXT NOT NULL CHECK (length(trim(text)) > 0),
  command TEXT,
  expect TEXT,
  tier TEXT NOT NULL CHECK (tier IN ('V', 'H')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, issue_id) REFERENCES issues(project_id, id) ON DELETE CASCADE,
  UNIQUE (issue_id, position)
);
CREATE INDEX IF NOT EXISTS idx_issue_criteria_issue ON issue_criteria (project_id, issue_id, position);

-- One authority row per project. next_number is the next local alias to mint
-- and is never recomputed from existing rows.
CREATE TABLE IF NOT EXISTS issue_identity (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  authority TEXT NOT NULL CHECK (authority IN ('local', 'linear', 'github')),
  prefix TEXT NOT NULL CHECK (prefix GLOB '[A-Za-z]*' AND prefix NOT GLOB '*[^A-Za-z0-9]*' AND length(prefix) = length(CAST(prefix AS BLOB))),
  next_number INTEGER NOT NULL CHECK (next_number >= 1),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id)
);

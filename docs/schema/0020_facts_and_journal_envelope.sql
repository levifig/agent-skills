-- Grow-only fact substrate: envelope v1 and journal corpus wrap.
-- journal_entries remains the local projection; facts are the synced source of truth.
-- Wrapped rows preserve journal ids and carry display timestamps in payload only.

CREATE TABLE IF NOT EXISTS facts (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (length(trim(kind)) > 0),
  payload TEXT NOT NULL CHECK (length(trim(payload)) > 0),
  env_id TEXT NOT NULL CHECK (length(trim(env_id)) > 0),
  seq INTEGER NOT NULL CHECK (seq >= 1),
  hlc TEXT NOT NULL CHECK (length(trim(hlc)) > 0),
  envelope_v INTEGER NOT NULL DEFAULT 1 CHECK (envelope_v = 1),
  FOREIGN KEY (project_id) REFERENCES projects(id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_facts_project_env_seq
  ON facts (project_id, env_id, seq);
CREATE INDEX IF NOT EXISTS idx_facts_project_order
  ON facts (project_id, hlc, env_id, id);
CREATE INDEX IF NOT EXISTS idx_facts_project_kind
  ON facts (project_id, kind);

CREATE TABLE IF NOT EXISTS fact_env_clocks (
  project_id TEXT NOT NULL,
  env_id TEXT NOT NULL,
  hlc_wall_ms INTEGER NOT NULL,
  hlc_logical INTEGER NOT NULL DEFAULT 0 CHECK (hlc_logical >= 0),
  next_seq INTEGER NOT NULL DEFAULT 1 CHECK (next_seq >= 1),
  PRIMARY KEY (project_id, env_id),
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

-- Wrap the existing journal corpus in place. env_id backfills as the legacy host
-- identity (single-host substrate today). HLC wall component is seeded from
-- created_at; logical is zero. Physical wall time stays in payload for display.
INSERT INTO facts (id, project_id, kind, payload, env_id, seq, hlc, envelope_v)
SELECT
  j.id,
  j.project_id,
  'journal',
  json_object(
    'entry_type', j.entry_type,
    'scope', COALESCE(j.scope, ''),
    'message', j.message,
    'observed_branch', COALESCE(j.observed_branch, ''),
    'observed_worktree', COALESCE(j.observed_worktree, ''),
    'harness_session_id', COALESCE(j.harness_session_id, ''),
    'created_at', j.created_at,
    'updated_at', j.updated_at
  ),
  'legacy-host',
  ROW_NUMBER() OVER (
    PARTITION BY j.project_id
    ORDER BY j.created_at ASC, j.rowid ASC
  ),
  printf(
    '%020d:%06d',
    COALESCE(
      CAST(unixepoch(substr(j.created_at, 1, 19)) AS INTEGER) * 1000,
      j.rowid
    ),
    0
  ),
  1
FROM journal_entries AS j;

INSERT INTO fact_env_clocks (project_id, env_id, hlc_wall_ms, hlc_logical, next_seq)
SELECT
  project_id,
  env_id,
  CAST(substr(hlc, 1, 20) AS INTEGER),
  CAST(substr(hlc, 22, 6) AS INTEGER),
  MAX(seq) + 1
FROM facts
GROUP BY project_id, env_id;

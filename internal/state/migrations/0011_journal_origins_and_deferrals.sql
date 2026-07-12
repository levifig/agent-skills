-- Journal provenance and deferred-intent envelopes.
--
-- These tables deliberately do not foreign-key journal_entries or sparks:
-- migration 0010 may rebuild journal_entries after this migration has run.
-- Referential integrity for optional provenance is therefore audited by the
-- owning workflows rather than enforced as a hard migration-order dependency.
CREATE TABLE IF NOT EXISTS journal_origins (
  journal_entry_id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  envelope_version INTEGER NOT NULL CHECK (envelope_version >= 1),
  capture_mechanism TEXT NOT NULL CHECK (length(trim(capture_mechanism)) > 0),
  observed_harness TEXT,
  observed_harness_version TEXT,
  harness_session_id TEXT,
  agent_id TEXT,
  source_event TEXT,
  branch TEXT,
  worktree TEXT,
  head TEXT,
  change_path TEXT,
  change_sha256 TEXT CHECK (change_sha256 IS NULL OR (length(change_sha256) = 64 AND change_sha256 NOT GLOB '*[^0-9a-fA-F]*')),
  dirty INTEGER CHECK (dirty IS NULL OR dirty IN (0, 1)),
  reconstructable INTEGER CHECK (reconstructable IS NULL OR reconstructable IN (0, 1)),
  durable_result_kind TEXT,
  durable_result_id TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);
CREATE INDEX IF NOT EXISTS idx_journal_origins_project_mechanism_created
  ON journal_origins (project_id, capture_mechanism, created_at);
CREATE INDEX IF NOT EXISTS idx_journal_origins_project_harness_session
  ON journal_origins (project_id, observed_harness, harness_session_id);
CREATE INDEX IF NOT EXISTS idx_journal_origins_project_change_path
  ON journal_origins (project_id, change_path);
CREATE INDEX IF NOT EXISTS idx_journal_origins_project_result
  ON journal_origins (project_id, durable_result_kind, durable_result_id);

-- Existing journal rows receive only an honest envelope and fields observable
-- in both schema 9 and the post-0010 journal-first schema. Harness, agent,
-- source-event, Change, and durable-result fields remain NULL because they are
-- not recoverable from journal prose.
INSERT INTO journal_origins (
  journal_entry_id, project_id, envelope_version, capture_mechanism,
  observed_harness, observed_harness_version, harness_session_id, agent_id,
  source_event, branch, worktree, head, change_path, change_sha256,
  dirty, reconstructable, durable_result_kind, durable_result_id, created_at
)
SELECT
  j.id, j.project_id, 1, 'unknown',
  NULL, NULL, j.harness_session_id, NULL,
  NULL, j.observed_branch, j.observed_worktree, NULL, NULL, NULL,
  NULL, NULL, NULL, NULL, j.created_at
FROM journal_entries AS j
WHERE NOT EXISTS (
  SELECT 1 FROM journal_origins AS o WHERE o.journal_entry_id = j.id
);

CREATE TABLE IF NOT EXISTS journal_deferrals (
  project_id TEXT NOT NULL,
  operation_key TEXT NOT NULL CHECK (length(trim(operation_key)) BETWEEN 1 AND 200),
  journal_entry_id TEXT NOT NULL UNIQUE,
  spark_id TEXT NOT NULL UNIQUE,
  stored_digest TEXT NOT NULL CHECK (length(stored_digest) = 64 AND stored_digest NOT GLOB '*[^0-9a-fA-F]*'),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, operation_key),
  FOREIGN KEY (project_id) REFERENCES projects(id)
);

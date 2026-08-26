-- Event facts for the mutable core (LOAF-72).
-- Projection tables stay as local latest-event-wins folds. The events table
-- remains readable as local archive; status-change facts are the synced events.
-- Verified replay (event facts, then birth-fact on fold mismatch) runs in Go
-- inside this migration. Discrepancies are machine-local and never sync.

CREATE TABLE IF NOT EXISTS fact_replay_discrepancies (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  entity_kind TEXT NOT NULL CHECK (length(trim(entity_kind)) > 0),
  entity_id TEXT NOT NULL CHECK (length(trim(entity_id)) > 0),
  reason TEXT NOT NULL CHECK (length(trim(reason)) > 0),
  logged_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);
CREATE INDEX IF NOT EXISTS idx_fact_replay_discrepancies_project
  ON fact_replay_discrepancies (project_id, entity_kind, entity_id);

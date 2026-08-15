-- Criterion-grain claims for derived issue readiness.
--
-- 0014 is the issue model. This migration adds the one child table that
-- makes decomposition honesty mechanical: a child criterion claims a parent
-- criterion by opaque id, never by position. Positions renumber; claims
-- must not.
--
-- Coverage (every parent criterion is claimed) and containment (every child
-- criterion claims some parent criterion) are derived at read time from
-- these rows. Promote writes a claim by construction. There is no data
-- backfill: existing issues have no claims until an operator records them.
--
-- Claim FKs are project-scoped: a row cannot satisfy coverage by pointing
-- at a criterion that belongs to another project. The unique index on
-- issue_criteria (project_id, id) is the parent key those FKs require.

CREATE UNIQUE INDEX IF NOT EXISTS idx_issue_criteria_project_id ON issue_criteria (project_id, id);

CREATE TABLE IF NOT EXISTS issue_criterion_claims (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  child_criterion_id TEXT NOT NULL,
  parent_criterion_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, child_criterion_id) REFERENCES issue_criteria(project_id, id) ON DELETE CASCADE,
  FOREIGN KEY (project_id, parent_criterion_id) REFERENCES issue_criteria(project_id, id) ON DELETE CASCADE,
  UNIQUE (child_criterion_id, parent_criterion_id),
  CHECK (child_criterion_id != parent_criterion_id)
);
CREATE INDEX IF NOT EXISTS idx_issue_criterion_claims_parent ON issue_criterion_claims (project_id, parent_criterion_id);
CREATE INDEX IF NOT EXISTS idx_issue_criterion_claims_child ON issue_criterion_claims (project_id, child_criterion_id);

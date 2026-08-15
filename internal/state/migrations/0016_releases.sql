-- Retroactive releases: facts about what landed, never a plan.
--
-- A release is recorded after a tag exists. Members are the attributed
-- issues observed in the baseline..HEAD range, plus optional prerelease
-- references when a stable is cut with --includes. There is no
-- target_release, no cohort, and no forward binding of issues to versions.
--
-- member_kind 'release' is a narrative reference, not a union: cutting a
-- stable does not re-include the prerelease's issue members. member_id is
-- polymorphic (issue or release) and cannot carry a hard FK; kind and
-- existence are validated on the write path. The composite FK on
-- (project_id, release_id) keeps membership inside the same project.

CREATE TABLE IF NOT EXISTS releases (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  version TEXT NOT NULL,
  tag TEXT NOT NULL,
  tagged_commit TEXT NOT NULL,
  notes TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, version),
  UNIQUE (project_id, tag)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_releases_project_id ON releases (project_id, id);

CREATE TABLE IF NOT EXISTS release_members (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  release_id TEXT NOT NULL,
  member_kind TEXT NOT NULL,
  member_id TEXT NOT NULL,
  recorded_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, release_id) REFERENCES releases(project_id, id) ON DELETE CASCADE,
  UNIQUE (release_id, member_kind, member_id),
  CHECK (member_kind IN ('issue', 'release'))
);
CREATE INDEX IF NOT EXISTS idx_release_members_release ON release_members (project_id, release_id);

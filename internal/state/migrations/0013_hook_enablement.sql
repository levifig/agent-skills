-- Hook enablement, absorption markers, and trusted executable paths.
--
-- User-scoped and host-local: no project_id. Absence of an enablement row means
-- the hook is enabled. A disabled row suppresses projection. Records for retired
-- hook IDs are retained as inert tombstones (no deletion path). Absorption
-- provenance (absorbed_at) is immutable once set. The absorption marker is
-- independent of installed-manifest rows and gates run-once migration.

CREATE TABLE IF NOT EXISTS hook_enablements (
  id TEXT PRIMARY KEY NOT NULL,
  target TEXT NOT NULL CHECK (length(trim(target)) > 0),
  event TEXT NOT NULL CHECK (length(trim(event)) > 0),
  hook_id TEXT NOT NULL CHECK (length(trim(hook_id)) > 0),
  enablement TEXT NOT NULL CHECK (enablement IN ('enabled', 'disabled')),
  absorbed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (target, event, hook_id)
);
CREATE INDEX IF NOT EXISTS idx_hook_enablements_target
  ON hook_enablements (target);

-- Per-target run-once absorption marker. Written in the same transaction as
-- the absorbed disabled records; never inferred from manifest rows.
CREATE TABLE IF NOT EXISTS hook_absorption_markers (
  id TEXT PRIMARY KEY NOT NULL,
  target TEXT NOT NULL CHECK (length(trim(target)) > 0),
  absorbed_from_version TEXT NOT NULL CHECK (length(trim(absorbed_from_version)) > 0),
  absorbed_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (target)
);

-- Per-target trusted Loaf executable install paths. is_current marks the
-- currently resolved install path; prior paths remain as recognition history.
CREATE TABLE IF NOT EXISTS hook_trusted_paths (
  id TEXT PRIMARY KEY NOT NULL,
  target TEXT NOT NULL CHECK (length(trim(target)) > 0),
  path TEXT NOT NULL CHECK (length(trim(path)) > 0),
  is_current INTEGER NOT NULL CHECK (is_current IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (target, path)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_hook_trusted_paths_current
  ON hook_trusted_paths (target)
  WHERE is_current = 1;
CREATE INDEX IF NOT EXISTS idx_hook_trusted_paths_target
  ON hook_trusted_paths (target);

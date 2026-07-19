-- Intent and Exploration relational foundation.
--
-- Every table in this migration is additive and append-only operational state:
-- stable identities, immutable content snapshots, transactionally sequenced
-- facts, and normalized conversation provenance. There is no mutable lifecycle
-- status, no current-session pointer, and no raw transcript storage.
--
-- Like migration 0011, tables that reference journal_entries or sparks
-- deliberately do not foreign-key them: the explicit journal-first migration
-- 0010 may rebuild journal_entries after this migration has run on a schema-9
-- database. Referential integrity for those optional projections is audited by
-- the owning workflows. Foreign keys to projects and to the new aggregate
-- roots are hard because those tables are never rebuilt.

CREATE TABLE IF NOT EXISTS intents (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, id)
);
CREATE INDEX IF NOT EXISTS idx_intents_project ON intents (project_id, created_at);

-- Immutable content revisions. The latest snapshot is derived from the
-- greatest committed per-intent sequence, never from timestamps.
CREATE TABLE IF NOT EXISTS intent_snapshots (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  intent_id TEXT NOT NULL,
  seq INTEGER NOT NULL CHECK (seq >= 1),
  title TEXT NOT NULL CHECK (length(trim(title)) > 0),
  body TEXT NOT NULL CHECK (length(trim(body)) > 0),
  content_digest TEXT NOT NULL CHECK (length(content_digest) = 64 AND content_digest NOT GLOB '*[^0-9a-fA-F]*'),
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, intent_id) REFERENCES intents(project_id, id),
  UNIQUE (intent_id, seq)
);

-- Immutable self-sufficient deferral payloads. The retained body plus why,
-- boundary, and revisit trigger form the portable contract; per-field byte
-- caps are enforced by the write path before insert.
CREATE TABLE IF NOT EXISTS intent_deferrals (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  intent_id TEXT NOT NULL,
  operation_key TEXT NOT NULL CHECK (length(trim(operation_key)) BETWEEN 1 AND 200),
  body TEXT NOT NULL CHECK (length(trim(body)) > 0),
  why TEXT NOT NULL CHECK (length(trim(why)) > 0),
  boundary TEXT NOT NULL CHECK (length(trim(boundary)) > 0),
  revisit_trigger TEXT NOT NULL CHECK (length(trim(revisit_trigger)) > 0),
  stored_digest TEXT NOT NULL CHECK (length(stored_digest) = 64 AND stored_digest NOT GLOB '*[^0-9a-fA-F]*'),
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, intent_id) REFERENCES intents(project_id, id),
  UNIQUE (project_id, operation_key),
  UNIQUE (project_id, id),
  UNIQUE (intent_id, id)
);
CREATE INDEX IF NOT EXISTS idx_intent_deferrals_intent ON intent_deferrals (intent_id, created_at);

-- Append-only disposition facts. Current disposition is the row with the
-- greatest committed per-intent sequence; concurrent appends are serialized by
-- the (intent_id, seq) uniqueness constraint rather than wall-clock time.
CREATE TABLE IF NOT EXISTS intent_dispositions (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  intent_id TEXT NOT NULL,
  seq INTEGER NOT NULL CHECK (seq >= 1),
  disposition TEXT NOT NULL CHECK (disposition IN ('tracked', 'deferred', 'resolved')),
  reason TEXT,
  deferral_id TEXT,
  supersedes_deferral_id TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, intent_id) REFERENCES intents(project_id, id),
  FOREIGN KEY (project_id, deferral_id) REFERENCES intent_deferrals(project_id, id),
  FOREIGN KEY (project_id, supersedes_deferral_id) REFERENCES intent_deferrals(project_id, id),
  FOREIGN KEY (intent_id, deferral_id) REFERENCES intent_deferrals(intent_id, id),
  FOREIGN KEY (intent_id, supersedes_deferral_id) REFERENCES intent_deferrals(intent_id, id),
  UNIQUE (intent_id, seq),
  CHECK ((disposition = 'deferred') = (deferral_id IS NOT NULL)),
  CHECK (supersedes_deferral_id IS NULL OR disposition = 'tracked')
);

-- One canonical operation mapping shared by intent create, intent defer, the
-- transitional journal defer adapter, and legacy conversion. Projection
-- version 0 records a canonical-first write with no legacy journal/spark
-- projection; version 1 requires both historical projection references.
CREATE TABLE IF NOT EXISTS intent_operations (
  project_id TEXT NOT NULL,
  operation_key TEXT NOT NULL CHECK (length(trim(operation_key)) BETWEEN 1 AND 200),
  intent_id TEXT NOT NULL,
  stored_digest TEXT NOT NULL CHECK (length(stored_digest) = 64 AND stored_digest NOT GLOB '*[^0-9a-fA-F]*'),
  journal_entry_id TEXT,
  spark_id TEXT,
  projection_version INTEGER NOT NULL CHECK (projection_version IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (project_id, operation_key),
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, intent_id) REFERENCES intents(project_id, id),
  CHECK ((projection_version = 0 AND journal_entry_id IS NULL AND spark_id IS NULL) OR (projection_version = 1 AND journal_entry_id IS NOT NULL AND spark_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_intent_operations_intent ON intent_operations (intent_id);

CREATE TABLE IF NOT EXISTS explorations (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  title TEXT NOT NULL CHECK (length(trim(title)) > 0),
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, id)
);
CREATE INDEX IF NOT EXISTS idx_explorations_project ON explorations (project_id, created_at);

-- Immutable portable checkpoints. A checkpoint is portable only when all four
-- required core fields are present; per-field 4096 UTF-8 byte caps are
-- enforced by the write path, which rejects overflow without truncation.
CREATE TABLE IF NOT EXISTS exploration_checkpoints (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  exploration_id TEXT NOT NULL,
  seq INTEGER NOT NULL CHECK (seq >= 1),
  purpose TEXT NOT NULL CHECK (length(trim(purpose)) > 0),
  conclusions TEXT NOT NULL CHECK (length(trim(conclusions)) > 0),
  unresolved TEXT NOT NULL CHECK (length(trim(unresolved)) > 0),
  next_action TEXT NOT NULL CHECK (length(trim(next_action)) > 0),
  operation_key TEXT CHECK (operation_key IS NULL OR length(trim(operation_key)) BETWEEN 1 AND 200),
  content_digest TEXT NOT NULL CHECK (length(content_digest) = 64 AND content_digest NOT GLOB '*[^0-9a-fA-F]*'),
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, exploration_id) REFERENCES explorations(project_id, id),
  UNIQUE (exploration_id, seq),
  UNIQUE (project_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_exploration_checkpoints_operation
  ON exploration_checkpoints (project_id, operation_key)
  WHERE operation_key IS NOT NULL;

-- Ordered typed checkpoint items carry bounded optional detail such as
-- candidates and evidence; item types are validated by the central registry.
CREATE TABLE IF NOT EXISTS exploration_checkpoint_items (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  checkpoint_id TEXT NOT NULL,
  item_type TEXT NOT NULL CHECK (length(trim(item_type)) > 0),
  position INTEGER NOT NULL CHECK (position >= 1),
  content TEXT NOT NULL CHECK (length(trim(content)) > 0),
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, checkpoint_id) REFERENCES exploration_checkpoints(project_id, id),
  UNIQUE (checkpoint_id, position)
);

-- A logical conversation groups machine-local handles. It carries no session
-- lifecycle and is never inferred from branch, worktree, or recency.
CREATE TABLE IF NOT EXISTS logical_conversations (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  title TEXT NOT NULL CHECK (length(trim(title)) > 0),
  operation_key TEXT CHECK (operation_key IS NULL OR length(trim(operation_key)) BETWEEN 1 AND 200),
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_logical_conversations_operation
  ON logical_conversations (project_id, operation_key)
  WHERE operation_key IS NOT NULL;

-- Machine/harness-local conversation handles: opaque IDs plus locality. The
-- presence of a handle never implies portable context or reachability.
CREATE TABLE IF NOT EXISTS conversation_handles (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  harness TEXT NOT NULL CHECK (length(trim(harness)) > 0),
  handle TEXT NOT NULL CHECK (length(trim(handle)) > 0),
  locality TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, conversation_id) REFERENCES logical_conversations(project_id, id),
  UNIQUE (project_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_handles_identity
  ON conversation_handles (conversation_id, harness, handle, COALESCE(locality, ''));

-- Bounded log references: locators, hashes, and ranges only, never transcript
-- bodies. Availability lives in observations, not on the reference row.
CREATE TABLE IF NOT EXISTS conversation_log_refs (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  handle_id TEXT NOT NULL,
  locator TEXT NOT NULL CHECK (length(trim(locator)) > 0),
  content_hash TEXT CHECK (content_hash IS NULL OR (length(content_hash) = 64 AND content_hash NOT GLOB '*[^0-9a-fA-F]*')),
  range_spec TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, handle_id) REFERENCES conversation_handles(project_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_log_refs_identity
  ON conversation_log_refs (handle_id, locator, COALESCE(range_spec, ''));

-- Exploration <-> logical conversation membership.
CREATE TABLE IF NOT EXISTS exploration_conversations (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  exploration_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, exploration_id) REFERENCES explorations(project_id, id),
  FOREIGN KEY (project_id, conversation_id) REFERENCES logical_conversations(project_id, id),
  UNIQUE (exploration_id, conversation_id)
);
CREATE INDEX IF NOT EXISTS idx_exploration_conversations_conversation
  ON exploration_conversations (conversation_id);

-- Journal <-> conversation-handle association. journal_entry_id is not a
-- foreign key because migration 0010 may rebuild journal_entries later.
CREATE TABLE IF NOT EXISTS journal_conversation_handles (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  journal_entry_id TEXT NOT NULL,
  handle_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (project_id, handle_id) REFERENCES conversation_handles(project_id, id),
  UNIQUE (journal_entry_id, handle_id)
);
CREATE INDEX IF NOT EXISTS idx_journal_conversation_handles_handle
  ON journal_conversation_handles (handle_id);

-- Immutable availability observations for handles and log references.
-- Reachability is observed at a moment from a locality; it is never a mutable
-- property of the observed row.
CREATE TABLE IF NOT EXISTS source_availability_observations (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  subject_kind TEXT NOT NULL CHECK (subject_kind IN ('conversation_handle', 'conversation_log_ref')),
  subject_id TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  observer TEXT,
  locality TEXT,
  available INTEGER NOT NULL CHECK (available IN (0, 1)),
  note TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id)
);
CREATE INDEX IF NOT EXISTS idx_source_availability_subject
  ON source_availability_observations (subject_kind, subject_id, observed_at);

package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	applicationID = 1280267825
	schemaLine    = "vnext"
	schemaVersion = 6
)

const schemaTableV1SQL = `CREATE TABLE continuity_schema (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  schema_line TEXT NOT NULL CHECK (schema_line = 'vnext'),
  schema_version INTEGER NOT NULL CHECK (schema_version = 1),
  schema_checksum TEXT NOT NULL CHECK (
    length(schema_checksum) = 64
    AND schema_checksum NOT GLOB '*[^0-9a-f]*'
  )
) STRICT`

const schemaTableV2SQL = `CREATE TABLE continuity_schema (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  schema_line TEXT NOT NULL CHECK (schema_line = 'vnext'),
  schema_version INTEGER NOT NULL CHECK (schema_version = 2),
  schema_checksum TEXT NOT NULL CHECK (
    length(schema_checksum) = 64
    AND schema_checksum NOT GLOB '*[^0-9a-f]*'
  )
) STRICT`

const schemaTableV3SQL = `CREATE TABLE continuity_schema (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  schema_line TEXT NOT NULL CHECK (schema_line = 'vnext'),
  schema_version INTEGER NOT NULL CHECK (schema_version = 3),
  schema_checksum TEXT NOT NULL CHECK (
    length(schema_checksum) = 64
    AND schema_checksum NOT GLOB '*[^0-9a-f]*'
  )
) STRICT`

const schemaTableV4SQL = `CREATE TABLE continuity_schema (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  schema_line TEXT NOT NULL CHECK (schema_line = 'vnext'),
  schema_version INTEGER NOT NULL CHECK (schema_version = 4),
  schema_checksum TEXT NOT NULL CHECK (
    length(schema_checksum) = 64
    AND schema_checksum NOT GLOB '*[^0-9a-f]*'
  )
) STRICT`

const schemaTableV5SQL = `CREATE TABLE continuity_schema (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  schema_line TEXT NOT NULL CHECK (schema_line = 'vnext'),
  schema_version INTEGER NOT NULL CHECK (schema_version = 5),
  schema_checksum TEXT NOT NULL CHECK (
    length(schema_checksum) = 64
    AND schema_checksum NOT GLOB '*[^0-9a-f]*'
  )
) STRICT`

const schemaTableSQL = `CREATE TABLE continuity_schema (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  schema_line TEXT NOT NULL CHECK (schema_line = 'vnext'),
  schema_version INTEGER NOT NULL CHECK (schema_version = 6),
  schema_checksum TEXT NOT NULL CHECK (
    length(schema_checksum) = 64
    AND schema_checksum NOT GLOB '*[^0-9a-f]*'
  )
) STRICT`

const factsTableSQL = `CREATE TABLE continuity_facts (
  fact_id TEXT NOT NULL PRIMARY KEY CHECK (
    length(fact_id) BETWEEN 1 AND 128
    AND fact_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  subject_kind TEXT NOT NULL CHECK (subject_kind IN (
    'project-identity',
    'journal-entry',
    'wrap',
    'spark',
    'idea',
    'decision',
    'exploration',
    'checkpoint',
    'finding',
    'handoff',
    'scratchpad',
    'external-reference',
    'verification-evidence'
  )),
  subject_id TEXT NOT NULL CHECK (
    length(subject_id) BETWEEN 1 AND 128
    AND subject_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  fact_kind TEXT NOT NULL CHECK (
    (subject_kind = 'project-identity' AND fact_kind IN (
      'project.registered', 'project.label-revised'
    ))
    OR (subject_kind = 'journal-entry' AND fact_kind IN (
      'journal.recorded', 'journal.correction-recorded'
    ))
    OR (subject_kind = 'wrap' AND fact_kind = 'wrap.recorded')
    OR (subject_kind = 'spark' AND fact_kind IN (
      'spark.captured', 'spark.dismissed', 'spark.promoted-to-idea'
    ))
    OR (subject_kind = 'idea' AND fact_kind IN (
      'idea.created', 'idea.revised', 'idea.resolved', 'idea.archived',
      'idea.promoted-to-external-reference'
    ))
    OR (subject_kind = 'decision' AND fact_kind IN (
      'decision.opened', 'decision.resolved', 'decision.superseded'
    ))
    OR (subject_kind = 'exploration' AND fact_kind = 'exploration.started')
    OR (subject_kind = 'checkpoint' AND fact_kind = 'checkpoint.recorded')
    OR (subject_kind = 'finding' AND fact_kind IN (
      'finding.recorded', 'finding.corrected', 'finding.retracted'
    ))
    OR (subject_kind = 'handoff' AND fact_kind = 'handoff.recorded')
    OR (subject_kind = 'scratchpad' AND fact_kind IN (
      'scratchpad.opened', 'scratchpad.participant-introduced',
      'scratchpad.message-recorded', 'scratchpad.claim-recorded',
      'scratchpad.claim-released', 'scratchpad.closed'
    ))
    OR (subject_kind = 'external-reference' AND fact_kind IN (
      'external-reference.registered', 'external-reference.attached',
      'external-reference.detached'
    ))
    OR (subject_kind = 'verification-evidence'
      AND fact_kind = 'verification-evidence.recorded')
  ),
  payload_version INTEGER NOT NULL CHECK (payload_version = 1),
  content_json TEXT NOT NULL CHECK (
    json_valid(content_json)
    AND json_type(content_json) = 'object'
    AND length(CAST(content_json AS BLOB)) BETWEEN 2 AND 1048576
  ),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_sequence INTEGER NOT NULL CHECK (environment_sequence > 0),
  hlc_wall_millis INTEGER NOT NULL CHECK (hlc_wall_millis >= 0),
  hlc_logical INTEGER NOT NULL CHECK (hlc_logical BETWEEN 0 AND 2147483647),
  envelope_version INTEGER NOT NULL CHECK (envelope_version = 1),
  UNIQUE (project_id, environment_id, environment_sequence),
  CHECK (subject_kind <> 'project-identity' OR subject_id = project_id)
) STRICT, WITHOUT ROWID`

const projectIdentityIndexSQL = `CREATE UNIQUE INDEX ux_continuity_project_identity
ON continuity_facts(project_id)
WHERE fact_kind = 'project.registered'`

const projectOrderIndexSQL = `CREATE INDEX ix_continuity_facts_project_order
ON continuity_facts(
  project_id,
  hlc_wall_millis,
  hlc_logical,
  environment_id,
  fact_id
)`

const subjectOrderIndexSQL = `CREATE INDEX ix_continuity_facts_subject_order
ON continuity_facts(
  project_id,
  subject_kind,
  subject_id,
  hlc_wall_millis,
  hlc_logical,
  environment_id,
  fact_id
)`

const projectIdentityTriggerSQL = `CREATE TRIGGER continuity_facts_require_project_identity
BEFORE INSERT ON continuity_facts
WHEN NEW.fact_kind <> 'project.registered'
 AND NOT EXISTS (
   SELECT 1
   FROM continuity_facts
   WHERE project_id = NEW.project_id
     AND fact_kind = 'project.registered'
 )
BEGIN
  SELECT RAISE(ABORT, 'project identity fact required');
END`

const syncProjectsTableSQL = `CREATE TABLE continuity_sync_projects (
  project_id TEXT NOT NULL PRIMARY KEY CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  channel_id BLOB NOT NULL CHECK (
    length(channel_id) = 32 AND channel_id <> zeroblob(32)
  ),
  relay_generation BLOB NOT NULL CHECK (
    length(relay_generation) = 32 AND relay_generation <> zeroblob(32)
  ),
  admin_public_key BLOB NOT NULL CHECK (
    length(admin_public_key) = 32 AND admin_public_key <> zeroblob(32)
  ),
  membership_generation INTEGER NOT NULL CHECK (
    membership_generation BETWEEN 1 AND 4294967295
  ),
  activation_state TEXT NOT NULL CHECK (
    activation_state IN ('staging', 'attached')
  ),
  downloaded_cursor INTEGER NOT NULL CHECK (downloaded_cursor >= 0),
  applied_cursor INTEGER NOT NULL CHECK (applied_cursor >= 0),
  relay_head INTEGER NOT NULL CHECK (relay_head >= 0),
  CHECK (applied_cursor <= downloaded_cursor),
  CHECK (downloaded_cursor <= relay_head)
) STRICT, WITHOUT ROWID`

const syncRelayWatermarksTableSQL = `CREATE TABLE continuity_sync_relay_watermarks (
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  channel_id BLOB NOT NULL CHECK (
    length(channel_id) = 32 AND channel_id <> zeroblob(32)
  ),
  relay_generation BLOB NOT NULL CHECK (
    length(relay_generation) = 32 AND relay_generation <> zeroblob(32)
  ),
  admin_public_key BLOB NOT NULL CHECK (
    length(admin_public_key) = 32 AND admin_public_key <> zeroblob(32)
  ),
  relay_head INTEGER NOT NULL CHECK (relay_head >= 0),
  PRIMARY KEY (project_id, channel_id, relay_generation)
) STRICT, WITHOUT ROWID`

const syncInboxTableV2SQL = `CREATE TABLE continuity_sync_inbox (
  project_id TEXT NOT NULL,
  arrival_sequence INTEGER NOT NULL CHECK (arrival_sequence > 0),
  envelope_digest BLOB NOT NULL CHECK (
    length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32)
  ),
  sealed_envelope BLOB NOT NULL CHECK (
    length(sealed_envelope) BETWEEN 1 AND 1102000
  ),
  state TEXT NOT NULL CHECK (state IN ('staged', 'quarantined')),
  PRIMARY KEY (project_id, arrival_sequence),
  UNIQUE (project_id, envelope_digest),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
) STRICT, WITHOUT ROWID`

const syncInboxTableSQL = `CREATE TABLE continuity_sync_inbox (
  project_id TEXT NOT NULL,
  arrival_sequence INTEGER NOT NULL CHECK (arrival_sequence > 0),
  envelope_digest BLOB NOT NULL CHECK (
    length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32)
  ),
  frame_kind TEXT NOT NULL CHECK (frame_kind IN ('sealed', 'pruned')),
  frame_bytes BLOB NOT NULL CHECK (
    (frame_kind = 'sealed' AND length(frame_bytes) BETWEEN 1 AND 1102000)
    OR (frame_kind = 'pruned' AND length(frame_bytes) BETWEEN 1 AND 1024)
  ),
  state TEXT NOT NULL CHECK (state IN ('staged', 'quarantined')),
  PRIMARY KEY (project_id, arrival_sequence),
  UNIQUE (project_id, envelope_digest),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
) STRICT, WITHOUT ROWID`

const syncReceiptsTableSQL = `CREATE TABLE continuity_sync_receipts (
  project_id TEXT NOT NULL,
  arrival_sequence INTEGER NOT NULL CHECK (arrival_sequence > 0),
  fact_id TEXT NOT NULL CHECK (
    length(fact_id) BETWEEN 1 AND 128
    AND fact_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_sequence INTEGER NOT NULL CHECK (environment_sequence > 0),
  previous_envelope_digest BLOB NOT NULL CHECK (
    length(previous_envelope_digest) = 32
  ),
  envelope_digest BLOB NOT NULL CHECK (
    length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32)
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32 AND certificate_id <> zeroblob(32)
  ),
  key_generation INTEGER NOT NULL CHECK (
    key_generation BETWEEN 1 AND 4294967295
  ),
  nonce BLOB NOT NULL CHECK (length(nonce) = 24),
  PRIMARY KEY (project_id, arrival_sequence),
  UNIQUE (project_id, fact_id),
  UNIQUE (project_id, environment_id, environment_sequence),
  UNIQUE (project_id, key_generation, nonce),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
) STRICT, WITHOUT ROWID`

const syncEnvironmentHeadsTableSQL = `CREATE TABLE continuity_sync_environment_heads (
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  highest_sequence INTEGER NOT NULL CHECK (highest_sequence > 0),
  hlc_wall_millis INTEGER NOT NULL CHECK (hlc_wall_millis >= 0),
  hlc_logical INTEGER NOT NULL CHECK (hlc_logical BETWEEN 0 AND 2147483647),
  sealed_sequence INTEGER NOT NULL CHECK (
    sealed_sequence BETWEEN 0 AND highest_sequence
  ),
  previous_envelope_digest BLOB CHECK (
    previous_envelope_digest IS NULL
    OR length(previous_envelope_digest) = 32
  ),
  envelope_digest BLOB CHECK (
    envelope_digest IS NULL
    OR (length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32))
  ),
  certificate_id BLOB CHECK (
    certificate_id IS NULL
    OR (length(certificate_id) = 32 AND certificate_id <> zeroblob(32))
  ),
  key_generation INTEGER CHECK (
    key_generation IS NULL OR key_generation BETWEEN 1 AND 4294967295
  ),
  nonce BLOB CHECK (nonce IS NULL OR length(nonce) = 24),
  CHECK (
    (sealed_sequence = 0
      AND previous_envelope_digest IS NULL
      AND envelope_digest IS NULL
      AND certificate_id IS NULL
      AND key_generation IS NULL
      AND nonce IS NULL)
    OR
    (sealed_sequence > 0
      AND previous_envelope_digest IS NOT NULL
      AND envelope_digest IS NOT NULL
      AND certificate_id IS NOT NULL
      AND key_generation IS NOT NULL
      AND nonce IS NOT NULL)
  ),
  PRIMARY KEY (project_id, environment_id)
) STRICT, WITHOUT ROWID`

const syncOutboxTableSQL = `CREATE TABLE continuity_sync_outbox (
  fact_id TEXT NOT NULL PRIMARY KEY,
  project_id TEXT NOT NULL,
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_sequence INTEGER NOT NULL CHECK (environment_sequence > 0),
  previous_envelope_digest BLOB NOT NULL CHECK (
    length(previous_envelope_digest) = 32
  ),
  envelope_digest BLOB NOT NULL UNIQUE CHECK (
    length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32)
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32 AND certificate_id <> zeroblob(32)
  ),
  key_generation INTEGER NOT NULL CHECK (
    key_generation BETWEEN 1 AND 4294967295
  ),
  nonce BLOB NOT NULL CHECK (length(nonce) = 24),
  sealed_envelope BLOB NOT NULL CHECK (
    length(sealed_envelope) BETWEEN 1 AND 1102000
  ),
  UNIQUE (project_id, environment_id, environment_sequence),
  UNIQUE (project_id, key_generation, nonce),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id),
  FOREIGN KEY (fact_id) REFERENCES continuity_facts(fact_id)
) STRICT, WITHOUT ROWID`

const syncTombstonesTableSQL = `CREATE TABLE continuity_sync_tombstones (
  fact_id TEXT NOT NULL PRIMARY KEY CHECK (
    length(fact_id) BETWEEN 1 AND 128
    AND fact_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  project_id TEXT NOT NULL,
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_sequence INTEGER NOT NULL CHECK (environment_sequence > 0),
  arrival_sequence INTEGER NOT NULL CHECK (arrival_sequence > 0),
  previous_envelope_digest BLOB NOT NULL CHECK (
    length(previous_envelope_digest) = 32
  ),
  envelope_digest BLOB NOT NULL CHECK (
    length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32)
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32 AND certificate_id <> zeroblob(32)
  ),
  key_generation INTEGER NOT NULL CHECK (
    key_generation BETWEEN 1 AND 4294967295
  ),
  nonce BLOB NOT NULL CHECK (length(nonce) = 24),
  prune_certificate_id BLOB NOT NULL CHECK (
    length(prune_certificate_id) = 32
    AND prune_certificate_id <> zeroblob(32)
  ),
  UNIQUE (project_id, environment_id, environment_sequence),
  UNIQUE (project_id, arrival_sequence),
  UNIQUE (project_id, key_generation, nonce),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
) STRICT, WITHOUT ROWID`

const syncEnvironmentCertificatesTableSQL = `CREATE TABLE continuity_sync_environment_certificates (
  project_id TEXT NOT NULL,
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32 AND certificate_id <> zeroblob(32)
  ),
  certificate_bytes BLOB NOT NULL CHECK (
    length(certificate_bytes) BETWEEN 1 AND 8192
  ),
  mode TEXT NOT NULL CHECK (mode IN ('trusted', 'ephemeral')),
  expires_at_millis INTEGER NOT NULL CHECK (expires_at_millis >= 0),
  join_membership_generation INTEGER NOT NULL CHECK (
    join_membership_generation BETWEEN 1 AND 4294967295
  ),
  retirement_relay_generation BLOB CHECK (
    retirement_relay_generation IS NULL
    OR (length(retirement_relay_generation) = 32 AND retirement_relay_generation <> zeroblob(32))
  ),
  retirement_membership_generation INTEGER CHECK (
    retirement_membership_generation IS NULL
    OR retirement_membership_generation BETWEEN 1 AND 4294967295
  ),
  retirement_final_environment_sequence INTEGER CHECK (
    retirement_final_environment_sequence IS NULL
    OR retirement_final_environment_sequence >= 0
  ),
  retirement_final_envelope_digest BLOB CHECK (
    retirement_final_envelope_digest IS NULL
    OR length(retirement_final_envelope_digest) = 32
  ),
  retirement_id BLOB CHECK (
    retirement_id IS NULL
    OR (length(retirement_id) = 32 AND retirement_id <> zeroblob(32))
  ),
  retirement_bytes BLOB CHECK (
    retirement_bytes IS NULL
    OR length(retirement_bytes) BETWEEN 1 AND 4096
  ),
  CHECK (
    (mode = 'trusted' AND expires_at_millis = 0)
    OR (mode = 'ephemeral' AND expires_at_millis > 0)
  ),
  CHECK (
    (retirement_membership_generation IS NULL
      AND retirement_relay_generation IS NULL
      AND retirement_final_environment_sequence IS NULL
      AND retirement_final_envelope_digest IS NULL
      AND retirement_id IS NULL
      AND retirement_bytes IS NULL)
    OR
    (retirement_membership_generation IS NOT NULL
      AND retirement_relay_generation IS NOT NULL
      AND retirement_final_environment_sequence IS NOT NULL
      AND retirement_final_envelope_digest IS NOT NULL
      AND retirement_id IS NOT NULL
      AND retirement_bytes IS NOT NULL)
  ),
  CHECK (
    retirement_membership_generation IS NULL
    OR retirement_membership_generation >= join_membership_generation
  ),
  CHECK (
    retirement_final_environment_sequence IS NULL
    OR (retirement_final_environment_sequence = 0
      AND retirement_final_envelope_digest = zeroblob(32))
    OR (retirement_final_environment_sequence > 0
      AND retirement_final_envelope_digest <> zeroblob(32))
  ),
  PRIMARY KEY (project_id, environment_id),
  UNIQUE (project_id, certificate_id),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
) STRICT, WITHOUT ROWID`

const syncTerminalCandidatesTableSQL = `CREATE TABLE continuity_sync_terminal_candidates (
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  candidate_id BLOB NOT NULL CHECK (
    length(candidate_id) = 32 AND candidate_id <> zeroblob(32)
  ),
  state TEXT NOT NULL CHECK (state IN ('staging', 'promoted')),
  channel_id BLOB NOT NULL CHECK (
    length(channel_id) = 32 AND channel_id <> zeroblob(32)
  ),
  relay_generation BLOB NOT NULL CHECK (
    length(relay_generation) = 32 AND relay_generation <> zeroblob(32)
  ),
  membership_generation INTEGER NOT NULL CHECK (
    membership_generation BETWEEN 1 AND 4294967295
  ),
  authority_digest BLOB NOT NULL CHECK (
    length(authority_digest) = 32 AND authority_digest <> zeroblob(32)
  ),
  start_arrival_sequence INTEGER NOT NULL CHECK (start_arrival_sequence > 0),
  through_arrival_sequence INTEGER NOT NULL CHECK (
    through_arrival_sequence >= start_arrival_sequence
  ),
  frame_count INTEGER NOT NULL CHECK (frame_count > 0),
  rolling_candidate_digest BLOB NOT NULL CHECK (
    length(rolling_candidate_digest) = 32
    AND rolling_candidate_digest <> zeroblob(32)
  ),
  post_promotion_corpus_digest BLOB CHECK (
    post_promotion_corpus_digest IS NULL
    OR (
      length(post_promotion_corpus_digest) = 32
      AND post_promotion_corpus_digest <> zeroblob(32)
    )
  ),
  resulting_applied_cursor INTEGER CHECK (
    resulting_applied_cursor IS NULL OR resulting_applied_cursor >= 0
  ),
  CHECK (
    frame_count = through_arrival_sequence - start_arrival_sequence + 1
  ),
  CHECK (
    (state = 'staging'
      AND post_promotion_corpus_digest IS NULL
      AND resulting_applied_cursor IS NULL)
    OR
    (state = 'promoted'
      AND post_promotion_corpus_digest IS NOT NULL
      AND resulting_applied_cursor IS NOT NULL
      AND resulting_applied_cursor = through_arrival_sequence)
  ),
  PRIMARY KEY (project_id, candidate_id),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
) STRICT, WITHOUT ROWID`

const syncTerminalCandidateFramesTableSQL = `CREATE TABLE continuity_sync_terminal_candidate_frames (
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  candidate_id BLOB NOT NULL CHECK (
    length(candidate_id) = 32 AND candidate_id <> zeroblob(32)
  ),
  arrival_sequence INTEGER NOT NULL CHECK (arrival_sequence > 0),
  frame_kind TEXT NOT NULL CHECK (frame_kind IN ('sealed', 'pruned')),
  fact_id TEXT NOT NULL CHECK (
    length(fact_id) BETWEEN 1 AND 128
    AND fact_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_sequence INTEGER NOT NULL CHECK (environment_sequence > 0),
  hlc_wall_millis INTEGER NOT NULL CHECK (hlc_wall_millis >= 0),
  hlc_logical INTEGER NOT NULL CHECK (
    hlc_logical BETWEEN 0 AND 2147483647
  ),
  previous_envelope_digest BLOB NOT NULL CHECK (
    length(previous_envelope_digest) = 32
  ),
  envelope_digest BLOB NOT NULL CHECK (
    length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32)
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32 AND certificate_id <> zeroblob(32)
  ),
  key_generation INTEGER NOT NULL CHECK (
    key_generation BETWEEN 1 AND 4294967295
  ),
  nonce BLOB NOT NULL CHECK (length(nonce) = 24),
  prune_certificate_id BLOB CHECK (
    prune_certificate_id IS NULL
    OR (
      length(prune_certificate_id) = 32
      AND prune_certificate_id <> zeroblob(32)
    )
  ),
  candidate_bytes BLOB NOT NULL,
  CHECK (
    (environment_sequence = 1
      AND previous_envelope_digest = zeroblob(32))
    OR
    (environment_sequence > 1
      AND previous_envelope_digest <> zeroblob(32))
  ),
  CHECK (
    (frame_kind = 'sealed'
      AND length(candidate_bytes) BETWEEN 2 AND 1052672
      AND prune_certificate_id IS NULL)
    OR
    (frame_kind = 'pruned'
      AND length(candidate_bytes) BETWEEN 1 AND 256
      AND prune_certificate_id IS NOT NULL)
  ),
  PRIMARY KEY (project_id, arrival_sequence),
  UNIQUE (project_id, candidate_id, fact_id),
  UNIQUE (
    project_id,
    candidate_id,
    environment_id,
    environment_sequence
  ),
  UNIQUE (project_id, candidate_id, key_generation, nonce),
  FOREIGN KEY (project_id, candidate_id)
    REFERENCES continuity_sync_terminal_candidates(project_id, candidate_id)
    ON DELETE CASCADE,
  FOREIGN KEY (project_id, arrival_sequence)
    REFERENCES continuity_sync_inbox(project_id, arrival_sequence)
    ON DELETE RESTRICT
) STRICT, WITHOUT ROWID`

const syncTerminalCandidatesStagingIndexSQL = `CREATE UNIQUE INDEX ux_continuity_sync_terminal_candidates_staging_project
ON continuity_sync_terminal_candidates(project_id)
WHERE state = 'staging'`

const syncAuthoritiesTableSQL = `CREATE TABLE continuity_sync_authorities (
  project_id TEXT NOT NULL PRIMARY KEY CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  digest_version INTEGER NOT NULL CHECK (digest_version IN (1, 2)),
  authority_digest BLOB NOT NULL CHECK (
    length(authority_digest) = 32 AND authority_digest <> zeroblob(32)
  ),
  inventory_arrival_head INTEGER NOT NULL CHECK (
    inventory_arrival_head >= 0
  ),
  CHECK (digest_version <> 1 OR inventory_arrival_head = 0),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
    ON DELETE CASCADE
) STRICT, WITHOUT ROWID`

const syncAuthorityCandidatesTableV5SQL = `CREATE TABLE continuity_sync_authority_candidates (
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  candidate_id BLOB NOT NULL CHECK (
    length(candidate_id) = 32 AND candidate_id <> zeroblob(32)
  ),
  state TEXT NOT NULL CHECK (state IN ('staging', 'ready', 'promoted')),
  channel_id BLOB NOT NULL CHECK (
    length(channel_id) = 32 AND channel_id <> zeroblob(32)
  ),
  relay_generation BLOB NOT NULL CHECK (
    length(relay_generation) = 32 AND relay_generation <> zeroblob(32)
  ),
  admin_public_key BLOB NOT NULL CHECK (
    length(admin_public_key) = 32 AND admin_public_key <> zeroblob(32)
  ),
  membership_generation INTEGER NOT NULL CHECK (
    membership_generation BETWEEN 1 AND 4294967295
  ),
  inventory_arrival_head INTEGER NOT NULL CHECK (
    inventory_arrival_head >= 0
  ),
  base_authority_digest_version INTEGER CHECK (
    base_authority_digest_version IS NULL
    OR base_authority_digest_version IN (1, 2)
  ),
  base_authority_digest BLOB CHECK (
    base_authority_digest IS NULL
    OR (
      length(base_authority_digest) = 32
      AND base_authority_digest <> zeroblob(32)
    )
  ),
  page_count INTEGER NOT NULL CHECK (page_count > 0),
  environment_count INTEGER NOT NULL CHECK (environment_count > 0),
  through_environment_id TEXT NOT NULL CHECK (
    length(through_environment_id) BETWEEN 1 AND 128
    AND through_environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  rolling_environment_digest BLOB NOT NULL CHECK (
    length(rolling_environment_digest) = 32
    AND rolling_environment_digest <> zeroblob(32)
  ),
  authority_digest_version INTEGER NOT NULL CHECK (
    authority_digest_version = 2
  ),
  authority_digest BLOB CHECK (
    authority_digest IS NULL
    OR (
      length(authority_digest) = 32
      AND authority_digest <> zeroblob(32)
    )
  ),
  CHECK (
    (base_authority_digest_version IS NULL AND base_authority_digest IS NULL)
    OR
    (base_authority_digest_version IS NOT NULL AND base_authority_digest IS NOT NULL)
  ),
  CHECK (
    (state = 'staging' AND authority_digest IS NULL)
    OR
    (state IN ('ready', 'promoted') AND authority_digest IS NOT NULL)
  ),
  PRIMARY KEY (project_id, candidate_id)
) STRICT, WITHOUT ROWID`

const syncAuthorityCandidatesTableSQL = `CREATE TABLE continuity_sync_authority_candidates (
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  candidate_id BLOB NOT NULL CHECK (
    length(candidate_id) = 32 AND candidate_id <> zeroblob(32)
  ),
  state TEXT NOT NULL CHECK (state IN ('staging', 'ready', 'promoted')),
  channel_id BLOB NOT NULL CHECK (
    length(channel_id) = 32 AND channel_id <> zeroblob(32)
  ),
  relay_generation BLOB NOT NULL CHECK (
    length(relay_generation) = 32 AND relay_generation <> zeroblob(32)
  ),
  admin_public_key BLOB NOT NULL CHECK (
    length(admin_public_key) = 32 AND admin_public_key <> zeroblob(32)
  ),
  membership_generation INTEGER NOT NULL CHECK (
    membership_generation BETWEEN 1 AND 4294967295
  ),
  inventory_arrival_head INTEGER NOT NULL CHECK (
    inventory_arrival_head >= 0
  ),
  base_authority_digest_version INTEGER CHECK (
    base_authority_digest_version IS NULL
    OR base_authority_digest_version IN (1, 2)
  ),
  base_authority_digest BLOB CHECK (
    base_authority_digest IS NULL
    OR (
      length(base_authority_digest) = 32
      AND base_authority_digest <> zeroblob(32)
    )
  ),
  page_count INTEGER NOT NULL CHECK (page_count > 0),
  environment_count INTEGER NOT NULL CHECK (environment_count > 0),
  through_environment_id TEXT NOT NULL CHECK (
    length(through_environment_id) BETWEEN 1 AND 128
    AND through_environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  rolling_environment_digest BLOB NOT NULL CHECK (
    length(rolling_environment_digest) = 32
    AND rolling_environment_digest <> zeroblob(32)
  ),
  authority_digest_version INTEGER NOT NULL CHECK (
    authority_digest_version = 2
  ),
  authority_digest BLOB CHECK (
    authority_digest IS NULL
    OR (
      length(authority_digest) = 32
      AND authority_digest <> zeroblob(32)
    )
  ),
  role TEXT NOT NULL DEFAULT 'ordinary' CHECK (
    role IN ('ordinary', 'recovery-predecessor', 'recovery-successor')
  ) CHECK (
    role = 'ordinary'
    OR (role = 'recovery-predecessor' AND state = 'ready')
    OR (role = 'recovery-successor' AND state IN ('staging', 'ready'))
  ),
  CHECK (
    (base_authority_digest_version IS NULL AND base_authority_digest IS NULL)
    OR
    (base_authority_digest_version IS NOT NULL AND base_authority_digest IS NOT NULL)
  ),
  CHECK (
    (state = 'staging' AND authority_digest IS NULL)
    OR
    (state IN ('ready', 'promoted') AND authority_digest IS NOT NULL)
  ),
  PRIMARY KEY (project_id, candidate_id)
) STRICT, WITHOUT ROWID`

const syncAuthorityCandidatePagesTableSQL = `CREATE TABLE continuity_sync_authority_candidate_pages (
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  candidate_id BLOB NOT NULL CHECK (
    length(candidate_id) = 32 AND candidate_id <> zeroblob(32)
  ),
  page_number INTEGER NOT NULL CHECK (page_number > 0),
  after_environment_id TEXT CHECK (
    after_environment_id IS NULL
    OR (
      length(after_environment_id) BETWEEN 1 AND 128
      AND after_environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
    )
  ),
  through_environment_id TEXT NOT NULL CHECK (
    length(through_environment_id) BETWEEN 1 AND 128
    AND through_environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_count INTEGER NOT NULL CHECK (
    environment_count BETWEEN 1 AND 4
  ),
  more INTEGER NOT NULL CHECK (more IN (0, 1)),
  page_digest BLOB NOT NULL CHECK (
    length(page_digest) = 32 AND page_digest <> zeroblob(32)
  ),
  resulting_environment_count INTEGER NOT NULL CHECK (
    resulting_environment_count >= environment_count
  ),
  resulting_rolling_digest BLOB NOT NULL CHECK (
    length(resulting_rolling_digest) = 32
    AND resulting_rolling_digest <> zeroblob(32)
  ),
  CHECK (
    (page_number = 1 AND after_environment_id IS NULL)
    OR
    (page_number > 1 AND after_environment_id IS NOT NULL)
  ),
  CHECK (
    after_environment_id IS NULL
    OR through_environment_id > after_environment_id
  ),
  PRIMARY KEY (project_id, candidate_id, page_number),
  UNIQUE (project_id, candidate_id, after_environment_id),
  FOREIGN KEY (project_id, candidate_id)
    REFERENCES continuity_sync_authority_candidates(project_id, candidate_id)
    ON DELETE CASCADE
) STRICT, WITHOUT ROWID`

const syncAuthorityCandidateEnvironmentsTableSQL = `CREATE TABLE continuity_sync_authority_candidate_environments (
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  candidate_id BLOB NOT NULL CHECK (
    length(candidate_id) = 32 AND candidate_id <> zeroblob(32)
  ),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_ordinal INTEGER NOT NULL CHECK (environment_ordinal > 0),
  page_number INTEGER NOT NULL CHECK (page_number > 0),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32 AND certificate_id <> zeroblob(32)
  ),
  certificate_bytes BLOB NOT NULL CHECK (
    length(certificate_bytes) BETWEEN 1 AND 8192
  ),
  mode TEXT NOT NULL CHECK (mode IN ('trusted', 'ephemeral')),
  expires_at_millis INTEGER NOT NULL CHECK (expires_at_millis >= 0),
  join_membership_generation INTEGER NOT NULL CHECK (
    join_membership_generation BETWEEN 1 AND 4294967295
  ),
  retirement_relay_generation BLOB CHECK (
    retirement_relay_generation IS NULL
    OR (
      length(retirement_relay_generation) = 32
      AND retirement_relay_generation <> zeroblob(32)
    )
  ),
  retirement_membership_generation INTEGER CHECK (
    retirement_membership_generation IS NULL
    OR retirement_membership_generation BETWEEN 1 AND 4294967295
  ),
  retirement_final_environment_sequence INTEGER CHECK (
    retirement_final_environment_sequence IS NULL
    OR retirement_final_environment_sequence >= 0
  ),
  retirement_final_envelope_digest BLOB CHECK (
    retirement_final_envelope_digest IS NULL
    OR length(retirement_final_envelope_digest) = 32
  ),
  retirement_id BLOB CHECK (
    retirement_id IS NULL
    OR (length(retirement_id) = 32 AND retirement_id <> zeroblob(32))
  ),
  retirement_bytes BLOB CHECK (
    retirement_bytes IS NULL
    OR length(retirement_bytes) BETWEEN 1 AND 4096
  ),
  CHECK (
    (mode = 'trusted' AND expires_at_millis = 0)
    OR (mode = 'ephemeral' AND expires_at_millis > 0)
  ),
  CHECK (
    (retirement_membership_generation IS NULL
      AND retirement_relay_generation IS NULL
      AND retirement_final_environment_sequence IS NULL
      AND retirement_final_envelope_digest IS NULL
      AND retirement_id IS NULL
      AND retirement_bytes IS NULL)
    OR
    (retirement_membership_generation IS NOT NULL
      AND retirement_relay_generation IS NOT NULL
      AND retirement_final_environment_sequence IS NOT NULL
      AND retirement_final_envelope_digest IS NOT NULL
      AND retirement_id IS NOT NULL
      AND retirement_bytes IS NOT NULL)
  ),
  CHECK (
    retirement_membership_generation IS NULL
    OR retirement_membership_generation >= join_membership_generation
  ),
  CHECK (
    retirement_final_environment_sequence IS NULL
    OR (retirement_final_environment_sequence = 0
      AND retirement_final_envelope_digest = zeroblob(32))
    OR (retirement_final_environment_sequence > 0
      AND retirement_final_envelope_digest <> zeroblob(32))
  ),
  PRIMARY KEY (project_id, candidate_id, environment_id),
  UNIQUE (project_id, candidate_id, environment_ordinal),
  UNIQUE (project_id, candidate_id, certificate_id),
  FOREIGN KEY (project_id, candidate_id, page_number)
    REFERENCES continuity_sync_authority_candidate_pages(
      project_id, candidate_id, page_number
    ) ON DELETE CASCADE
) STRICT, WITHOUT ROWID`

const syncAuthorityCandidateMembershipEventsTableSQL = `CREATE TABLE continuity_sync_authority_candidate_membership_events (
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  candidate_id BLOB NOT NULL CHECK (
    length(candidate_id) = 32 AND candidate_id <> zeroblob(32)
  ),
  membership_generation INTEGER NOT NULL CHECK (
    membership_generation BETWEEN 1 AND 4294967295
  ),
  event_kind TEXT NOT NULL CHECK (event_kind IN ('join', 'retirement')),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  PRIMARY KEY (project_id, candidate_id, membership_generation),
  UNIQUE (project_id, candidate_id, environment_id, event_kind),
  FOREIGN KEY (project_id, candidate_id, environment_id)
    REFERENCES continuity_sync_authority_candidate_environments(
      project_id, candidate_id, environment_id
    ) ON DELETE CASCADE
) STRICT, WITHOUT ROWID`

const syncAuthorityCandidatesActiveIndexV5SQL = `CREATE UNIQUE INDEX ux_continuity_sync_authority_candidates_active_project
ON continuity_sync_authority_candidates(project_id)
WHERE state IN ('staging', 'ready')`

const syncAuthorityCandidatesActiveIndexSQL = `CREATE UNIQUE INDEX ux_continuity_sync_authority_candidates_active_project
ON continuity_sync_authority_candidates(project_id)
WHERE state IN ('staging', 'ready')
  AND role IN ('ordinary', 'recovery-successor')`

const syncAuthorityCandidatesRecoveryPredecessorIndexSQL = `CREATE UNIQUE INDEX ux_continuity_sync_authority_candidates_recovery_predecessor_project
ON continuity_sync_authority_candidates(project_id)
WHERE state = 'ready' AND role = 'recovery-predecessor'`

const syncAuthorityRecoveryTransitionsTableSQL = `CREATE TABLE continuity_sync_authority_recovery_transitions (
  project_id TEXT NOT NULL PRIMARY KEY CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  predecessor_candidate_id BLOB CHECK (
    predecessor_candidate_id IS NULL
    OR (
      length(predecessor_candidate_id) = 32
      AND predecessor_candidate_id <> zeroblob(32)
    )
  ),
  successor_candidate_id BLOB NOT NULL CHECK (
    length(successor_candidate_id) = 32
    AND successor_candidate_id <> zeroblob(32)
  ),
  writer_environment_id TEXT NOT NULL CHECK (
    length(writer_environment_id) BETWEEN 1 AND 128
    AND writer_environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  writer_certificate_id BLOB NOT NULL CHECK (
    length(writer_certificate_id) = 32
    AND writer_certificate_id <> zeroblob(32)
  ),
  target_membership_generation INTEGER NOT NULL CHECK (
    target_membership_generation BETWEEN 1 AND 4294967295
  ),
  CHECK (
    (predecessor_candidate_id IS NULL AND target_membership_generation = 1)
    OR
    (predecessor_candidate_id IS NOT NULL AND target_membership_generation >= 2)
  ),
  CHECK (
    predecessor_candidate_id IS NULL
    OR predecessor_candidate_id <> successor_candidate_id
  ),
  FOREIGN KEY (project_id, predecessor_candidate_id)
    REFERENCES continuity_sync_authority_candidates(project_id, candidate_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (project_id, successor_candidate_id)
    REFERENCES continuity_sync_authority_candidates(project_id, candidate_id)
    ON DELETE RESTRICT
) STRICT, WITHOUT ROWID`

const syncAuthorityCandidatePagesFinalIndexSQL = `CREATE UNIQUE INDEX ux_continuity_sync_authority_candidate_pages_final
ON continuity_sync_authority_candidate_pages(project_id, candidate_id)
WHERE more = 0`

const schemaV1DDL = schemaTableV1SQL + ";\n" +
	factsTableSQL + ";\n" +
	projectIdentityIndexSQL + ";\n" +
	projectOrderIndexSQL + ";\n" +
	subjectOrderIndexSQL + ";\n" +
	projectIdentityTriggerSQL + ";\n"

const syncSchemaV2DDL = syncProjectsTableSQL + ";\n" +
	syncInboxTableV2SQL + ";\n" +
	syncReceiptsTableSQL + ";\n" +
	syncEnvironmentHeadsTableSQL + ";\n" +
	syncOutboxTableSQL + ";\n" +
	syncTombstonesTableSQL + ";\n" +
	syncEnvironmentCertificatesTableSQL + ";\n"

const schemaV2DDL = schemaTableV2SQL + ";\n" +
	factsTableSQL + ";\n" +
	projectIdentityIndexSQL + ";\n" +
	projectOrderIndexSQL + ";\n" +
	subjectOrderIndexSQL + ";\n" +
	projectIdentityTriggerSQL + ";\n" +
	syncSchemaV2DDL

const syncSchemaV3DDL = syncProjectsTableSQL + ";\n" +
	syncInboxTableSQL + ";\n" +
	syncReceiptsTableSQL + ";\n" +
	syncEnvironmentHeadsTableSQL + ";\n" +
	syncOutboxTableSQL + ";\n" +
	syncTombstonesTableSQL + ";\n" +
	syncEnvironmentCertificatesTableSQL + ";\n" +
	syncTerminalCandidatesTableSQL + ";\n" +
	syncTerminalCandidateFramesTableSQL + ";\n" +
	syncTerminalCandidatesStagingIndexSQL + ";\n"

const schemaV3DDL = schemaTableV3SQL + ";\n" +
	factsTableSQL + ";\n" +
	projectIdentityIndexSQL + ";\n" +
	projectOrderIndexSQL + ";\n" +
	subjectOrderIndexSQL + ";\n" +
	projectIdentityTriggerSQL + ";\n" +
	syncSchemaV3DDL

const syncAuthorityCandidateSchemaV5DDL = syncAuthoritiesTableSQL + ";\n" +
	syncAuthorityCandidatesTableV5SQL + ";\n" +
	syncAuthorityCandidatePagesTableSQL + ";\n" +
	syncAuthorityCandidateEnvironmentsTableSQL + ";\n" +
	syncAuthorityCandidateMembershipEventsTableSQL + ";\n" +
	syncAuthorityCandidatesActiveIndexV5SQL + ";\n" +
	syncAuthorityCandidatePagesFinalIndexSQL + ";\n"

const syncAuthorityCandidateSchemaDDL = syncAuthoritiesTableSQL + ";\n" +
	syncAuthorityCandidatesTableSQL + ";\n" +
	syncAuthorityCandidatePagesTableSQL + ";\n" +
	syncAuthorityCandidateEnvironmentsTableSQL + ";\n" +
	syncAuthorityCandidateMembershipEventsTableSQL + ";\n" +
	syncAuthorityCandidatesActiveIndexSQL + ";\n" +
	syncAuthorityCandidatesRecoveryPredecessorIndexSQL + ";\n" +
	syncAuthorityCandidatePagesFinalIndexSQL + ";\n" +
	syncAuthorityRecoveryTransitionsTableSQL + ";\n"

const schemaV4DDL = schemaTableV4SQL + ";\n" +
	factsTableSQL + ";\n" +
	projectIdentityIndexSQL + ";\n" +
	projectOrderIndexSQL + ";\n" +
	subjectOrderIndexSQL + ";\n" +
	projectIdentityTriggerSQL + ";\n" +
	syncSchemaV3DDL +
	syncAuthorityCandidateSchemaV5DDL

const schemaV5DDL = schemaTableV5SQL + ";\n" +
	factsTableSQL + ";\n" +
	projectIdentityIndexSQL + ";\n" +
	projectOrderIndexSQL + ";\n" +
	subjectOrderIndexSQL + ";\n" +
	projectIdentityTriggerSQL + ";\n" +
	syncSchemaV3DDL +
	syncAuthorityCandidateSchemaV5DDL +
	syncRelayWatermarksTableSQL + ";\n"

const schemaDDL = schemaTableSQL + ";\n" +
	factsTableSQL + ";\n" +
	projectIdentityIndexSQL + ";\n" +
	projectOrderIndexSQL + ";\n" +
	subjectOrderIndexSQL + ";\n" +
	projectIdentityTriggerSQL + ";\n" +
	syncSchemaV3DDL +
	syncAuthorityCandidateSchemaDDL +
	syncRelayWatermarksTableSQL + ";\n"

type schemaObject struct {
	kind  string
	name  string
	table string
	sql   string
}

func expectedSchemaObjects() []schemaObject {
	return []schemaObject{
		{kind: "index", name: "ix_continuity_facts_project_order", table: "continuity_facts", sql: projectOrderIndexSQL},
		{kind: "index", name: "ix_continuity_facts_subject_order", table: "continuity_facts", sql: subjectOrderIndexSQL},
		{kind: "index", name: "ux_continuity_project_identity", table: "continuity_facts", sql: projectIdentityIndexSQL},
		{kind: "index", name: "ux_continuity_sync_authority_candidate_pages_final", table: "continuity_sync_authority_candidate_pages", sql: syncAuthorityCandidatePagesFinalIndexSQL},
		{kind: "index", name: "ux_continuity_sync_authority_candidates_active_project", table: "continuity_sync_authority_candidates", sql: syncAuthorityCandidatesActiveIndexSQL},
		{kind: "index", name: "ux_continuity_sync_authority_candidates_recovery_predecessor_project", table: "continuity_sync_authority_candidates", sql: syncAuthorityCandidatesRecoveryPredecessorIndexSQL},
		{kind: "index", name: "ux_continuity_sync_terminal_candidates_staging_project", table: "continuity_sync_terminal_candidates", sql: syncTerminalCandidatesStagingIndexSQL},
		{kind: "table", name: "continuity_facts", table: "continuity_facts", sql: factsTableSQL},
		{kind: "table", name: "continuity_schema", table: "continuity_schema", sql: schemaTableSQL},
		{kind: "table", name: "continuity_sync_authorities", table: "continuity_sync_authorities", sql: syncAuthoritiesTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidate_environments", table: "continuity_sync_authority_candidate_environments", sql: syncAuthorityCandidateEnvironmentsTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidate_membership_events", table: "continuity_sync_authority_candidate_membership_events", sql: syncAuthorityCandidateMembershipEventsTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidate_pages", table: "continuity_sync_authority_candidate_pages", sql: syncAuthorityCandidatePagesTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidates", table: "continuity_sync_authority_candidates", sql: syncAuthorityCandidatesTableSQL},
		{kind: "table", name: "continuity_sync_authority_recovery_transitions", table: "continuity_sync_authority_recovery_transitions", sql: syncAuthorityRecoveryTransitionsTableSQL},
		{kind: "table", name: "continuity_sync_environment_certificates", table: "continuity_sync_environment_certificates", sql: syncEnvironmentCertificatesTableSQL},
		{kind: "table", name: "continuity_sync_environment_heads", table: "continuity_sync_environment_heads", sql: syncEnvironmentHeadsTableSQL},
		{kind: "table", name: "continuity_sync_inbox", table: "continuity_sync_inbox", sql: syncInboxTableSQL},
		{kind: "table", name: "continuity_sync_outbox", table: "continuity_sync_outbox", sql: syncOutboxTableSQL},
		{kind: "table", name: "continuity_sync_projects", table: "continuity_sync_projects", sql: syncProjectsTableSQL},
		{kind: "table", name: "continuity_sync_receipts", table: "continuity_sync_receipts", sql: syncReceiptsTableSQL},
		{kind: "table", name: "continuity_sync_relay_watermarks", table: "continuity_sync_relay_watermarks", sql: syncRelayWatermarksTableSQL},
		{kind: "table", name: "continuity_sync_terminal_candidate_frames", table: "continuity_sync_terminal_candidate_frames", sql: syncTerminalCandidateFramesTableSQL},
		{kind: "table", name: "continuity_sync_terminal_candidates", table: "continuity_sync_terminal_candidates", sql: syncTerminalCandidatesTableSQL},
		{kind: "table", name: "continuity_sync_tombstones", table: "continuity_sync_tombstones", sql: syncTombstonesTableSQL},
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", sql: projectIdentityTriggerSQL},
	}
}

func expectedSchemaV5Objects() []schemaObject {
	return []schemaObject{
		{kind: "index", name: "ix_continuity_facts_project_order", table: "continuity_facts", sql: projectOrderIndexSQL},
		{kind: "index", name: "ix_continuity_facts_subject_order", table: "continuity_facts", sql: subjectOrderIndexSQL},
		{kind: "index", name: "ux_continuity_project_identity", table: "continuity_facts", sql: projectIdentityIndexSQL},
		{kind: "index", name: "ux_continuity_sync_authority_candidate_pages_final", table: "continuity_sync_authority_candidate_pages", sql: syncAuthorityCandidatePagesFinalIndexSQL},
		{kind: "index", name: "ux_continuity_sync_authority_candidates_active_project", table: "continuity_sync_authority_candidates", sql: syncAuthorityCandidatesActiveIndexV5SQL},
		{kind: "index", name: "ux_continuity_sync_terminal_candidates_staging_project", table: "continuity_sync_terminal_candidates", sql: syncTerminalCandidatesStagingIndexSQL},
		{kind: "table", name: "continuity_facts", table: "continuity_facts", sql: factsTableSQL},
		{kind: "table", name: "continuity_schema", table: "continuity_schema", sql: schemaTableV5SQL},
		{kind: "table", name: "continuity_sync_authorities", table: "continuity_sync_authorities", sql: syncAuthoritiesTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidate_environments", table: "continuity_sync_authority_candidate_environments", sql: syncAuthorityCandidateEnvironmentsTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidate_membership_events", table: "continuity_sync_authority_candidate_membership_events", sql: syncAuthorityCandidateMembershipEventsTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidate_pages", table: "continuity_sync_authority_candidate_pages", sql: syncAuthorityCandidatePagesTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidates", table: "continuity_sync_authority_candidates", sql: syncAuthorityCandidatesTableV5SQL},
		{kind: "table", name: "continuity_sync_environment_certificates", table: "continuity_sync_environment_certificates", sql: syncEnvironmentCertificatesTableSQL},
		{kind: "table", name: "continuity_sync_environment_heads", table: "continuity_sync_environment_heads", sql: syncEnvironmentHeadsTableSQL},
		{kind: "table", name: "continuity_sync_inbox", table: "continuity_sync_inbox", sql: syncInboxTableSQL},
		{kind: "table", name: "continuity_sync_outbox", table: "continuity_sync_outbox", sql: syncOutboxTableSQL},
		{kind: "table", name: "continuity_sync_projects", table: "continuity_sync_projects", sql: syncProjectsTableSQL},
		{kind: "table", name: "continuity_sync_receipts", table: "continuity_sync_receipts", sql: syncReceiptsTableSQL},
		{kind: "table", name: "continuity_sync_relay_watermarks", table: "continuity_sync_relay_watermarks", sql: syncRelayWatermarksTableSQL},
		{kind: "table", name: "continuity_sync_terminal_candidate_frames", table: "continuity_sync_terminal_candidate_frames", sql: syncTerminalCandidateFramesTableSQL},
		{kind: "table", name: "continuity_sync_terminal_candidates", table: "continuity_sync_terminal_candidates", sql: syncTerminalCandidatesTableSQL},
		{kind: "table", name: "continuity_sync_tombstones", table: "continuity_sync_tombstones", sql: syncTombstonesTableSQL},
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", sql: projectIdentityTriggerSQL},
	}
}

func expectedSchemaV4Objects() []schemaObject {
	return []schemaObject{
		{kind: "index", name: "ix_continuity_facts_project_order", table: "continuity_facts", sql: projectOrderIndexSQL},
		{kind: "index", name: "ix_continuity_facts_subject_order", table: "continuity_facts", sql: subjectOrderIndexSQL},
		{kind: "index", name: "ux_continuity_project_identity", table: "continuity_facts", sql: projectIdentityIndexSQL},
		{kind: "index", name: "ux_continuity_sync_authority_candidate_pages_final", table: "continuity_sync_authority_candidate_pages", sql: syncAuthorityCandidatePagesFinalIndexSQL},
		{kind: "index", name: "ux_continuity_sync_authority_candidates_active_project", table: "continuity_sync_authority_candidates", sql: syncAuthorityCandidatesActiveIndexV5SQL},
		{kind: "index", name: "ux_continuity_sync_terminal_candidates_staging_project", table: "continuity_sync_terminal_candidates", sql: syncTerminalCandidatesStagingIndexSQL},
		{kind: "table", name: "continuity_facts", table: "continuity_facts", sql: factsTableSQL},
		{kind: "table", name: "continuity_schema", table: "continuity_schema", sql: schemaTableV4SQL},
		{kind: "table", name: "continuity_sync_authorities", table: "continuity_sync_authorities", sql: syncAuthoritiesTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidate_environments", table: "continuity_sync_authority_candidate_environments", sql: syncAuthorityCandidateEnvironmentsTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidate_membership_events", table: "continuity_sync_authority_candidate_membership_events", sql: syncAuthorityCandidateMembershipEventsTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidate_pages", table: "continuity_sync_authority_candidate_pages", sql: syncAuthorityCandidatePagesTableSQL},
		{kind: "table", name: "continuity_sync_authority_candidates", table: "continuity_sync_authority_candidates", sql: syncAuthorityCandidatesTableV5SQL},
		{kind: "table", name: "continuity_sync_environment_certificates", table: "continuity_sync_environment_certificates", sql: syncEnvironmentCertificatesTableSQL},
		{kind: "table", name: "continuity_sync_environment_heads", table: "continuity_sync_environment_heads", sql: syncEnvironmentHeadsTableSQL},
		{kind: "table", name: "continuity_sync_inbox", table: "continuity_sync_inbox", sql: syncInboxTableSQL},
		{kind: "table", name: "continuity_sync_outbox", table: "continuity_sync_outbox", sql: syncOutboxTableSQL},
		{kind: "table", name: "continuity_sync_projects", table: "continuity_sync_projects", sql: syncProjectsTableSQL},
		{kind: "table", name: "continuity_sync_receipts", table: "continuity_sync_receipts", sql: syncReceiptsTableSQL},
		{kind: "table", name: "continuity_sync_terminal_candidate_frames", table: "continuity_sync_terminal_candidate_frames", sql: syncTerminalCandidateFramesTableSQL},
		{kind: "table", name: "continuity_sync_terminal_candidates", table: "continuity_sync_terminal_candidates", sql: syncTerminalCandidatesTableSQL},
		{kind: "table", name: "continuity_sync_tombstones", table: "continuity_sync_tombstones", sql: syncTombstonesTableSQL},
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", sql: projectIdentityTriggerSQL},
	}
}

func expectedSchemaV3Objects() []schemaObject {
	return []schemaObject{
		{kind: "index", name: "ix_continuity_facts_project_order", table: "continuity_facts", sql: projectOrderIndexSQL},
		{kind: "index", name: "ix_continuity_facts_subject_order", table: "continuity_facts", sql: subjectOrderIndexSQL},
		{kind: "index", name: "ux_continuity_project_identity", table: "continuity_facts", sql: projectIdentityIndexSQL},
		{kind: "index", name: "ux_continuity_sync_terminal_candidates_staging_project", table: "continuity_sync_terminal_candidates", sql: syncTerminalCandidatesStagingIndexSQL},
		{kind: "table", name: "continuity_facts", table: "continuity_facts", sql: factsTableSQL},
		{kind: "table", name: "continuity_schema", table: "continuity_schema", sql: schemaTableV3SQL},
		{kind: "table", name: "continuity_sync_environment_certificates", table: "continuity_sync_environment_certificates", sql: syncEnvironmentCertificatesTableSQL},
		{kind: "table", name: "continuity_sync_environment_heads", table: "continuity_sync_environment_heads", sql: syncEnvironmentHeadsTableSQL},
		{kind: "table", name: "continuity_sync_inbox", table: "continuity_sync_inbox", sql: syncInboxTableSQL},
		{kind: "table", name: "continuity_sync_outbox", table: "continuity_sync_outbox", sql: syncOutboxTableSQL},
		{kind: "table", name: "continuity_sync_projects", table: "continuity_sync_projects", sql: syncProjectsTableSQL},
		{kind: "table", name: "continuity_sync_receipts", table: "continuity_sync_receipts", sql: syncReceiptsTableSQL},
		{kind: "table", name: "continuity_sync_terminal_candidate_frames", table: "continuity_sync_terminal_candidate_frames", sql: syncTerminalCandidateFramesTableSQL},
		{kind: "table", name: "continuity_sync_terminal_candidates", table: "continuity_sync_terminal_candidates", sql: syncTerminalCandidatesTableSQL},
		{kind: "table", name: "continuity_sync_tombstones", table: "continuity_sync_tombstones", sql: syncTombstonesTableSQL},
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", sql: projectIdentityTriggerSQL},
	}
}

func expectedSchemaV2Objects() []schemaObject {
	return []schemaObject{
		{kind: "index", name: "ix_continuity_facts_project_order", table: "continuity_facts", sql: projectOrderIndexSQL},
		{kind: "index", name: "ix_continuity_facts_subject_order", table: "continuity_facts", sql: subjectOrderIndexSQL},
		{kind: "index", name: "ux_continuity_project_identity", table: "continuity_facts", sql: projectIdentityIndexSQL},
		{kind: "table", name: "continuity_facts", table: "continuity_facts", sql: factsTableSQL},
		{kind: "table", name: "continuity_schema", table: "continuity_schema", sql: schemaTableV2SQL},
		{kind: "table", name: "continuity_sync_environment_certificates", table: "continuity_sync_environment_certificates", sql: syncEnvironmentCertificatesTableSQL},
		{kind: "table", name: "continuity_sync_environment_heads", table: "continuity_sync_environment_heads", sql: syncEnvironmentHeadsTableSQL},
		{kind: "table", name: "continuity_sync_inbox", table: "continuity_sync_inbox", sql: syncInboxTableV2SQL},
		{kind: "table", name: "continuity_sync_outbox", table: "continuity_sync_outbox", sql: syncOutboxTableSQL},
		{kind: "table", name: "continuity_sync_projects", table: "continuity_sync_projects", sql: syncProjectsTableSQL},
		{kind: "table", name: "continuity_sync_receipts", table: "continuity_sync_receipts", sql: syncReceiptsTableSQL},
		{kind: "table", name: "continuity_sync_tombstones", table: "continuity_sync_tombstones", sql: syncTombstonesTableSQL},
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", sql: projectIdentityTriggerSQL},
	}
}

func expectedSchemaV1Objects() []schemaObject {
	return []schemaObject{
		{kind: "index", name: "ix_continuity_facts_project_order", table: "continuity_facts", sql: projectOrderIndexSQL},
		{kind: "index", name: "ix_continuity_facts_subject_order", table: "continuity_facts", sql: subjectOrderIndexSQL},
		{kind: "index", name: "ux_continuity_project_identity", table: "continuity_facts", sql: projectIdentityIndexSQL},
		{kind: "table", name: "continuity_facts", table: "continuity_facts", sql: factsTableSQL},
		{kind: "table", name: "continuity_schema", table: "continuity_schema", sql: schemaTableV1SQL},
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", sql: projectIdentityTriggerSQL},
	}
}

func checksumSchema() string {
	return checksumDDL(schemaDDL)
}

func checksumSchemaV5() string {
	return checksumDDL(schemaV5DDL)
}

func checksumSchemaV4() string {
	return checksumDDL(schemaV4DDL)
}

func checksumSchemaV3() string {
	return checksumDDL(schemaV3DDL)
}

func checksumSchemaV2() string {
	return checksumDDL(schemaV2DDL)
}

func checksumSchemaV1() string {
	return checksumDDL(schemaV1DDL)
}

func checksumDDL(ddl string) string {
	sum := sha256.Sum256([]byte(ddl))
	return hex.EncodeToString(sum[:])
}

func initializeSchemaIfEmpty(db *sql.DB) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin continuity schema transaction: %w", err)
	}
	defer tx.Rollback()

	var objectCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&objectCount); err != nil {
		return false, fmt.Errorf("inspect continuity schema inventory in transaction: %w", err)
	}
	if objectCount != 0 {
		return false, nil
	}
	var existingApplicationID, existingUserVersion int
	if err := tx.QueryRow(`PRAGMA application_id`).Scan(&existingApplicationID); err != nil {
		return false, fmt.Errorf("read empty continuity application id: %w", err)
	}
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&existingUserVersion); err != nil {
		return false, fmt.Errorf("read empty continuity schema version: %w", err)
	}
	if existingApplicationID != 0 || existingUserVersion != 0 {
		return false, fmt.Errorf("empty continuity database carries foreign schema identity")
	}

	if _, err := tx.Exec(schemaDDL); err != nil {
		return false, fmt.Errorf("create continuity schema: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA application_id = 1280267825`); err != nil {
		return false, fmt.Errorf("set continuity application id: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 6`); err != nil {
		return false, fmt.Errorf("set continuity schema version: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, ?, ?, ?)`,
		schemaLine,
		schemaVersion,
		checksumSchema(),
	); err != nil {
		return false, fmt.Errorf("record continuity schema identity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit continuity schema: %w", err)
	}
	return true, nil
}

func validateSchema(db *sql.DB) error {
	return validateSchemaVersion(db, schemaVersion, checksumSchema(), expectedSchemaObjects())
}

type schemaQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

func validateSchemaVersion(db schemaQuerier, version int, checksum string, want []schemaObject) error {
	var gotApplicationID int
	if err := db.QueryRow(`PRAGMA application_id`).Scan(&gotApplicationID); err != nil {
		return fmt.Errorf("read continuity application id: %w", err)
	}
	if gotApplicationID != applicationID {
		return fmt.Errorf("continuity application id = %d, want %d", gotApplicationID, applicationID)
	}

	var gotVersion int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&gotVersion); err != nil {
		return fmt.Errorf("read continuity schema version: %w", err)
	}
	if gotVersion != version {
		return fmt.Errorf("continuity schema version = %d, want %d", gotVersion, version)
	}

	var gotLine, gotChecksum string
	var gotRowVersion int
	if err := db.QueryRow(
		`SELECT schema_line, schema_version, schema_checksum FROM continuity_schema WHERE singleton = 1`,
	).Scan(&gotLine, &gotRowVersion, &gotChecksum); err != nil {
		return fmt.Errorf("read continuity schema identity: %w", err)
	}
	if gotLine != schemaLine || gotRowVersion != version || gotChecksum != checksum {
		return fmt.Errorf("continuity schema identity does not match this build")
	}

	rows, err := db.Query(`
SELECT type, name, tbl_name, sql
FROM sqlite_schema
WHERE name NOT LIKE 'sqlite_%'
ORDER BY type, name`)
	if err != nil {
		return fmt.Errorf("inspect continuity schema: %w", err)
	}
	defer rows.Close()

	index := 0
	for rows.Next() {
		var got schemaObject
		if err := rows.Scan(&got.kind, &got.name, &got.table, &got.sql); err != nil {
			return fmt.Errorf("scan continuity schema object: %w", err)
		}
		if index >= len(want) {
			return fmt.Errorf("unexpected continuity schema object %s %s", got.kind, got.name)
		}
		expected := want[index]
		if got.kind != expected.kind || got.name != expected.name || got.table != expected.table || normalizeSQL(got.sql) != normalizeSQL(expected.sql) {
			return fmt.Errorf("continuity schema object %s %s differs from this build", got.kind, got.name)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate continuity schema objects: %w", err)
	}
	if index != len(want) {
		return fmt.Errorf("continuity schema has %d recognized objects, want %d", index, len(want))
	}
	return nil
}

func migrateSchema(db *sql.DB) error {
	for {
		var version int
		if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
			return fmt.Errorf("read continuity schema version before migration: %w", err)
		}
		switch version {
		case schemaVersion:
			return nil
		case 1:
			if err := migrateSchemaV1ToV2(db); err != nil {
				return err
			}
		case 2:
			if err := migrateSchemaV2ToV3(db); err != nil {
				return err
			}
		case 3:
			if err := migrateSchemaV3ToV4(db); err != nil {
				return err
			}
		case 4:
			if err := migrateSchemaV4ToV5(db); err != nil {
				return err
			}
		case 5:
			if err := migrateSchemaV5ToV6(db); err != nil {
				return err
			}
		default:
			return fmt.Errorf("continuity schema version = %d, want 1, 2, 3, 4, 5, or %d", version, schemaVersion)
		}
	}
}

func migrateSchemaV1ToV2(db *sql.DB) error {
	advanced, err := validateMigrationPreflight(db, 1)
	if err != nil {
		return fmt.Errorf("validate continuity v1 before migration: %w", err)
	}
	if advanced {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin continuity v2 migration: %w", err)
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("recheck continuity v1 schema version in migration: %w", err)
	}
	if version > 1 && version <= schemaVersion {
		if err := validateKnownSchemaVersion(tx, version); err != nil {
			return fmt.Errorf("validate concurrently advanced continuity schema: %w", err)
		}
		return nil
	}
	if version != 1 {
		return fmt.Errorf("continuity schema changed during v2 migration")
	}
	if err := validateSchemaVersion(tx, 1, checksumSchemaV1(), expectedSchemaV1Objects()); err != nil {
		return fmt.Errorf("revalidate continuity v1 in migration: %w", err)
	}
	if err := validateV1EnvironmentHistory(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE continuity_schema`); err != nil {
		return fmt.Errorf("drop continuity v1 schema identity: %w", err)
	}
	if _, err := tx.Exec(schemaTableV2SQL + ";\n" + syncSchemaV2DDL); err != nil {
		return fmt.Errorf("create continuity v2 sync schema: %w", err)
	}
	if _, err := tx.Exec(`
INSERT INTO continuity_sync_environment_heads(
  project_id,
  environment_id,
  highest_sequence,
  hlc_wall_millis,
  hlc_logical,
  sealed_sequence
)
SELECT
  fact.project_id,
  fact.environment_id,
  fact.environment_sequence,
  fact.hlc_wall_millis,
  fact.hlc_logical,
  0
FROM continuity_facts AS fact
WHERE NOT EXISTS (
  SELECT 1
  FROM continuity_facts AS later
  WHERE later.project_id = fact.project_id
    AND later.environment_id = fact.environment_id
    AND later.environment_sequence > fact.environment_sequence
)`); err != nil {
		return fmt.Errorf("seed continuity environment heads: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
		return fmt.Errorf("set continuity v2 schema version: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, ?, ?, ?)`,
		schemaLine,
		2,
		checksumSchemaV2(),
	); err != nil {
		return fmt.Errorf("record continuity v2 schema identity: %w", err)
	}
	if err := validateSchemaVersion(tx, 2, checksumSchemaV2(), expectedSchemaV2Objects()); err != nil {
		return fmt.Errorf("validate continuity v2 migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit continuity v2 migration: %w", err)
	}
	return nil
}

func migrateSchemaV2ToV3(db *sql.DB) error {
	advanced, err := validateMigrationPreflight(db, 2)
	if err != nil {
		return fmt.Errorf("validate continuity v2 before migration: %w", err)
	}
	if advanced {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin continuity v3 migration: %w", err)
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("recheck continuity v2 schema version in migration: %w", err)
	}
	if version > 2 && version <= schemaVersion {
		if err := validateKnownSchemaVersion(tx, version); err != nil {
			return fmt.Errorf("validate concurrently advanced continuity schema: %w", err)
		}
		return nil
	}
	if version != 2 {
		return fmt.Errorf("continuity schema changed during v3 migration")
	}
	if err := validateSchemaVersion(tx, 2, checksumSchemaV2(), expectedSchemaV2Objects()); err != nil {
		return fmt.Errorf("revalidate continuity v2 in migration: %w", err)
	}

	var legacyInboxCount int64
	if err := tx.QueryRow(`SELECT COUNT(*) FROM continuity_sync_inbox`).Scan(&legacyInboxCount); err != nil {
		return fmt.Errorf("count continuity v2 inbox rows: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE continuity_sync_inbox RENAME TO continuity_sync_inbox_v2`); err != nil {
		return fmt.Errorf("rename continuity v2 inbox: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE continuity_schema`); err != nil {
		return fmt.Errorf("drop continuity v2 schema identity: %w", err)
	}
	if _, err := tx.Exec(
		schemaTableV3SQL + ";\n" +
			syncInboxTableSQL + ";\n" +
			syncTerminalCandidatesTableSQL + ";\n" +
			syncTerminalCandidateFramesTableSQL + ";\n" +
			syncTerminalCandidatesStagingIndexSQL + ";\n",
	); err != nil {
		return fmt.Errorf("create continuity v3 sync schema: %w", err)
	}
	result, err := tx.Exec(`
INSERT INTO continuity_sync_inbox(
  project_id,
  arrival_sequence,
  envelope_digest,
  frame_kind,
  frame_bytes,
  state
)
SELECT
  project_id,
  arrival_sequence,
  envelope_digest,
  'sealed',
  sealed_envelope,
  state
FROM continuity_sync_inbox_v2
ORDER BY project_id, arrival_sequence`)
	if err != nil {
		return fmt.Errorf("copy continuity v2 inbox bytes: %w", err)
	}
	copiedInboxCount, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count copied continuity v2 inbox rows: %w", err)
	}
	if copiedInboxCount != legacyInboxCount {
		return fmt.Errorf("copied continuity v2 inbox rows = %d, want %d", copiedInboxCount, legacyInboxCount)
	}
	if err := validateV2InboxCopy(tx, legacyInboxCount); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE continuity_sync_inbox_v2`); err != nil {
		return fmt.Errorf("drop continuity v2 inbox: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 3`); err != nil {
		return fmt.Errorf("set continuity v3 schema version: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, ?, ?, ?)`,
		schemaLine,
		3,
		checksumSchemaV3(),
	); err != nil {
		return fmt.Errorf("record continuity v3 schema identity: %w", err)
	}
	if err := validateSchemaVersion(tx, 3, checksumSchemaV3(), expectedSchemaV3Objects()); err != nil {
		return fmt.Errorf("validate continuity v3 migration: %w", err)
	}
	if err := validateForeignKeys(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit continuity v3 migration: %w", err)
	}
	return nil
}

func migrateSchemaV3ToV4(db *sql.DB) error {
	advanced, err := validateMigrationPreflight(db, 3)
	if err != nil {
		return fmt.Errorf("validate continuity v3 before migration: %w", err)
	}
	if advanced {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin continuity v4 migration: %w", err)
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("recheck continuity v3 schema version in migration: %w", err)
	}
	if version > 3 && version <= schemaVersion {
		if err := validateKnownSchemaVersion(tx, version); err != nil {
			return fmt.Errorf("validate concurrently advanced continuity schema: %w", err)
		}
		return nil
	}
	if version != 3 {
		return fmt.Errorf("continuity schema changed during v4 migration")
	}
	if err := validateSchemaVersion(tx, 3, checksumSchemaV3(), expectedSchemaV3Objects()); err != nil {
		return fmt.Errorf("revalidate continuity v3 in migration: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE continuity_schema`); err != nil {
		return fmt.Errorf("drop continuity v3 schema identity: %w", err)
	}
	if _, err := tx.Exec(schemaTableV4SQL + ";\n" + syncAuthorityCandidateSchemaV5DDL); err != nil {
		return fmt.Errorf("create continuity v4 authority candidate schema: %w", err)
	}
	if err := seedV3SyncAuthorityMetadata(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA user_version = 4`); err != nil {
		return fmt.Errorf("set continuity v4 schema version: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, ?, ?, ?)`,
		schemaLine,
		4,
		checksumSchemaV4(),
	); err != nil {
		return fmt.Errorf("record continuity v4 schema identity: %w", err)
	}
	if err := validateSchemaVersion(tx, 4, checksumSchemaV4(), expectedSchemaV4Objects()); err != nil {
		return fmt.Errorf("validate continuity v4 migration: %w", err)
	}
	if err := validateForeignKeys(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit continuity v4 migration: %w", err)
	}
	return nil
}

func migrateSchemaV4ToV5(db *sql.DB) error {
	advanced, err := validateMigrationPreflight(db, 4)
	if err != nil {
		return fmt.Errorf("validate continuity v4 before migration: %w", err)
	}
	if advanced {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin continuity v5 migration: %w", err)
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("recheck continuity v4 schema version in migration: %w", err)
	}
	if version > 4 && version <= schemaVersion {
		if err := validateKnownSchemaVersion(tx, version); err != nil {
			return fmt.Errorf("validate concurrently advanced continuity schema: %w", err)
		}
		return nil
	}
	if version != 4 {
		return fmt.Errorf("continuity schema changed during v5 migration")
	}
	if err := validateSchemaVersion(tx, 4, checksumSchemaV4(), expectedSchemaV4Objects()); err != nil {
		return fmt.Errorf("revalidate continuity v4 in migration: %w", err)
	}
	seeds, err := collectV4SyncRelayWatermarkSeeds(tx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE continuity_schema`); err != nil {
		return fmt.Errorf("drop continuity v4 schema identity: %w", err)
	}
	if _, err := tx.Exec(schemaTableV5SQL + ";\n" + syncRelayWatermarksTableSQL + ";\n"); err != nil {
		return fmt.Errorf("create continuity v5 relay watermark schema: %w", err)
	}
	if err := insertV5SyncRelayWatermarkSeeds(tx, seeds); err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA user_version = 5`); err != nil {
		return fmt.Errorf("set continuity v5 schema version: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, ?, ?, ?)`,
		schemaLine,
		5,
		checksumSchemaV5(),
	); err != nil {
		return fmt.Errorf("record continuity v5 schema identity: %w", err)
	}
	if err := validateSchemaVersion(tx, 5, checksumSchemaV5(), expectedSchemaV5Objects()); err != nil {
		return fmt.Errorf("validate continuity v5 migration: %w", err)
	}
	if err := validateForeignKeys(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit continuity v5 migration: %w", err)
	}
	return nil
}

func migrateSchemaV5ToV6(db *sql.DB) error {
	advanced, err := validateMigrationPreflight(db, 5)
	if err != nil {
		return fmt.Errorf("validate continuity v5 before migration: %w", err)
	}
	if advanced {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin continuity v6 migration: %w", err)
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("recheck continuity v5 schema version in migration: %w", err)
	}
	if version == schemaVersion {
		if err := validateKnownSchemaVersion(tx, version); err != nil {
			return fmt.Errorf("validate concurrently advanced continuity schema: %w", err)
		}
		return nil
	}
	if version != 5 {
		return fmt.Errorf("continuity schema changed during v6 migration")
	}
	if err := validateSchemaVersion(tx, 5, checksumSchemaV5(), expectedSchemaV5Objects()); err != nil {
		return fmt.Errorf("revalidate continuity v5 in migration: %w", err)
	}
	if _, err := tx.Exec(`DROP INDEX ux_continuity_sync_authority_candidates_active_project`); err != nil {
		return fmt.Errorf("drop continuity v5 authority candidate active index: %w", err)
	}
	if _, err := tx.Exec(`
ALTER TABLE continuity_sync_authority_candidates
ADD COLUMN role TEXT NOT NULL DEFAULT 'ordinary' CHECK (
  role IN ('ordinary', 'recovery-predecessor', 'recovery-successor')
) CHECK (
  role = 'ordinary'
  OR (role = 'recovery-predecessor' AND state = 'ready')
  OR (role = 'recovery-successor' AND state IN ('staging', 'ready'))
)`); err != nil {
		return fmt.Errorf("add continuity v6 authority candidate role: %w", err)
	}
	if _, err := tx.Exec(
		syncAuthorityCandidatesActiveIndexSQL + ";\n" +
			syncAuthorityCandidatesRecoveryPredecessorIndexSQL + ";\n" +
			syncAuthorityRecoveryTransitionsTableSQL + ";\n",
	); err != nil {
		return fmt.Errorf("create continuity v6 recovery transition schema: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE continuity_schema`); err != nil {
		return fmt.Errorf("drop continuity v5 schema identity: %w", err)
	}
	if _, err := tx.Exec(schemaTableSQL); err != nil {
		return fmt.Errorf("create continuity v6 schema identity: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 6`); err != nil {
		return fmt.Errorf("set continuity v6 schema version: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, ?, ?, ?)`,
		schemaLine,
		schemaVersion,
		checksumSchema(),
	); err != nil {
		return fmt.Errorf("record continuity v6 schema identity: %w", err)
	}
	if err := validateSchemaVersion(tx, schemaVersion, checksumSchema(), expectedSchemaObjects()); err != nil {
		return fmt.Errorf("validate continuity v6 migration: %w", err)
	}
	if err := validateForeignKeys(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit continuity v6 migration: %w", err)
	}
	return nil
}

func seedV3SyncAuthorityMetadata(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT project_id FROM continuity_sync_projects ORDER BY project_id`)
	if err != nil {
		return fmt.Errorf("list continuity v3 sync authorities: %w", err)
	}
	var projectIDs []continuity.ProjectID
	for rows.Next() {
		var projectID continuity.ProjectID
		if err := rows.Scan(&projectID); err != nil {
			return closeMigrationRows(rows, fmt.Errorf("scan continuity v3 sync project: %w", err))
		}
		projectIDs = append(projectIDs, projectID)
	}
	if err := rows.Err(); err != nil {
		return closeMigrationRows(rows, fmt.Errorf("iterate continuity v3 sync projects: %w", err))
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close continuity v3 sync projects: %w", err)
	}

	for _, projectID := range projectIDs {
		authority, err := readLegacySyncAuthorityV3(context.Background(), tx, projectID)
		if err != nil {
			return fmt.Errorf("read continuity v3 sync authority %q: %w", projectID, err)
		}
		digest, err := frozenSyncAuthorityDigestV1(projectID, authority)
		if err != nil {
			return fmt.Errorf("digest continuity v3 sync authority %q: %w", projectID, err)
		}
		if digest == [32]byte{} {
			return fmt.Errorf("digest continuity v3 sync authority %q: zero digest", projectID)
		}
		if _, err := tx.Exec(`
INSERT INTO continuity_sync_authorities(
  project_id, digest_version, authority_digest, inventory_arrival_head
) VALUES(?, 1, ?, 0)`, string(projectID), digest[:]); err != nil {
			return fmt.Errorf("seed continuity v3 sync authority metadata %q: %w", projectID, err)
		}
	}

	var mismatchedActiveCandidates int64
	if err := tx.QueryRow(`
SELECT COUNT(*)
FROM continuity_sync_terminal_candidates AS candidate
JOIN continuity_sync_authorities AS authority
  ON authority.project_id = candidate.project_id
WHERE candidate.state = 'staging'
  AND candidate.authority_digest <> authority.authority_digest`).Scan(&mismatchedActiveCandidates); err != nil {
		return fmt.Errorf("validate continuity v3 active terminal authority digests: %w", err)
	}
	if mismatchedActiveCandidates != 0 {
		return fmt.Errorf("continuity v3 active terminal candidate authority digest does not match canonical authority")
	}
	return nil
}

func closeMigrationRows(rows *sql.Rows, cause error) error {
	if err := rows.Close(); err != nil {
		return fmt.Errorf("%v; close migration rows: %w", cause, err)
	}
	return cause
}

func validateMigrationPreflight(db *sql.DB, fromVersion int) (bool, error) {
	for {
		var version int
		if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
			return false, fmt.Errorf("read continuity schema version in migration preflight: %w", err)
		}
		if version < fromVersion || version > schemaVersion {
			return false, fmt.Errorf("continuity schema changed from version %d to %d during migration preflight", fromVersion, version)
		}
		validationErr := validateKnownSchemaVersion(db, version)
		if validationErr == nil {
			return version != fromVersion, nil
		}

		var afterVersion int
		if err := db.QueryRow(`PRAGMA user_version`).Scan(&afterVersion); err != nil {
			return false, fmt.Errorf("recheck continuity schema version in migration preflight: %w", err)
		}
		if afterVersion == version {
			return false, validationErr
		}
	}
}

func validateKnownSchemaVersion(db schemaQuerier, version int) error {
	switch version {
	case 1:
		return validateSchemaVersion(db, 1, checksumSchemaV1(), expectedSchemaV1Objects())
	case 2:
		return validateSchemaVersion(db, 2, checksumSchemaV2(), expectedSchemaV2Objects())
	case 3:
		return validateSchemaVersion(db, 3, checksumSchemaV3(), expectedSchemaV3Objects())
	case 4:
		return validateSchemaVersion(db, 4, checksumSchemaV4(), expectedSchemaV4Objects())
	case 5:
		return validateSchemaVersion(db, 5, checksumSchemaV5(), expectedSchemaV5Objects())
	case schemaVersion:
		return validateSchemaVersion(db, schemaVersion, checksumSchema(), expectedSchemaObjects())
	default:
		return fmt.Errorf("continuity schema version = %d is not recognized", version)
	}
}

func validateV2InboxCopy(tx *sql.Tx, legacyInboxCount int64) error {
	var migratedInboxCount, nonSealedCount, candidateCount int64
	if err := tx.QueryRow(`SELECT COUNT(*) FROM continuity_sync_inbox`).Scan(&migratedInboxCount); err != nil {
		return fmt.Errorf("count continuity v3 inbox rows: %w", err)
	}
	if migratedInboxCount != legacyInboxCount {
		return fmt.Errorf("continuity v3 inbox rows = %d, want %d", migratedInboxCount, legacyInboxCount)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM continuity_sync_inbox WHERE frame_kind <> 'sealed'`).Scan(&nonSealedCount); err != nil {
		return fmt.Errorf("inspect migrated continuity inbox tags: %w", err)
	}
	if nonSealedCount != 0 {
		return fmt.Errorf("continuity v3 migration synthesized non-sealed inbox rows")
	}
	if err := tx.QueryRow(`
SELECT
  (SELECT COUNT(*) FROM continuity_sync_terminal_candidates)
  + (SELECT COUNT(*) FROM continuity_sync_terminal_candidate_frames)`).Scan(&candidateCount); err != nil {
		return fmt.Errorf("inspect migrated continuity candidates: %w", err)
	}
	if candidateCount != 0 {
		return fmt.Errorf("continuity v3 migration synthesized terminal candidates")
	}

	queries := []string{
		`SELECT EXISTS (
  SELECT project_id, arrival_sequence, envelope_digest, 'sealed', sealed_envelope, state
  FROM continuity_sync_inbox_v2
  EXCEPT
  SELECT project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
  FROM continuity_sync_inbox
)`,
		`SELECT EXISTS (
  SELECT project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
  FROM continuity_sync_inbox
  EXCEPT
  SELECT project_id, arrival_sequence, envelope_digest, 'sealed', sealed_envelope, state
  FROM continuity_sync_inbox_v2
)`,
	}
	for _, query := range queries {
		var mismatch int
		if err := tx.QueryRow(query).Scan(&mismatch); err != nil {
			return fmt.Errorf("compare continuity v2 and v3 inbox bytes: %w", err)
		}
		if mismatch != 0 {
			return fmt.Errorf("continuity v3 inbox differs from continuity v2")
		}
	}
	return nil
}

func validateForeignKeys(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("validate continuity foreign keys: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("continuity migration violates foreign keys")
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate continuity foreign-key validation: %w", err)
	}
	return nil
}

func validateV1EnvironmentHistory(tx *sql.Tx) error {
	var invalidGroups int
	if err := tx.QueryRow(`
SELECT COUNT(*)
FROM (
  SELECT project_id, environment_id
  FROM continuity_facts
  GROUP BY project_id, environment_id
  HAVING MIN(environment_sequence) <> 1
     OR MAX(environment_sequence) <> COUNT(*)
)`).Scan(&invalidGroups); err != nil {
		return fmt.Errorf("validate continuity v1 environment sequences: %w", err)
	}
	if invalidGroups != 0 {
		return fmt.Errorf("continuity v1 contains noncontiguous environment sequences")
	}

	var invalidClocks int
	if err := tx.QueryRow(`
SELECT COUNT(*)
FROM continuity_facts AS current
JOIN continuity_facts AS next
  ON next.project_id = current.project_id
 AND next.environment_id = current.environment_id
 AND next.environment_sequence = current.environment_sequence + 1
WHERE next.hlc_wall_millis < current.hlc_wall_millis
   OR (
     next.hlc_wall_millis = current.hlc_wall_millis
     AND next.hlc_logical <= current.hlc_logical
   )`).Scan(&invalidClocks); err != nil {
		return fmt.Errorf("validate continuity v1 environment clocks: %w", err)
	}
	if invalidClocks != 0 {
		return fmt.Errorf("continuity v1 contains non-increasing environment clocks")
	}
	return nil
}

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

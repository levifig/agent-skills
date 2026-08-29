package sqlite

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/levifig/loaf/vnext/sync/relay"
)

const (
	applicationID = 1_280_262_482
	schemaLine    = "vnext-relay"
	schemaVersion = 1
)

const relaySchemaTableSQL = `CREATE TABLE relay_schema (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  schema_line TEXT NOT NULL CHECK (schema_line = 'vnext-relay'),
  schema_version INTEGER NOT NULL CHECK (schema_version = 1),
  schema_checksum TEXT NOT NULL CHECK (
    length(schema_checksum) = 64
    AND schema_checksum NOT GLOB '*[^0-9a-f]*'
  )
) STRICT`

const relayInstanceTableSQL = `CREATE TABLE relay_instance (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  relay_generation BLOB NOT NULL UNIQUE CHECK (
    length(relay_generation) = 32
    AND relay_generation <> X'0000000000000000000000000000000000000000000000000000000000000000'
  )
) STRICT`

const relayChannelsTableSQL = `CREATE TABLE relay_channels (
  channel_id BLOB NOT NULL PRIMARY KEY CHECK (
    length(channel_id) = 32
    AND channel_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  admin_public_key BLOB NOT NULL CHECK (
    length(admin_public_key) = 32
    AND admin_public_key <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  owner_token_id BLOB NOT NULL UNIQUE CHECK (
    length(owner_token_id) = 16
    AND owner_token_id <> X'00000000000000000000000000000000'
  ),
  owner_token_hash BLOB NOT NULL CHECK (
    length(owner_token_hash) = 32
    AND owner_token_hash <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  membership_generation INTEGER NOT NULL DEFAULT 0 CHECK (
    membership_generation BETWEEN 0 AND 4294967295
  ),
  created_at_millis INTEGER NOT NULL CHECK (created_at_millis >= 0)
) STRICT, WITHOUT ROWID`

const relayEnvironmentsTableSQL = `CREATE TABLE relay_environments (
  channel_id BLOB NOT NULL CHECK (length(channel_id) = 32),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32
    AND certificate_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  certificate_bytes BLOB NOT NULL CHECK (length(certificate_bytes) BETWEEN 1 AND 8192),
  mode TEXT NOT NULL CHECK (mode IN ('trusted', 'ephemeral')),
  expires_at_millis INTEGER NOT NULL CHECK (
    (mode = 'trusted' AND expires_at_millis = 0)
    OR (mode = 'ephemeral' AND expires_at_millis > 0)
  ),
  membership_generation INTEGER NOT NULL CHECK (
    membership_generation BETWEEN 1 AND 4294967295
  ),
  token_id BLOB NOT NULL UNIQUE CHECK (
    length(token_id) = 16
    AND token_id <> X'00000000000000000000000000000000'
  ),
  token_hash BLOB NOT NULL CHECK (
    length(token_hash) = 32
    AND token_hash <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  token_expires_at_millis INTEGER NOT NULL CHECK (
    (mode = 'trusted' AND token_expires_at_millis = 0)
    OR (mode = 'ephemeral' AND token_expires_at_millis > 0 AND token_expires_at_millis <= expires_at_millis)
  ),
  retired_at_millis INTEGER CHECK (retired_at_millis >= 0),
  retirement_generation INTEGER CHECK (retirement_generation BETWEEN 1 AND 4294967295),
  retirement_relay_generation BLOB CHECK (
    length(retirement_relay_generation) = 32
    AND retirement_relay_generation <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  retirement_certificate_id BLOB CHECK (
    length(retirement_certificate_id) = 32
    AND retirement_certificate_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  retirement_final_environment_sequence INTEGER CHECK (retirement_final_environment_sequence >= 0),
  retirement_final_envelope_digest BLOB CHECK (
    length(retirement_final_envelope_digest) = 32
    AND (
      (retirement_final_environment_sequence = 0 AND retirement_final_envelope_digest = X'0000000000000000000000000000000000000000000000000000000000000000')
      OR (retirement_final_environment_sequence > 0 AND retirement_final_envelope_digest <> X'0000000000000000000000000000000000000000000000000000000000000000')
    )
  ),
  retirement_id BLOB CHECK (
    length(retirement_id) = 32
    AND retirement_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  retirement_bytes BLOB CHECK (length(retirement_bytes) BETWEEN 1 AND 4096),
  created_at_millis INTEGER NOT NULL CHECK (created_at_millis >= 0),
  PRIMARY KEY (channel_id, environment_id),
  UNIQUE (channel_id, certificate_id),
  UNIQUE (channel_id, environment_id, certificate_id),
  FOREIGN KEY (channel_id) REFERENCES relay_channels(channel_id) ON DELETE RESTRICT,
  CHECK (
    (retired_at_millis IS NULL AND retirement_generation IS NULL AND retirement_relay_generation IS NULL
      AND retirement_certificate_id IS NULL AND retirement_final_environment_sequence IS NULL
      AND retirement_final_envelope_digest IS NULL AND retirement_id IS NULL AND retirement_bytes IS NULL)
    OR (retired_at_millis IS NOT NULL AND retirement_generation IS NOT NULL AND retirement_relay_generation IS NOT NULL
      AND retirement_certificate_id IS NOT NULL AND retirement_final_environment_sequence IS NOT NULL
      AND retirement_final_envelope_digest IS NOT NULL AND retirement_id IS NOT NULL AND retirement_bytes IS NOT NULL)
  )
) STRICT, WITHOUT ROWID`

const relayPruneCertificatesTableSQL = `CREATE TABLE relay_prune_certificates (
  channel_id BLOB NOT NULL CHECK (length(channel_id) = 32),
  prune_sequence INTEGER NOT NULL CHECK (prune_sequence > 0),
  prune_id BLOB NOT NULL CHECK (
    length(prune_id) = 32
    AND prune_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  membership_generation INTEGER NOT NULL CHECK (
    membership_generation BETWEEN 1 AND 4294967295
  ),
  barrier_arrival_sequence INTEGER NOT NULL CHECK (barrier_arrival_sequence > 0),
  closure_fact_id TEXT NOT NULL CHECK (
    length(closure_fact_id) BETWEEN 1 AND 128
    AND closure_fact_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  closure_environment_id TEXT NOT NULL CHECK (
    length(closure_environment_id) BETWEEN 1 AND 128
    AND closure_environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  closure_environment_sequence INTEGER NOT NULL CHECK (closure_environment_sequence > 0),
  closure_arrival_sequence INTEGER NOT NULL CHECK (
    closure_arrival_sequence > 0
    AND closure_arrival_sequence <= barrier_arrival_sequence
  ),
  closure_envelope_digest BLOB NOT NULL CHECK (
    length(closure_envelope_digest) = 32
    AND closure_envelope_digest <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  closure_certificate_id BLOB NOT NULL CHECK (
    length(closure_certificate_id) = 32
    AND closure_certificate_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32
    AND certificate_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  certificate_bytes BLOB NOT NULL CHECK (length(certificate_bytes) BETWEEN 1 AND 1048576),
  target_count INTEGER NOT NULL CHECK (target_count BETWEEN 1 AND 4096),
  created_at_millis INTEGER NOT NULL CHECK (created_at_millis >= 0),
  PRIMARY KEY (channel_id, prune_id),
  UNIQUE (channel_id, prune_sequence),
  UNIQUE (channel_id, certificate_id),
  FOREIGN KEY (channel_id) REFERENCES relay_channels(channel_id) ON DELETE RESTRICT,
  FOREIGN KEY (channel_id, closure_arrival_sequence)
    REFERENCES relay_arrivals(channel_id, arrival_sequence) ON DELETE RESTRICT
) STRICT, WITHOUT ROWID`

const relayArrivalsTableSQL = `CREATE TABLE relay_arrivals (
  channel_id BLOB NOT NULL CHECK (
    length(channel_id) = 32
    AND channel_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
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
  protocol_version INTEGER NOT NULL CHECK (protocol_version = 1),
  cipher_suite INTEGER NOT NULL CHECK (cipher_suite = 1),
  key_generation INTEGER NOT NULL CHECK (key_generation BETWEEN 1 AND 4294967295),
  previous_envelope_digest BLOB NOT NULL CHECK (
    length(previous_envelope_digest) = 32
    AND (
      (environment_sequence = 1 AND previous_envelope_digest = X'0000000000000000000000000000000000000000000000000000000000000000')
      OR (environment_sequence > 1 AND previous_envelope_digest <> X'0000000000000000000000000000000000000000000000000000000000000000')
    )
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32
    AND certificate_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  nonce BLOB NOT NULL CHECK (length(nonce) = 24),
  envelope_digest BLOB NOT NULL CHECK (
    length(envelope_digest) = 32
    AND envelope_digest <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  ciphertext BLOB,
  ciphertext_size INTEGER NOT NULL CHECK (ciphertext_size BETWEEN 16 AND 1100016),
  signature BLOB NOT NULL CHECK (
    length(signature) = 64
    AND signature <> X'00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000'
  ),
  arrived_at_millis INTEGER NOT NULL CHECK (arrived_at_millis >= 0),
  prune_id BLOB CHECK (length(prune_id) = 32),
  pruned_at_millis INTEGER CHECK (pruned_at_millis >= 0),
  PRIMARY KEY (channel_id, arrival_sequence),
  UNIQUE (channel_id, fact_id),
  UNIQUE (channel_id, environment_id, environment_sequence),
  UNIQUE (channel_id, envelope_digest),
  UNIQUE (channel_id, key_generation, nonce),
  FOREIGN KEY (channel_id, environment_id, certificate_id)
    REFERENCES relay_environments(channel_id, environment_id, certificate_id) ON DELETE RESTRICT,
  FOREIGN KEY (channel_id, prune_id)
    REFERENCES relay_prune_certificates(channel_id, prune_id) ON DELETE RESTRICT,
  CHECK (
    (ciphertext IS NOT NULL AND length(ciphertext) = ciphertext_size AND prune_id IS NULL AND pruned_at_millis IS NULL)
    OR (ciphertext IS NULL AND prune_id IS NOT NULL AND pruned_at_millis IS NOT NULL)
  )
) STRICT, WITHOUT ROWID`

const relayAcknowledgementsTableSQL = `CREATE TABLE relay_acknowledgements (
  channel_id BLOB NOT NULL CHECK (
    length(channel_id) = 32
    AND channel_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  membership_generation INTEGER NOT NULL CHECK (
    membership_generation BETWEEN 1 AND 4294967295
  ),
  applied_arrival_sequence INTEGER NOT NULL CHECK (applied_arrival_sequence >= 0),
  producer_sequence INTEGER NOT NULL CHECK (producer_sequence >= 0),
  producer_envelope_digest BLOB NOT NULL CHECK (
    length(producer_envelope_digest) = 32
    AND (
      (producer_sequence = 0 AND producer_envelope_digest = X'0000000000000000000000000000000000000000000000000000000000000000')
      OR (producer_sequence > 0 AND producer_envelope_digest <> X'0000000000000000000000000000000000000000000000000000000000000000')
    )
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32
    AND certificate_id <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  acknowledgement_digest BLOB NOT NULL CHECK (
    length(acknowledgement_digest) = 32
    AND acknowledgement_digest <> X'0000000000000000000000000000000000000000000000000000000000000000'
  ),
  acknowledgement_bytes BLOB NOT NULL CHECK (length(acknowledgement_bytes) BETWEEN 1 AND 4096),
  acknowledged_at_millis INTEGER NOT NULL CHECK (acknowledged_at_millis >= 0),
  PRIMARY KEY (channel_id, environment_id),
  FOREIGN KEY (channel_id, environment_id, certificate_id)
    REFERENCES relay_environments(channel_id, environment_id, certificate_id) ON DELETE RESTRICT
) STRICT, WITHOUT ROWID`

const relayChannelsImmutableTriggerSQL = `CREATE TRIGGER relay_channels_immutable_identity
BEFORE UPDATE OF channel_id, admin_public_key, owner_token_id, owner_token_hash, created_at_millis
ON relay_channels
BEGIN
  SELECT RAISE(ABORT, 'relay channel identity is immutable');
END`

const relayInstanceUpdateTriggerSQL = `CREATE TRIGGER relay_instance_no_update
BEFORE UPDATE ON relay_instance
BEGIN
  SELECT RAISE(ABORT, 'relay generation is immutable');
END`

const relayInstanceDeleteTriggerSQL = `CREATE TRIGGER relay_instance_no_delete
BEFORE DELETE ON relay_instance
BEGIN
  SELECT RAISE(ABORT, 'relay generation is durable');
END`

const relayChannelsMembershipTriggerSQL = `CREATE TRIGGER relay_channels_membership_monotonic
BEFORE UPDATE OF membership_generation ON relay_channels
WHEN NEW.membership_generation <> OLD.membership_generation + 1
BEGIN
  SELECT RAISE(ABORT, 'relay membership generation must advance by one');
END`

const relayChannelsDeleteTriggerSQL = `CREATE TRIGGER relay_channels_no_delete
BEFORE DELETE ON relay_channels
BEGIN
  SELECT RAISE(ABORT, 'relay channels are immutable');
END`

const relayEnvironmentsImmutableTriggerSQL = `CREATE TRIGGER relay_environments_immutable_identity
BEFORE UPDATE OF channel_id, environment_id, certificate_id, certificate_bytes, mode, expires_at_millis, membership_generation, token_id, token_hash, token_expires_at_millis, created_at_millis
ON relay_environments
BEGIN
  SELECT RAISE(ABORT, 'relay environment identity is immutable');
END`

const relayEnvironmentsRetirementTriggerSQL = `CREATE TRIGGER relay_environments_terminal_retirement
BEFORE UPDATE OF retired_at_millis, retirement_generation, retirement_relay_generation, retirement_certificate_id, retirement_final_environment_sequence, retirement_final_envelope_digest, retirement_id, retirement_bytes ON relay_environments
WHEN OLD.retired_at_millis IS NOT NULL
  OR NEW.retired_at_millis IS NULL
  OR NEW.retirement_generation IS NULL
  OR NEW.retirement_relay_generation IS NULL
  OR NEW.retirement_certificate_id IS NULL
  OR NEW.retirement_final_environment_sequence IS NULL
  OR NEW.retirement_final_envelope_digest IS NULL
  OR NEW.retirement_id IS NULL
  OR NEW.retirement_bytes IS NULL
BEGIN
  SELECT RAISE(ABORT, 'relay environment retirement is terminal');
END`

const relayEnvironmentsDeleteTriggerSQL = `CREATE TRIGGER relay_environments_no_delete
BEFORE DELETE ON relay_environments
BEGIN
  SELECT RAISE(ABORT, 'relay environments are immutable');
END`

const relayArrivalsContiguousTriggerSQL = `CREATE TRIGGER relay_arrivals_contiguous
BEFORE INSERT ON relay_arrivals
WHEN NEW.arrival_sequence <> 1 + COALESCE((
    SELECT MAX(arrival_sequence) FROM relay_arrivals WHERE channel_id = NEW.channel_id
  ), 0)
  OR NEW.environment_sequence <> 1 + COALESCE((
    SELECT MAX(environment_sequence)
    FROM relay_arrivals
    WHERE channel_id = NEW.channel_id AND environment_id = NEW.environment_id
  ), 0)
BEGIN
  SELECT RAISE(ABORT, 'relay arrival sequence must be contiguous');
END`

const relayArrivalsImmutableTriggerSQL = `CREATE TRIGGER relay_arrivals_immutable_identity
BEFORE UPDATE OF channel_id, arrival_sequence, fact_id, environment_id, environment_sequence, protocol_version, cipher_suite, key_generation, previous_envelope_digest, certificate_id, nonce, envelope_digest, ciphertext_size, signature, arrived_at_millis
ON relay_arrivals
BEGIN
  SELECT RAISE(ABORT, 'relay arrival identity is immutable');
END`

const relayArrivalsTombstoneTriggerSQL = `CREATE TRIGGER relay_arrivals_tombstone_once
BEFORE UPDATE OF ciphertext, prune_id, pruned_at_millis ON relay_arrivals
WHEN OLD.ciphertext IS NULL
  OR NEW.ciphertext IS NOT NULL
  OR OLD.prune_id IS NOT NULL
  OR NEW.prune_id IS NULL
  OR OLD.pruned_at_millis IS NOT NULL
  OR NEW.pruned_at_millis IS NULL
  OR EXISTS (
    SELECT 1
    FROM relay_prune_certificates
    WHERE channel_id = OLD.channel_id
      AND closure_arrival_sequence = OLD.arrival_sequence
  )
BEGIN
  SELECT RAISE(ABORT, 'relay arrival tombstone transition is invalid');
END`

const relayArrivalsDeleteTriggerSQL = `CREATE TRIGGER relay_arrivals_no_delete
BEFORE DELETE ON relay_arrivals
BEGIN
  SELECT RAISE(ABORT, 'relay arrivals are append-only');
END`

const relayAcknowledgementsMonotonicTriggerSQL = `CREATE TRIGGER relay_acknowledgements_monotonic
BEFORE UPDATE ON relay_acknowledgements
WHEN NEW.channel_id <> OLD.channel_id
  OR NEW.environment_id <> OLD.environment_id
  OR NEW.membership_generation < OLD.membership_generation
  OR NEW.applied_arrival_sequence < OLD.applied_arrival_sequence
  OR NEW.producer_sequence < OLD.producer_sequence
  OR (NEW.producer_sequence = OLD.producer_sequence
    AND NEW.producer_envelope_digest <> OLD.producer_envelope_digest)
BEGIN
  SELECT RAISE(ABORT, 'relay acknowledgement rollback refused');
END`

const relayAcknowledgementsDeleteTriggerSQL = `CREATE TRIGGER relay_acknowledgements_no_delete
BEFORE DELETE ON relay_acknowledgements
BEGIN
  SELECT RAISE(ABORT, 'relay acknowledgements are durable');
END`

const relayPruneCertificatesImmutableTriggerSQL = `CREATE TRIGGER relay_prune_certificates_immutable
BEFORE UPDATE ON relay_prune_certificates
BEGIN
  SELECT RAISE(ABORT, 'relay prune certificates are immutable');
END`

const relayPruneCertificatesContiguousTriggerSQL = `CREATE TRIGGER relay_prune_certificates_contiguous
BEFORE INSERT ON relay_prune_certificates
WHEN NEW.prune_sequence <> 1 + COALESCE((
  SELECT MAX(prune_sequence)
  FROM relay_prune_certificates
  WHERE channel_id = NEW.channel_id
), 0)
BEGIN
  SELECT RAISE(ABORT, 'relay prune sequence must be contiguous');
END`

const relayPruneCertificatesDeleteTriggerSQL = `CREATE TRIGGER relay_prune_certificates_no_delete
BEFORE DELETE ON relay_prune_certificates
BEGIN
  SELECT RAISE(ABORT, 'relay prune certificates are durable');
END`

const relayArrivalsSourceIndexSQL = `CREATE INDEX ix_relay_arrivals_source
ON relay_arrivals(channel_id, environment_id, environment_sequence)`

const schemaDDL = relaySchemaTableSQL + ";\n" +
	relayInstanceTableSQL + ";\n" +
	relayChannelsTableSQL + ";\n" +
	relayEnvironmentsTableSQL + ";\n" +
	relayPruneCertificatesTableSQL + ";\n" +
	relayArrivalsTableSQL + ";\n" +
	relayAcknowledgementsTableSQL + ";\n" +
	relayArrivalsSourceIndexSQL + ";\n" +
	relayInstanceUpdateTriggerSQL + ";\n" +
	relayInstanceDeleteTriggerSQL + ";\n" +
	relayChannelsImmutableTriggerSQL + ";\n" +
	relayChannelsMembershipTriggerSQL + ";\n" +
	relayChannelsDeleteTriggerSQL + ";\n" +
	relayEnvironmentsImmutableTriggerSQL + ";\n" +
	relayEnvironmentsRetirementTriggerSQL + ";\n" +
	relayEnvironmentsDeleteTriggerSQL + ";\n" +
	relayArrivalsContiguousTriggerSQL + ";\n" +
	relayArrivalsImmutableTriggerSQL + ";\n" +
	relayArrivalsTombstoneTriggerSQL + ";\n" +
	relayArrivalsDeleteTriggerSQL + ";\n" +
	relayAcknowledgementsMonotonicTriggerSQL + ";\n" +
	relayAcknowledgementsDeleteTriggerSQL + ";\n" +
	relayPruneCertificatesContiguousTriggerSQL + ";\n" +
	relayPruneCertificatesImmutableTriggerSQL + ";\n" +
	relayPruneCertificatesDeleteTriggerSQL + ";\n"

type schemaObject struct {
	kind  string
	name  string
	table string
	sql   string
}

func expectedSchemaObjects() []schemaObject {
	objects := []schemaObject{
		{kind: "table", name: "relay_schema", table: "relay_schema", sql: relaySchemaTableSQL},
		{kind: "table", name: "relay_instance", table: "relay_instance", sql: relayInstanceTableSQL},
		{kind: "table", name: "relay_channels", table: "relay_channels", sql: relayChannelsTableSQL},
		{kind: "table", name: "relay_environments", table: "relay_environments", sql: relayEnvironmentsTableSQL},
		{kind: "table", name: "relay_prune_certificates", table: "relay_prune_certificates", sql: relayPruneCertificatesTableSQL},
		{kind: "table", name: "relay_arrivals", table: "relay_arrivals", sql: relayArrivalsTableSQL},
		{kind: "table", name: "relay_acknowledgements", table: "relay_acknowledgements", sql: relayAcknowledgementsTableSQL},
		{kind: "index", name: "ix_relay_arrivals_source", table: "relay_arrivals", sql: relayArrivalsSourceIndexSQL},
		{kind: "trigger", name: "relay_instance_no_update", table: "relay_instance", sql: relayInstanceUpdateTriggerSQL},
		{kind: "trigger", name: "relay_instance_no_delete", table: "relay_instance", sql: relayInstanceDeleteTriggerSQL},
		{kind: "trigger", name: "relay_channels_immutable_identity", table: "relay_channels", sql: relayChannelsImmutableTriggerSQL},
		{kind: "trigger", name: "relay_channels_membership_monotonic", table: "relay_channels", sql: relayChannelsMembershipTriggerSQL},
		{kind: "trigger", name: "relay_channels_no_delete", table: "relay_channels", sql: relayChannelsDeleteTriggerSQL},
		{kind: "trigger", name: "relay_environments_immutable_identity", table: "relay_environments", sql: relayEnvironmentsImmutableTriggerSQL},
		{kind: "trigger", name: "relay_environments_terminal_retirement", table: "relay_environments", sql: relayEnvironmentsRetirementTriggerSQL},
		{kind: "trigger", name: "relay_environments_no_delete", table: "relay_environments", sql: relayEnvironmentsDeleteTriggerSQL},
		{kind: "trigger", name: "relay_arrivals_contiguous", table: "relay_arrivals", sql: relayArrivalsContiguousTriggerSQL},
		{kind: "trigger", name: "relay_arrivals_immutable_identity", table: "relay_arrivals", sql: relayArrivalsImmutableTriggerSQL},
		{kind: "trigger", name: "relay_arrivals_tombstone_once", table: "relay_arrivals", sql: relayArrivalsTombstoneTriggerSQL},
		{kind: "trigger", name: "relay_arrivals_no_delete", table: "relay_arrivals", sql: relayArrivalsDeleteTriggerSQL},
		{kind: "trigger", name: "relay_acknowledgements_monotonic", table: "relay_acknowledgements", sql: relayAcknowledgementsMonotonicTriggerSQL},
		{kind: "trigger", name: "relay_acknowledgements_no_delete", table: "relay_acknowledgements", sql: relayAcknowledgementsDeleteTriggerSQL},
		{kind: "trigger", name: "relay_prune_certificates_contiguous", table: "relay_prune_certificates", sql: relayPruneCertificatesContiguousTriggerSQL},
		{kind: "trigger", name: "relay_prune_certificates_immutable", table: "relay_prune_certificates", sql: relayPruneCertificatesImmutableTriggerSQL},
		{kind: "trigger", name: "relay_prune_certificates_no_delete", table: "relay_prune_certificates", sql: relayPruneCertificatesDeleteTriggerSQL},
	}
	sortSchemaObjects(objects)
	return objects
}

func sortSchemaObjects(objects []schemaObject) {
	for index := 1; index < len(objects); index++ {
		for position := index; position > 0; position-- {
			left := objects[position-1]
			right := objects[position]
			if left.kind < right.kind || (left.kind == right.kind && left.name <= right.name) {
				break
			}
			objects[position-1], objects[position] = right, left
		}
	}
}

func schemaChecksum() string {
	digest := sha256.Sum256([]byte(schemaDDL))
	return hex.EncodeToString(digest[:])
}

func initializeSchemaIfEmpty(db *sql.DB) (returnErr error) {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin relay schema transaction: %w", err)
	}
	rollback := true
	defer func() {
		if rollback {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				returnErr = errors.Join(returnErr, fmt.Errorf("rollback relay schema transaction: %w", err))
			}
		}
	}()

	var objectCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&objectCount); err != nil {
		return fmt.Errorf("inspect relay schema inventory: %w", err)
	}
	if objectCount != 0 {
		return nil
	}
	var existingApplicationID, existingUserVersion int
	if err := tx.QueryRow(`PRAGMA application_id`).Scan(&existingApplicationID); err != nil {
		return fmt.Errorf("read empty relay application id: %w", err)
	}
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&existingUserVersion); err != nil {
		return fmt.Errorf("read empty relay schema version: %w", err)
	}
	if existingApplicationID != 0 || existingUserVersion != 0 {
		return fmt.Errorf("empty relay database carries foreign schema identity")
	}
	if _, err := tx.Exec(schemaDDL); err != nil {
		return fmt.Errorf("create relay schema: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA application_id = 1280262482`); err != nil {
		return fmt.Errorf("set relay application id: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 1`); err != nil {
		return fmt.Errorf("set relay schema version: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO relay_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, ?, ?, ?)`,
		schemaLine,
		schemaVersion,
		schemaChecksum(),
	); err != nil {
		return fmt.Errorf("record relay schema identity: %w", err)
	}
	var generation relay.RelayGeneration
	for generation == (relay.RelayGeneration{}) {
		if _, err := rand.Read(generation[:]); err != nil {
			return fmt.Errorf("generate relay database incarnation: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO relay_instance(singleton, relay_generation) VALUES(1, ?)`, generation[:]); err != nil {
		return fmt.Errorf("record relay database incarnation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit relay schema: %w", err)
	}
	rollback = false
	return nil
}

func validateSchema(db *sql.DB) error {
	var gotApplicationID, gotVersion int
	if err := db.QueryRow(`PRAGMA application_id`).Scan(&gotApplicationID); err != nil {
		return fmt.Errorf("read relay application id: %w", err)
	}
	if gotApplicationID != applicationID {
		return fmt.Errorf("relay application id = %d, want %d", gotApplicationID, applicationID)
	}
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&gotVersion); err != nil {
		return fmt.Errorf("read relay schema version: %w", err)
	}
	if gotVersion != schemaVersion {
		return fmt.Errorf("relay schema version = %d, want %d", gotVersion, schemaVersion)
	}

	var gotLine, gotChecksum string
	var gotRowVersion int
	if err := db.QueryRow(`SELECT schema_line, schema_version, schema_checksum FROM relay_schema WHERE singleton = 1`).Scan(&gotLine, &gotRowVersion, &gotChecksum); err != nil {
		return fmt.Errorf("read relay schema identity: %w", err)
	}
	if gotLine != schemaLine || gotRowVersion != schemaVersion || gotChecksum != schemaChecksum() {
		return fmt.Errorf("relay schema identity does not match this build")
	}

	if _, err := readRelayGeneration(db); err != nil {
		return err
	}

	rows, err := db.Query(`
SELECT type, name, tbl_name, sql
FROM sqlite_schema
WHERE name NOT LIKE 'sqlite_%'
ORDER BY type, name`)
	if err != nil {
		return fmt.Errorf("inspect relay schema: %w", err)
	}
	defer rows.Close()
	want := expectedSchemaObjects()
	index := 0
	for rows.Next() {
		var got schemaObject
		if err := rows.Scan(&got.kind, &got.name, &got.table, &got.sql); err != nil {
			return fmt.Errorf("scan relay schema object: %w", err)
		}
		if index >= len(want) {
			return fmt.Errorf("unexpected relay schema object %s %s", got.kind, got.name)
		}
		expected := want[index]
		if got.kind != expected.kind || got.name != expected.name || got.table != expected.table || normalizeSQL(got.sql) != normalizeSQL(expected.sql) {
			return fmt.Errorf("relay schema object %s %s differs from this build", got.kind, got.name)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate relay schema objects: %w", err)
	}
	if index != len(want) {
		return fmt.Errorf("relay schema has %d recognized objects, want %d", index, len(want))
	}
	return nil
}

func readRelayGeneration(db *sql.DB) (relay.RelayGeneration, error) {
	var generation relay.RelayGeneration
	var encoded []byte
	if err := db.QueryRow(`SELECT relay_generation FROM relay_instance WHERE singleton = 1`).Scan(&encoded); err != nil {
		return relay.RelayGeneration{}, fmt.Errorf("read relay database incarnation: %w", err)
	}
	if len(encoded) != len(generation) {
		return relay.RelayGeneration{}, fmt.Errorf("relay database incarnation has invalid width")
	}
	copy(generation[:], encoded)
	if generation == (relay.RelayGeneration{}) {
		return relay.RelayGeneration{}, fmt.Errorf("relay database incarnation is zero")
	}
	return generation, nil
}

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

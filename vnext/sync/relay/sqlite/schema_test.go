package sqlite

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestRelaySchemaInventoryIsExactAndOpaqueOnly(t *testing.T) {
	t.Parallel()

	store, owner, environment := newTestStoreWithEnvironment(t, relay.TrustedEnvironment, 0)

	wantColumns := map[string][]string{
		"relay_acknowledgements": {
			"channel_id", "environment_id", "membership_generation", "applied_arrival_sequence",
			"producer_sequence", "producer_envelope_digest", "certificate_id", "acknowledgement_digest", "acknowledgement_bytes",
			"acknowledged_at_millis",
		},
		"relay_arrivals": {
			"channel_id", "arrival_sequence", "fact_id", "environment_id", "environment_sequence",
			"protocol_version", "cipher_suite", "key_generation", "previous_envelope_digest",
			"certificate_id", "nonce", "envelope_digest", "ciphertext", "ciphertext_size", "signature",
			"arrived_at_millis", "prune_id", "pruned_at_millis",
		},
		"relay_channels": {
			"channel_id", "admin_public_key", "owner_token_id", "owner_token_hash",
			"membership_generation", "created_at_millis",
		},
		"relay_environments": {
			"channel_id", "environment_id", "certificate_id", "certificate_bytes", "mode", "expires_at_millis",
			"membership_generation", "token_id", "token_hash", "token_expires_at_millis", "retired_at_millis", "retirement_generation",
			"retirement_relay_generation", "retirement_certificate_id", "retirement_final_environment_sequence",
			"retirement_final_envelope_digest", "retirement_id", "retirement_bytes", "created_at_millis",
		},
		"relay_prune_certificates": {
			"channel_id", "prune_sequence", "prune_id", "membership_generation", "barrier_arrival_sequence",
			"closure_fact_id", "closure_environment_id", "closure_environment_sequence", "closure_arrival_sequence",
			"closure_envelope_digest", "closure_certificate_id", "certificate_id", "certificate_bytes", "target_count", "created_at_millis",
		},
		"relay_schema":   {"singleton", "schema_line", "schema_version", "schema_checksum"},
		"relay_instance": {"singleton", "relay_generation"},
	}

	rows, err := store.db.Query(`
SELECT name, sql
FROM sqlite_schema
WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
ORDER BY name`)
	if err != nil {
		t.Fatalf("query schema tables: %v", err)
	}
	type tableDefinition struct {
		name string
		sql  string
	}
	definitions := make([]tableDefinition, 0, len(wantColumns))
	for rows.Next() {
		var table, definition string
		if err := rows.Scan(&table, &definition); err != nil {
			t.Fatalf("scan schema table: %v", err)
		}
		definitions = append(definitions, tableDefinition{name: table, sql: definition})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema tables: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close schema tables: %v", err)
	}

	gotTables := make([]string, 0, len(wantColumns))
	for _, definition := range definitions {
		gotTables = append(gotTables, definition.name)
		assertNoSemanticOrSecretSchemaVocabulary(t, definition.name+" "+definition.sql)
		gotColumns := schemaColumnNames(t, store, definition.name)
		want, ok := wantColumns[definition.name]
		if !ok {
			t.Fatalf("unexpected schema table %q", definition.name)
		}
		if fmt.Sprint(gotColumns) != fmt.Sprint(want) {
			t.Fatalf("columns for %s = %v, want %v", definition.name, gotColumns, want)
		}
	}
	wantTables := make([]string, 0, len(wantColumns))
	for table := range wantColumns {
		wantTables = append(wantTables, table)
	}
	sort.Strings(wantTables)
	if fmt.Sprint(gotTables) != fmt.Sprint(wantTables) {
		t.Fatalf("schema tables = %v, want %v", gotTables, wantTables)
	}
	assertExactSchemaObjectInventory(t, store)

	var ownerHash, environmentHash []byte
	if err := store.db.QueryRow(`
SELECT c.owner_token_hash, e.token_hash
FROM relay_channels AS c
JOIN relay_environments AS e ON e.channel_id = c.channel_id
WHERE c.channel_id = ? AND e.environment_id = ?`, owner.ChannelID[:], string(environment.EnvironmentID)).Scan(&ownerHash, &environmentHash); err != nil {
		t.Fatalf("read persisted token hashes: %v", err)
	}
	if bytes.Equal(ownerHash, owner.TokenSecret[:]) || bytes.Equal(environmentHash, environment.TokenSecret[:]) {
		t.Fatal("relay persisted a plaintext token secret instead of its hash")
	}
}

func TestRelaySchemaEnforcesKeyGenerationAndAppendOnlyIdentity(t *testing.T) {
	t.Parallel()

	store, owner, environment := newTestStoreWithEnvironment(t, relay.TrustedEnvironment, 0)
	envelope := testEnvelope(environment, "fact-zero-generation", 1, relay.Digest{}, 0xc1)
	if _, err := store.db.Exec(`
INSERT INTO relay_arrivals(
  channel_id, arrival_sequence, fact_id, environment_id, environment_sequence,
  protocol_version, cipher_suite, key_generation, previous_envelope_digest,
  certificate_id, nonce, envelope_digest, ciphertext, ciphertext_size,
  signature, arrived_at_millis, prune_id, pruned_at_millis
) VALUES(?, 1, ?, ?, 1, 1, 1, 0, ?, ?, ?, ?, ?, ?, ?, 1, NULL, NULL)`,
		owner.ChannelID[:],
		string(envelope.FactID),
		string(environment.EnvironmentID),
		envelope.PreviousEnvelopeDigest[:],
		envelope.CertificateID[:],
		envelope.Nonce[:],
		envelope.EnvelopeDigest[:],
		envelope.Ciphertext,
		len(envelope.Ciphertext),
		envelope.Signature[:],
	); err == nil {
		t.Fatal("raw insert with key_generation = 0 succeeded")
	}
	accepted := testEnvelope(environment, "fact-a", 1, relay.Digest{}, 0xc2)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: accepted}); err != nil {
		t.Fatalf("Append(valid) error = %v", err)
	}
	zeroDigest := relay.Digest{}
	invalidAcknowledgementDigest := testDigest(0xc4)
	if _, err := store.db.Exec(`
INSERT INTO relay_acknowledgements(
  channel_id, environment_id, membership_generation, applied_arrival_sequence,
  producer_sequence, producer_envelope_digest, certificate_id,
  acknowledgement_digest, acknowledgement_bytes, acknowledged_at_millis
) VALUES(?, ?, 1, 1, 1, ?, ?, ?, ?, 1)`,
		owner.ChannelID[:], string(environment.EnvironmentID), zeroDigest[:], environment.CertificateID[:],
		invalidAcknowledgementDigest[:], []byte("opaque-signed-acknowledgement"),
	); err == nil {
		t.Fatal("raw acknowledgement with nonzero producer and zero envelope digest succeeded")
	}
	if err := store.Acknowledge(t.Context(), relay.AcknowledgeRequest{
		Authorization: environment,
		Acknowledgement: relay.Acknowledgement{
			ChannelID:              owner.ChannelID,
			EnvironmentID:          environment.EnvironmentID,
			MembershipGeneration:   1,
			AppliedArrivalSequence: 1,
			ProducerSequence:       1,
			ProducerEnvelopeDigest: accepted.EnvelopeDigest,
			CertificateID:          environment.CertificateID,
			AcknowledgementDigest:  testDigest(0xc5),
			AcknowledgementBytes:   []byte("opaque-signed-acknowledgement"),
		},
	}); err != nil {
		t.Fatalf("Acknowledge(valid) error = %v", err)
	}
	replacementDigest := testDigest(0xc6)
	if _, err := store.db.Exec(`
UPDATE relay_acknowledgements
SET producer_envelope_digest = ?
WHERE channel_id = ? AND environment_id = ?`, replacementDigest[:], owner.ChannelID[:], string(environment.EnvironmentID)); err == nil {
		t.Fatal("raw acknowledgement producer digest replacement at the same sequence succeeded")
	}
	if _, err := store.db.Exec(`DELETE FROM relay_arrivals WHERE channel_id = ?`, owner.ChannelID[:]); err == nil {
		t.Fatal("raw delete of append-only arrival succeeded")
	}
	replacementGeneration := testRelayGeneration(0xc3)
	if _, err := store.db.Exec(`UPDATE relay_instance SET relay_generation = ? WHERE singleton = 1`, replacementGeneration[:]); err == nil {
		t.Fatal("raw relay generation mutation succeeded")
	}
	if _, err := store.db.Exec(`UPDATE relay_arrivals SET ciphertext = NULL WHERE channel_id = ?`, owner.ChannelID[:]); err == nil {
		t.Fatal("raw ciphertext null without prune metadata succeeded")
	}
}

func assertExactSchemaObjectInventory(t *testing.T, store *Store) {
	t.Helper()
	rows, err := store.db.Query(`
SELECT type, name, tbl_name, sql
FROM sqlite_schema
WHERE name NOT LIKE 'sqlite_%'
ORDER BY type, name`)
	if err != nil {
		t.Fatalf("query exact schema inventory: %v", err)
	}
	want := expectedSchemaObjects()
	index := 0
	for rows.Next() {
		var got schemaObject
		if err := rows.Scan(&got.kind, &got.name, &got.table, &got.sql); err != nil {
			t.Fatalf("scan exact schema inventory: %v", err)
		}
		if index >= len(want) {
			t.Fatalf("unexpected schema object %s %s", got.kind, got.name)
		}
		expected := want[index]
		if got.kind != expected.kind || got.name != expected.name || got.table != expected.table || normalizeSQL(got.sql) != normalizeSQL(expected.sql) {
			t.Fatalf("schema object %s %s differs from exact inventory", got.kind, got.name)
		}
		assertNoSemanticOrSecretSchemaVocabulary(t, got.name+" "+got.sql)
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate exact schema inventory: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close exact schema inventory: %v", err)
	}
	if index != len(want) {
		t.Fatalf("schema object count = %d, want %d", index, len(want))
	}
}

func schemaColumnNames(t *testing.T, store *Store, table string) []string {
	t.Helper()
	rows, err := store.db.Query(`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		t.Fatalf("query columns for %s: %v", table, err)
	}
	defer rows.Close()
	columns := make([]string, 0)
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan column for %s: %v", table, err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns for %s: %v", table, err)
	}
	return columns
}

func assertNoSemanticOrSecretSchemaVocabulary(t *testing.T, schema string) {
	t.Helper()
	lower := strings.ToLower(schema)
	for _, forbidden := range []string{
		"subject_kind", "subject_id", "fact_kind", "payload", "content_json", "hlc_", "external_locator",
		"tracker", "provider", "work_", "task_", "issue_", "ticket_", "assignment", "workflow", "hierarchy", "project_root",
		"private_key", "signing_key", "generation_key", "token_secret", "bearer_token", "plaintext",
	} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("schema contains forbidden semantic or plaintext-secret vocabulary %q", forbidden)
		}
	}
}

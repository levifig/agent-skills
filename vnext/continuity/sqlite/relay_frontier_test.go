package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestAdvanceSyncRelayWatermarkRetainsComponentwiseJoinAcrossReopen(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "relay-frontier-componentwise-join")
	store, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	projectID := continuity.ProjectID("project-relay-frontier-componentwise-join")
	first := testSyncRelayWatermark(projectID, 20)
	first.MembershipGeneration = 7
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), first); err != nil || got != first {
		t.Fatalf("AdvanceSyncRelayWatermark(first) = (%#v, %v), want (%#v, nil)", got, err, first)
	}

	crossed := first
	crossed.MembershipGeneration = 6
	crossed.RelayHead = 21
	want := first
	want.RelayHead = 21
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), crossed); err != nil || got != want {
		t.Fatalf("AdvanceSyncRelayWatermark(crossed) = (%#v, %v), want retained join (%#v, nil)", got, err, want)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	defer reopened.Close()
	regressed := first
	regressed.MembershipGeneration = 6
	if got, err := reopened.AdvanceSyncRelayWatermark(context.Background(), regressed); err != nil || got != want {
		t.Fatalf("AdvanceSyncRelayWatermark(after reopen) = (%#v, %v), want retained join (%#v, nil)", got, err, want)
	}
}

func TestContinuitySQLiteV7RelayFrontierMigrationFailsClosedUntilDominatingObservation(t *testing.T) {
	t.Parallel()
	stateRoot := filepath.Join(testTempDir(t), "relay-frontier-v7-migration")
	databasePath := createV7ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v7 database: %v", err)
	}

	legacyAndSource := testSyncRelayWatermark("project-v7-legacy-and-source", 20)
	legacyAndSource.MembershipGeneration = 6
	seedV7RelayWatermarkV1(t, db, legacyAndSource)
	seedV7AuthorityCandidateFrontierV1(t, db, legacyAndSource, 0x41)

	legacyOnly := testSyncRelayWatermark("project-v7-legacy-only", 9)
	legacyOnly.MembershipGeneration = 0
	seedV7RelayWatermarkV1(t, db, legacyOnly)

	sourceOnly := testSyncRelayWatermark("project-v7-source-only", 11)
	sourceOnly.MembershipGeneration = 4
	seedV7AuthorityCandidateFrontierV1(t, db, sourceOnly, 0x51)
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded v7 database: %v", err)
	}

	store, err := Open(stateRoot, "environment-v8")
	if err != nil {
		t.Fatalf("Open(v7) error = %v", err)
	}
	assertSyncRelayWatermarkRowWithKnown(t, store, legacyAndSource, false)
	assertSyncRelayWatermarkRowWithKnown(t, store, legacyOnly, false)
	assertSyncRelayWatermarkRowWithKnown(t, store, sourceOnly, true)
	if err := store.Close(); err != nil {
		t.Fatalf("close migrated store: %v", err)
	}

	store, err = Open(stateRoot, "environment-v8")
	if err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	}
	assertSyncRelayWatermarkRowWithKnown(t, store, legacyAndSource, false)

	crossedMembership := legacyAndSource
	crossedMembership.MembershipGeneration = 7
	crossedMembership.RelayHead = 19
	want := legacyAndSource
	want.MembershipGeneration = 7
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), crossedMembership); err != nil || got != want {
		t.Fatalf("AdvanceSyncRelayWatermark(crossed membership) = (%#v, %v), want retained join (%#v, nil)", got, err, want)
	}
	assertSyncRelayWatermarkRowWithKnown(t, store, want, false)

	crossedHead := legacyAndSource
	crossedHead.MembershipGeneration = 6
	crossedHead.RelayHead = 21
	want.RelayHead = 21
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), crossedHead); err != nil || got != want {
		t.Fatalf("AdvanceSyncRelayWatermark(crossed head) = (%#v, %v), want retained join (%#v, nil)", got, err, want)
	}
	assertSyncRelayWatermarkRowWithKnown(t, store, want, false)

	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), want); err != nil || got != want {
		t.Fatalf("AdvanceSyncRelayWatermark(dominating exact join) = (%#v, %v), want (%#v, nil)", got, err, want)
	}
	assertSyncRelayWatermarkRowWithKnown(t, store, want, true)
	if err := store.Close(); err != nil {
		t.Fatalf("close known migrated store: %v", err)
	}
	store, err = Open(stateRoot, "environment-v8")
	if err != nil {
		t.Fatalf("reopen known migrated store: %v", err)
	}
	defer store.Close()
	assertSyncRelayWatermarkRowWithKnown(t, store, want, true)
}

func TestContinuitySQLiteV7RelayFrontierMigrationRejectsAdminEquivocationWithoutMutation(t *testing.T) {
	t.Parallel()
	stateRoot := filepath.Join(testTempDir(t), "relay-frontier-v7-admin-equivocation")
	databasePath := createV7ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v7 database: %v", err)
	}
	legacy := testSyncRelayWatermark("project-v7-admin-equivocation", 20)
	legacy.MembershipGeneration = 6
	seedV7RelayWatermarkV1(t, db, legacy)
	equivocated := legacy
	equivocated.AdminPublicKey = testAuthorityDigest(0x6f)
	seedV7AuthorityCandidateFrontierV1(t, db, equivocated, 0x61)
	before := schemaIdentitySnapshot(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded v7 database: %v", err)
	}

	if store, err := Open(stateRoot, "environment-v8"); err == nil {
		store.Close()
		t.Fatal("Open(v7 admin equivocation) error = nil, want refusal")
	}

	db, err = openDatabase(databasePath)
	if err != nil {
		t.Fatalf("reopen refused v7 database: %v", err)
	}
	defer db.Close()
	after := schemaIdentitySnapshot(t, db)
	if after != before {
		t.Fatalf("refused v7 migration mutated schema identity: before=%#v after=%#v", before, after)
	}
	var membershipColumns, legacyRows, migrationTables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('continuity_sync_relay_watermarks') WHERE name = 'membership_generation'`).Scan(&membershipColumns); err != nil {
		t.Fatalf("inspect refused v7 membership column: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_relay_watermarks`).Scan(&legacyRows); err != nil {
		t.Fatalf("count refused v7 relay watermarks: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name = 'continuity_sync_relay_watermarks_v7'`).Scan(&migrationTables); err != nil {
		t.Fatalf("inspect refused v7 migration table: %v", err)
	}
	if membershipColumns != 0 || legacyRows != 1 || migrationTables != 0 {
		t.Fatalf("refused v7 migration changed schema: membership columns=%d legacy rows=%d migration tables=%d", membershipColumns, legacyRows, migrationTables)
	}
}

func TestContinuitySQLiteV7RelayFrontierMigrationJoinsCanonicalAndCrossedCandidates(t *testing.T) {
	t.Parallel()
	stateRoot := filepath.Join(testTempDir(t), "relay-frontier-v7-source-join")
	databasePath := createV7ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v7 database: %v", err)
	}
	projectID := continuity.ProjectID("project-v7-source-join")
	authority := testSyncAuthority()
	seedV3SyncAuthority(t, db, projectID, authority, nil)
	if _, err := db.Exec(`
UPDATE continuity_sync_projects
SET relay_head = 9223372036854775807
WHERE project_id = ?`, string(projectID)); err != nil {
		db.Close()
		t.Fatalf("seed non-authoritative data-plane head: %v", err)
	}
	canonicalDigest := testAuthorityDigest(0x70)
	if _, err := db.Exec(`
UPDATE continuity_sync_authorities
SET digest_version = 2, authority_digest = ?, inventory_arrival_head = 12
WHERE project_id = ?`, canonicalDigest[:], string(projectID)); err != nil {
		db.Close()
		t.Fatalf("seed canonical inventory frontier: %v", err)
	}
	legacy := SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            authority.ChannelID,
		RelayGeneration:      authority.RelayGeneration,
		AdminPublicKey:       authority.AdminPublicKey,
		MembershipGeneration: 0,
		RelayHead:            13,
	}
	seedV7RelayWatermarkV1(t, db, legacy)
	membershipCandidate := legacy
	membershipCandidate.MembershipGeneration = 6
	membershipCandidate.RelayHead = 9
	seedV7AuthorityCandidateFrontierV1(t, db, membershipCandidate, 0x71)
	headCandidate := legacy
	headCandidate.MembershipGeneration = 5
	headCandidate.RelayHead = 15
	seedV7AuthorityCandidateFrontierV1(t, db, headCandidate, 0x75)
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded v7 database: %v", err)
	}

	store, err := Open(stateRoot, "environment-v8")
	if err != nil {
		t.Fatalf("Open(v7 source join) error = %v", err)
	}
	defer store.Close()
	want := legacy
	want.MembershipGeneration = 6
	want.RelayHead = 15
	assertSyncRelayWatermarkRowWithKnown(t, store, want, false)
}

func seedV7RelayWatermarkV1(t *testing.T, db interface {
	Exec(string, ...any) (sql.Result, error)
}, watermark SyncRelayWatermark) {
	t.Helper()
	if _, err := db.Exec(`
INSERT INTO continuity_sync_relay_watermarks(
  project_id, channel_id, relay_generation, admin_public_key, relay_head
) VALUES(?, ?, ?, ?, ?)`,
		string(watermark.ProjectID), watermark.ChannelID[:], watermark.RelayGeneration[:],
		watermark.AdminPublicKey[:], watermark.RelayHead,
	); err != nil {
		t.Fatalf("seed v7 relay watermark: %v", err)
	}
}

func seedV7AuthorityCandidateFrontierV1(t *testing.T, db interface {
	Exec(string, ...any) (sql.Result, error)
}, frontier SyncRelayWatermark, candidateSeed byte) {
	t.Helper()
	candidateID := testAuthorityDigest(candidateSeed)
	rollingDigest := testAuthorityDigest(candidateSeed + 1)
	authorityDigest := testAuthorityDigest(candidateSeed + 2)
	if _, err := db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest, role
) VALUES(?, ?, 'promoted', ?, ?, ?, ?, ?, 1, 1, 'environment-v7', ?, 2, ?, 'ordinary')`,
		string(frontier.ProjectID), candidateID[:], frontier.ChannelID[:], frontier.RelayGeneration[:],
		frontier.AdminPublicKey[:], frontier.MembershipGeneration, frontier.RelayHead, rollingDigest[:], authorityDigest[:],
	); err != nil {
		t.Fatalf("seed v7 authority candidate frontier: %v", err)
	}
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestSyncAuthorityRecoveryTransitionSchemaIsSecretFreeAndForeignKeyed(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	rows, err := store.db.Query(`PRAGMA table_info(continuity_sync_authority_recovery_transitions)`)
	if err != nil {
		t.Fatalf("inspect transition columns: %v", err)
	}
	var columns []string
	for rows.Next() {
		var ordinal, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&ordinal, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			t.Fatalf("scan transition column: %v", err)
		}
		columns = append(columns, name)
		for _, forbidden := range []string{"credential", "token", "secret", "bearer", "admin_seed", "root", "request", "prepared"} {
			if strings.Contains(name, forbidden) {
				rows.Close()
				t.Fatalf("transition column %q contains forbidden fragment %q", name, forbidden)
			}
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close transition columns: %v", err)
	}
	wantColumns := []string{
		"project_id", "attempt_id", "predecessor_candidate_id", "successor_candidate_id",
		"writer_environment_id", "writer_certificate_id", "target_membership_generation",
	}
	if strings.Join(columns, ",") != strings.Join(wantColumns, ",") {
		t.Fatalf("transition columns = %v, want %v", columns, wantColumns)
	}

	foreignKeys, err := store.db.Query(`PRAGMA foreign_key_list(continuity_sync_authority_recovery_transitions)`)
	if err != nil {
		t.Fatalf("inspect transition foreign keys: %v", err)
	}
	foreignKeyRows := 0
	for foreignKeys.Next() {
		foreignKeyRows++
	}
	if err := foreignKeys.Close(); err != nil {
		t.Fatalf("close transition foreign keys: %v", err)
	}
	if foreignKeyRows != 4 {
		t.Fatalf("transition foreign-key columns = %d, want 4", foreignKeyRows)
	}
	violations, err := store.db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("check fresh foreign keys: %v", err)
	}
	if violations.Next() {
		violations.Close()
		t.Fatal("fresh v7 schema has a foreign-key violation")
	}
	if err := violations.Close(); err != nil {
		t.Fatalf("close foreign-key check: %v", err)
	}
}

func TestSyncAuthorityRecoveryCandidateRoleAndActiveSlotConstraints(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	projectID := continuity.ProjectID("project-transition-constraints")
	insertRawRecoveryCandidateV1(t, store, projectID, 0x01, "ready", syncAuthorityCandidateRoleRecoveryPredecessorV1, 1)
	insertRawRecoveryCandidateV1(t, store, projectID, 0x02, "staging", syncAuthorityCandidateRoleRecoverySuccessorV1, 2)
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest, role
) VALUES(?, ?, 'ready', ?, ?, ?, 1, 0, 1, 1, 'environment-raw', ?, 2, ?, 'recovery-predecessor')`,
		string(projectID), schemaDigestBytes(0x61), schemaDigestBytes(0x62), schemaDigestBytes(0x63),
		schemaDigestBytes(0x64), schemaDigestBytes(0x65), schemaDigestBytes(0x66),
	); err == nil {
		t.Fatal("second ready recovery predecessor error = nil, want unique-index refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest, role
) VALUES(?, ?, 'staging', ?, ?, ?, 2, 0, 1, 1, 'environment-raw', ?, 2, NULL, 'ordinary')`,
		string(projectID), schemaDigestBytes(0x71), schemaDigestBytes(0x72), schemaDigestBytes(0x73),
		schemaDigestBytes(0x74), schemaDigestBytes(0x75),
	); err == nil {
		t.Fatal("ordinary active candidate beside recovery successor error = nil, want logical-slot refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest, role
) VALUES('project-transition-invalid-role-state', ?, 'staging', ?, ?, ?, 1, 0, 1, 1, 'environment-raw', ?, 2, NULL, 'recovery-predecessor')`,
		schemaDigestBytes(0x81), schemaDigestBytes(0x82), schemaDigestBytes(0x83), schemaDigestBytes(0x84), schemaDigestBytes(0x85),
	); err == nil {
		t.Fatal("staging recovery predecessor error = nil, want role-state CHECK refusal")
	}
}

func TestSyncAuthorityRecoveryTransitionReaderAuditsRawRoleAndLinkCorruption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		seed func(*testing.T, *Store, continuity.ProjectID)
	}{
		{
			name: "participant without link",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				insertRawRecoveryCandidateV1(t, store, projectID, 0x11, "staging", syncAuthorityCandidateRoleRecoverySuccessorV1, 1)
			},
		},
		{
			name: "ordinary candidate linked as successor",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				successor := insertRawRecoveryCandidateV1(t, store, projectID, 0x21, "staging", syncAuthorityCandidateRoleOrdinaryV1, 1)
				insertRawRecoveryTransitionV1(t, store, projectID, nil, successor, 0x22, 1)
			},
		},
		{
			name: "orphan successor link",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				if _, err := store.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
					t.Fatalf("disable foreign keys: %v", err)
				}
				insertRawRecoveryTransitionV1(t, store, projectID, nil, recoveryDigestV1(0x31), 0x32, 1)
				if _, err := store.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
					t.Fatalf("restore foreign keys: %v", err)
				}
			},
		},
		{
			name: "extra unlinked participant",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				successor := insertRawRecoveryCandidateV1(t, store, projectID, 0x41, "staging", syncAuthorityCandidateRoleRecoverySuccessorV1, 1)
				insertRawRecoveryCandidateV1(t, store, projectID, 0x42, "ready", syncAuthorityCandidateRoleRecoveryPredecessorV1, 7)
				insertRawRecoveryTransitionV1(t, store, projectID, nil, successor, 0x43, 1)
			},
		},
		{
			name: "successor below target membership",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				predecessor := insertRawRecoveryCandidateV1(t, store, projectID, 0x44, "ready", syncAuthorityCandidateRoleRecoveryPredecessorV1, 1)
				successor := insertRawRecoveryCandidateV1(t, store, projectID, 0x45, "staging", syncAuthorityCandidateRoleRecoverySuccessorV1, 1)
				insertRawRecoveryTransitionV1(t, store, projectID, &predecessor, successor, 0x46, 2)
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer store.Close()
			projectID := continuity.ProjectID("project-transition-corruption")
			test.seed(t, store, projectID)

			_, found, err := store.CurrentSyncAuthorityRecoveryTransition(context.Background(), projectID)
			if err == nil || found {
				t.Fatalf("CurrentSyncAuthorityRecoveryTransition() = (_, %v, %v), want fielded store error", found, err)
			}
			assertSyncAuthorityRecoveryTransitionStoreErrorV1(t, err)
		})
	}
}

func TestSyncAuthorityRecoveryTransitionAllowsSuccessorAheadOfTargetMembership(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	projectID := continuity.ProjectID("project-transition-successor-ahead")
	predecessor := insertRawRecoveryCandidateV1(t, store, projectID, 0x48, "ready", syncAuthorityCandidateRoleRecoveryPredecessorV1, 1)
	successor := insertRawRecoveryCandidateV1(t, store, projectID, 0x49, "staging", syncAuthorityCandidateRoleRecoverySuccessorV1, 4)
	insertRawRecoveryTransitionV1(t, store, projectID, &predecessor, successor, 0x4a, 2)

	transition, found, err := store.CurrentSyncAuthorityRecoveryTransition(context.Background(), projectID)
	if err != nil || !found {
		t.Fatalf("CurrentSyncAuthorityRecoveryTransition() = (%#v, %v, %v)", transition, found, err)
	}
	if transition.PredecessorCandidateID != predecessor || transition.SuccessorCandidateID != successor || transition.TargetMembershipGeneration != 2 {
		t.Fatalf("transition = %#v", transition)
	}
}

func TestContinuitySQLiteV5ToV8MigrationPreservesAuthorityCandidatesAndSeedsRelayFrontiers(t *testing.T) {
	t.Parallel()
	stateRoot := filepath.Join(testTempDir(t), "state")
	databasePath := createV5ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v5 database: %v", err)
	}
	seedV5RecoveryMigrationRowsV1(t, db)
	before := snapshotV5AuthorityCandidateRowsV1(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded v5 database: %v", err)
	}

	store, err := Open(stateRoot, "environment-v6")
	if err != nil {
		t.Fatalf("Open(v5) error = %v", err)
	}
	defer store.Close()
	after := snapshotV5AuthorityCandidateRowsV1(t, store.db)
	if after != before {
		t.Fatalf("migrated authority candidate bytes changed:\nbefore=%s\nafter=%s", before, after)
	}
	var ordinary, nonOrdinary, transitions int64
	if err := store.db.QueryRow(`
SELECT
  SUM(CASE WHEN role = 'ordinary' THEN 1 ELSE 0 END),
  SUM(CASE WHEN role <> 'ordinary' THEN 1 ELSE 0 END)
FROM continuity_sync_authority_candidates`).Scan(&ordinary, &nonOrdinary); err != nil {
		t.Fatalf("inspect migrated roles: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_recovery_transitions`).Scan(&transitions); err != nil {
		t.Fatalf("inspect migrated transitions: %v", err)
	}
	if ordinary != 3 || nonOrdinary != 0 || transitions != 0 {
		t.Fatalf("migrated transition state: ordinary=%d non-ordinary=%d transitions=%d", ordinary, nonOrdinary, transitions)
	}

	type relayFrontier struct {
		membershipGeneration uint32
		relayHead            int64
		membershipKnown      int
	}
	wantFrontiers := map[string]relayFrontier{
		"project-v5-promoted":  {membershipGeneration: 1, relayHead: 17, membershipKnown: 1},
		"project-v5-ready":     {membershipGeneration: 1, relayHead: 17, membershipKnown: 1},
		"project-v5-staging":   {membershipGeneration: 1, relayHead: 17, membershipKnown: 1},
		"project-v5-watermark": {membershipGeneration: 0, relayHead: 987654321, membershipKnown: 0},
	}
	rows, err := store.db.Query(`
SELECT project_id, membership_generation, relay_head, membership_floor_known
FROM continuity_sync_relay_watermarks
ORDER BY project_id`)
	if err != nil {
		t.Fatalf("list migrated relay frontiers: %v", err)
	}
	defer rows.Close()
	gotFrontiers := make(map[string]relayFrontier, len(wantFrontiers))
	for rows.Next() {
		var projectID string
		var frontier relayFrontier
		if err := rows.Scan(
			&projectID,
			&frontier.membershipGeneration,
			&frontier.relayHead,
			&frontier.membershipKnown,
		); err != nil {
			t.Fatalf("scan migrated relay frontier: %v", err)
		}
		gotFrontiers[projectID] = frontier
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migrated relay frontiers: %v", err)
	}
	if !reflect.DeepEqual(gotFrontiers, wantFrontiers) {
		t.Fatalf("migrated relay frontiers = %#v, want %#v", gotFrontiers, wantFrontiers)
	}
}

func TestGenericSyncAuthorityAPIsRefuseRecoveryTransitionWithoutMutation(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	projectID := continuity.ProjectID("project-transition-gate")
	predecessor := insertRawRecoveryCandidateV1(t, store, projectID, 0x51, "ready", syncAuthorityCandidateRoleRecoveryPredecessorV1, 1)
	successor := insertRawRecoveryCandidateV1(t, store, projectID, 0x52, "staging", syncAuthorityCandidateRoleRecoverySuccessorV1, 2)
	insertRawRecoveryTransitionV1(t, store, projectID, &predecessor, successor, 0x53, 2)

	transition, found, err := store.CurrentSyncAuthorityRecoveryTransition(context.Background(), projectID)
	if err != nil || !found {
		t.Fatalf("CurrentSyncAuthorityRecoveryTransition() = (%#v, %v, %v)", transition, found, err)
	}
	if transition.PredecessorCandidateID != predecessor || transition.SuccessorCandidateID != successor || transition.TargetMembershipGeneration != 2 {
		t.Fatalf("transition = %#v", transition)
	}
	before := rawRecoveryTransitionStateV1(t, store, projectID)

	assertTransitionConflict := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s error = nil", name)
		}
		var problem *SyncError
		if !errors.As(err, &problem) || problem.Code != SyncErrorConflict || problem.Field != "sync_authority_recovery_transition" {
			t.Fatalf("%s error = %#v", name, err)
		}
		if got := rawRecoveryTransitionStateV1(t, store, projectID); got != before {
			t.Fatalf("%s mutated transition state: before=%q after=%q", name, before, got)
		}
	}

	_, err = store.InstallVerifiedSyncAuthority(context.Background(), projectID, testSyncAuthority())
	assertTransitionConflict("InstallVerifiedSyncAuthority", err)
	_, err = store.CurrentSyncAuthority(context.Background(), projectID)
	assertTransitionConflict("CurrentSyncAuthority", err)
	_, err = store.CurrentSyncAuthorityBinding(context.Background(), projectID)
	assertTransitionConflict("CurrentSyncAuthorityBinding", err)
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
	page := syncAuthorityCandidatePageV2("", syncAuthorityCandidateManyEnvironmentsV2(1), false)
	_, err = store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
	assertTransitionConflict("StageVerifiedSyncAuthorityCandidatePage", err)
	_, _, err = store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
	assertTransitionConflict("CurrentSyncAuthorityCandidate", err)
	stagingCheckpoint := SyncAuthorityCandidateCheckpoint{
		CandidateID: successor, PageCount: 1, EnvironmentCount: 1,
		ThroughEnvironmentID: "environment-raw", RollingEnvironmentDigest: recoveryDigestV1(0x62),
	}
	err = store.DiscardSyncAuthorityCandidate(context.Background(), projectID, stagingCheckpoint)
	assertTransitionConflict("DiscardSyncAuthorityCandidate", err)
	readyCheckpoint := SyncAuthorityCandidateCheckpoint{
		CandidateID: predecessor, PageCount: 1, EnvironmentCount: 1,
		ThroughEnvironmentID: "environment-raw", RollingEnvironmentDigest: recoveryDigestV1(0x61),
		Ready: true, AuthorityDigest: recoveryDigestV1(0x71),
	}
	_, err = store.PromoteSyncAuthorityCandidate(context.Background(), projectID, readyCheckpoint)
	assertTransitionConflict("PromoteSyncAuthorityCandidate", err)
}

func insertRawRecoveryCandidateV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	seed byte,
	state string,
	role string,
	membershipGeneration uint32,
) [32]byte {
	t.Helper()
	candidateID := recoveryDigestV1(seed)
	var authorityDigest any
	if state != "staging" {
		value := recoveryDigestV1(seed + 0x20)
		authorityDigest = value[:]
	}
	channelID := recoveryDigestV1(seed + 1)
	relayGeneration := recoveryDigestV1(seed + 2)
	adminPublicKey := recoveryDigestV1(seed + 3)
	rollingDigest := recoveryDigestV1(seed + 0x10)
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest, role
) VALUES(?, ?, ?, ?, ?, ?, ?, 0, 1, 1, 'environment-raw', ?, 2, ?, ?)`,
		string(projectID), candidateID[:], state, channelID[:], relayGeneration[:],
		adminPublicKey[:], membershipGeneration, rollingDigest[:], authorityDigest, role,
	); err != nil {
		t.Fatalf("insert raw recovery candidate: %v", err)
	}
	return candidateID
}

func insertRawRecoveryTransitionV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	predecessor *[32]byte,
	successor [32]byte,
	seed byte,
	targetMembershipGeneration uint32,
) {
	t.Helper()
	var predecessorValue any
	if predecessor != nil {
		predecessorValue = predecessor[:]
	}
	writerCertificateID := recoveryDigestV1(seed)
	attemptID := recoveryDigestV1(seed + 0x40)
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_recovery_transitions(
  project_id, attempt_id, predecessor_candidate_id, successor_candidate_id,
  writer_environment_id, writer_certificate_id, target_membership_generation
) VALUES(?, ?, ?, ?, 'environment-writer', ?, ?)`,
		string(projectID), attemptID[:], predecessorValue, successor[:], writerCertificateID[:], targetMembershipGeneration,
	); err != nil {
		t.Fatalf("insert raw recovery transition: %v", err)
	}
}

func rawRecoveryTransitionStateV1(t *testing.T, store *Store, projectID continuity.ProjectID) string {
	t.Helper()
	var candidates, transitions string
	if err := store.db.QueryRow(`
SELECT COALESCE(group_concat(hex(candidate_id) || ':' || state || ':' || role, ','), '')
FROM (
  SELECT candidate_id, state, role
  FROM continuity_sync_authority_candidates
  WHERE project_id = ?
  ORDER BY candidate_id
)`, string(projectID)).Scan(&candidates); err != nil {
		t.Fatalf("snapshot recovery candidates: %v", err)
	}
	if err := store.db.QueryRow(`
SELECT COALESCE(group_concat(
  hex(attempt_id) || ':' || COALESCE(hex(predecessor_candidate_id), 'NULL') || ':' || hex(successor_candidate_id) || ':' ||
  writer_environment_id || ':' || hex(writer_certificate_id) || ':' || target_membership_generation, ','), '')
FROM continuity_sync_authority_recovery_transitions
WHERE project_id = ?`, string(projectID)).Scan(&transitions); err != nil {
		t.Fatalf("snapshot recovery transition: %v", err)
	}
	return candidates + "|" + transitions
}

func assertSyncAuthorityRecoveryTransitionStoreErrorV1(t *testing.T, err error) {
	t.Helper()
	var problem *SyncError
	if !errors.As(err, &problem) || problem.Code != SyncErrorStore || problem.Field != "sync_authority_recovery_transition" {
		t.Fatalf("error = %#v, want fielded transition store error", err)
	}
}

func recoveryDigestV1(seed byte) [32]byte {
	var value [32]byte
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}

func seedV5RecoveryMigrationRowsV1(t *testing.T, db *sql.DB) {
	t.Helper()
	states := []string{"staging", "ready", "promoted"}
	for index, state := range states {
		seed := byte(0x80 + index*0x10)
		projectID := "project-v5-" + state
		candidateID := recoveryDigestV1(seed)
		channelID := recoveryDigestV1(seed + 1)
		relayGeneration := recoveryDigestV1(seed + 2)
		adminPublicKey := recoveryDigestV1(seed + 3)
		rollingDigest := recoveryDigestV1(seed + 4)
		var authorityDigest any
		if state != "staging" {
			value := recoveryDigestV1(seed + 5)
			authorityDigest = value[:]
		}
		if _, err := db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest
) VALUES(?, ?, ?, ?, ?, ?, 1, 17, 1, 1, 'environment-v5', ?, 2, ?)`,
			projectID, candidateID[:], state, channelID[:], relayGeneration[:], adminPublicKey[:], rollingDigest[:], authorityDigest,
		); err != nil {
			t.Fatalf("seed v5 %s candidate: %v", state, err)
		}
		if state == "promoted" {
			continue
		}
		pageDigest := recoveryDigestV1(seed + 6)
		more := 0
		if state == "staging" {
			more = 1
		}
		if _, err := db.Exec(`
INSERT INTO continuity_sync_authority_candidate_pages(
  project_id, candidate_id, page_number, after_environment_id,
  through_environment_id, environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
) VALUES(?, ?, 1, NULL, 'environment-v5', 1, ?, ?, 1, ?)`,
			projectID, candidateID[:], more, pageDigest[:], rollingDigest[:],
		); err != nil {
			t.Fatalf("seed v5 %s candidate page: %v", state, err)
		}
		certificateID := recoveryDigestV1(seed + 7)
		certificateBytes := []byte{0x00, seed, 0xff, 0x7f}
		if _, err := db.Exec(`
INSERT INTO continuity_sync_authority_candidate_environments(
  project_id, candidate_id, environment_id, environment_ordinal,
  page_number, certificate_id, certificate_bytes, mode,
  expires_at_millis, join_membership_generation
) VALUES(?, ?, 'environment-v5', 1, 1, ?, ?, 'trusted', 0, 1)`,
			projectID, candidateID[:], certificateID[:], certificateBytes,
		); err != nil {
			t.Fatalf("seed v5 %s candidate environment: %v", state, err)
		}
		if _, err := db.Exec(`
INSERT INTO continuity_sync_authority_candidate_membership_events(
  project_id, candidate_id, membership_generation, event_kind, environment_id
) VALUES(?, ?, 1, 'join', 'environment-v5')`, projectID, candidateID[:]); err != nil {
			t.Fatalf("seed v5 %s candidate event: %v", state, err)
		}
	}
	channelID := recoveryDigestV1(0xd0)
	relayGeneration := recoveryDigestV1(0xd1)
	adminPublicKey := recoveryDigestV1(0xd2)
	if _, err := db.Exec(`
INSERT INTO continuity_sync_relay_watermarks(
  project_id, channel_id, relay_generation, admin_public_key, relay_head
) VALUES('project-v5-watermark', ?, ?, ?, 987654321)`, channelID[:], relayGeneration[:], adminPublicKey[:]); err != nil {
		t.Fatalf("seed v5 relay watermark: %v", err)
	}
}

func snapshotV5AuthorityCandidateRowsV1(t *testing.T, db *sql.DB) string {
	t.Helper()
	queries := []string{
		`SELECT COALESCE(group_concat(row_value, '|'), '') FROM (
  SELECT project_id || ':' || hex(candidate_id) || ':' || state || ':' ||
    hex(channel_id) || ':' || hex(relay_generation) || ':' || hex(admin_public_key) || ':' ||
    membership_generation || ':' || inventory_arrival_head || ':' ||
    COALESCE(base_authority_digest_version, -1) || ':' || COALESCE(hex(base_authority_digest), 'NULL') || ':' ||
    page_count || ':' || environment_count || ':' || through_environment_id || ':' ||
    hex(rolling_environment_digest) || ':' || authority_digest_version || ':' || COALESCE(hex(authority_digest), 'NULL') AS row_value
  FROM continuity_sync_authority_candidates ORDER BY project_id
)`,
		`SELECT COALESCE(group_concat(row_value, '|'), '') FROM (
  SELECT project_id || ':' || hex(candidate_id) || ':' || page_number || ':' ||
    COALESCE(after_environment_id, 'NULL') || ':' || through_environment_id || ':' ||
    environment_count || ':' || more || ':' || hex(page_digest) || ':' ||
    resulting_environment_count || ':' || hex(resulting_rolling_digest) AS row_value
  FROM continuity_sync_authority_candidate_pages ORDER BY project_id, candidate_id, page_number
)`,
		`SELECT COALESCE(group_concat(row_value, '|'), '') FROM (
  SELECT project_id || ':' || hex(candidate_id) || ':' || environment_id || ':' ||
    environment_ordinal || ':' || page_number || ':' || hex(certificate_id) || ':' ||
    hex(certificate_bytes) || ':' || mode || ':' || expires_at_millis || ':' || join_membership_generation AS row_value
  FROM continuity_sync_authority_candidate_environments ORDER BY project_id, candidate_id, environment_id
)`,
		`SELECT COALESCE(group_concat(row_value, '|'), '') FROM (
  SELECT project_id || ':' || hex(candidate_id) || ':' || membership_generation || ':' ||
    event_kind || ':' || environment_id AS row_value
  FROM continuity_sync_authority_candidate_membership_events ORDER BY project_id, candidate_id, membership_generation
)`,
	}
	parts := make([]string, 0, len(queries))
	for _, query := range queries {
		var value string
		if err := db.QueryRow(query).Scan(&value); err != nil {
			t.Fatalf("snapshot v5 migration row: %v", err)
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, "||")
}

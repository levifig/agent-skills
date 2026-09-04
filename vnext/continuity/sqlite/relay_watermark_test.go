package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestAdvanceSyncRelayWatermarkInsertsReplaysAdvancesReopensAndRetainsJoin(t *testing.T) {
	root := filepath.Join(testTempDir(t), "relay-watermark-lifecycle")
	store, err := Open(root, "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	identity := testSyncRelayWatermark("project-relay-watermark-lifecycle", 5)

	first, err := store.AdvanceSyncRelayWatermark(context.Background(), identity)
	if err != nil || first != identity {
		t.Fatalf("AdvanceSyncRelayWatermark(first) = (%#v, %v), want (%#v, nil)", first, err, identity)
	}
	replayed, err := store.AdvanceSyncRelayWatermark(context.Background(), identity)
	if err != nil || replayed != identity {
		t.Fatalf("AdvanceSyncRelayWatermark(replay) = (%#v, %v), want (%#v, nil)", replayed, err, identity)
	}
	advanced := identity
	advanced.RelayHead = 9
	got, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced)
	if err != nil || got != advanced {
		t.Fatalf("AdvanceSyncRelayWatermark(advance) = (%#v, %v), want (%#v, nil)", got, err, advanced)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	store, err = Open(root, "environment-local")
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	reopened, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced)
	if err != nil || reopened != advanced {
		t.Fatalf("AdvanceSyncRelayWatermark(reopen replay) = (%#v, %v), want (%#v, nil)", reopened, err, advanced)
	}
	lower := advanced
	lower.RelayHead--
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), lower); err != nil || got != advanced {
		t.Fatalf("AdvanceSyncRelayWatermark(lower) = (%#v, %v), want retained join (%#v, nil)", got, err, advanced)
	}
	assertSyncRelayWatermarkRow(t, store, advanced)
}

func TestAdvanceSyncRelayWatermarkWriteFailuresAreAtomicAndFieldless(t *testing.T) {
	const forbidden = "relay-watermark-sensitive-write-detail"
	for _, test := range []struct {
		name      string
		operation string
		seed      bool
	}{
		{name: "insert", operation: "INSERT"},
		{name: "update", operation: "UPDATE", seed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "relay-watermark-write-failure-"+test.name)
			initial := testSyncRelayWatermark(continuity.ProjectID("project-relay-watermark-write-failure-"+test.name), 5)
			if test.seed {
				if _, err := store.AdvanceSyncRelayWatermark(context.Background(), initial); err != nil {
					t.Fatalf("seed relay watermark: %v", err)
				}
			}
			if _, err := store.db.Exec(`CREATE TRIGGER reject_relay_watermark_write
BEFORE ` + test.operation + ` ON continuity_sync_relay_watermarks
BEGIN SELECT RAISE(FAIL, '` + forbidden + `'); END`); err != nil {
				t.Fatalf("install %s failure trigger: %v", test.name, err)
			}

			want := initial
			if test.seed {
				want.RelayHead++
			}
			got, err := store.AdvanceSyncRelayWatermark(context.Background(), want)
			if !reflect.DeepEqual(got, SyncRelayWatermark{}) || err == nil {
				t.Fatalf("AdvanceSyncRelayWatermark(%s failure) = (%#v, %v), want zero and error", test.name, got, err)
			}
			var problem *SyncError
			if !errors.As(err, &problem) || problem.Code != SyncErrorStore || problem.Field != "" {
				t.Fatalf("%s failure error = %#v, want fieldless store error", test.name, err)
			}
			if strings.Contains(err.Error(), forbidden) {
				t.Fatalf("%s failure leaked SQLite detail: %v", test.name, err)
			}

			if test.seed {
				assertSyncRelayWatermarkRow(t, store, initial)
				return
			}
			var count int
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_relay_watermarks`).Scan(&count); err != nil {
				t.Fatalf("count relay watermark rows: %v", err)
			}
			if count != 0 {
				t.Fatalf("failed insert retained %d relay watermark rows, want zero", count)
			}
		})
	}
}

func TestAdvanceSyncRelayWatermarkRejectsAdminEquivocationAndKeepsRelayGenerationsIndependent(t *testing.T) {
	store := openSyncStore(t, "relay-watermark-identity")
	first := testSyncRelayWatermark("project-relay-watermark-identity", 7)
	if _, err := store.AdvanceSyncRelayWatermark(context.Background(), first); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark(first) error = %v", err)
	}
	equivocated := first
	equivocated.AdminPublicKey[0] ^= 0xff
	if _, err := store.AdvanceSyncRelayWatermark(context.Background(), equivocated); err == nil {
		t.Fatal("AdvanceSyncRelayWatermark(admin equivocation) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	assertSyncRelayWatermarkRow(t, store, first)

	nextGeneration := first
	nextGeneration.RelayGeneration[0] ^= 0xff
	nextGeneration.AdminPublicKey = testAuthorityDigest(0x91)
	nextGeneration.RelayHead = 2
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), nextGeneration); err != nil || got != nextGeneration {
		t.Fatalf("AdvanceSyncRelayWatermark(next generation) = (%#v, %v)", got, err)
	}
	assertSyncRelayWatermarkRow(t, store, first)
	assertSyncRelayWatermarkRow(t, store, nextGeneration)
}

func TestAdvanceSyncRelayWatermarkConcurrentCallsRetainMaximum(t *testing.T) {
	root := filepath.Join(testTempDir(t), "relay-watermark-concurrent")
	const callers = 12
	stores := make([]*Store, callers)
	for index := range stores {
		store, err := Open(root, "environment-local")
		if err != nil {
			t.Fatalf("Open(%d) error = %v", index, err)
		}
		stores[index] = store
		candidateStore := store
		t.Cleanup(func() { _ = candidateStore.Close() })
	}
	base := testSyncRelayWatermark("project-relay-watermark-concurrent", 0)
	var wait sync.WaitGroup
	errorsByHead := make([]error, callers)
	for index := range stores {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			candidate := base
			candidate.RelayHead = int64(index + 1)
			_, errorsByHead[index] = stores[index].AdvanceSyncRelayWatermark(context.Background(), candidate)
		}(index)
	}
	wait.Wait()
	for index, err := range errorsByHead {
		if err != nil {
			t.Fatalf("concurrent head %d error = %v", index+1, err)
		}
	}
	want := base
	want.RelayHead = callers
	if got, err := stores[0].AdvanceSyncRelayWatermark(context.Background(), want); err != nil || got != want {
		t.Fatalf("AdvanceSyncRelayWatermark(max replay) = (%#v, %v), want (%#v, nil)", got, err, want)
	}
	assertSyncRelayWatermarkRow(t, stores[0], want)
}

func TestAdvanceSyncRelayWatermarkInheritsDurableSourceMaximumAndAdmin(t *testing.T) {
	t.Run("ready candidate", func(t *testing.T) {
		store := openSyncStore(t, "relay-watermark-candidate-source")
		projectID := continuity.ProjectID("project-relay-watermark-candidate-source")
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(5)
		snapshot.InventoryArrivalHead = 13
		environments := syncAuthorityCandidateManyEnvironmentsV2(5)
		stageReadySyncAuthorityCandidateFromSnapshot(t, store, projectID, snapshot, environments)
		watermark := syncRelayWatermarkFromSnapshot(projectID, snapshot, 12)
		want := watermark
		want.RelayHead = snapshot.InventoryArrivalHead
		if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != want {
			t.Fatalf("AdvanceSyncRelayWatermark(below candidate) = (%#v, %v), want (%#v, nil)", got, err, want)
		}
		watermark.RelayHead = 13
		if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
			t.Fatalf("AdvanceSyncRelayWatermark(candidate head) = (%#v, %v)", got, err)
		}
	})

	t.Run("candidate admin equivocation", func(t *testing.T) {
		store := openSyncStore(t, "relay-watermark-candidate-admin-source")
		projectID := continuity.ProjectID("project-relay-watermark-candidate-admin-source")
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(5)
		stageReadySyncAuthorityCandidateFromSnapshot(t, store, projectID, snapshot, syncAuthorityCandidateManyEnvironmentsV2(5))
		watermark := syncRelayWatermarkFromSnapshot(projectID, snapshot, snapshot.InventoryArrivalHead)
		watermark.AdminPublicKey[0] ^= 0xff
		if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err == nil {
			t.Fatal("AdvanceSyncRelayWatermark(candidate admin equivocation) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorConflict)
		}
		var rows int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_relay_watermarks`).Scan(&rows); err != nil {
			t.Fatalf("count watermarks after admin equivocation: %v", err)
		}
		if rows != 1 {
			t.Fatalf("admin equivocation retained %d watermark rows", rows)
		}
		assertSyncRelayWatermarkRow(
			t, store, syncRelayWatermarkFromSnapshot(projectID, snapshot, snapshot.InventoryArrivalHead),
		)
	})

	t.Run("canonical ignores data-plane progress", func(t *testing.T) {
		store := openSyncStore(t, "relay-watermark-canonical-source")
		projectID := continuity.ProjectID("project-relay-watermark-canonical-source")
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(5)
		snapshot.InventoryArrivalHead = 7
		environments := syncAuthorityCandidateManyEnvironmentsV2(5)
		ready := stageReadySyncAuthorityCandidateFromSnapshot(t, store, projectID, snapshot, environments)
		if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
			t.Fatalf("PromoteSyncAuthorityCandidate() error = %v", err)
		}
		if _, err := store.db.Exec(`UPDATE continuity_sync_projects SET relay_head = 9 WHERE project_id = ?`, string(projectID)); err != nil {
			t.Fatalf("advance sync progress source: %v", err)
		}
		watermark := syncRelayWatermarkFromSnapshot(projectID, snapshot, 8)
		if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
			t.Fatalf("AdvanceSyncRelayWatermark(above canonical) = (%#v, %v), want (%#v, nil)", got, err, watermark)
		}
		watermark.RelayHead = 9
		if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
			t.Fatalf("AdvanceSyncRelayWatermark(progress head) = (%#v, %v)", got, err)
		}
	})
}

func TestSyncRelayWatermarkSurvivesAuthorityCandidateLifecycle(t *testing.T) {
	for _, lifecycle := range []string{"discard", "promote"} {
		t.Run(lifecycle, func(t *testing.T) {
			store := openSyncStore(t, "relay-watermark-lifecycle-"+lifecycle)
			projectID := continuity.ProjectID("project-relay-watermark-lifecycle-" + lifecycle)
			snapshot := syncAuthorityCandidateBootstrapSnapshotV2(5)
			snapshot.InventoryArrivalHead = 17
			ready := stageReadySyncAuthorityCandidateFromSnapshot(t, store, projectID, snapshot, syncAuthorityCandidateManyEnvironmentsV2(5))
			watermark := syncRelayWatermarkFromSnapshot(projectID, snapshot, 17)
			if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
				t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
			}
			if lifecycle == "discard" {
				if err := store.DiscardSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
					t.Fatalf("DiscardSyncAuthorityCandidate() error = %v", err)
				}
			} else if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
				t.Fatalf("PromoteSyncAuthorityCandidate() error = %v", err)
			}
			if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
				t.Fatalf("AdvanceSyncRelayWatermark(after %s) = (%#v, %v)", lifecycle, got, err)
			}
			assertSyncRelayWatermarkRow(t, store, watermark)
		})
	}
}

func TestAdvanceSyncRelayWatermarkValidatesPublicInputs(t *testing.T) {
	store := openSyncStore(t, "relay-watermark-validation")
	valid := testSyncRelayWatermark("project-relay-watermark-validation", 1)
	for _, test := range []struct {
		name   string
		store  *Store
		ctx    context.Context
		mutate func(*SyncRelayWatermark)
	}{
		{name: "nil store", ctx: context.Background()},
		{name: "nil context", store: store},
		{name: "project", store: store, ctx: context.Background(), mutate: func(value *SyncRelayWatermark) { value.ProjectID = "invalid project" }},
		{name: "channel", store: store, ctx: context.Background(), mutate: func(value *SyncRelayWatermark) { value.ChannelID = SyncChannelID{} }},
		{name: "relay generation", store: store, ctx: context.Background(), mutate: func(value *SyncRelayWatermark) { value.RelayGeneration = [32]byte{} }},
		{name: "admin public key", store: store, ctx: context.Background(), mutate: func(value *SyncRelayWatermark) { value.AdminPublicKey = [32]byte{} }},
		{name: "head", store: store, ctx: context.Background(), mutate: func(value *SyncRelayWatermark) { value.RelayHead = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			if got, err := test.store.AdvanceSyncRelayWatermark(test.ctx, candidate); !reflect.DeepEqual(got, SyncRelayWatermark{}) || err == nil {
				t.Fatalf("AdvanceSyncRelayWatermark(invalid) = (%#v, %v), want refusal", got, err)
			}
		})
	}
}

func TestSyncRelayWatermarkSchemaIsStrictCredentialFreeAndConstrained(t *testing.T) {
	store := openSyncStore(t, "relay-watermark-schema")
	valueType := reflect.TypeOf(SyncRelayWatermark{})
	var valueFields []string
	for index := 0; index < valueType.NumField(); index++ {
		valueFields = append(valueFields, valueType.Field(index).Name)
	}
	wantValueFields := []string{"ProjectID", "ChannelID", "RelayGeneration", "AdminPublicKey", "MembershipGeneration", "RelayHead"}
	if !reflect.DeepEqual(valueFields, wantValueFields) {
		t.Fatalf("watermark value fields = %v, want public identity only %v", valueFields, wantValueFields)
	}
	rows, err := store.db.Query(`PRAGMA table_info(continuity_sync_relay_watermarks)`)
	if err != nil {
		t.Fatalf("inspect watermark columns: %v", err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var ordinal, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&ordinal, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan watermark column: %v", err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate watermark columns: %v", err)
	}
	wantColumns := []string{
		"project_id", "channel_id", "relay_generation", "admin_public_key",
		"membership_generation", "relay_head", "membership_floor_known",
	}
	if !reflect.DeepEqual(columns, wantColumns) {
		t.Fatalf("watermark columns = %v, want credential-free %v", columns, wantColumns)
	}
	var tableSQL string
	if err := store.db.QueryRow(`SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'continuity_sync_relay_watermarks'`).Scan(&tableSQL); err != nil {
		t.Fatalf("read watermark DDL: %v", err)
	}
	for _, forbidden := range []string{"token", "secret", "certificate", "environment", "candidate"} {
		if strings.Contains(strings.ToLower(tableSQL), forbidden) {
			t.Fatalf("watermark DDL contains credential/attempt field %q: %s", forbidden, tableSQL)
		}
	}
	if !strings.Contains(tableSQL, "STRICT, WITHOUT ROWID") {
		t.Fatalf("watermark DDL is not strict and rowid-free: %s", tableSQL)
	}
	foreignKeys, err := store.db.Query(`PRAGMA foreign_key_list(continuity_sync_relay_watermarks)`)
	if err != nil {
		t.Fatalf("inspect watermark foreign keys: %v", err)
	}
	if foreignKeys.Next() {
		foreignKeys.Close()
		t.Fatal("watermark table has a foreign key and is not lifecycle-independent")
	}
	if err := foreignKeys.Close(); err != nil {
		t.Fatalf("close watermark foreign keys: %v", err)
	}
	validDigest := testAuthorityDigest(0xd1)
	for _, test := range []struct {
		name       string
		projectID  string
		channel    []byte
		generation []byte
		admin      []byte
		membership int64
		head       int64
		known      int64
	}{
		{name: "project", projectID: "invalid project", channel: validDigest[:], generation: validDigest[:], admin: validDigest[:], membership: 1, known: 1},
		{name: "channel length", projectID: "project-bad-channel-length", channel: []byte{1}, generation: validDigest[:], admin: validDigest[:], membership: 1, known: 1},
		{name: "channel zero", projectID: "project-bad-channel-zero", channel: make([]byte, 32), generation: validDigest[:], admin: validDigest[:], membership: 1, known: 1},
		{name: "generation", projectID: "project-bad-generation", channel: validDigest[:], generation: make([]byte, 32), admin: validDigest[:], membership: 1, known: 1},
		{name: "admin", projectID: "project-bad-admin", channel: validDigest[:], generation: validDigest[:], admin: make([]byte, 32), membership: 1, known: 1},
		{name: "membership below", projectID: "project-bad-membership-below", channel: validDigest[:], generation: validDigest[:], admin: validDigest[:], membership: -1, known: 1},
		{name: "membership above", projectID: "project-bad-membership-above", channel: validDigest[:], generation: validDigest[:], admin: validDigest[:], membership: 4294967296, known: 1},
		{name: "head", projectID: "project-bad-head", channel: validDigest[:], generation: validDigest[:], admin: validDigest[:], membership: 1, head: -1, known: 1},
		{name: "known", projectID: "project-bad-known", channel: validDigest[:], generation: validDigest[:], admin: validDigest[:], membership: 1, known: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.db.Exec(`
INSERT INTO continuity_sync_relay_watermarks(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, relay_head, membership_floor_known
) VALUES(?, ?, ?, ?, ?, ?, ?)`,
				test.projectID, test.channel, test.generation, test.admin, test.membership, test.head, test.known,
			); err == nil {
				t.Fatal("watermark constraints accepted malformed row")
			}
		})
	}
}

func TestContinuitySQLiteMigratesV4RelayWatermarksFromDurableSourceMaximums(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "relay-watermark-v4-migration")
	databasePath := createV4ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v4 database for seed: %v", err)
	}
	projectID := continuity.ProjectID("project-relay-watermark-v4-sources")
	channelID := testSyncChannelID("relay-watermark-v4-channel")
	relayGeneration := testAuthorityDigest(0xa1)
	adminPublicKey := testAuthorityDigest(0xa2)
	if _, err := db.Exec(`
INSERT INTO continuity_sync_projects(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, activation_state, downloaded_cursor,
  applied_cursor, relay_head
) VALUES(?, ?, ?, ?, 1, 'staging', 0, 0, ?)`,
		string(projectID), channelID[:], relayGeneration[:], adminPublicKey[:], int64(math.MaxInt64),
	); err != nil {
		db.Close()
		t.Fatalf("seed v4 sync progress source: %v", err)
	}
	canonicalDigest := testAuthorityDigest(0xa3)
	if _, err := db.Exec(`
INSERT INTO continuity_sync_authorities(
  project_id, digest_version, authority_digest, inventory_arrival_head
) VALUES(?, 2, ?, 12)`, string(projectID), canonicalDigest[:]); err != nil {
		db.Close()
		t.Fatalf("seed v4 canonical source: %v", err)
	}
	seedV4RelayWatermarkCandidate(t, db, projectID, "ready", channelID, relayGeneration, adminPublicKey, 15, 0xa4)
	seedV4RelayWatermarkCandidate(t, db, projectID, "promoted", channelID, relayGeneration, adminPublicKey, 14, 0xa5)

	candidateOnlyProjectID := continuity.ProjectID("project-relay-watermark-v4-candidate-only")
	candidateOnlyChannelID := testSyncChannelID("relay-watermark-v4-candidate-only-channel")
	candidateOnlyRelayGeneration := testAuthorityDigest(0xb1)
	candidateOnlyAdminPublicKey := testAuthorityDigest(0xb2)
	seedV4RelayWatermarkCandidate(t, db, candidateOnlyProjectID, "ready", candidateOnlyChannelID, candidateOnlyRelayGeneration, candidateOnlyAdminPublicKey, 8, 0xb3)
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded v4 database: %v", err)
	}

	store, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(v4) error = %v", err)
	}
	migrated := SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            channelID,
		RelayGeneration:      relayGeneration,
		AdminPublicKey:       adminPublicKey,
		MembershipGeneration: 1,
		RelayHead:            15,
	}
	assertSyncRelayWatermarkRowWithKnown(t, store, migrated, false)
	assertSyncRelayWatermarkRowWithKnown(t, store, SyncRelayWatermark{
		ProjectID:            candidateOnlyProjectID,
		ChannelID:            candidateOnlyChannelID,
		RelayGeneration:      candidateOnlyRelayGeneration,
		AdminPublicKey:       candidateOnlyAdminPublicKey,
		MembershipGeneration: 1,
		RelayHead:            8,
	}, false)
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), migrated); err != nil || got != migrated {
		t.Fatalf("AdvanceSyncRelayWatermark(migrated frontier) = (%#v, %v), want (%#v, nil)", got, err, migrated)
	}
	assertSyncRelayWatermarkRowWithKnown(t, store, migrated, true)
	if err := store.Close(); err != nil {
		t.Fatalf("close healed migrated store: %v", err)
	}

	store, err = Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("reopen healed v4 migration: %v", err)
	}
	defer store.Close()
	assertSyncRelayWatermarkRowWithKnown(t, store, migrated, true)
}

func TestContinuitySQLiteV4RelayWatermarkMigrationRejectsAdminEquivocationWithoutMutation(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "relay-watermark-v4-equivocation")
	databasePath := createV4ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v4 database for seed: %v", err)
	}
	projectID := continuity.ProjectID("project-relay-watermark-v4-equivocation")
	channelID := testSyncChannelID("relay-watermark-v4-equivocation-channel")
	relayGeneration := testAuthorityDigest(0xc1)
	adminPublicKey := testAuthorityDigest(0xc2)
	if _, err := db.Exec(`
INSERT INTO continuity_sync_projects(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, activation_state, downloaded_cursor,
  applied_cursor, relay_head
) VALUES(?, ?, ?, ?, 1, 'staging', 0, 0, 3)`,
		string(projectID), channelID[:], relayGeneration[:], adminPublicKey[:],
	); err != nil {
		db.Close()
		t.Fatalf("seed v4 sync progress source: %v", err)
	}
	equivocatedAdmin := adminPublicKey
	equivocatedAdmin[0] ^= 0xff
	seedV4RelayWatermarkCandidate(t, db, projectID, "ready", channelID, relayGeneration, equivocatedAdmin, 4, 0xc3)
	before := schemaIdentitySnapshot(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("close equivocated v4 database: %v", err)
	}

	if store, err := Open(stateRoot, "environment-local"); err == nil {
		store.Close()
		t.Fatal("Open(v4 admin equivocation) error = nil, want refusal")
	}
	db, err = openDatabase(databasePath)
	if err != nil {
		t.Fatalf("reopen refused v4 database: %v", err)
	}
	defer db.Close()
	after := schemaIdentitySnapshot(t, db)
	if after != before {
		t.Fatalf("refused v4 migration mutated schema identity: before=%#v after=%#v", before, after)
	}
	var watermarkTable int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name = 'continuity_sync_relay_watermarks'`).Scan(&watermarkTable); err != nil {
		t.Fatalf("inspect refused watermark table: %v", err)
	}
	if watermarkTable != 0 {
		t.Fatal("refused v4 migration retained relay watermark table")
	}
}

func testSyncRelayWatermark(projectID continuity.ProjectID, head int64) SyncRelayWatermark {
	return SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            testSyncChannelID("relay-watermark-channel"),
		RelayGeneration:      testAuthorityDigest(0x81),
		AdminPublicKey:       testAuthorityDigest(0x82),
		MembershipGeneration: 1,
		RelayHead:            head,
	}
}

func syncRelayWatermarkFromSnapshot(projectID continuity.ProjectID, snapshot SyncAuthoritySnapshot, head int64) SyncRelayWatermark {
	return SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            snapshot.ChannelID,
		RelayGeneration:      snapshot.RelayGeneration,
		AdminPublicKey:       snapshot.AdminPublicKey,
		MembershipGeneration: snapshot.MembershipGeneration,
		RelayHead:            head,
	}
}

func stageReadySyncAuthorityCandidateFromSnapshot(t *testing.T, store *Store, projectID continuity.ProjectID, snapshot SyncAuthoritySnapshot, environments []SyncEnvironmentCertificate) SyncAuthorityCandidate {
	t.Helper()
	after := ""
	var candidate SyncAuthorityCandidate
	for start := 0; start < len(environments); start += maximumSyncAuthorityCandidatePageEnvironments {
		end := start + maximumSyncAuthorityCandidatePageEnvironments
		if end > len(environments) {
			end = len(environments)
		}
		page := syncAuthorityCandidatePageV2(after, environments[start:end], end < len(environments))
		var err error
		candidate, err = store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
		if err != nil {
			t.Fatalf("stage authority candidate page: %v", err)
		}
		after = page.ThroughEnvironmentID
	}
	if !candidate.Ready {
		t.Fatalf("candidate is not ready: %#v", candidate)
	}
	return candidate
}

func assertSyncRelayWatermarkRow(t *testing.T, store *Store, want SyncRelayWatermark) {
	t.Helper()
	assertSyncRelayWatermarkRowWithKnown(t, store, want, true)
}

func assertSyncRelayWatermarkRowWithKnown(t *testing.T, store *Store, want SyncRelayWatermark, wantKnown bool) {
	t.Helper()
	var got SyncRelayWatermark
	var channel, generation, admin []byte
	var known int64
	if err := store.db.QueryRow(`
SELECT project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, relay_head, membership_floor_known
FROM continuity_sync_relay_watermarks
WHERE project_id = ? AND channel_id = ? AND relay_generation = ?`,
		string(want.ProjectID), want.ChannelID[:], want.RelayGeneration[:],
	).Scan(
		&got.ProjectID, &channel, &generation, &admin,
		&got.MembershipGeneration, &got.RelayHead, &known,
	); err != nil {
		t.Fatalf("read relay watermark: %v", err)
	}
	copy(got.ChannelID[:], channel)
	copy(got.RelayGeneration[:], generation)
	copy(got.AdminPublicKey[:], admin)
	if got != want {
		t.Fatalf("relay watermark row = %#v, want %#v", got, want)
	}
	if known != int64(boolIntV2(wantKnown)) {
		t.Fatalf("relay watermark membership floor known = %d, want %t", known, wantKnown)
	}
}

func seedV4RelayWatermarkCandidate(
	t *testing.T,
	db *sql.DB,
	projectID continuity.ProjectID,
	state string,
	channelID SyncChannelID,
	relayGeneration, adminPublicKey [32]byte,
	relayHead int64,
	seed byte,
) {
	t.Helper()
	candidateID := testAuthorityDigest(seed)
	rollingDigest := testAuthorityDigest(seed + 1)
	authorityDigest := testAuthorityDigest(seed + 2)
	if _, err := db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest
) VALUES(?, ?, ?, ?, ?, ?, 1, ?, 1, 1, 'environment-1', ?, 2, ?)`,
		string(projectID), candidateID[:], state, channelID[:], relayGeneration[:],
		adminPublicKey[:], relayHead, rollingDigest[:], authorityDigest[:],
	); err != nil {
		t.Fatalf("seed v4 %s authority candidate: %v", state, err)
	}
}

package rehearsal

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/migration/archive"
)

func TestImportStagesVerifiesAndExactlyReplaysRehearsalArchive(t *testing.T) {
	ctx := context.Background()
	encoded, sealed := rehearsalArchiveV1(t)
	store := rehearsalStoreV1(t, "environment-import")

	first, err := Import(ctx, encoded, store)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if first.RecordCount != len(sealed.Content.Records) ||
		first.Expected != sealed.Content.Expected || first.Actual != sealed.Content.Expected {
		t.Fatalf("first rehearsal report = %#v", first)
	}
	if first.Format != archive.Format || first.Version != archive.Version || first.ProjectID != sealed.Content.Project.ProjectID ||
		first.SourceBackupSHA256 != sealed.Content.Source.BackupSHA256 || first.ContentSHA256 != sealed.ContentSHA256 {
		t.Fatalf("first rehearsal identity = %#v", first)
	}

	snapshot, err := store.Snapshot(ctx, first.ProjectID, continuity.SnapshotRequest{AtMillis: 0})
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Project.Identity.Label != "Loaf" || len(snapshot.EffectiveJournal.Entries) != 2 ||
		snapshot.EffectiveJournal.Entries[0].Content.Text != "second" ||
		snapshot.EffectiveJournal.Entries[1].Content.Text != "first" ||
		len(snapshot.LatestWraps.Wraps) != 1 || snapshot.LatestWraps.Wraps[0].Synthesis != "latest wrap" {
		t.Fatalf("staged snapshot = %#v", snapshot)
	}

	replayed, err := Import(ctx, encoded, store)
	if err != nil {
		t.Fatalf("Import(replay) error = %v", err)
	}
	if replayed != first {
		t.Fatalf("replayed rehearsal report = %#v", replayed)
	}
	assertNoRehearsalSyncStateV1(t, store, first.ProjectID)
}

func TestImportResumesAnExactPersistedPrefix(t *testing.T) {
	ctx := context.Background()
	encoded, sealed := rehearsalArchiveV1(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := continuitysqlite.Open(stateRoot, "environment-prefix")
	if err != nil {
		t.Fatalf("sqlite.Open(prefix) error = %v", err)
	}
	prefix := make([]continuitysqlite.RehearsalFact, 0, 3)
	for index := 0; index < 3; index++ {
		fact, err := prepareRecordV1(sealed.Content.Project.ProjectID, sealed.Content.Records[index])
		if err != nil {
			t.Fatalf("prepareRecordV1(prefix %d) error = %v", index, err)
		}
		prefix = append(prefix, fact)
	}
	if _, err := store.ImportRehearsalFacts(ctx, sealed.Content.Project.ProjectID, prefix, acceptSnapshotV1); err != nil {
		t.Fatalf("ImportRehearsalFacts(prefix) error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Store.Close(prefix) error = %v", err)
	}
	store, err = continuitysqlite.Open(stateRoot, "environment-resume")
	if err != nil {
		t.Fatalf("sqlite.Open(resume) error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	report, err := Import(ctx, encoded, store)
	if err != nil {
		t.Fatalf("Import(resume) error = %v", err)
	}
	if report.Expected != sealed.Content.Expected || report.Actual != sealed.Content.Expected {
		t.Fatalf("resumed rehearsal report = %#v", report)
	}
}

func TestImportRejectsInvalidArchiveBeforeMutation(t *testing.T) {
	ctx := context.Background()
	encoded, sealed := rehearsalArchiveV1(t)
	store := rehearsalStoreV1(t, "environment-invalid")
	tampered := bytes.Replace(encoded, []byte(`"text":"first"`), []byte(`"text":"other"`), 1)

	report, err := Import(ctx, tampered, store)
	if err == nil || !strings.Contains(err.Error(), "digest") || report.ProjectID != "" {
		t.Fatalf("Import(tampered) = (%#v, %v)", report, err)
	}
	after, err := Import(ctx, encoded, store)
	if err != nil || after.Expected != sealed.Content.Expected || after.Actual != sealed.Content.Expected {
		t.Fatalf("Import(after tamper) = (%#v, %v), want entirely fresh destination", after, err)
	}
}

func TestImportRejectsContaminatedSameProjectProjection(t *testing.T) {
	ctx := context.Background()
	encoded, sealed := rehearsalArchiveV1(t)
	store := rehearsalStoreV1(t, "environment-contaminated")
	if _, err := Import(ctx, encoded, store); err != nil {
		t.Fatalf("Import(seed) error = %v", err)
	}
	observation := continuity.Observation{ObservedAtMillis: 9_000}
	if _, err := store.RecordJournalEntry(
		ctx, sealed.Content.Project.ProjectID, "fact-extra", "journal-extra",
		continuity.JournalRecordedPayload{
			Observation: observation,
			Content: continuity.JournalContent{
				Category: continuity.JournalNote,
				Text:     "not part of the rehearsal archive",
			},
		},
	); err != nil {
		t.Fatalf("RecordJournalEntry(contamination) error = %v", err)
	}

	report, err := Import(ctx, encoded, store)
	if err == nil || !strings.Contains(err.Error(), "exact prefix") || report.Actual != (archive.ProjectionManifest{}) {
		t.Fatalf("Import(contaminated) = (%#v, %v)", report, err)
	}
}

func TestImportRejectsHiddenTerminalHistoryBeforeMutation(t *testing.T) {
	ctx := context.Background()
	encoded, sealed := rehearsalArchiveV1(t)
	store := rehearsalStoreV1(t, "environment-hidden-history")
	projectRecord := sealed.Content.Records[0]
	project, err := prepareRecordV1(sealed.Content.Project.ProjectID, projectRecord)
	if err != nil {
		t.Fatalf("prepareRecordV1(project) error = %v", err)
	}
	if _, err := store.ImportRehearsalFacts(ctx, sealed.Content.Project.ProjectID, []continuitysqlite.RehearsalFact{project}, acceptSnapshotV1); err != nil {
		t.Fatalf("ImportRehearsalFacts(project) error = %v", err)
	}
	observation := continuity.Observation{ObservedAtMillis: 1_500}
	captured, err := store.CaptureSpark(
		ctx, sealed.Content.Project.ProjectID, "fact-hidden-spark", "hidden-spark",
		continuity.SparkCapturedPayload{Observation: observation, Text: "hidden terminal history"},
	)
	if err != nil {
		t.Fatalf("CaptureSpark() error = %v", err)
	}
	if _, err := store.DismissSpark(
		ctx, sealed.Content.Project.ProjectID, "fact-hidden-dismissal", "hidden-spark",
		continuity.SparkDismissedPayload{Observation: observation, Predecessor: captured.FactID},
	); err != nil {
		t.Fatalf("DismissSpark() error = %v", err)
	}

	report, err := Import(ctx, encoded, store)
	if err == nil || !strings.Contains(err.Error(), "exact prefix") || report.Actual != (archive.ProjectionManifest{}) {
		t.Fatalf("Import(hidden history) = (%#v, %v)", report, err)
	}
	snapshot, err := store.Snapshot(ctx, sealed.Content.Project.ProjectID, continuity.SnapshotRequest{})
	if err != nil {
		t.Fatalf("Snapshot(hidden history) error = %v", err)
	}
	if len(snapshot.EffectiveJournal.Entries) != 0 || len(snapshot.LatestWraps.Wraps) != 0 || len(snapshot.ActiveSparks.Sparks) != 0 {
		t.Fatalf("snapshot after refusal = %#v, want no archive suffix and dismissed spark hidden", snapshot)
	}
}

func TestImportRehearsalFactsRollsBackWhenSemanticVerifierFails(t *testing.T) {
	ctx := context.Background()
	_, sealed := rehearsalArchiveV1(t)
	store := rehearsalStoreV1(t, "environment-verifier-refusal")
	facts := make([]continuitysqlite.RehearsalFact, 0, len(sealed.Content.Records))
	for index, record := range sealed.Content.Records {
		fact, err := prepareRecordV1(sealed.Content.Project.ProjectID, record)
		if err != nil {
			t.Fatalf("prepareRecordV1(%d) error = %v", index, err)
		}
		facts = append(facts, fact)
	}
	if _, err := store.ImportRehearsalFacts(
		ctx, sealed.Content.Project.ProjectID, facts[:1], acceptSnapshotV1,
	); err != nil {
		t.Fatalf("ImportRehearsalFacts(prefix) error = %v", err)
	}
	forced := errors.New("forced semantic verifier refusal")
	if _, err := store.ImportRehearsalFacts(
		ctx, sealed.Content.Project.ProjectID, facts,
		func(continuity.Snapshot) error { return forced },
	); !errors.Is(err, forced) {
		t.Fatalf("ImportRehearsalFacts(forced refusal) error = %v", err)
	}
	snapshot, err := store.Snapshot(ctx, sealed.Content.Project.ProjectID, continuity.SnapshotRequest{})
	if err != nil {
		t.Fatalf("Snapshot(after verifier refusal) error = %v", err)
	}
	if snapshot.Project.Identity.Label != "Loaf" || len(snapshot.EffectiveJournal.Entries) != 0 || len(snapshot.LatestWraps.Wraps) != 0 {
		t.Fatalf("snapshot after verifier refusal = %#v, want exact project-only prefix", snapshot)
	}
}

func TestImportRejectsConflictingFactWithoutReminting(t *testing.T) {
	ctx := context.Background()
	encoded, sealed := rehearsalArchiveV1(t)
	store := rehearsalStoreV1(t, "environment-conflict")
	projectRecord := sealed.Content.Records[0]
	observation := continuity.Observation{ObservedAtMillis: projectRecord.Observation.ObservedAtMillis}
	if _, err := store.RegisterProject(
		ctx, sealed.Content.Project.ProjectID, projectRecord.FactID,
		continuity.ProjectRegistrationPayload{Observation: observation, Label: "Conflicting Label"},
	); err != nil {
		t.Fatalf("RegisterProject(conflict seed) error = %v", err)
	}

	if _, err := Import(ctx, encoded, store); err == nil || !strings.Contains(err.Error(), "exact prefix") {
		t.Fatalf("Import(conflicting fact) error = %v", err)
	}
	snapshot, err := store.Snapshot(ctx, sealed.Content.Project.ProjectID, continuity.SnapshotRequest{})
	if err != nil {
		t.Fatalf("Snapshot(conflicting fact) error = %v", err)
	}
	if snapshot.Project.Identity.Label != "Conflicting Label" || len(snapshot.EffectiveJournal.Entries) != 0 || len(snapshot.LatestWraps.Wraps) != 0 {
		t.Fatalf("conflicting snapshot = %#v, want only original conflicting project fact", snapshot)
	}
}

func TestImportRollsBackEntireSuffixWhenALaterFactConflicts(t *testing.T) {
	ctx := context.Background()
	encoded, sealed := rehearsalArchiveV1(t)
	store := rehearsalStoreV1(t, "environment-late-conflict")
	observation := continuity.Observation{ObservedAtMillis: 500}
	if _, err := store.RegisterProject(
		ctx, "other-project", "fact-other-project",
		continuity.ProjectRegistrationPayload{Observation: observation, Label: "Other"},
	); err != nil {
		t.Fatalf("RegisterProject(other) error = %v", err)
	}
	if _, err := store.RecordJournalEntry(
		ctx, "other-project", "fact-journal-2", "other-journal",
		continuity.JournalRecordedPayload{
			Observation: observation,
			Content:     continuity.JournalContent{Category: continuity.JournalNote, Text: "conflicting global fact id"},
		},
	); err != nil {
		t.Fatalf("RecordJournalEntry(other conflict) error = %v", err)
	}

	report, err := Import(ctx, encoded, store)
	if err == nil || !strings.Contains(err.Error(), "fact-conflict") || report.Actual != (archive.ProjectionManifest{}) {
		t.Fatalf("Import(late conflict) = (%#v, %v)", report, err)
	}
	if _, err := store.Snapshot(ctx, sealed.Content.Project.ProjectID, continuity.SnapshotRequest{}); err == nil {
		t.Fatal("Snapshot(target after rollback) error = nil, want no target facts")
	}
	other, err := store.Snapshot(ctx, "other-project", continuity.SnapshotRequest{})
	if err != nil {
		t.Fatalf("Snapshot(other) error = %v", err)
	}
	if other.Project.Identity.Label != "Other" || len(other.EffectiveJournal.Entries) != 1 {
		t.Fatalf("other project snapshot = %#v", other)
	}
}

func TestImportRejectsDestinationProjectWithSyncStateBeforeMutation(t *testing.T) {
	ctx := context.Background()
	encoded, sealed := rehearsalArchiveV1(t)
	store := rehearsalStoreV1(t, "environment-synced")
	var channelID continuitysqlite.SyncChannelID
	channelID[0] = 1
	var relayGeneration, adminPublicKey, certificateID [32]byte
	relayGeneration[0], adminPublicKey[0], certificateID[0] = 2, 3, 4
	if _, err := store.InstallVerifiedSyncAuthority(ctx, sealed.Content.Project.ProjectID, continuitysqlite.SyncAuthority{
		ChannelID: channelID, RelayGeneration: relayGeneration, AdminPublicKey: adminPublicKey, MembershipGeneration: 1,
		Environments: []continuitysqlite.SyncEnvironmentCertificate{{
			EnvironmentID: "environment-synced", CertificateID: certificateID, CertificateBytes: []byte{1},
			Mode: continuitysqlite.SyncEnvironmentTrusted, JoinMembershipGeneration: 1,
		}},
	}); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}

	report, err := Import(ctx, encoded, store)
	if err == nil || !strings.Contains(err.Error(), "already has sync state") || report.Actual != (archive.ProjectionManifest{}) {
		t.Fatalf("Import(sync destination) = (%#v, %v)", report, err)
	}
	if _, err := store.Snapshot(ctx, sealed.Content.Project.ProjectID, continuity.SnapshotRequest{}); err == nil {
		t.Fatal("Snapshot(sync destination) error = nil, want no imported project facts")
	}
}

func rehearsalStoreV1(t *testing.T, environmentID continuity.EnvironmentID) *continuitysqlite.Store {
	t.Helper()
	store, err := continuitysqlite.Open(filepath.Join(t.TempDir(), "state"), environmentID)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Store.Close() error = %v", err)
		}
	})
	return store
}

func acceptSnapshotV1(continuity.Snapshot) error { return nil }

func assertNoRehearsalSyncStateV1(t *testing.T, store *continuitysqlite.Store, projectID continuity.ProjectID) {
	t.Helper()
	_, err := store.CurrentSyncProgress(context.Background(), projectID)
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) || syncErr.Code != continuitysqlite.SyncErrorNotFound {
		t.Fatalf("CurrentSyncProgress() error = %v, want not-found", err)
	}
}

func rehearsalArchiveV1(t *testing.T) ([]byte, archive.Archive) {
	t.Helper()
	observation := func(at int64) archive.Observation {
		return archive.Observation{ObservedAtMillis: at, HarnessSessionID: "migration-session", Branch: "main", Worktree: "/workspace/loaf"}
	}
	content := archive.Content{
		Source: archive.Source{
			LegacySchemaVersion: 35, BackupSHA256: strings.Repeat("a", 64), BackupBytes: 4096,
			JournalFactRows: 4, JournalProjectionRows: 4,
		},
		Project:  archive.ProjectMapping{LegacyProjectID: "proj_legacy", ProjectID: "proj_legacy", Label: "Loaf"},
		Families: archive.FamilyManifest{Project: true, Journal: true, Wrap: true},
		Records: []archive.Record{
			{
				Kind: archive.RecordProject, FactID: "fact-project", SubjectID: "proj_legacy", Observation: observation(1_000),
				Project: &archive.ProjectRecord{Label: "Loaf"},
			},
			{
				Kind: archive.RecordJournal, SourceID: "legacy-journal-1", FactID: "fact-journal-1", SubjectID: "journal-1", Observation: observation(2_000),
				Journal: &archive.JournalRecord{Category: continuity.JournalDiscover, Scope: "migration", Text: "first"},
			},
			{
				Kind: archive.RecordWrap, SourceID: "legacy-wrap-1", FactID: "fact-wrap-1", SubjectID: "wrap-1", Observation: observation(3_000),
				Wrap: &archive.WrapRecord{Scope: "migration", Synthesis: "earlier wrap"},
			},
			{
				Kind: archive.RecordJournal, SourceID: "legacy-journal-2", FactID: "fact-journal-2", SubjectID: "journal-2", Observation: observation(4_000),
				Journal: &archive.JournalRecord{Category: continuity.JournalDecision, Scope: "migration", Text: "second"},
			},
			{
				Kind: archive.RecordWrap, SourceID: "legacy-wrap-2", FactID: "fact-wrap-2", SubjectID: "wrap-2", Observation: observation(5_000),
				Wrap: &archive.WrapRecord{Scope: "migration", Synthesis: "latest wrap"},
			},
		},
	}
	sealed, err := archive.Seal(content)
	if err != nil {
		t.Fatalf("archive.Seal() error = %v", err)
	}
	encoded, err := archive.Marshal(sealed)
	if err != nil {
		t.Fatalf("archive.Marshal() error = %v", err)
	}
	return encoded, sealed
}

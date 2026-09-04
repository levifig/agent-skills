package state

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/migration/archive"
	"github.com/levifig/loaf/vnext/migration/rehearsal"
)

func TestRunVNextRehearsalRoundTripsVerifiedLegacyContinuityWithoutCutover(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := LogJournal(ctx, root, resolver, JournalLogOptions{
		Entry:          "discover(migration): legacy continuity survives the handoff",
		ObservedBranch: "main", ObservedWorktree: root.Path(), HarnessSessionID: "session-roundtrip",
	}); err != nil {
		t.Fatalf("LogJournal(discover) error = %v", err)
	}
	if _, err := LogJournal(ctx, root, resolver, JournalLogOptions{
		Entry:          "wrap(migration): next is the isolated vNext rehearsal",
		ObservedBranch: "main", ObservedWorktree: root.Path(), HarnessSessionID: "session-roundtrip",
	}); err != nil {
		t.Fatalf("LogJournal(wrap) error = %v", err)
	}
	legacyHandoffBody := "# Rehearsal handoff\n\nPreserve this Markdown verbatim.\n"
	if _, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{
		Kind: "handoff", Title: "Continue isolated rehearsal", Body: legacyHandoffBody,
	}); err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	backupBefore := testFileSHA256(t, backup.BackupPath)
	liveBefore := testFileSHA256(t, status.DatabasePath)
	destination, err := continuitysqlite.Open(filepath.Join(t.TempDir(), "destination"), "environment-rehearsal")
	if err != nil {
		t.Fatalf("sqlite.Open(destination) error = %v", err)
	}
	t.Cleanup(func() {
		if err := destination.Close(); err != nil {
			t.Errorf("destination.Close() error = %v", err)
		}
	})
	options := VNextRehearsalExportOptions{
		Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver,
	}

	first, err := RunVNextRehearsal(ctx, options, destination)
	if err != nil {
		t.Fatalf("RunVNextRehearsal() error = %v", err)
	}
	sealed, err := archive.Parse(first.Archive)
	if err != nil {
		t.Fatalf("archive.Parse(result) error = %v", err)
	}
	if first.Report.ProjectID != continuity.ProjectID(status.ProjectID) ||
		first.Report.ContentSHA256 != sealed.ContentSHA256 ||
		first.Report.SourceBackupSHA256 != backup.SHA256 ||
		first.Report.Expected != sealed.Content.Expected || first.Report.Actual != sealed.Content.Expected {
		t.Fatalf("rehearsal result = %#v, archive = %#v", first.Report, sealed)
	}
	snapshot, err := destination.Snapshot(ctx, continuity.ProjectID(status.ProjectID), continuity.SnapshotRequest{})
	if err != nil {
		t.Fatalf("destination.Snapshot() error = %v", err)
	}
	if snapshot.Project.Identity.Label != status.ProjectName || len(snapshot.EffectiveJournal.Entries) != 1 ||
		snapshot.EffectiveJournal.Entries[0].Content.Text != "legacy continuity survives the handoff" ||
		len(snapshot.LatestWraps.Wraps) != 1 || snapshot.LatestWraps.Wraps[0].Synthesis != "next is the isolated vNext rehearsal" ||
		len(snapshot.LatestHandoffs.Handoffs) != 1 || snapshot.LatestHandoffs.Handoffs[0].Purpose != "Continue isolated rehearsal" ||
		snapshot.LatestHandoffs.Handoffs[0].Situation != legacyHandoffBody {
		t.Fatalf("rehearsal snapshot = %#v", snapshot)
	}
	if _, err := destination.CurrentSyncProgress(ctx, continuity.ProjectID(status.ProjectID)); err == nil {
		t.Fatal("CurrentSyncProgress() error = nil, want no sync state")
	} else {
		var syncErr *continuitysqlite.SyncError
		if !errors.As(err, &syncErr) || syncErr.Code != continuitysqlite.SyncErrorNotFound {
			t.Fatalf("CurrentSyncProgress() error = %v, want not-found", err)
		}
	}

	replayed, err := RunVNextRehearsal(ctx, options, destination)
	if err != nil {
		t.Fatalf("RunVNextRehearsal(replay) error = %v", err)
	}
	if !bytes.Equal(replayed.Archive, first.Archive) || replayed.Report != first.Report {
		t.Fatalf("replayed result differs: first=%#v replayed=%#v", first.Report, replayed.Report)
	}
	if _, err := destination.RecordJournalEntry(
		ctx, continuity.ProjectID(status.ProjectID), "fact-rehearsal-contamination", "journal-rehearsal-contamination",
		continuity.JournalRecordedPayload{
			Observation: continuity.Observation{ObservedAtMillis: 1},
			Content:     continuity.JournalContent{Category: continuity.JournalNote, Text: "destination contamination"},
		},
	); err != nil {
		t.Fatalf("RecordJournalEntry(contamination) error = %v", err)
	}
	failed, err := RunVNextRehearsal(ctx, options, destination)
	if err == nil || !bytes.Equal(failed.Archive, first.Archive) || failed.Report != (rehearsal.Report{}) {
		t.Fatalf("RunVNextRehearsal(contaminated) = (%#v, %v), want retry archive without receipt", failed, err)
	}
	if got := testFileSHA256(t, backup.BackupPath); got != backupBefore {
		t.Fatalf("backup changed during rehearsal: %s -> %s", backupBefore, got)
	}
	if got := testFileSHA256(t, status.DatabasePath); got != liveBefore {
		t.Fatalf("live database changed during rehearsal: %s -> %s", liveBefore, got)
	}
}

func TestRunVNextRehearsalRejectsNilDestinationBeforeExport(t *testing.T) {
	result, err := RunVNextRehearsal(context.Background(), VNextRehearsalExportOptions{}, nil)
	if err == nil || len(result.Archive) != 0 || result.Report.ProjectID != "" {
		t.Fatalf("RunVNextRehearsal(nil destination) = (%#v, %v)", result, err)
	}
}

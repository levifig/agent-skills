// Package rehearsal imports a sealed legacy continuity archive into an
// isolated vNext store and verifies the resulting semantic projection. It has
// no activation or cutover surface.
package rehearsal

import (
	"context"
	"fmt"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/migration/archive"
)

// Report is the deterministic semantic receipt for one successful staged
// rehearsal. It deliberately contains no activation or cutover vocabulary.
type Report struct {
	Format             string                     `json:"format"`
	Version            int                        `json:"version"`
	ProjectID          continuity.ProjectID       `json:"project_id"`
	SourceBackupSHA256 string                     `json:"source_backup_sha256"`
	ContentSHA256      string                     `json:"content_sha256"`
	RecordCount        int                        `json:"record_count"`
	Expected           archive.ProjectionManifest `json:"expected_projection"`
	Actual             archive.ProjectionManifest `json:"actual_projection"`
}

// Import parses and stages one canonical archive into store, then derives the
// actual vNext snapshot and verifies it against the archive manifest. Exact
// fact replays are idempotent, so a failed or interrupted isolated rehearsal
// can be retried without minting replacement identities.
func Import(ctx context.Context, encoded []byte, store *continuitysqlite.Store) (Report, error) {
	report := Report{}
	if ctx == nil {
		return report, fmt.Errorf("import vNext rehearsal archive: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if store == nil {
		return report, fmt.Errorf("import vNext rehearsal archive: destination store is nil")
	}
	sealed, err := archive.Parse(encoded)
	if err != nil {
		return report, fmt.Errorf("import vNext rehearsal archive: %w", err)
	}
	report.Format = sealed.Format
	report.Version = sealed.Version
	report.ProjectID = sealed.Content.Project.ProjectID
	report.SourceBackupSHA256 = sealed.Content.Source.BackupSHA256
	report.ContentSHA256 = sealed.ContentSHA256
	report.RecordCount = len(sealed.Content.Records)
	report.Expected = sealed.Content.Expected
	facts := make([]continuitysqlite.RehearsalFact, 0, len(sealed.Content.Records))
	for index, record := range sealed.Content.Records {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		fact, err := prepareRecordV1(report.ProjectID, record)
		if err != nil {
			return report, fmt.Errorf("import vNext rehearsal archive: prepare record %d (%s): %w", index, record.Kind, err)
		}
		facts = append(facts, fact)
	}
	_, err = store.ImportRehearsalFacts(ctx, report.ProjectID, facts, func(snapshot continuity.Snapshot) error {
		actual, verifyErr := archive.ProjectionManifestForSnapshot(snapshot)
		if verifyErr != nil {
			return fmt.Errorf("derive staged projection: %w", verifyErr)
		}
		report.Actual = actual
		if report.Actual != report.Expected {
			return fmt.Errorf("staged projection does not match archive manifest")
		}
		return nil
	})
	if err != nil {
		return report, fmt.Errorf("import vNext rehearsal archive: stage facts: %w", err)
	}
	return report, nil
}

func prepareRecordV1(
	projectID continuity.ProjectID,
	record archive.Record,
) (continuitysqlite.RehearsalFact, error) {
	observation := continuity.Observation{
		ObservedAtMillis: record.Observation.ObservedAtMillis,
		HarnessSessionID: record.Observation.HarnessSessionID,
		Branch:           record.Observation.Branch,
		Worktree:         record.Observation.Worktree,
	}
	switch record.Kind {
	case archive.RecordProject:
		return continuitysqlite.NewProjectRehearsalFact(projectID, record.FactID, continuity.ProjectRegistrationPayload{
			Observation: observation,
			Label:       record.Project.Label,
		})
	case archive.RecordJournal:
		return continuitysqlite.NewJournalRehearsalFact(projectID, record.FactID, record.SubjectID, continuity.JournalRecordedPayload{
			Observation: observation,
			Content: continuity.JournalContent{
				Category: record.Journal.Category,
				Scope:    record.Journal.Scope,
				Text:     record.Journal.Text,
			},
		})
	case archive.RecordWrap:
		return continuitysqlite.NewWrapRehearsalFact(projectID, record.FactID, record.SubjectID, continuity.WrapRecordedPayload{
			Observation: observation,
			Scope:       record.Wrap.Scope,
			Synthesis:   record.Wrap.Synthesis,
		})
	case archive.RecordHandoff:
		suggestedSkills := make([]string, len(record.Handoff.SuggestedSkills))
		copy(suggestedSkills, record.Handoff.SuggestedSkills)
		return continuitysqlite.NewHandoffRehearsalFact(projectID, record.FactID, record.SubjectID, continuity.HandoffRecordedPayload{
			Observation: observation,
			Purpose:     record.Handoff.Purpose, Situation: record.Handoff.Situation,
			NextActions: record.Handoff.NextActions, QuestionsAndRisks: record.Handoff.QuestionsAndRisks,
			SuggestedSkills: suggestedSkills,
		})
	default:
		return continuitysqlite.RehearsalFact{}, fmt.Errorf("record kind %q is unsupported by rehearsal importer version 1", record.Kind)
	}
}

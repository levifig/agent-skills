package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/levifig/loaf/vnext/continuity"
)

const projectionDigestDomainV1 = "loaf:vnext:migration:projection:v1\x00"

type semanticProjectionV1 struct {
	Project []semanticRecordV1 `json:"project"`
	Journal []semanticRecordV1 `json:"journal"`
	Wrap    []semanticRecordV1 `json:"wrap"`
}

type semanticRecordV1 struct {
	FactID      string         `json:"fact_id"`
	SubjectID   string         `json:"subject_id"`
	Observation Observation    `json:"observation"`
	Project     *ProjectRecord `json:"project,omitempty"`
	Journal     *JournalRecord `json:"journal,omitempty"`
	Wrap        *WrapRecord    `json:"wrap,omitempty"`
}

// ProjectionManifestForRecords derives the semantic vNext projection for one
// project-first, oldest-first archive record sequence. It excludes local
// writer clocks and environment identities assigned during rehearsal import.
func ProjectionManifestForRecords(records []Record) (ProjectionManifest, error) {
	return expectedProjectionV1(records)
}

func expectedProjectionV1(records []Record) (ProjectionManifest, error) {
	projection := semanticProjectionV1{
		Project: []semanticRecordV1{}, Journal: []semanticRecordV1{}, Wrap: []semanticRecordV1{},
	}
	if len(records) != 0 {
		record := records[0]
		semantic := semanticRecordV1{
			FactID: string(record.FactID), SubjectID: string(record.SubjectID), Observation: record.Observation,
			Project: record.Project, Journal: record.Journal, Wrap: record.Wrap,
		}
		if record.Kind != RecordProject {
			return ProjectionManifest{}, fmt.Errorf("cannot project archive without a leading project record")
		}
		projection.Project = append(projection.Project, semantic)
	}
	for index := len(records) - 1; index >= 1; index-- {
		record := records[index]
		semantic := semanticRecordV1{
			FactID: string(record.FactID), SubjectID: string(record.SubjectID), Observation: record.Observation,
			Project: record.Project, Journal: record.Journal, Wrap: record.Wrap,
		}
		switch record.Kind {
		case RecordJournal:
			projection.Journal = append(projection.Journal, semantic)
		case RecordWrap:
			if len(projection.Wrap) == 0 {
				projection.Wrap = append(projection.Wrap, semantic)
			}
		default:
			return ProjectionManifest{}, fmt.Errorf("cannot project unsupported archive record kind %q", record.Kind)
		}
	}
	return projectionManifestV1(projection)
}

// ProjectionManifestForSnapshot derives the same rehearsal semantic digest
// from an actual vNext continuity snapshot. It rejects unrelated families so a
// contaminated destination cannot masquerade as an isolated rehearsal.
func ProjectionManifestForSnapshot(snapshot continuity.Snapshot) (ProjectionManifest, error) {
	if len(snapshot.ActiveSparks.Sparks) != 0 || len(snapshot.CurrentIdeas.Ideas) != 0 ||
		len(snapshot.CurrentDecisions.Decisions) != 0 || len(snapshot.Explorations.Explorations) != 0 ||
		len(snapshot.LatestCheckpoints.Checkpoints) != 0 || len(snapshot.CurrentFindings.Findings) != 0 ||
		len(snapshot.LatestHandoffs.Handoffs) != 0 || len(snapshot.Scratchpads.Scratchpads) != 0 ||
		len(snapshot.ExternalReferences.References) != 0 || len(snapshot.VerificationEvidence.Evidence) != 0 {
		return ProjectionManifest{}, fmt.Errorf("rehearsal snapshot contains continuity families outside archive version 1")
	}
	identity := snapshot.Project.Identity
	projection := semanticProjectionV1{
		Project: []semanticRecordV1{{
			FactID: string(identity.Record.Root.FactID), SubjectID: string(identity.Record.Subject.ID),
			Observation: observationFromContinuityV1(identity.RegisteredObservation),
			Project:     &ProjectRecord{Label: identity.Label},
		}},
		Journal: make([]semanticRecordV1, 0, len(snapshot.EffectiveJournal.Entries)),
		Wrap:    make([]semanticRecordV1, 0, len(snapshot.LatestWraps.Wraps)),
	}
	for _, entry := range snapshot.EffectiveJournal.Entries {
		projection.Journal = append(projection.Journal, semanticRecordV1{
			FactID: string(entry.Record.Root.FactID), SubjectID: string(entry.Record.Subject.ID),
			Observation: observationFromContinuityV1(entry.RecordedObservation),
			Journal: &JournalRecord{
				Category: entry.Content.Category, Scope: entry.Content.Scope, Text: entry.Content.Text,
			},
		})
	}
	for _, wrap := range snapshot.LatestWraps.Wraps {
		if wrap.Focus != nil {
			return ProjectionManifest{}, fmt.Errorf("archive version 1 rehearsal snapshot contains a focused wrap")
		}
		projection.Wrap = append(projection.Wrap, semanticRecordV1{
			FactID: string(wrap.Record.Root.FactID), SubjectID: string(wrap.Record.Subject.ID),
			Observation: observationFromContinuityV1(wrap.HeadObservation),
			Wrap:        &WrapRecord{Scope: wrap.Scope, Synthesis: wrap.Synthesis},
		})
	}
	return projectionManifestV1(projection)
}

func projectionManifestV1(projection semanticProjectionV1) (ProjectionManifest, error) {
	encoded, err := canonicalJSONV1(projection)
	if err != nil {
		return ProjectionManifest{}, fmt.Errorf("encode expected projection: %w", err)
	}
	digest := sha256.Sum256(append([]byte(projectionDigestDomainV1), encoded...))
	return ProjectionManifest{
		ProjectCount:          len(projection.Project),
		EffectiveJournalCount: len(projection.Journal),
		WrapCount:             len(projection.Wrap),
		SHA256:                hex.EncodeToString(digest[:]),
	}, nil
}

func observationFromContinuityV1(observation continuity.Observation) Observation {
	return Observation{
		ObservedAtMillis: observation.ObservedAtMillis,
		HarnessSessionID: observation.HarnessSessionID,
		Branch:           observation.Branch,
		Worktree:         observation.Worktree,
	}
}

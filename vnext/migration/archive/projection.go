package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

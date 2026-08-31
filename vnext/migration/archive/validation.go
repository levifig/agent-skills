package archive

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/levifig/loaf/vnext/continuity"
)

func validateContentV1(content Content, verifyExpected bool) error {
	if content.Source.LegacySchemaVersion < 1 {
		return fmt.Errorf("legacy schema version must be positive")
	}
	if !validSHA256V1(content.Source.BackupSHA256) {
		return fmt.Errorf("backup sha256 must be 64 lowercase hexadecimal characters")
	}
	if content.Source.BackupBytes < 1 || content.Source.JournalFactRows < 0 ||
		content.Source.JournalProjectionRows < 0 || content.Source.CollapsedRevisionRows < 0 ||
		content.Source.JournalOriginRows < 0 || content.Source.DroppedSpecLinks < 0 ||
		content.Source.DroppedTaskLinks < 0 {
		return fmt.Errorf("source counts and backup bytes must be nonnegative with a nonempty backup")
	}
	if content.Source.CollapsedRevisionRows != content.Source.JournalFactRows-content.Source.JournalProjectionRows {
		return fmt.Errorf("collapsed revision count must equal journal fact rows minus projection rows")
	}
	if strings.TrimSpace(content.Project.LegacyProjectID) == "" {
		return fmt.Errorf("legacy project id must not be empty")
	}
	if err := content.Project.ProjectID.Validate(); err != nil {
		return fmt.Errorf("destination project id: %w", err)
	}
	if content.Project.LegacyProjectID != string(content.Project.ProjectID) {
		return fmt.Errorf("archive version 1 must preserve the legacy project id exactly")
	}
	if !content.Families.Project || !content.Families.Journal || !content.Families.Wrap {
		return fmt.Errorf("archive version 1 requires project, journal, and wrap families")
	}
	if content.Families.Ideas || content.Families.Sparks || content.Families.Handoffs ||
		content.Families.Scratchpads || content.Families.Decisions || content.Families.Explorations ||
		content.Families.Findings || content.Families.CompleteForCutover {
		return fmt.Errorf("archive version 1 is rehearsal-only and cannot include later families or cutover completeness")
	}
	if len(content.Records) == 0 || content.Records[0].Kind != RecordProject {
		return fmt.Errorf("project registration must be the first archive record")
	}
	factIDs := make(map[continuity.FactID]struct{}, len(content.Records))
	subjects := make(map[string]struct{}, len(content.Records))
	projectRecords := 0
	timelineRecords := 0
	for index := range content.Records {
		record := content.Records[index]
		if err := validateRecordV1(record, content.Project, index); err != nil {
			return err
		}
		if _, duplicate := factIDs[record.FactID]; duplicate {
			return fmt.Errorf("record %d duplicates fact id %q", index, record.FactID)
		}
		factIDs[record.FactID] = struct{}{}
		subjectKey := string(record.Kind) + "\x00" + string(record.SubjectID)
		if _, duplicate := subjects[subjectKey]; duplicate {
			return fmt.Errorf("record %d duplicates %s subject %q", index, record.Kind, record.SubjectID)
		}
		subjects[subjectKey] = struct{}{}
		if record.Kind == RecordProject {
			projectRecords++
		} else {
			timelineRecords++
		}
	}
	if projectRecords != 1 {
		return fmt.Errorf("archive must contain exactly one project record")
	}
	if timelineRecords != content.Source.JournalProjectionRows {
		return fmt.Errorf("journal projection row count must equal archived journal and wrap records")
	}
	expected, err := expectedProjectionV1(content.Records)
	if err != nil {
		return err
	}
	if verifyExpected && content.Expected != expected {
		return fmt.Errorf("expected projection manifest does not match archive records")
	}
	return nil
}

func validateRecordV1(record Record, project ProjectMapping, index int) error {
	if err := record.FactID.Validate(); err != nil {
		return fmt.Errorf("record %d fact id: %w", index, err)
	}
	if err := record.SubjectID.Validate(); err != nil {
		return fmt.Errorf("record %d subject id: %w", index, err)
	}
	observation := record.Observation.continuityV1()
	if err := observation.Validate(); err != nil {
		return fmt.Errorf("record %d observation: %w", index, err)
	}
	payloads := 0
	for _, present := range []bool{record.Project != nil, record.Journal != nil, record.Wrap != nil} {
		if present {
			payloads++
		}
	}
	if payloads != 1 {
		return fmt.Errorf("record %d must contain exactly one typed payload", index)
	}
	switch record.Kind {
	case RecordProject:
		if record.SourceID != "" {
			return fmt.Errorf("record %d project source id must be empty", index)
		}
		if record.Project == nil || record.Journal != nil || record.Wrap != nil {
			return fmt.Errorf("record %d project tag does not match its payload", index)
		}
		if continuity.ProjectID(record.SubjectID) != project.ProjectID || record.Project.Label != project.Label {
			return fmt.Errorf("record %d project payload does not match project mapping", index)
		}
		if err := (continuity.ProjectRegistrationPayload{Observation: observation, Label: record.Project.Label}).Validate(); err != nil {
			return fmt.Errorf("record %d project payload: %w", index, err)
		}
	case RecordJournal:
		if strings.TrimSpace(record.SourceID) == "" {
			return fmt.Errorf("record %d journal source id must not be empty", index)
		}
		if record.Journal == nil || record.Project != nil || record.Wrap != nil {
			return fmt.Errorf("record %d journal tag does not match its payload", index)
		}
		if record.Journal.Category == continuity.JournalWrap {
			return fmt.Errorf("record %d deliberate wrap must use a wrap record", index)
		}
		payload := continuity.JournalRecordedPayload{Observation: observation, Content: continuity.JournalContent{
			Category: record.Journal.Category, Scope: record.Journal.Scope, Text: record.Journal.Text,
		}}
		if err := payload.Validate(); err != nil {
			return fmt.Errorf("record %d journal payload: %w", index, err)
		}
	case RecordWrap:
		if strings.TrimSpace(record.SourceID) == "" {
			return fmt.Errorf("record %d wrap source id must not be empty", index)
		}
		if record.Wrap == nil || record.Project != nil || record.Journal != nil {
			return fmt.Errorf("record %d wrap tag does not match its payload", index)
		}
		payload := continuity.WrapRecordedPayload{
			Observation: observation, Scope: record.Wrap.Scope, Synthesis: record.Wrap.Synthesis,
		}
		if err := payload.Validate(); err != nil {
			return fmt.Errorf("record %d wrap payload: %w", index, err)
		}
	default:
		return fmt.Errorf("record %d kind %q is not supported by archive version 1", index, record.Kind)
	}
	return nil
}

func (observation Observation) continuityV1() continuity.Observation {
	return continuity.Observation{
		ObservedAtMillis: observation.ObservedAtMillis,
		HarnessSessionID: observation.HarnessSessionID,
		Branch:           observation.Branch,
		Worktree:         observation.Worktree,
	}
}

func validSHA256V1(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

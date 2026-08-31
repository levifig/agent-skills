package archive

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestSealMarshalParseIsDeterministicAndIntegrityProtected(t *testing.T) {
	content := validContentV1()
	first, err := Seal(content)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	second, err := Seal(content)
	if err != nil {
		t.Fatalf("Seal(replay) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Seal() is not deterministic:\nfirst  = %#v\nsecond = %#v", first, second)
	}
	encoded, err := Marshal(first)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	replayed, err := Marshal(second)
	if err != nil {
		t.Fatalf("Marshal(replay) error = %v", err)
	}
	if !bytes.Equal(encoded, replayed) || len(encoded) == 0 || encoded[len(encoded)-1] != '\n' {
		t.Fatalf("canonical archive bytes differ or lack final newline")
	}
	parsed, err := Parse(encoded)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !reflect.DeepEqual(parsed, first) {
		t.Fatalf("Parse(Marshal(archive)) = %#v, want %#v", parsed, first)
	}

	tampered := bytes.Replace(encoded, []byte(`"text":"learned"`), []byte(`"text":"altered"`), 1)
	if _, err := Parse(tampered); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("Parse(tampered) error = %v, want digest failure", err)
	}
	unknown := bytes.Replace(encoded, []byte(`"format":`), []byte(`"unknown":true,"format":`), 1)
	if _, err := Parse(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Parse(unknown field) error = %v", err)
	}
	if _, err := Parse(bytes.TrimSuffix(encoded, []byte{'\n'})); err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("Parse(without final newline) error = %v", err)
	}
}

func TestSealBuildsExpectedSemanticProjectionWithoutDuplicatingWraps(t *testing.T) {
	content := validContentV1()
	content.Source.JournalFactRows++
	content.Source.JournalProjectionRows++
	content.Records = append(content.Records, Record{
		Kind: RecordWrap, SourceID: "legacy-wrap-2", FactID: "fact-wrap-2", SubjectID: "wrap-2",
		Observation: Observation{ObservedAtMillis: 2_000},
		Wrap:        &WrapRecord{Scope: "migration", Synthesis: "latest wrap"},
	})
	sealed, err := Seal(content)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	if sealed.Content.Expected.ProjectCount != 1 || sealed.Content.Expected.EffectiveJournalCount != 1 || sealed.Content.Expected.WrapCount != 1 {
		t.Fatalf("expected projection counts = %#v", sealed.Content.Expected)
	}
	if sealed.Content.Expected.SHA256 == "" {
		t.Fatal("expected projection digest is empty")
	}
	recomputed, err := ProjectionManifestForRecords(sealed.Content.Records)
	if err != nil || recomputed != sealed.Content.Expected {
		t.Fatalf("ProjectionManifestForRecords() = (%#v, %v), want %#v", recomputed, err, sealed.Content.Expected)
	}
	if sealed.Content.Families.CompleteForCutover {
		t.Fatal("partial continuity archive declared itself cutover-complete")
	}
}

func TestSealRejectsInvalidOrderDuplicateSubjectsAndIncompleteFamilyDeclaration(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Content)
	}{
		{name: "project record is not first", mutate: func(content *Content) {
			content.Records[0], content.Records[1] = content.Records[1], content.Records[0]
		}},
		{name: "duplicate journal subject", mutate: func(content *Content) {
			content.Records = append(content.Records, content.Records[1])
			content.Records[len(content.Records)-1].FactID = "fact-journal-copy"
		}},
		{name: "missing journal family", mutate: func(content *Content) {
			content.Families.Journal = false
		}},
		{name: "premature cutover", mutate: func(content *Content) {
			content.Families.CompleteForCutover = true
		}},
		{name: "project id remap", mutate: func(content *Content) {
			content.Project.ProjectID = "proj_remapped"
			content.Records[0].SubjectID = "proj_remapped"
		}},
		{name: "unknown journal category", mutate: func(content *Content) {
			content.Records[1].Journal.Category = "legacy_session"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := validContentV1()
			test.mutate(&content)
			if _, err := Seal(content); err == nil {
				t.Fatal("Seal(invalid content) error = nil")
			}
		})
	}
}

func validContentV1() Content {
	observation := Observation{
		ObservedAtMillis: 1_000,
		HarnessSessionID: "migration-session",
		Branch:           "main",
		Worktree:         "/workspace/loaf",
	}
	return Content{
		Source: Source{
			LegacySchemaVersion:   35,
			BackupSHA256:          strings.Repeat("a", 64),
			BackupBytes:           4096,
			JournalFactRows:       2,
			JournalProjectionRows: 2,
		},
		Project: ProjectMapping{
			LegacyProjectID: "proj_legacy",
			ProjectID:       "proj_legacy",
			Label:           "Loaf",
		},
		Families: FamilyManifest{Project: true, Journal: true, Wrap: true},
		Records: []Record{
			{
				Kind: RecordProject, FactID: "fact-project", SubjectID: "proj_legacy", Observation: observation,
				Project: &ProjectRecord{Label: "Loaf"},
			},
			{
				Kind: RecordJournal, SourceID: "legacy-journal-1", FactID: "fact-journal", SubjectID: "journal-1", Observation: observation,
				Journal: &JournalRecord{Category: continuity.JournalDiscover, Scope: "migration", Text: "learned"},
			},
			{
				Kind: RecordWrap, SourceID: "legacy-wrap-1", FactID: "fact-wrap", SubjectID: "wrap-1", Observation: observation,
				Wrap: &WrapRecord{Scope: "migration", Synthesis: "next is staged import"},
			},
		},
	}
}

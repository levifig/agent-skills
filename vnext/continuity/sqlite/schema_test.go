package sqlite

import (
	"strings"
	"testing"
)

func TestContinuitySQLiteSchemaRejectsInvalidFacts(t *testing.T) {
	t.Parallel()

	store, err := Open(testTempDir(t), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	testsBeforeIdentity := []struct {
		name        string
		factID      string
		projectID   string
		subjectKind string
		subjectID   string
		factKind    string
		content     string
		sequence    int
	}{
		{name: "missing identity", factID: "fact-missing", projectID: "project-missing", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "journal.recorded", content: `{}`, sequence: 1},
		{name: "unknown subject", factID: "fact-subject", projectID: "project-a", subjectKind: "derived-context", subjectID: "context-1", factKind: "journal.recorded", content: `{}`, sequence: 1},
		{name: "unknown fact", factID: "fact-kind", projectID: "project-a", subjectKind: "project-identity", subjectID: "project-a", factKind: "project.unknown", content: `{}`, sequence: 1},
		{name: "mismatched project identity", factID: "fact-mismatch", projectID: "project-a", subjectKind: "project-identity", subjectID: "project-b", factKind: "project.registered", content: `{}`, sequence: 1},
	}
	for _, test := range testsBeforeIdentity {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := rawInsertFact(store, test.factID, test.projectID, test.subjectKind, test.subjectID, test.factKind, test.content, test.sequence); err == nil {
				t.Fatal("insert error = nil, want schema refusal")
			}
		})
	}

	if err := rawInsertFact(store, "fact-project", "project-a", "project-identity", "project-a", "project.registered", `{}`, 1); err != nil {
		t.Fatalf("insert valid project identity: %v", err)
	}

	testsAfterIdentity := []struct {
		name        string
		factID      string
		subjectKind string
		subjectID   string
		factKind    string
		content     string
		sequence    int
	}{
		{name: "duplicate identity", factID: "fact-project-2", subjectKind: "project-identity", subjectID: "project-a", factKind: "project.registered", content: `{}`, sequence: 2},
		{name: "kind mismatch", factID: "fact-mismatch-2", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "wrap.recorded", content: `{}`, sequence: 2},
		{name: "invalid json", factID: "fact-json", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "journal.recorded", content: `{`, sequence: 2},
		{name: "non-object json", factID: "fact-array", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "journal.recorded", content: `[]`, sequence: 2},
		{name: "oversized json", factID: "fact-large", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "journal.recorded", content: `{"value":"` + strings.Repeat("x", 1048576) + `"}`, sequence: 2},
		{name: "duplicate environment sequence", factID: "fact-sequence", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "journal.recorded", content: `{}`, sequence: 1},
	}
	for _, test := range testsAfterIdentity {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := rawInsertFact(store, test.factID, "project-a", test.subjectKind, test.subjectID, test.factKind, test.content, test.sequence); err == nil {
				t.Fatal("insert error = nil, want schema refusal")
			}
		})
	}
}

func rawInsertFact(store *Store, factID, projectID, subjectKind, subjectID, factKind, content string, sequence int) error {
	_, err := store.db.Exec(`
INSERT INTO continuity_facts(
  fact_id,
  project_id,
  subject_kind,
  subject_id,
  fact_kind,
  payload_version,
  content_json,
  environment_id,
  environment_sequence,
  hlc_wall_millis,
  hlc_logical,
  envelope_version
) VALUES(?, ?, ?, ?, ?, 1, ?, 'environment-a', ?, 1, 0, 1)`,
		factID,
		projectID,
		subjectKind,
		subjectID,
		factKind,
		content,
		sequence,
	)
	return err
}

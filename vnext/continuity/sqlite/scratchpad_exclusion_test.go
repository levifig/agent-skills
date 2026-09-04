package sqlite

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func TestCurrentStoreSurfaceExcludesScratchpad(t *testing.T) {
	t.Parallel()

	storeType := reflect.TypeOf((*Store)(nil))
	for _, method := range []string{
		"OpenScratchpad",
		"IntroduceScratchpadParticipant",
		"RecordScratchpadMessage",
		"RecordScratchpadClaim",
		"ReleaseScratchpadClaim",
		"CloseScratchpad",
		"ApplyVerifiedPrune",
	} {
		if _, ok := storeType.MethodByName(method); ok {
			t.Fatalf("Store exposes deferred scratchpad operation %s", method)
		}
	}
}

func syncScratchpadFactV1(t *testing.T, projectID continuity.ProjectID, factID continuity.FactID, subjectID continuity.SubjectID, kind continuity.FactKind, environmentID continuity.EnvironmentID, sequence, wall int64) continuitywire.Fact {
	t.Helper()
	observation := `{"observed_at_millis":1,"harness_session_id":"conversation-1","branch":"issue/loaf-93","worktree":"/workspace/loaf"}`
	var content string
	switch kind {
	case "scratchpad.opened":
		content = fmt.Sprintf(`{"observation":%s,"focus":null,"label":%q}`, observation, subjectID)
	case "scratchpad.participant-introduced":
		content = fmt.Sprintf(`{"observation":%s,"participant_id":"participant-1","name":"agent","focus":null}`, observation)
	case "scratchpad.message-recorded":
		content = fmt.Sprintf(`{"observation":%s,"participant_id":"participant-1","text":"message"}`, observation)
	default:
		t.Fatalf("unsupported legacy scratchpad fixture kind %q", kind)
	}
	return syncFact(projectID, factID, continuity.RecordKind("scratchpad"), subjectID, kind, canonicalContentV1(content), environmentID, sequence, wall)
}

func TestSnapshotIgnoresAndPreservesLegacyScratchpadRows(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, testTempDir(t), "environment-a", 100)
	projectID := continuity.ProjectID("project-legacy-scratchpad")
	if _, err := store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Label: "Loaf"}); err != nil {
		t.Fatalf("RegisterProject: %v", err)
	}
	_, err := store.db.ExecContext(context.Background(), `
INSERT INTO continuity_facts(
  fact_id, project_id, subject_kind, subject_id, fact_kind,
  payload_version, content_json, environment_id, environment_sequence,
  hlc_wall_millis, hlc_logical, envelope_version
) VALUES(?, ?, 'scratchpad', 'legacy-scratchpad', 'scratchpad.opened', 1,
  '{"focus":null,"label":"Legacy","observation":{"branch":"","harness_session_id":"","observed_at_millis":0,"worktree":""}}',
  'environment-a', 2, 100, 1, 1)`, "fact-legacy-scratchpad", string(projectID))
	if err != nil {
		t.Fatalf("insert legacy scratchpad row: %v", err)
	}

	if _, err := store.Snapshot(context.Background(), projectID, continuity.SnapshotRequest{}); err != nil {
		t.Fatalf("Snapshot with legacy scratchpad row: %v", err)
	}
	var rows int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM continuity_facts WHERE fact_id = 'fact-legacy-scratchpad'`).Scan(&rows); err != nil {
		t.Fatalf("count legacy scratchpad rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("legacy scratchpad row count = %d, want preserved row", rows)
	}
	if _, err := store.ExportFact(context.Background(), "fact-legacy-scratchpad"); err == nil {
		t.Fatal("legacy scratchpad row was exposed through the current sync export")
	}
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM continuity_facts WHERE fact_id = 'fact-legacy-scratchpad'`).Scan(&rows); err != nil {
		t.Fatalf("count legacy scratchpad rows after sync export: %v", err)
	}
	if rows != 1 {
		t.Fatalf("legacy scratchpad row count after sync export = %d, want preserved row", rows)
	}
}

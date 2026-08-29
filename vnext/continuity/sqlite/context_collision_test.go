package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestContinuityContextResolvesCheckpointFocusFromCanonicalLeastMint(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-root", 100)
	projectID := continuity.ProjectID("project-checkpoint-collision")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Collision"}))
	mustAppendV1(t)(store.StartExploration(context.Background(), projectID, "fact-exploration-a", "exploration-a", continuity.ExplorationStartedPayload{Observation: snapshotObservationV1(2, "main"), Label: "A", Purpose: "A"}))
	mustAppendV1(t)(store.StartExploration(context.Background(), projectID, "fact-exploration-z", "exploration-z", continuity.ExplorationStartedPayload{Observation: snapshotObservationV1(3, "main"), Label: "Z", Purpose: "Z"}))

	checkpointContent := func(explorationID continuity.SubjectID, framing string) canonicalContentV1 {
		return canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
			return encodeCheckpointRecordedV1(continuity.CheckpointRecordedPayload{
				Observation:        snapshotObservationV1(4, "main"),
				ExplorationID:      explorationID,
				CurrentFraming:     framing,
				Conclusions:        framing,
				UnresolvedQuestion: framing + "?",
				NextAction:         framing,
			})
		})
	}
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-checkpoint-collision-a", continuity.RecordCheckpoint, "checkpoint-collision", continuity.FactCheckpointRecorded, checkpointContent("exploration-a", "canonical"), "environment-a", 1, 200, 0))
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-checkpoint-collision-z", continuity.RecordCheckpoint, "checkpoint-collision", continuity.FactCheckpointRecorded, checkpointContent("exploration-z", "losing"), "environment-z", 1, 201, 0))
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-checkpoint-latest-a", continuity.RecordCheckpoint, "checkpoint-latest-a", continuity.FactCheckpointRecorded, checkpointContent("exploration-a", "latest-a"), "environment-a", 2, 202, 0))
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-checkpoint-latest-z", continuity.RecordCheckpoint, "checkpoint-latest-z", continuity.FactCheckpointRecorded, checkpointContent("exploration-z", "latest-z"), "environment-z", 2, 203, 0))

	focus := continuity.SubjectRef{Kind: continuity.RecordCheckpoint, ID: "checkpoint-collision"}
	digest := mustContextV1(t, store, projectID, continuity.ContextRequest{Focus: &focus})
	if got := contextSubjectIDsV1(digest.Checkpoints.Checkpoints); len(got) != 1 || got[0] != "checkpoint-latest-a" {
		t.Fatalf("checkpoint collision resolved to %v, want canonical exploration latest checkpoint", got)
	}
}

package continuity

import (
	"errors"
	"reflect"
	"testing"
)

func TestProjectionRequestsValidateDeterministicSelectors(t *testing.T) {
	t.Parallel()

	if err := (SnapshotRequest{AtMillis: 0}).Validate(); err != nil {
		t.Fatalf("zero snapshot instant: %v", err)
	}
	if err := (ContextRequest{
		Focus:    &SubjectRef{Kind: RecordDecision, ID: "decision_1"},
		Scope:    "release%_literal",
		Branch:   "Issue/LOAF-96",
		AtMillis: 42,
	}).Validate(); err != nil {
		t.Fatalf("valid context selectors: %v", err)
	}

	tests := []struct {
		name   string
		err    error
		field  string
		detail string
	}{
		{name: "snapshot instant", err: (SnapshotRequest{AtMillis: -1}).Validate(), field: "at_millis", detail: "must be zero or positive"},
		{name: "context instant", err: (ContextRequest{AtMillis: -1}).Validate(), field: "at_millis", detail: "must be zero or positive"},
		{name: "focus identity", err: (ContextRequest{Focus: &SubjectRef{Kind: RecordIdea}, AtMillis: 0}).Validate(), field: "focus.id", detail: "must contain 1 to 128 characters"},
		{name: "scratchpad focus", err: (ContextRequest{Focus: &SubjectRef{Kind: RecordScratchpad, ID: "scratch_1"}, AtMillis: 0}).Validate(), field: "focus.kind", detail: "is not available to derived context"},
		{name: "evidence focus", err: (ContextRequest{Focus: &SubjectRef{Kind: RecordVerificationEvidence, ID: "evidence_1"}, AtMillis: 0}).Validate(), field: "focus.kind", detail: "is not available to derived context"},
		{name: "computed focus", err: (ContextRequest{Focus: &SubjectRef{Kind: RecordDerivedContext, ID: "context_1"}, AtMillis: 0}).Validate(), field: "focus.kind", detail: "is not available to derived context"},
		{name: "scope whitespace", err: (ContextRequest{Scope: " scope", AtMillis: 0}).Validate(), field: "scope", detail: "must not have outer whitespace"},
		{name: "branch control", err: (ContextRequest{Branch: "main\rnext", AtMillis: 0}).Validate(), field: "branch", detail: "must use LF line endings"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var problem *Problem
			if !errors.As(test.err, &problem) {
				t.Fatalf("error = %v, want continuity problem", test.err)
			}
			if problem.Code != ProblemInvalid || problem.Field != test.field || problem.Detail != test.detail {
				t.Fatalf("problem = %#v, want invalid at %s: %s", problem, test.field, test.detail)
			}
		})
	}
}

func TestProjectionClosedStatesAndSnapshotShape(t *testing.T) {
	t.Parallel()

	if got, want := []IdeaDisposition{IdeaActive, IdeaResolved, IdeaArchived, IdeaPromoted}, []IdeaDisposition{"active", "resolved", "archived", "promoted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("idea dispositions = %v, want %v", got, want)
	}
	if got, want := []DecisionState{DecisionOpen, DecisionResolved, DecisionSuperseded}, []DecisionState{"open", "resolved", "superseded"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("decision states = %v, want %v", got, want)
	}
	if got, want := []FindingState{FindingCurrent, FindingRetracted}, []FindingState{"current", "retracted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("finding states = %v, want %v", got, want)
	}
	if got, want := []ScratchpadState{ScratchpadOpen, ScratchpadClosed}, []ScratchpadState{"open", "closed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("scratchpad states = %v, want %v", got, want)
	}

	snapshot := Snapshot{
		EffectiveJournal:     EffectiveJournalProjection{Entries: []JournalEntry{}},
		LatestWraps:          LatestWrapsProjection{Wraps: []Wrap{}},
		ActiveSparks:         ActiveSparksProjection{Sparks: []Spark{}},
		CurrentIdeas:         CurrentIdeasProjection{Ideas: []Idea{}},
		CurrentDecisions:     CurrentDecisionsProjection{Decisions: []Decision{}},
		Explorations:         ExplorationsProjection{Explorations: []Exploration{}},
		LatestCheckpoints:    LatestCheckpointsProjection{Checkpoints: []Checkpoint{}},
		CurrentFindings:      CurrentFindingsProjection{Findings: []Finding{}},
		LatestHandoffs:       LatestHandoffsProjection{Handoffs: []Handoff{}},
		Scratchpads:          ScratchpadsProjection{Scratchpads: []Scratchpad{}},
		ExternalReferences:   ExternalReferencesProjection{References: []ExternalReference{}},
		VerificationEvidence: VerificationEvidenceProjection{Evidence: []VerificationEvidence{}},
	}
	if snapshot.EffectiveJournal.Entries == nil || snapshot.LatestWraps.Wraps == nil || snapshot.ActiveSparks.Sparks == nil ||
		snapshot.CurrentIdeas.Ideas == nil || snapshot.CurrentDecisions.Decisions == nil || snapshot.Explorations.Explorations == nil ||
		snapshot.LatestCheckpoints.Checkpoints == nil || snapshot.CurrentFindings.Findings == nil || snapshot.LatestHandoffs.Handoffs == nil ||
		snapshot.Scratchpads.Scratchpads == nil || snapshot.ExternalReferences.References == nil || snapshot.VerificationEvidence.Evidence == nil {
		t.Fatal("snapshot collection fields must support explicit non-nil empty slices")
	}

	digest := ContextDigest{
		FocusedJournal:       ContextJournalLayer{Entries: []JournalEntry{}},
		ProjectJournal:       ContextJournalLayer{Entries: []JournalEntry{}},
		Wraps:                ContextWrapLayer{Wraps: []Wrap{}},
		Sparks:               ContextSparkLayer{Sparks: []Spark{}},
		Ideas:                ContextIdeaLayer{Ideas: []Idea{}},
		Decisions:            ContextDecisionLayer{Decisions: []Decision{}},
		Checkpoints:          ContextCheckpointLayer{Checkpoints: []Checkpoint{}},
		Findings:             ContextFindingLayer{Findings: []Finding{}},
		Handoffs:             ContextHandoffLayer{Handoffs: []Handoff{}},
		ExternalReferences:   ContextExternalReferenceLayer{References: []ContextExternalReference{}},
		VerificationEvidence: ContextVerificationEvidenceLayer{Evidence: []VerificationEvidence{}},
	}
	if digest.FocusedJournal.Entries == nil || digest.ProjectJournal.Entries == nil || digest.Wraps.Wraps == nil ||
		digest.Sparks.Sparks == nil || digest.Ideas.Ideas == nil || digest.Decisions.Decisions == nil ||
		digest.Checkpoints.Checkpoints == nil || digest.Findings.Findings == nil || digest.Handoffs.Handoffs == nil ||
		digest.ExternalReferences.References == nil || digest.VerificationEvidence.Evidence == nil {
		t.Fatal("context collection fields must support explicit non-nil empty slices")
	}
}

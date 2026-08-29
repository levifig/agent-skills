package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func TestApplyVerifiedPruneSuccessRetryAndStaleReplay(t *testing.T) {
	fixture := newPruneFixtureV1(t)
	before := fixture.inventory(t)

	if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err != nil {
		t.Fatalf("ApplyVerifiedPrune() error = %v", err)
	}
	fixture.assertPruned(t, before)
	if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err != nil {
		t.Fatalf("ApplyVerifiedPrune(exact retry) error = %v", err)
	}
	fixture.assertPruned(t, before)

	stale := fixture.frames[2]
	if _, err := fixture.store.ApplySyncBatch(context.Background(), fixture.projectID, []VerifiedSyncFrame{stale}, 1_000, 100); err == nil {
		t.Fatal("ApplySyncBatch(stale pruned fact) error = nil")
	}
	fixture.assertPruned(t, before)
}

func TestApplyVerifiedPruneBlocksLocalAppendFactIDResurrection(t *testing.T) {
	tests := []struct {
		name      string
		projectID func(*testing.T, *pruneFixtureV1) continuity.ProjectID
	}{
		{
			name: "same project tombstone",
			projectID: func(t *testing.T, fixture *pruneFixtureV1) continuity.ProjectID {
				mustAppendV1(t)(fixture.store.RecordJournalEntry(context.Background(), fixture.projectID, "fact-local-seed", "journal-local-seed", continuity.JournalRecordedPayload{
					Observation: appendObservationV1(),
					Content:     continuity.JournalContent{Category: continuity.JournalNote, Text: "seed local environment head"},
				}))
				return fixture.projectID
			},
		},
		{
			name: "other project tombstone",
			projectID: func(t *testing.T, fixture *pruneFixtureV1) continuity.ProjectID {
				projectID := continuity.ProjectID("project-local-other")
				mustAppendV1(t)(fixture.store.RegisterProject(context.Background(), projectID, "fact-project-other", continuity.ProjectRegistrationPayload{
					Observation: appendObservationV1(),
					Label:       "Other",
				}))
				return projectID
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPruneFixtureV1(t)
			appendProjectID := test.projectID(t, fixture)
			if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err != nil {
				t.Fatalf("ApplyVerifiedPrune() error = %v", err)
			}
			before := fixture.inventory(t)
			factsBefore := fixture.count(t, `SELECT COUNT(*) FROM continuity_facts`)
			tombstonesBefore := fixture.count(t, `SELECT COUNT(*) FROM continuity_sync_tombstones`)
			headBefore := readPruneEnvironmentHeadV1(t, fixture.store, appendProjectID, "environment-local")

			_, err := fixture.store.RecordJournalEntry(context.Background(), appendProjectID, fixture.participant.FactID, "journal-resurrection", continuity.JournalRecordedPayload{
				Observation: appendObservationV1(),
				Content:     continuity.JournalContent{Category: continuity.JournalNote, Text: "must not resurrect a pruned fact id"},
			})
			assertProblemCodeV1(t, err, continuity.ProblemFactConflict)
			fixture.assertInventory(t, before)
			if got := fixture.count(t, `SELECT COUNT(*) FROM continuity_facts`); got != factsBefore {
				t.Fatalf("global fact count = %d, want %d", got, factsBefore)
			}
			if got := fixture.count(t, `SELECT COUNT(*) FROM continuity_sync_tombstones`); got != tombstonesBefore {
				t.Fatalf("global tombstone count = %d, want %d", got, tombstonesBefore)
			}
			if got := readPruneEnvironmentHeadV1(t, fixture.store, appendProjectID, "environment-local"); got != headBefore {
				t.Fatalf("local environment head changed: got %#v, want %#v", got, headBefore)
			}
			if got := fixture.count(t, `SELECT COUNT(*) FROM continuity_facts AS fact JOIN continuity_sync_tombstones AS tombstone ON tombstone.fact_id = fact.fact_id WHERE fact.fact_id = ?`, string(fixture.participant.FactID)); got != 0 {
				t.Fatalf("live fact/tombstone pairs = %d, want 0", got)
			}
		})
	}
}

func TestApplyVerifiedPruneRejectsReferenceTampering(t *testing.T) {
	tests := []struct {
		name string
		edit func(*VerifiedPruneReference)
	}{
		{name: "fact id", edit: func(value *VerifiedPruneReference) { value.FactID = "fact-message-other" }},
		{name: "environment id", edit: func(value *VerifiedPruneReference) { value.EnvironmentID = "environment-b" }},
		{name: "environment sequence", edit: func(value *VerifiedPruneReference) { value.EnvironmentSequence++ }},
		{name: "arrival sequence", edit: func(value *VerifiedPruneReference) { value.ArrivalSequence++ }},
		{name: "envelope digest", edit: func(value *VerifiedPruneReference) { value.EnvelopeDigest[0] ^= 0xff }},
		{name: "certificate id", edit: func(value *VerifiedPruneReference) { value.CertificateID[0] ^= 0xff }},
		{name: "previous envelope digest", edit: func(value *VerifiedPruneReference) { value.PreviousEnvelopeDigest[0] ^= 0xff }},
		{name: "key generation", edit: func(value *VerifiedPruneReference) { value.KeyGeneration++ }},
		{name: "nonce", edit: func(value *VerifiedPruneReference) { value.Nonce[0] ^= 0xff }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPruneFixtureV1(t)
			before := fixture.inventory(t)
			plan := cloneVerifiedPrunePlanV1(fixture.plan)
			test.edit(&plan.Targets[0])
			if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, plan); err == nil {
				t.Fatal("ApplyVerifiedPrune(tampered) error = nil")
			}
			fixture.assertInventory(t, before)
		})
	}
}

func TestApplyVerifiedPruneRejectsAlteredExactRetries(t *testing.T) {
	tests := []struct {
		name string
		edit func(*VerifiedPruneReference)
	}{
		{name: "fact id", edit: func(value *VerifiedPruneReference) { value.FactID = "fact-message-other" }},
		{name: "environment id", edit: func(value *VerifiedPruneReference) { value.EnvironmentID = "environment-b" }},
		{name: "environment sequence", edit: func(value *VerifiedPruneReference) { value.EnvironmentSequence++ }},
		{name: "arrival sequence", edit: func(value *VerifiedPruneReference) { value.ArrivalSequence++ }},
		{name: "envelope digest", edit: func(value *VerifiedPruneReference) { value.EnvelopeDigest[0] ^= 0xff }},
		{name: "certificate id", edit: func(value *VerifiedPruneReference) { value.CertificateID[0] ^= 0xff }},
		{name: "previous envelope digest", edit: func(value *VerifiedPruneReference) { value.PreviousEnvelopeDigest[0] ^= 0xff }},
		{name: "key generation", edit: func(value *VerifiedPruneReference) { value.KeyGeneration++ }},
		{name: "nonce", edit: func(value *VerifiedPruneReference) { value.Nonce[0] ^= 0xff }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPruneFixtureV1(t)
			if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err != nil {
				t.Fatalf("ApplyVerifiedPrune() error = %v", err)
			}
			before := fixture.inventory(t)
			altered := cloneVerifiedPrunePlanV1(fixture.plan)
			test.edit(&altered.Targets[0])
			if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, altered); err == nil {
				t.Fatal("ApplyVerifiedPrune(altered retry) error = nil")
			}
			fixture.assertInventory(t, before)
		})
	}
}

func TestApplyVerifiedPruneValidatesPlanAndStoreLiveness(t *testing.T) {
	fixture := newPruneFixtureV1(t)
	before := fixture.inventory(t)
	tests := []struct {
		name string
		edit func(*VerifiedPrunePlan)
	}{
		{name: "zero channel", edit: func(plan *VerifiedPrunePlan) { plan.ChannelID = SyncChannelID{} }},
		{name: "zero membership", edit: func(plan *VerifiedPrunePlan) { plan.MembershipGeneration = 0 }},
		{name: "zero barrier", edit: func(plan *VerifiedPrunePlan) { plan.BarrierArrivalSequence = 0 }},
		{name: "closure past barrier", edit: func(plan *VerifiedPrunePlan) { plan.BarrierArrivalSequence = plan.Closure.ArrivalSequence - 1 }},
		{name: "target past barrier", edit: func(plan *VerifiedPrunePlan) { plan.Targets[0].ArrivalSequence = plan.BarrierArrivalSequence + 1 }},
		{name: "zero prune certificate", edit: func(plan *VerifiedPrunePlan) { plan.PruneCertificateID = [32]byte{} }},
		{name: "empty targets", edit: func(plan *VerifiedPrunePlan) { plan.Targets = nil }},
		{name: "too many targets", edit: func(plan *VerifiedPrunePlan) {
			plan.Targets = make([]VerifiedPruneReference, maximumVerifiedPruneTargets+1)
		}},
		{name: "invalid closure", edit: func(plan *VerifiedPrunePlan) { plan.Closure = VerifiedPruneReference{} }},
		{name: "invalid target", edit: func(plan *VerifiedPrunePlan) { plan.Targets[0] = VerifiedPruneReference{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := cloneVerifiedPrunePlanV1(fixture.plan)
			test.edit(&plan)
			if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, plan); err == nil {
				t.Fatal("ApplyVerifiedPrune(invalid plan) error = nil")
			}
			fixture.assertInventory(t, before)
		})
	}

	if err := (*Store)(nil).ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
		t.Fatal("nil Store.ApplyVerifiedPrune() error = nil")
	}
	if err := fixture.store.ApplyVerifiedPrune(nil, fixture.projectID, fixture.plan); err == nil {
		t.Fatal("ApplyVerifiedPrune(nil context) error = nil")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fixture.store.ApplyVerifiedPrune(canceled, fixture.projectID, fixture.plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("ApplyVerifiedPrune(canceled context) error = %v, want context.Canceled", err)
	}
	closed := openSyncStore(t, "prune-closed")
	if err := closed.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := closed.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
		t.Fatal("closed Store.ApplyVerifiedPrune() error = nil")
	}
	fixture.assertInventory(t, before)
}

func TestApplyVerifiedPruneRejectsPlanAndStateConflicts(t *testing.T) {
	tests := []struct {
		name string
		edit func(*pruneFixtureV1)
	}{
		{name: "wrong channel", edit: func(f *pruneFixtureV1) { f.plan.ChannelID = testSyncChannelID("other") }},
		{name: "wrong membership", edit: func(f *pruneFixtureV1) { f.plan.MembershipGeneration++ }},
		{name: "barrier ahead", edit: func(f *pruneFixtureV1) { f.plan.BarrierArrivalSequence++ }},
		{name: "staging activation", edit: func(f *pruneFixtureV1) {
			f.exec(t, `UPDATE continuity_sync_projects SET activation_state = 'staging' WHERE project_id = ?`, string(f.projectID))
		}},
		{name: "download convergence", edit: func(f *pruneFixtureV1) {
			f.exec(t, `UPDATE continuity_sync_projects SET relay_head = relay_head + 1 WHERE project_id = ?`, string(f.projectID))
		}},
		{name: "inbox not empty", edit: func(f *pruneFixtureV1) {
			f.exec(t, `INSERT INTO continuity_sync_inbox(project_id, arrival_sequence, envelope_digest, sealed_envelope, state) VALUES(?, 99, ?, X'01', 'staged')`, string(f.projectID), pruneDigestBytesV1("inbox"))
		}},
		{name: "quarantined inbox not empty", edit: func(f *pruneFixtureV1) {
			f.exec(t, `INSERT INTO continuity_sync_inbox(project_id, arrival_sequence, envelope_digest, sealed_envelope, state) VALUES(?, 99, ?, X'01', 'quarantined')`, string(f.projectID), pruneDigestBytesV1("quarantined-inbox"))
		}},
		{name: "outbox not empty", edit: func(f *pruneFixtureV1) {
			f.insertOutboxFor(t, f.opening)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPruneFixtureV1(t)
			test.edit(fixture)
			before := fixture.inventory(t)
			if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
				t.Fatal("ApplyVerifiedPrune(conflict) error = nil")
			}
			fixture.assertInventory(t, before)
		})
	}
}

func TestApplyVerifiedPruneRejectsForbiddenAndInvalidTargetSets(t *testing.T) {
	tests := []struct {
		name string
		edit func(*pruneFixtureV1)
	}{
		{name: "opened target", edit: func(f *pruneFixtureV1) { f.plan.Targets = []VerifiedPruneReference{f.opening} }},
		{name: "closed target", edit: func(f *pruneFixtureV1) { f.plan.Targets = []VerifiedPruneReference{f.closure} }},
		{name: "permanent target", edit: func(f *pruneFixtureV1) { f.plan.Targets = []VerifiedPruneReference{f.root} }},
		{name: "cross scratchpad", edit: func(f *pruneFixtureV1) { f.plan.Targets = []VerifiedPruneReference{f.otherMessage} }},
		{name: "opening closure", edit: func(f *pruneFixtureV1) { f.plan.Closure = f.opening }},
		{name: "permanent closure", edit: func(f *pruneFixtureV1) { f.plan.Closure = f.root }},
		{name: "closure also target", edit: func(f *pruneFixtureV1) { f.plan.Targets = []VerifiedPruneReference{f.closure} }},
		{name: "unsorted arrivals", edit: func(f *pruneFixtureV1) { f.plan.Targets = []VerifiedPruneReference{f.message, f.participant} }},
		{name: "duplicate fact", edit: func(f *pruneFixtureV1) { f.plan.Targets = []VerifiedPruneReference{f.participant, f.participant} }},
		{name: "duplicate source", edit: func(f *pruneFixtureV1) {
			other := f.message
			other.EnvironmentID = f.participant.EnvironmentID
			other.EnvironmentSequence = f.participant.EnvironmentSequence
			f.plan.Targets = []VerifiedPruneReference{f.participant, other}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPruneFixtureV1(t)
			before := fixture.inventory(t)
			test.edit(fixture)
			if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
				t.Fatal("ApplyVerifiedPrune(invalid targets) error = nil")
			}
			fixture.assertInventory(t, before)
		})
	}
}

func TestApplyVerifiedPruneRejectsReceiptAndRetryConflictsAtomically(t *testing.T) {
	t.Run("missing closure fact", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		fixture.exec(t, `DELETE FROM continuity_facts WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(fixture.closure.FactID))
		before := fixture.inventory(t)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(missing closure fact) error = nil")
		}
		fixture.assertInventory(t, before)
	})

	t.Run("missing target fact", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		fixture.exec(t, `DELETE FROM continuity_facts WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(fixture.message.FactID))
		before := fixture.inventory(t)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(missing target fact) error = nil")
		}
		fixture.assertInventory(t, before)
	})

	t.Run("missing closure receipt", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		fixture.exec(t, `DELETE FROM continuity_sync_receipts WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(fixture.closure.FactID))
		before := fixture.inventory(t)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(missing closure receipt) error = nil")
		}
		fixture.assertInventory(t, before)
	})

	t.Run("missing target receipt", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		fixture.exec(t, `DELETE FROM continuity_sync_receipts WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(fixture.message.FactID))
		before := fixture.inventory(t)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(missing receipt) error = nil")
		}
		fixture.assertInventory(t, before)
	})

	t.Run("mismatched closure receipt", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		fixture.exec(t, `UPDATE continuity_sync_receipts SET envelope_digest = ? WHERE project_id = ? AND fact_id = ?`, pruneDigestBytesV1("wrong"), string(fixture.projectID), string(fixture.closure.FactID))
		before := fixture.inventory(t)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(mismatched closure receipt) error = nil")
		}
		fixture.assertInventory(t, before)
	})

	t.Run("mixed live and tombstoned", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		firstOnly := cloneVerifiedPrunePlanV1(fixture.plan)
		firstOnly.Targets = firstOnly.Targets[1:]
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, firstOnly); err != nil {
			t.Fatalf("ApplyVerifiedPrune(first target) error = %v", err)
		}
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(mixed retry) error = nil")
		}
		if got := fixture.count(t, `SELECT COUNT(*) FROM continuity_facts WHERE fact_id = ?`, string(fixture.participant.FactID)); got != 1 {
			t.Fatalf("live target fact count = %d, want 1 after rollback", got)
		}
	})

	t.Run("altered retry certificate", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err != nil {
			t.Fatalf("ApplyVerifiedPrune() error = %v", err)
		}
		before := fixture.inventory(t)
		altered := cloneVerifiedPrunePlanV1(fixture.plan)
		altered.PruneCertificateID[0] ^= 0xff
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, altered); err == nil {
			t.Fatal("ApplyVerifiedPrune(altered retry) error = nil")
		}
		fixture.assertInventory(t, before)
	})

	t.Run("retained fold rejects dangling message", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		before := fixture.inventory(t)
		fixture.plan.Targets = []VerifiedPruneReference{fixture.participant}
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(dangling retained message) error = nil")
		}
		fixture.assertInventory(t, before)
	})
}

func TestApplyVerifiedPruneRollsBackForcedWritePhaseFailure(t *testing.T) {
	fixture := newPruneFixtureV1(t)
	before := fixture.inventory(t)
	fixture.exec(t, `CREATE TEMP TRIGGER fail_second_prune_delete
BEFORE DELETE ON continuity_facts
WHEN OLD.fact_id = 'fact-message'
BEGIN
  SELECT RAISE(ABORT, 'forced prune write failure');
END`)

	if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
		t.Fatal("ApplyVerifiedPrune(forced second-target delete failure) error = nil")
	}
	fixture.assertInventory(t, before)
	for _, target := range fixture.plan.Targets {
		if got := fixture.count(t, `SELECT COUNT(*) FROM continuity_facts WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(target.FactID)); got != 1 {
			t.Errorf("target fact %q count = %d, want 1 after rollback", target.FactID, got)
		}
		if got := fixture.count(t, `SELECT COUNT(*) FROM continuity_sync_tombstones WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(target.FactID)); got != 0 {
			t.Errorf("target tombstone %q count = %d, want 0 after rollback", target.FactID, got)
		}
	}
}

func TestApplyVerifiedPruneRejectsMalformedPersistedEnvelopeMetadata(t *testing.T) {
	t.Run("receipt oversized key generation", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		fixture.corruptEnvelopeMetadataV1(t, "continuity_sync_receipts", fixture.participant.FactID, "key_generation = 4294967296")
		before := fixture.inventory(t)
		metadataBefore := fixture.receiptMetadataV1(t, fixture.participant.FactID)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(oversized receipt key generation) error = nil")
		}
		fixture.assertInventory(t, before)
		if got := fixture.receiptMetadataV1(t, fixture.participant.FactID); got != metadataBefore {
			t.Fatalf("receipt metadata changed: got %#v, want %#v", got, metadataBefore)
		}
	})

	t.Run("receipt bad fixed width", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		fixture.corruptEnvelopeMetadataV1(t, "continuity_sync_receipts", fixture.participant.FactID, "envelope_digest = X'01'")
		before := fixture.inventory(t)
		metadataBefore := fixture.receiptMetadataV1(t, fixture.participant.FactID)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(short receipt digest) error = nil")
		}
		fixture.assertInventory(t, before)
		if got := fixture.receiptMetadataV1(t, fixture.participant.FactID); got != metadataBefore {
			t.Fatalf("receipt metadata changed: got %#v, want %#v", got, metadataBefore)
		}
	})

	t.Run("tombstone oversized key generation", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err != nil {
			t.Fatalf("ApplyVerifiedPrune() error = %v", err)
		}
		fixture.corruptEnvelopeMetadataV1(t, "continuity_sync_tombstones", fixture.participant.FactID, "key_generation = 4294967296")
		before := fixture.inventory(t)
		metadataBefore := fixture.tombstoneMetadataV1(t, fixture.participant.FactID)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(oversized tombstone key generation) error = nil")
		}
		fixture.assertInventory(t, before)
		if got := fixture.tombstoneMetadataV1(t, fixture.participant.FactID); got != metadataBefore {
			t.Fatalf("tombstone metadata changed: got %#v, want %#v", got, metadataBefore)
		}
	})

	t.Run("tombstone bad fixed width", func(t *testing.T) {
		fixture := newPruneFixtureV1(t)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err != nil {
			t.Fatalf("ApplyVerifiedPrune() error = %v", err)
		}
		fixture.corruptEnvelopeMetadataV1(t, "continuity_sync_tombstones", fixture.participant.FactID, "nonce = X'01'")
		before := fixture.inventory(t)
		metadataBefore := fixture.tombstoneMetadataV1(t, fixture.participant.FactID)
		if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err == nil {
			t.Fatal("ApplyVerifiedPrune(short tombstone nonce) error = nil")
		}
		fixture.assertInventory(t, before)
		if got := fixture.tombstoneMetadataV1(t, fixture.participant.FactID); got != metadataBefore {
			t.Fatalf("tombstone metadata changed: got %#v, want %#v", got, metadataBefore)
		}
	})
}

type pruneFixtureV1 struct {
	store        *Store
	projectID    continuity.ProjectID
	frames       []VerifiedSyncFrame
	plan         VerifiedPrunePlan
	root         VerifiedPruneReference
	opening      VerifiedPruneReference
	participant  VerifiedPruneReference
	message      VerifiedPruneReference
	claim        VerifiedPruneReference
	release      VerifiedPruneReference
	closure      VerifiedPruneReference
	otherMessage VerifiedPruneReference
}

type pruneInventoryV1 struct {
	facts, receipts, tombstones, heads, inbox, outbox int
	downloaded, applied, relayHead                    int64
	activation                                        string
	channelHex, relayHex, adminHex                    string
	membership                                        uint32
	headHighest, headWall, headLogical, headSealed    int64
	headPreviousHex, headDigestHex, headCertHex       string
	headKeyGeneration                                 uint32
	headNonceHex                                      string
}

type pruneEnvironmentHeadV1 struct {
	highestSequence int64
	wallMillis      int64
	logical         int64
	sealedSequence  int64
	previousHex     string
	digestHex       string
	certificateHex  string
	keyGeneration   uint32
	nonceHex        string
}

type pruneEnvelopeMetadataFingerprintV1 struct {
	keyGeneration     int64
	previousDigestHex string
	digestHex         string
	certificateHex    string
	nonceHex          string
	pruneCertHex      string
}

func newPruneFixtureV1(t *testing.T) *pruneFixtureV1 {
	t.Helper()
	store := openSyncStore(t, "prune-"+syncSlug(t.Name()))
	projectID := continuity.ProjectID("project-prune-" + syncSlug(t.Name()))
	facts := []continuitywire.Fact{
		syncProjectFact(t, projectID, "fact-project", "environment-a", 1, 100),
		syncScratchpadFactV1(t, projectID, "fact-open", "scratchpad-a", continuity.FactScratchpadOpened, "environment-a", 2, 101),
		syncScratchpadFactV1(t, projectID, "fact-participant", "scratchpad-a", continuity.FactScratchpadParticipantIntroduced, "environment-a", 3, 102),
		syncScratchpadFactV1(t, projectID, "fact-message", "scratchpad-a", continuity.FactScratchpadMessageRecorded, "environment-a", 4, 103),
		syncScratchpadFactV1(t, projectID, "fact-claim", "scratchpad-a", continuity.FactScratchpadClaimRecorded, "environment-a", 5, 104),
		syncScratchpadFactV1(t, projectID, "fact-release", "scratchpad-a", continuity.FactScratchpadClaimReleased, "environment-a", 6, 105),
		syncScratchpadFactV1(t, projectID, "fact-close", "scratchpad-a", continuity.FactScratchpadClosed, "environment-a", 7, 106),
		syncScratchpadFactV1(t, projectID, "fact-open-other", "scratchpad-b", continuity.FactScratchpadOpened, "environment-a", 8, 107),
		syncScratchpadFactV1(t, projectID, "fact-participant-other", "scratchpad-b", continuity.FactScratchpadParticipantIntroduced, "environment-a", 9, 108),
		syncScratchpadFactV1(t, projectID, "fact-message-other", "scratchpad-b", continuity.FactScratchpadMessageRecorded, "environment-a", 10, 109),
		syncScratchpadFactV1(t, projectID, "fact-close-other", "scratchpad-b", continuity.FactScratchpadClosed, "environment-a", 11, 110),
	}
	frames := stageSyncFacts(t, store, projectID, 1, facts)
	if _, err := store.ApplySyncBatch(context.Background(), projectID, frames, 1_000, 100); err != nil {
		t.Fatalf("ApplySyncBatch() error = %v", err)
	}
	if _, err := store.db.Exec(`UPDATE continuity_sync_projects SET activation_state = 'attached' WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("attach fixture: %v", err)
	}
	references := make([]VerifiedPruneReference, len(frames))
	for index, frame := range frames {
		references[index] = pruneReferenceFromFrameV1(frame)
	}
	certificate := pruneDigestV1("prune-certificate")
	return &pruneFixtureV1{
		store: store, projectID: projectID, frames: frames,
		root: references[0], opening: references[1], participant: references[2], message: references[3], claim: references[4], release: references[5], closure: references[6], otherMessage: references[9],
		plan: VerifiedPrunePlan{
			ChannelID: testSyncChannelID("channel-a"), MembershipGeneration: testSyncAuthority().MembershipGeneration,
			BarrierArrivalSequence: int64(len(frames)), PruneCertificateID: certificate,
			Closure: references[6], Targets: []VerifiedPruneReference{references[2], references[3], references[4], references[5]},
		},
	}
}

func syncScratchpadFactV1(t *testing.T, projectID continuity.ProjectID, factID continuity.FactID, subjectID continuity.SubjectID, kind continuity.FactKind, environmentID continuity.EnvironmentID, sequence, wall int64) continuitywire.Fact {
	t.Helper()
	observation := appendObservationV1()
	var content canonicalContentV1
	var err error
	switch kind {
	case continuity.FactScratchpadOpened:
		content, err = encodeScratchpadOpenedV1(continuity.ScratchpadOpenedPayload{Observation: observation, Label: string(subjectID)})
	case continuity.FactScratchpadParticipantIntroduced:
		content, err = encodeScratchpadParticipantV1(continuity.ScratchpadParticipantPayload{Observation: observation, ParticipantID: "participant-1", Name: "agent"})
	case continuity.FactScratchpadMessageRecorded:
		content, err = encodeScratchpadMessageV1(continuity.ScratchpadMessagePayload{Observation: observation, ParticipantID: "participant-1", Text: "message"})
	case continuity.FactScratchpadClaimRecorded:
		content, err = encodeScratchpadClaimV1(continuity.ScratchpadClaimPayload{Observation: observation, ClaimID: "claim-1", ParticipantID: "participant-1", Resource: "resource-1", ExpiresAtMillis: observation.ObservedAtMillis + 1})
	case continuity.FactScratchpadClaimReleased:
		content, err = encodeScratchpadClaimReleaseV1(continuity.ScratchpadClaimReleasePayload{Observation: observation, ClaimID: "claim-1", ReleasedBy: "participant-1", Reason: "done"})
	case continuity.FactScratchpadClosed:
		content, err = encodeScratchpadCloseV1(continuity.ScratchpadClosePayload{Observation: observation, ClosedBy: "participant-1", Reason: "done"})
	default:
		t.Fatalf("unsupported scratchpad fixture kind %q", kind)
	}
	if err != nil {
		t.Fatalf("encode %s: %v", kind, err)
	}
	return syncFact(projectID, factID, continuity.RecordScratchpad, subjectID, kind, content, environmentID, sequence, wall)
}

func pruneReferenceFromFrameV1(frame VerifiedSyncFrame) VerifiedPruneReference {
	return VerifiedPruneReference{
		FactID: frame.Fact.FactID, EnvironmentID: frame.Fact.EnvironmentID, EnvironmentSequence: frame.Fact.EnvironmentSequence,
		ArrivalSequence: frame.ArrivalSequence, EnvelopeDigest: frame.EnvelopeDigest, CertificateID: frame.CertificateID,
		PreviousEnvelopeDigest: frame.PreviousEnvelopeDigest, KeyGeneration: frame.KeyGeneration, Nonce: frame.Nonce,
	}
}

func pruneDigestV1(label string) [32]byte { return sha256.Sum256([]byte(label)) }

func pruneDigestBytesV1(label string) []byte {
	digest := pruneDigestV1(label)
	return digest[:]
}

func cloneVerifiedPrunePlanV1(plan VerifiedPrunePlan) VerifiedPrunePlan {
	clone := plan
	clone.Targets = append([]VerifiedPruneReference(nil), plan.Targets...)
	return clone
}

func (fixture *pruneFixtureV1) inventory(t *testing.T) pruneInventoryV1 {
	t.Helper()
	value := pruneInventoryV1{
		facts:      fixture.count(t, `SELECT COUNT(*) FROM continuity_facts WHERE project_id = ?`, string(fixture.projectID)),
		receipts:   fixture.count(t, `SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?`, string(fixture.projectID)),
		tombstones: fixture.count(t, `SELECT COUNT(*) FROM continuity_sync_tombstones WHERE project_id = ?`, string(fixture.projectID)),
		heads:      fixture.count(t, `SELECT COUNT(*) FROM continuity_sync_environment_heads WHERE project_id = ?`, string(fixture.projectID)),
		inbox:      fixture.count(t, `SELECT COUNT(*) FROM continuity_sync_inbox WHERE project_id = ?`, string(fixture.projectID)),
		outbox:     fixture.count(t, `SELECT COUNT(*) FROM continuity_sync_outbox WHERE project_id = ?`, string(fixture.projectID)),
	}
	if err := fixture.store.db.QueryRow(`SELECT activation_state, downloaded_cursor, applied_cursor, relay_head FROM continuity_sync_projects WHERE project_id = ?`, string(fixture.projectID)).Scan(&value.activation, &value.downloaded, &value.applied, &value.relayHead); err != nil {
		t.Fatalf("read sync progress: %v", err)
	}
	if err := fixture.store.db.QueryRow(`SELECT hex(channel_id), hex(relay_generation), hex(admin_public_key), membership_generation FROM continuity_sync_projects WHERE project_id = ?`, string(fixture.projectID)).Scan(&value.channelHex, &value.relayHex, &value.adminHex, &value.membership); err != nil {
		t.Fatalf("read sync authority identity: %v", err)
	}
	if err := fixture.store.db.QueryRow(`SELECT highest_sequence, hlc_wall_millis, hlc_logical, sealed_sequence, hex(previous_envelope_digest), hex(envelope_digest), hex(certificate_id), key_generation, hex(nonce) FROM continuity_sync_environment_heads WHERE project_id = ? AND environment_id = 'environment-a'`, string(fixture.projectID)).Scan(
		&value.headHighest, &value.headWall, &value.headLogical, &value.headSealed,
		&value.headPreviousHex, &value.headDigestHex, &value.headCertHex, &value.headKeyGeneration, &value.headNonceHex,
	); err != nil {
		t.Fatalf("read exact environment head: %v", err)
	}
	return value
}

func (fixture *pruneFixtureV1) assertInventory(t *testing.T, want pruneInventoryV1) {
	t.Helper()
	if got := fixture.inventory(t); got != want {
		t.Fatalf("inventory = %#v, want %#v", got, want)
	}
}

func (fixture *pruneFixtureV1) assertPruned(t *testing.T, before pruneInventoryV1) {
	t.Helper()
	want := before
	want.facts -= len(fixture.plan.Targets)
	want.tombstones += len(fixture.plan.Targets)
	fixture.assertInventory(t, want)
	for _, retained := range []VerifiedPruneReference{fixture.root, fixture.opening, fixture.closure, fixture.otherMessage} {
		if got := fixture.count(t, `SELECT COUNT(*) FROM continuity_facts WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(retained.FactID)); got != 1 {
			t.Errorf("retained fact %q count = %d, want 1", retained.FactID, got)
		}
	}
	for _, target := range fixture.plan.Targets {
		if got := fixture.count(t, `SELECT COUNT(*) FROM continuity_facts WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(target.FactID)); got != 0 {
			t.Errorf("pruned fact %q count = %d, want 0", target.FactID, got)
		}
		if got := fixture.count(t, `SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(target.FactID)); got != 1 {
			t.Errorf("retained receipt %q count = %d, want 1", target.FactID, got)
		}
		fixture.assertExactTombstone(t, target)
	}
}

func (fixture *pruneFixtureV1) assertExactTombstone(t *testing.T, reference VerifiedPruneReference) {
	t.Helper()
	var factID continuity.FactID
	var environmentID continuity.EnvironmentID
	var environmentSequence, arrivalSequence int64
	var previousDigest, digest, certificateID, nonce, pruneCertificateID []byte
	var keyGeneration uint32
	if err := fixture.store.db.QueryRow(`SELECT fact_id, environment_id, environment_sequence, arrival_sequence, previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce, prune_certificate_id FROM continuity_sync_tombstones WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(reference.FactID)).Scan(
		&factID, &environmentID, &environmentSequence, &arrivalSequence, &previousDigest, &digest, &certificateID, &keyGeneration, &nonce, &pruneCertificateID,
	); err != nil {
		t.Fatalf("read tombstone %q: %v", reference.FactID, err)
	}
	if factID != reference.FactID || environmentID != reference.EnvironmentID || environmentSequence != reference.EnvironmentSequence || arrivalSequence != reference.ArrivalSequence ||
		!bytes.Equal(previousDigest, reference.PreviousEnvelopeDigest[:]) || !bytes.Equal(digest, reference.EnvelopeDigest[:]) ||
		!bytes.Equal(certificateID, reference.CertificateID[:]) || keyGeneration != reference.KeyGeneration || !bytes.Equal(nonce, reference.Nonce[:]) ||
		!bytes.Equal(pruneCertificateID, fixture.plan.PruneCertificateID[:]) {
		t.Fatalf("tombstone %q does not exactly match its verified reference", reference.FactID)
	}
}

func (fixture *pruneFixtureV1) count(t *testing.T, query string, arguments ...any) int {
	t.Helper()
	var count int
	if err := fixture.store.db.QueryRow(query, arguments...).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return count
}

func (fixture *pruneFixtureV1) exec(t *testing.T, query string, arguments ...any) {
	t.Helper()
	if _, err := fixture.store.db.Exec(query, arguments...); err != nil {
		t.Fatalf("fixture exec: %v", err)
	}
}

func (fixture *pruneFixtureV1) insertOutboxFor(t *testing.T, reference VerifiedPruneReference) {
	t.Helper()
	fixture.exec(t, `INSERT INTO continuity_sync_outbox(fact_id, project_id, environment_id, environment_sequence, previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce, sealed_envelope) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, X'01')`,
		string(reference.FactID), string(fixture.projectID), string(reference.EnvironmentID), reference.EnvironmentSequence,
		reference.PreviousEnvelopeDigest[:], reference.EnvelopeDigest[:], reference.CertificateID[:], reference.KeyGeneration, reference.Nonce[:])
}

func (fixture *pruneFixtureV1) corruptEnvelopeMetadataV1(t *testing.T, table string, factID continuity.FactID, assignment string) {
	t.Helper()
	fixture.exec(t, `PRAGMA ignore_check_constraints = ON`)
	fixture.exec(t, fmt.Sprintf("UPDATE %s SET %s WHERE project_id = ? AND fact_id = ?", table, assignment), string(fixture.projectID), string(factID))
	fixture.exec(t, `PRAGMA ignore_check_constraints = OFF`)
}

func (fixture *pruneFixtureV1) receiptMetadataV1(t *testing.T, factID continuity.FactID) pruneEnvelopeMetadataFingerprintV1 {
	t.Helper()
	var value pruneEnvelopeMetadataFingerprintV1
	if err := fixture.store.db.QueryRow(`SELECT key_generation, hex(previous_envelope_digest), hex(envelope_digest), hex(certificate_id), hex(nonce) FROM continuity_sync_receipts WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(factID)).Scan(
		&value.keyGeneration, &value.previousDigestHex, &value.digestHex, &value.certificateHex, &value.nonceHex,
	); err != nil {
		t.Fatalf("read receipt envelope metadata: %v", err)
	}
	return value
}

func (fixture *pruneFixtureV1) tombstoneMetadataV1(t *testing.T, factID continuity.FactID) pruneEnvelopeMetadataFingerprintV1 {
	t.Helper()
	var value pruneEnvelopeMetadataFingerprintV1
	if err := fixture.store.db.QueryRow(`SELECT key_generation, hex(previous_envelope_digest), hex(envelope_digest), hex(certificate_id), hex(nonce), hex(prune_certificate_id) FROM continuity_sync_tombstones WHERE project_id = ? AND fact_id = ?`, string(fixture.projectID), string(factID)).Scan(
		&value.keyGeneration, &value.previousDigestHex, &value.digestHex, &value.certificateHex, &value.nonceHex, &value.pruneCertHex,
	); err != nil {
		t.Fatalf("read tombstone envelope metadata: %v", err)
	}
	return value
}

func readPruneEnvironmentHeadV1(t *testing.T, store *Store, projectID continuity.ProjectID, environmentID continuity.EnvironmentID) pruneEnvironmentHeadV1 {
	t.Helper()
	var head pruneEnvironmentHeadV1
	if err := store.db.QueryRow(`SELECT highest_sequence, hlc_wall_millis, hlc_logical, sealed_sequence, hex(previous_envelope_digest), hex(envelope_digest), hex(certificate_id), COALESCE(key_generation, 0), hex(nonce) FROM continuity_sync_environment_heads WHERE project_id = ? AND environment_id = ?`, string(projectID), string(environmentID)).Scan(
		&head.highestSequence, &head.wallMillis, &head.logical, &head.sealedSequence,
		&head.previousHex, &head.digestHex, &head.certificateHex, &head.keyGeneration, &head.nonceHex,
	); err != nil {
		t.Fatalf("read environment head: %v", err)
	}
	return head
}

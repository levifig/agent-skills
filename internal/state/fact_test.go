package state

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAppendFactGrowOnlyAndEnvelopeV1(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	projectID, err := store.projectID(ctx, root)
	if err != nil {
		t.Fatalf("projectID() error = %v", err)
	}
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	first, err := AppendFact(ctx, store, AppendFactInput{
		ProjectID: projectID,
		Kind:      FactKindJournal,
		Payload:   `{"entry_type":"discover","message":"first"}`,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("AppendFact() first error = %v", err)
	}
	if first.EnvelopeV != 1 {
		t.Fatalf("EnvelopeV = %d, want 1", first.EnvelopeV)
	}
	if first.Permanence != "notebook" {
		t.Fatalf("Permanence = %q, want notebook", first.Permanence)
	}
	if strings.TrimSpace(first.ID) == "" {
		t.Fatal("id is empty")
	}
	second, err := AppendFact(ctx, store, AppendFactInput{
		ProjectID: projectID,
		Kind:      FactKindJournal,
		Payload:   `{"entry_type":"discover","message":"second"}`,
		Now:       now.Add(time.Millisecond),
	})
	if err != nil {
		t.Fatalf("AppendFact() second error = %v", err)
	}
	if compareFactOrder(parseHLCMust(t, first.HLC), first.EnvID, first.ID, parseHLCMust(t, second.HLC), second.EnvID, second.ID) >= 0 {
		t.Fatalf("order = (%s,%s,%s) before (%s,%s,%s)", first.HLC, first.EnvID, first.ID, second.HLC, second.EnvID, second.ID)
	}
	if _, err := AppendFact(ctx, store, AppendFactInput{
		ProjectID: projectID,
		Kind:      FactKindJournal,
		Payload:   `{"entry_type":"discover","message":"duplicate"}`,
		ID:        first.ID,
		Now:       now,
	}); err == nil {
		t.Fatal("AppendFact() duplicate id error = nil, want failure")
	}
	if _, err := AppendFact(ctx, store, AppendFactInput{
		ProjectID: projectID,
		Kind:      "unknown-kind",
		Payload:   `{}`,
		Now:       now,
	}); err == nil {
		t.Fatal("AppendFact() unknown kind error = nil, want failure")
	}
}

func TestHLCOrderingUsesEnvAndIDTiebreak(t *testing.T) {
	left := HLC{WallMS: 100, Logical: 0}
	right := HLC{WallMS: 100, Logical: 0}
	if compareFactOrder(left, "a-host", "fact-a", right, "b-host", "fact-b") >= 0 {
		t.Fatal("env_id tiebreak failed")
	}
	if compareFactOrder(left, "same-host", "fact-a", right, "same-host", "fact-b") >= 0 {
		t.Fatal("id tiebreak failed")
	}
}

func parseHLCMust(t *testing.T, value string) HLC {
	t.Helper()
	parsed, err := parseHLC(value)
	if err != nil {
		t.Fatalf("parseHLC(%q) error = %v", value, err)
	}
	return parsed
}

package state

import (
	"context"
	"testing"
)

func TestArtifactEntityCreateShowListAndLink(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	plan, err := CreateArtifactEntity(context.Background(), root, PathResolver{StateHome: stateHome}, ArtifactEntityCreateOptions{
		Kind:  "plan",
		Title: "SQLite Body Plan",
		Body:  "# SQLite Body Plan\n\nPlan body.",
	})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(plan) error = %v", err)
	}
	handoff, err := CreateArtifactEntity(context.Background(), root, PathResolver{StateHome: stateHome}, ArtifactEntityCreateOptions{
		Kind:  "handoff",
		Title: "Design Handoff",
		Body:  "# Design Handoff\n\nHandoff body.",
	})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	if _, err := CreateLink(context.Background(), root, PathResolver{StateHome: stateHome}, LinkMutationOptions{
		From: plan.Entity.Alias,
		To:   handoff.Entity.Alias,
		Type: "reviewed_by",
	}); err != nil {
		t.Fatalf("CreateLink() error = %v", err)
	}

	show, err := ShowArtifactEntity(context.Background(), root, PathResolver{StateHome: stateHome}, "plan", plan.Entity.Alias)
	if err != nil {
		t.Fatalf("ShowArtifactEntity(plan) error = %v", err)
	}
	if show.Entity.Body != "# SQLite Body Plan\n\nPlan body." || len(show.Entity.Sources) != 0 {
		t.Fatalf("show.Entity = %#v, want SQLite body without source", show.Entity)
	}
	if !hasStateTraceRelationship(show.Entity.Relationships, "outbound", "reviewed_by", "handoff", handoff.Entity.Alias) {
		t.Fatalf("Relationships = %#v, want reviewed_by handoff", show.Entity.Relationships)
	}

	list, err := ListArtifactEntities(context.Background(), root, PathResolver{StateHome: stateHome}, ArtifactEntityListOptions{Kind: "plan"})
	if err != nil {
		t.Fatalf("ListArtifactEntities(plan) error = %v", err)
	}
	if list.Entities[plan.Entity.Alias].Title != "SQLite Body Plan" {
		t.Fatalf("list.Entities = %#v, want created plan", list.Entities)
	}
}

package state

import (
	"reflect"
	"testing"
)

func TestLifecycleStatusRegistryDefinesEntitySubsets(t *testing.T) {
	expectedGlobal := []string{"draft", "open", "todo", "in_progress", "blocked", "review", "done", "paused", "archived"}
	if got := LifecycleStatuses(); !reflect.DeepEqual(got, expectedGlobal) {
		t.Fatalf("LifecycleStatuses() = %#v, want %#v", got, expectedGlobal)
	}

	expected := map[string][]string{
		"spec":       {"draft", "todo", "in_progress", "done", "archived"},
		"task":       {"todo", "in_progress", "blocked", "review", "done", "archived"},
		"report":     {"draft", "done", "archived"},
		"idea":       {"open", "done", "archived"},
		"spark":      {"open", "done", "archived"},
		"brainstorm": {"open", "done", "archived"},
		"plan":       {"draft", "done", "archived"},
		"handoff":    {"draft", "done", "archived"},
		"council":    {"draft", "done", "archived"},
	}
	for _, kind := range LifecycleEntityKinds() {
		want, ok := expected[kind]
		if !ok {
			t.Fatalf("LifecycleEntityKinds includes unexpected kind %q", kind)
		}
		if got := LifecycleStatusesForEntity(kind); !reflect.DeepEqual(got, want) {
			t.Fatalf("LifecycleStatusesForEntity(%q) = %#v, want %#v", kind, got, want)
		}
		for _, status := range want {
			if !ValidLifecycleStatus(status) {
				t.Fatalf("ValidLifecycleStatus(%q) = false", status)
			}
			if !ValidLifecycleEntityStatus(kind, status) {
				t.Fatalf("ValidLifecycleEntityStatus(%q, %q) = false", kind, status)
			}
		}
		delete(expected, kind)
	}
	if len(expected) != 0 {
		t.Fatalf("LifecycleEntityKinds missing expected kinds: %#v", expected)
	}
}

func TestLifecycleStatusRegistryRejectsOutOfSubsetStatuses(t *testing.T) {
	for _, tc := range []struct {
		kind   string
		status string
	}{
		{kind: "report", status: "active"},
		{kind: "report", status: "open"},
		{kind: "session", status: "active"},
		{kind: "idea", status: "todo"},
		{kind: "task", status: "paused"},
		{kind: "finding", status: "confirmed"},
		{kind: "run", status: "running"},
		{kind: "unknown", status: "done"},
	} {
		if ValidLifecycleEntityStatus(tc.kind, tc.status) {
			t.Fatalf("ValidLifecycleEntityStatus(%q, %q) = true, want false", tc.kind, tc.status)
		}
	}
}

func TestCanonicalLifecycleStatusMapsLegacySpellings(t *testing.T) {
	for _, tc := range []struct {
		kind      string
		legacy    string
		canonical string
	}{
		{kind: "spec", legacy: "drafting", canonical: "draft"},
		{kind: "spec", legacy: "approved", canonical: "todo"},
		{kind: "spec", legacy: "implementing", canonical: "in_progress"},
		{kind: "spec", legacy: "complete", canonical: "done"},
		{kind: "task", legacy: "review", canonical: "review"},
		{kind: "report", legacy: "final", canonical: "done"},
		{kind: "idea", legacy: "resolved", canonical: "done"},
		{kind: "spark", legacy: "resolved", canonical: "done"},
		{kind: "brainstorm", legacy: "resolved", canonical: "done"},
		{kind: "plan", legacy: "final", canonical: "done"},
		{kind: "handoff", legacy: "final", canonical: "done"},
		{kind: "council", legacy: "final", canonical: "done"},
	} {
		got, ok := CanonicalLifecycleStatus(tc.kind, tc.legacy)
		if !ok || got != tc.canonical {
			t.Fatalf("CanonicalLifecycleStatus(%q, %q) = %q, %v; want %q, true", tc.kind, tc.legacy, got, ok, tc.canonical)
		}
	}
}

func TestNonLifecycleStatusVocabulariesAreExplicit(t *testing.T) {
	expected := []string{"finding_status", "verdict_outcome", "run_status"}
	if got := NonLifecycleStatusVocabularies(); !reflect.DeepEqual(got, expected) {
		t.Fatalf("NonLifecycleStatusVocabularies() = %#v, want %#v", got, expected)
	}
	if canonical, ok := CanonicalLifecycleStatus("finding", "confirmed"); ok {
		t.Fatalf("CanonicalLifecycleStatus(finding, confirmed) = %q, true; want excluded", canonical)
	}
	if !ValidFindingStatus("confirmed") || !ValidRunStatus("running") || !ValidVerdictOutcome("partial") {
		t.Fatal("domain-specific status vocabularies are not valid through their explicit registries")
	}
}

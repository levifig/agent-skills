package state

import (
	"strings"
	"testing"
)

func TestFindingVocabularyValidators(t *testing.T) {
	for _, status := range []string{"open", "confirmed", "refuted", "partial", "archived"} {
		if !ValidFindingStatus(status) {
			t.Fatalf("ValidFindingStatus(%q) = false", status)
		}
	}
	for _, severity := range []string{"critical", "high", "medium", "low", "info"} {
		if !ValidFindingSeverity(severity) {
			t.Fatalf("ValidFindingSeverity(%q) = false", severity)
		}
	}
	for _, confidence := range []string{"high", "medium", "low"} {
		if !ValidFindingConfidence(confidence) {
			t.Fatalf("ValidFindingConfidence(%q) = false", confidence)
		}
	}
	for _, outcome := range []string{"confirmed", "refuted", "partial"} {
		if !ValidVerdictOutcome(outcome) {
			t.Fatalf("ValidVerdictOutcome(%q) = false", outcome)
		}
	}
	for _, status := range []string{"pending", "running", "completed", "failed", "archived"} {
		if !ValidRunStatus(status) {
			t.Fatalf("ValidRunStatus(%q) = false", status)
		}
	}
}

func TestFindingVocabularyRejectsUnknownValues(t *testing.T) {
	if ValidFindingStatus("done") {
		t.Fatal("ValidFindingStatus(done) = true, want false")
	}
	if ValidFindingSeverity("blocker") {
		t.Fatal("ValidFindingSeverity(blocker) = true, want false")
	}
	if ValidFindingConfidence("certain") {
		t.Fatal("ValidFindingConfidence(certain) = true, want false")
	}
	if ValidVerdictOutcome("open") {
		t.Fatal("ValidVerdictOutcome(open) = true, want false")
	}
	if ValidRunStatus("started") {
		t.Fatal("ValidRunStatus(started) = true, want false")
	}
}

func TestRunSchemaStoresProvenanceNotCode(t *testing.T) {
	runColumns := schemaColumnNames(t, currentSchemaSQL())["runs"]
	joined := strings.Join(runColumns, "\n")
	for _, forbidden := range []string{"script", "code", "command", "body", "content"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("runs columns include forbidden code storage term %q:\n%s", forbidden, joined)
		}
	}
	for _, required := range []string{"generator_ref", "generator_version", "generator_hash", "status", "metadata"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("runs columns missing provenance field %q:\n%s", required, joined)
		}
	}
}

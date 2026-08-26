package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSanitizeMarkdownReportPathSegmentNormalizesSpaces(t *testing.T) {
	got := sanitizeMarkdownReportPathSegment("Hello World: Draft")
	if strings.Contains(got, " ") {
		t.Fatalf("sanitizeMarkdownReportPathSegment() = %q, want no spaces", got)
	}
	if got != "Hello-World-Draft" {
		t.Fatalf("sanitizeMarkdownReportPathSegment() = %q, want Hello-World-Draft", got)
	}
}

func TestCouncilAliasFromTitleHasNoSpaces(t *testing.T) {
	alias := councilAliasFromTitle("MQTT Identity Model", time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC))
	if strings.Contains(alias, " ") {
		t.Fatalf("councilAliasFromTitle() = %q, want no spaces", alias)
	}
	if !strings.HasSuffix(alias, "-mqtt-identity-model") {
		t.Fatalf("councilAliasFromTitle() = %q, want mqtt-identity-model suffix", alias)
	}
}

func TestRunnerCouncilListReadsFileBackedCouncils(t *testing.T) {
	workingDir := t.TempDir()
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "councils/COUNCIL-20260826-demo.md", `---
id: COUNCIL-20260826-demo
title: Demo Council
status: draft
---
# Demo Council
`)
	initOut := &bytes.Buffer{}
	if err := (Runner{Stdout: initOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v (%s)", err, initOut.String())
	}
	out := &bytes.Buffer{}
	if err := (Runner{Stdout: out, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"council", "list"}); err != nil {
		t.Fatalf("council list error = %v (%s)", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "COUNCIL-20260826-demo") || !strings.Contains(got, "Demo Council") {
		t.Fatalf("council list output = %q, want file-backed council", got)
	}
}

func TestRunnerCouncilHelpDoesNotClaimSQLite(t *testing.T) {
	out := &bytes.Buffer{}
	if err := (Runner{Stdout: out}).Run([]string{"council", "--help"}); err != nil {
		t.Fatalf("council --help error = %v", err)
	}
	got := out.String()
	if strings.Contains(got, "in native SQLite state") {
		t.Fatalf("council help still claims SQLite: %q", got)
	}
	if !strings.Contains(got, ".agents/councils") {
		t.Fatalf("council help missing file-backed guidance: %q", got)
	}
}

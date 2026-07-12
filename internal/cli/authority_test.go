package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestCommandAuthorityForUsesExplicitBasicLeafPrefixes(t *testing.T) {
	for _, args := range [][]string{
		{"journal", "log", "--execpolicy-safe", "decision(scope): message"},
		{"task", "create", "--title", "task"},
		{"docs", "index", "--rebuild"},
		{"state", "backup", "verify", "/tmp/backup.sqlite"},
		{"state", "export", "all", "--format", "json"},
		{"state", "export", "triage", "--format", "markdown"},
		{"state", "export", "spec", "SPEC-001", "--format", "markdown"},
		{"state", "export", "release-readiness", "--format", "markdown"},
		{"report", "generate", "triage", "--format", "markdown"},
		{"report", "generate", "release-readiness", "--format", "markdown"},
		{"project", "identity"},
		{"spec", "status", "SPEC-001", "done"},
		{"kb", "glossary", "list", "--all"},
		{"check", "--hook", "check-secrets"},
		{"trace", "task:TASK-001"},
	} {
		if got := CommandAuthorityFor(args); got != CommandAuthorityBasic {
			t.Errorf("CommandAuthorityFor(%v) = %s, want basic", args, got)
		}
	}
}

func TestCommandAuthorityForDefaultsToOperatorForParentsAndUnsafeLeaves(t *testing.T) {
	for _, args := range [][]string{
		{"journal"},
		{"journal", "log", "decision(scope): message"},
		{"task", "unknown"},
		{"state", "doctor"}, // --fix means the whole leaf is operator.
		{"state", "init"},
		{"state", "repair", "journal-search"},
		{"state", "export"},
		{"state", "export", "unknown"},
		{"report", "generate"},
		{"report", "generate", "unknown"},
		{"report", "create", "report", "--message", "body"},
		{"finding", "create", "--report", "report", "--title", "finding"},
		{"finding", "import-json", "--report", "report"},
		{"plan", "new", "--title", "plan"},
		{"handoff", "new", "--title", "handoff"},
		{"council", "new", "--title", "council"},
		{"brainstorm", "capture", "--title", "brainstorm"},
		{"idea", "capture", "--title", "idea"},
		{"project", "rename", "new-name"},
		{"spec", "finalize", "SPEC-001"},
		{"spec", "new", "slug", "--body-file", "body.md"},
		{"spec", "delete", "SPEC-001", "--yes"},
		{"kb", "review", "docs/knowledge/example.md"},
		{"kb", "glossary", "upsert", "term"},
		{"change", "init", "new-change"},
		{"change", "check"},
		{"release"},
		{"not-a-command"},
	} {
		if got := CommandAuthorityFor(args); got != CommandAuthorityOperator {
			t.Errorf("CommandAuthorityFor(%v) = %s, want operator", args, got)
		}
	}
}

func TestBasicCommandAuthorityPrefixesAreSortedAndDefensive(t *testing.T) {
	prefixes := BasicCommandAuthorityPrefixes()
	if len(prefixes) < 2 {
		t.Fatalf("basic prefixes = %d, want explicit registry", len(prefixes))
	}
	for i := 1; i < len(prefixes); i++ {
		previous := strings.Join(prefixes[i-1], "\x00")
		current := strings.Join(prefixes[i], "\x00")
		if previous >= current {
			t.Fatalf("prefixes are not sorted: %q >= %q", previous, current)
		}
	}
	prefixes[0][0] = "mutated"
	if reflect.DeepEqual(prefixes, BasicCommandAuthorityPrefixes()) {
		t.Fatal("BasicCommandAuthorityPrefixes returned mutable registry storage")
	}
}

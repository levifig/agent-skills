package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/state"
)

func TestRunnerMigrateTrackerExportEmitsProviderNeutralPacket(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "identity", "--authority", "local", "--prefix", "DOJO"); err != nil {
		t.Fatalf("issue identity error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "First external issue", "--body", "Move this work."); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "DOJO-1", "Migration is verified"); err != nil {
		t.Fatalf("issue dod add error = %v", err)
	}

	var stdout bytes.Buffer
	runner := Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}
	if err := runner.Run([]string{"migrate", "tracker-export", "--json"}); err != nil {
		t.Fatalf("migrate tracker-export --json error = %v", err)
	}
	var packet state.TrackerMigrationPacket
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil {
		t.Fatalf("json.Unmarshal(output) error = %v\n%s", err, stdout.String())
	}
	if packet.ExportKind != state.ExportKindTrackerMigration || len(packet.Issues) != 1 || packet.Issues[0].Alias != "DOJO-1" {
		t.Fatalf("packet = %#v", packet)
	}
	for _, forbidden := range []string{"database_path", "project_current_path", "started_worktree", "journal", "handoff", "spark"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("output leaked %q: %s", forbidden, stdout.String())
		}
	}

	stdout.Reset()
	if err := runner.Run([]string{"migrate", "tracker-export"}); err != nil {
		t.Fatalf("migrate tracker-export error = %v", err)
	}
	if !json.Valid(stdout.Bytes()) {
		t.Fatalf("default output is not JSON: %s", stdout.String())
	}
}

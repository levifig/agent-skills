package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func decodeAbsorbResult(t *testing.T, data string) state.AbsorbResult {
	t.Helper()
	var result state.AbsorbResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", data, err)
	}
	return result
}

func TestIssueAbsorbMintsIssueAndArchivesSource(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	created, err := state.CreateTask(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.TaskCreateOptions{Title: "Leftover CLI work", Priority: "P2"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "absorb", created.Task.Alias, "--json")
	if err != nil {
		t.Fatalf("issue absorb error = %v\n%s", err, out)
	}
	result := decodeAbsorbResult(t, out)
	if result.Issue == nil || result.Issue.Alias != "LOAF-1" || result.Issue.Title != "Leftover CLI work" {
		t.Fatalf("absorb result = %#v, want LOAF-1", result)
	}
	if !strings.Contains(result.Issue.Body, "Absorbed from TASK-001") || !strings.Contains(result.Issue.Body, "task:"+created.Task.ID) {
		t.Fatalf("body = %q, want provenance", result.Issue.Body)
	}

	shown, err := runIssue(t, workingDir, stateHome, "show", "LOAF-1")
	if err != nil {
		t.Fatalf("issue show LOAF-1 error = %v", err)
	}
	if !strings.Contains(shown, "alias: LOAF-1") {
		t.Fatalf("show = %q, want LOAF-1", shown)
	}
	if _, err := runIssue(t, workingDir, stateHome, "show", "TASK-001"); err == nil {
		t.Fatal("issue show TASK-001 error = nil, want missing issue (source alias is not a live issue identity)")
	}

	var taskOut bytes.Buffer
	if err := (Runner{Stdout: &taskOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"task", "show", "TASK-001", "--json"}); err != nil {
		t.Fatalf("task show error = %v\n%s", err, taskOut.String())
	}
	if !strings.Contains(taskOut.String(), `"status": "archived"`) {
		t.Fatalf("task show = %s, want archived", taskOut.String())
	}

	helpOut, err := runIssue(t, workingDir, stateHome, "--help")
	if err != nil {
		t.Fatalf("issue --help error = %v", err)
	}
	if !strings.Contains(helpOut, "absorb") {
		t.Fatalf("issue help missing absorb:\n%s", helpOut)
	}
}

func TestIssueAbsorbIntentMintsIssueAndArchivesSource(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	created, err := state.CreateIntent(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.IntentCreateOptions{Title: "Intent leftover", Body: "Body."})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "absorb", created.Intent.Alias, "--json")
	if err != nil {
		t.Fatalf("issue absorb intent error = %v\n%s", err, out)
	}
	result := decodeAbsorbResult(t, out)
	if result.Issue == nil || result.Issue.Alias != "LOAF-1" || result.Source.Kind != "intent" {
		t.Fatalf("result = %#v, want intent absorbed into LOAF-1", result)
	}
	if _, err := runIssue(t, workingDir, stateHome, "show", created.Intent.Alias); err == nil {
		t.Fatal("issue show INTENT alias error = nil, want missing issue")
	}

	var intentOut bytes.Buffer
	if err := (Runner{Stdout: &intentOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"intent", "show", created.Intent.Alias, "--json"}); err != nil {
		t.Fatalf("intent show error = %v\n%s", err, intentOut.String())
	}
	if !strings.Contains(intentOut.String(), `"disposition": "resolved"`) || !strings.Contains(intentOut.String(), "absorbed into LOAF-1") {
		t.Fatalf("intent show = %s, want resolved absorbed disposition", intentOut.String())
	}
}

func TestIssueAbsorbDismissArchivesWithoutMinting(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.CreateTask(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.TaskCreateOptions{Title: "Dismiss me"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "absorb", "TASK-001", "--dismiss", "--json")
	if err != nil {
		t.Fatalf("issue absorb --dismiss error = %v\n%s", err, out)
	}
	result := decodeAbsorbResult(t, out)
	if !result.Dismiss || result.Issue != nil || result.Disposition != state.AbsorbDispositionSuperseded {
		t.Fatalf("result = %#v, want dismissed without issue", result)
	}

	listOut, err := runIssue(t, workingDir, stateHome, "list")
	if err != nil {
		t.Fatalf("issue list error = %v", err)
	}
	if strings.Contains(listOut, "LOAF-") || strings.Contains(listOut, "Dismiss me") {
		t.Fatalf("list = %q, want no minted issue", listOut)
	}

	plain, err := runIssue(t, workingDir, stateHome, "absorb", "--dismiss", "TASK-001")
	if err == nil {
		t.Fatalf("repeat dismiss error = nil\n%s, want not leftover open work", plain)
	}
	if !strings.Contains(err.Error(), "not leftover open work") || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("repeat dismiss error = %v, want not leftover open work archived", err)
	}
}

func TestIssueAbsorbDismissIntentWithoutMinting(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	created, err := state.CreateIntent(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.IntentCreateOptions{Title: "Drop intent", Body: "Body."})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "absorb", created.Intent.Alias, "--dismiss")
	if err != nil {
		t.Fatalf("issue absorb --dismiss intent error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "dismissed") || !strings.Contains(out, "superseded") {
		t.Fatalf("output = %q, want dismissed as superseded", out)
	}
	listOut, err := runIssue(t, workingDir, stateHome, "list", "--json")
	if err != nil {
		t.Fatalf("issue list error = %v", err)
	}
	listed := decodeIssueList(t, listOut)
	if len(listed.Issues) != 0 {
		t.Fatalf("issues = %#v, want none", listed.Issues)
	}
}

func decodeIssueList(t *testing.T, data string) state.IssueListResult {
	t.Helper()
	var result state.IssueListResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("json.Unmarshal list error = %v", err)
	}
	return result
}

func TestIssueAbsorbRefuseChangeLocalAlreadyAbsorbedUnknownAndUnregistered(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}

	_, err = runIssue(t, workingDir, stateHome, "absorb", "docs/changes/foo/tasks/TASK-001.md")
	if err == nil || !strings.Contains(err.Error(), "change-local") {
		t.Fatalf("path ref error = %v, want change-local refusal", err)
	}

	created, err := state.CreateTask(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.TaskCreateOptions{Title: "Imported change task"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	attachCLITaskSource(t, workingDir, stateHome, created.Task.ID, "docs/changes/example/tasks/TASK-001-example.md")
	_, err = runIssue(t, workingDir, stateHome, "absorb", created.Task.Alias)
	if err == nil || !strings.Contains(err.Error(), "change-local") {
		t.Fatalf("source-path error = %v, want change-local refusal", err)
	}

	open, err := state.CreateTask(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.TaskCreateOptions{Title: "Once"})
	if err != nil {
		t.Fatalf("CreateTask(open) error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "absorb", open.Task.Alias); err != nil {
		t.Fatalf("first absorb error = %v", err)
	}
	_, err = runIssue(t, workingDir, stateHome, "absorb", open.Task.Alias)
	if err == nil || !strings.Contains(err.Error(), "not leftover open work") || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("second absorb error = %v, want not leftover open work archived", err)
	}

	_, err = runIssue(t, workingDir, stateHome, "absorb", "TASK-999")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown ref error = %v, want not found", err)
	}

	bare := realpath(t, t.TempDir())
	_, err = runIssue(t, bare, t.TempDir(), "absorb", "TASK-001")
	if err == nil || !strings.Contains(err.Error(), "requires initialized SQLite state") {
		t.Fatalf("uninitialized error = %v, want sqlite required (no implicit project creation)", err)
	}

	other := realpath(t, t.TempDir())
	_, err = runIssue(t, other, stateHome, "absorb", "TASK-001")
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unregistered cwd error = %v, want unregistered project (no implicit project creation)", err)
	}
}

func TestIssueAbsorbRefusesDoneAndArchivedTasks(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}

	done, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Done leftover"})
	if err != nil {
		t.Fatalf("CreateTask(done) error = %v", err)
	}
	if _, err := state.UpdateTaskStatus(context.Background(), root, resolver, done.Task.Alias, state.LifecycleStatusDone); err != nil {
		t.Fatalf("UpdateTaskStatus(done) error = %v", err)
	}
	_, err = runIssue(t, workingDir, stateHome, "absorb", done.Task.Alias)
	if err == nil || !strings.Contains(err.Error(), done.Task.Alias) || !strings.Contains(err.Error(), "not leftover open work") || !strings.Contains(err.Error(), "done") {
		t.Fatalf("absorb done error = %v, want named leftover-open refusal", err)
	}
	_, err = runIssue(t, workingDir, stateHome, "absorb", "--dismiss", done.Task.Alias)
	if err == nil || !strings.Contains(err.Error(), "not leftover open work") || !strings.Contains(err.Error(), "done") {
		t.Fatalf("absorb --dismiss done error = %v, want named leftover-open refusal", err)
	}

	archived, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Archived leftover"})
	if err != nil {
		t.Fatalf("CreateTask(archived) error = %v", err)
	}
	if _, err := state.UpdateTaskStatus(context.Background(), root, resolver, archived.Task.Alias, state.LifecycleStatusDone); err != nil {
		t.Fatalf("UpdateTaskStatus(archived) error = %v", err)
	}
	if _, err := state.ArchiveTasks(context.Background(), root, resolver, state.TaskArchiveOptions{Refs: []string{archived.Task.Alias}}); err != nil {
		t.Fatalf("ArchiveTasks() error = %v", err)
	}
	_, err = runIssue(t, workingDir, stateHome, "absorb", archived.Task.Alias)
	if err == nil || !strings.Contains(err.Error(), "not leftover open work") || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("absorb archived error = %v, want named leftover-open refusal", err)
	}
}

func TestIssueAbsorbJSONEscapesControlCharactersInTaskBody(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	created, err := state.CreateTask(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.TaskCreateOptions{Title: "Control leftover"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	store, err := state.OpenStore(created.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	body := "line one\nctrl\x01here"
	if _, err := store.UpsertArtifactBody(context.Background(), created.ProjectID, "task", created.Task.ID, state.ArtifactBodyKindMarkdown, body, ""); err != nil {
		store.Close()
		t.Fatalf("UpsertArtifactBody() error = %v", err)
	}
	store.Close()

	out, err := runIssue(t, workingDir, stateHome, "absorb", created.Task.Alias, "--json")
	if err != nil {
		t.Fatalf("issue absorb --json error = %v\n%s", err, out)
	}
	var parsed state.AbsorbResult
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json.Unmarshal absorb output error = %v\n%s", err, out)
	}
	if !strings.Contains(parsed.Source.Body, "\x01") {
		t.Fatalf("source body = %q, want preserved control character", parsed.Source.Body)
	}
}

func attachCLITaskSource(t *testing.T, workingDir, stateHome, taskID, sourcePath string) {
	t.Helper()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	path, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	var projectID string
	if err := db.QueryRow(`SELECT project_id FROM tasks WHERE id = ?`, taskID).Scan(&projectID); err != nil {
		t.Fatalf("read project_id: %v", err)
	}
	sourceID := "src-" + filepath.Base(taskID)
	now := "2026-08-17T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO sources (id, project_id, source_kind, path, imported_at, created_at, updated_at) VALUES (?, ?, 'markdown', ?, ?, ?, ?)`, sourceID, projectID, sourcePath, now, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := db.Exec(`UPDATE tasks SET body_source_id = ? WHERE id = ?`, sourceID, taskID); err != nil {
		t.Fatalf("set body_source_id: %v", err)
	}
}

func decodeAbsorbProjection(t *testing.T, data string) state.AbsorbProjectionResult {
	t.Helper()
	var result state.AbsorbProjectionResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", data, err)
	}
	return result
}

func TestParseIssueAbsorbArgsAllAndHistory(t *testing.T) {
	ok, err := parseIssueAbsorbArgs([]string{"--all", "--history", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("parseIssueAbsorbArgs() error = %v", err)
	}
	if !ok.all || !ok.history || !ok.dryRun || !ok.jsonOutput || ok.ref != "" {
		t.Fatalf("options = %#v, want --all --history --dry-run --json", ok)
	}
	if _, err := parseIssueAbsorbArgs([]string{"--history"}); err == nil || !strings.Contains(err.Error(), "--history requires --all") {
		t.Fatalf("parse --history error = %v, want requires --all", err)
	}
	if _, err := parseIssueAbsorbArgs([]string{"--dry-run"}); err == nil || !strings.Contains(err.Error(), "--dry-run requires --all") {
		t.Fatalf("parse --dry-run error = %v, want requires --all", err)
	}
	if _, err := parseIssueAbsorbArgs([]string{"--all", "--dismiss", "--history"}); err == nil || !strings.Contains(err.Error(), "--dismiss cannot be combined with --history") {
		t.Fatalf("parse --dismiss --history error = %v, want combination refusal", err)
	}
	if _, err := parseIssueAbsorbArgs([]string{"--all", "TASK-001"}); err == nil || !strings.Contains(err.Error(), "--all does not accept") {
		t.Fatalf("parse --all TASK-001 error = %v, want ref refusal", err)
	}
	if _, err := parseIssueAbsorbArgs([]string{}); err == nil || !strings.Contains(err.Error(), "requires a task or intent ref, or --all") {
		t.Fatalf("parse empty error = %v, want ref or --all", err)
	}
}

func TestIssueAbsorbAllProjectsOpenLeftovers(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	if _, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Open leftover"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := state.CreateIntent(context.Background(), root, resolver, state.IntentCreateOptions{Title: "Tracked leftover", Body: "Body."}); err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	done, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Done leftover"})
	if err != nil {
		t.Fatalf("CreateTask(done) error = %v", err)
	}
	if _, err := state.UpdateTaskStatus(context.Background(), root, resolver, done.Task.Alias, state.LifecycleStatusDone); err != nil {
		t.Fatalf("UpdateTaskStatus(done) error = %v", err)
	}

	dry, err := runIssue(t, workingDir, stateHome, "absorb", "--all", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("issue absorb --all --dry-run error = %v\n%s", err, dry)
	}
	dryPlan := decodeAbsorbProjection(t, dry)
	if !dryPlan.DryRun || dryPlan.Absorbed != 2 {
		t.Fatalf("dry-run = %#v, want 2 planned absorbs", dryPlan)
	}

	out, err := runIssue(t, workingDir, stateHome, "absorb", "--all", "--json")
	if err != nil {
		t.Fatalf("issue absorb --all error = %v\n%s", err, out)
	}
	result := decodeAbsorbProjection(t, out)
	if result.DryRun || result.Absorbed != 2 {
		t.Fatalf("absorb --all = %#v, want 2 absorbed", result)
	}
	if _, err := runIssue(t, workingDir, stateHome, "show", "TASK-001"); err == nil {
		t.Fatal("issue show TASK-001 error = nil, want missing issue")
	}
	shown, err := runIssue(t, workingDir, stateHome, "show", "LOAF-1")
	if err != nil {
		t.Fatalf("issue show LOAF-1 error = %v", err)
	}
	if !strings.Contains(shown, "alias: LOAF-1") {
		t.Fatalf("show = %q, want LOAF-1", shown)
	}

	helpOut, err := runIssue(t, workingDir, stateHome, "absorb", "--help")
	if err != nil {
		t.Fatalf("issue absorb --help error = %v", err)
	}
	for _, want := range []string{"--all", "--history", "--dry-run"} {
		if !strings.Contains(helpOut, want) {
			t.Fatalf("absorb help missing %s:\n%s", want, helpOut)
		}
	}
}

func TestIssueAbsorbAllHistoryRefusesIndependentIssues(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	done, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Done leftover"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := state.UpdateTaskStatus(context.Background(), root, resolver, done.Task.Alias, state.LifecycleStatusDone); err != nil {
		t.Fatalf("UpdateTaskStatus() error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Hand-made"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	_, err = runIssue(t, workingDir, stateHome, "absorb", "--all", "--history")
	if err == nil || !strings.Contains(err.Error(), "not minted by absorb") {
		t.Fatalf("absorb --all --history error = %v, want independent-issue refusal", err)
	}
}

func TestIssueAbsorbAllHistoryMintsDoneWhenIssueTableEmpty(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	done, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Done leftover"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := state.UpdateTaskStatus(context.Background(), root, resolver, done.Task.Alias, state.LifecycleStatusDone); err != nil {
		t.Fatalf("UpdateTaskStatus() error = %v", err)
	}
	out, err := runIssue(t, workingDir, stateHome, "absorb", "--all", "--history", "--json")
	if err != nil {
		t.Fatalf("issue absorb --all --history error = %v\n%s", err, out)
	}
	result := decodeAbsorbProjection(t, out)
	if result.Absorbed != 1 || result.Items[0].Issue == nil || result.Items[0].Issue.Status != state.IssueStatusDone {
		t.Fatalf("result = %#v, want one done issue", result)
	}
}

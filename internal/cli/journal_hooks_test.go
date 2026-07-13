package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// freshHookRunnerDir returns a working directory with NO SQLite state database
// (state is never initialized), modeling a fresh install where hook-invoked
// journal commands must degrade gracefully instead of failing the harness.
func freshHookRunnerDir(t *testing.T) (workingDir string, stateHome string) {
	t.Helper()
	return realpath(t, t.TempDir()), t.TempDir()
}

// setupJournalHookRunner initializes a temp project with SQLite state so the
// journal hook commands (SessionStart digest, --from-hook logging, guidance
// emitters) can run against an isolated database.
func setupJournalHookRunner(t *testing.T) (workingDir string, stateHome string) {
	t.Helper()
	workingDir = realpath(t, t.TempDir())
	stateHome = t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-active.md", "---\nid: SPEC-001\ntitle: Active Spec\nstatus: implementing\n---\n# Active Spec\n")
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v\n%s", err, stdout.String())
	}
	return workingDir, stateHome
}

func TestJournalContextFromHookExitsSilentlyForSubagent(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1","agent_id":"subagent"}`),
	}.Run([]string{"journal", "context", "--from-hook"})
	if err != nil {
		t.Fatalf("journal context --from-hook (subagent) error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("journal context --from-hook (subagent) stdout = %q, want empty (silent exit)", stdout.String())
	}
}

func TestJournalContextFromHookEmitsDigestForPrimarySession(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	logRun := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}
	if err := logRun.Run([]string{"journal", "log", "wrap(hooks): checkpoint one"}); err != nil {
		t.Fatalf("journal log wrap error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1"}`),
	}.Run([]string{"journal", "context", "--from-hook"})
	if err != nil {
		t.Fatalf("journal context --from-hook error = %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"loaf journal context", "project-synthesis: none", "scoped-checkpoint: showing 1 of 1", "latest checkpoint (not project synthesis)", "wrap(hooks): checkpoint one"} {
		if !strings.Contains(out, want) {
			t.Fatalf("journal context --from-hook stdout = %q, want %q", out, want)
		}
	}
}

func TestJournalLogFromHookDisablesUnprovenTaskCompletedEntry(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1","hook_event_name":"TaskCompleted","task_description":"wire journal hooks"}`),
	}.Run([]string{"journal", "log", "--from-hook"})
	if err != nil {
		t.Fatalf("journal log --from-hook (task) error = %v", err)
	}
	var warning journalHookWarning
	if err := json.Unmarshal(stdout.Bytes(), &warning); err != nil {
		t.Fatalf("decode journal hook warning: %v\n%s", err, stdout.String())
	}
	if warning.Code != journalHookDiagnosticUnsupported || !warning.NonBlocking {
		t.Fatalf("journal hook warning = %#v, want unsupported nonblocking diagnostic", warning)
	}
}

func TestJournalLogFromHookExitsSilentlyForSubagent(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1","agent_id":"sub","hook_event_name":"TaskCompleted","task_description":"x"}`),
	}.Run([]string{"journal", "log", "--from-hook"})
	if err != nil {
		t.Fatalf("journal log --from-hook (subagent) error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("journal log --from-hook (subagent) stdout = %q, want empty (no write)", stdout.String())
	}
}

func TestJournalLogFromHookNoOpsOnUnrecognizedPayload(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1"}`),
	}.Run([]string{"journal", "log", "--from-hook"})
	if err != nil {
		t.Fatalf("journal log --from-hook (empty) error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("journal log --from-hook (empty) stdout = %q, want empty (no derivable entry)", stdout.String())
	}
}

func TestJournalContextForPromptInjectsJournalFirstPrinciples(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1"}`),
	}.Run([]string{"journal", "context", "for-prompt"})
	if err != nil {
		t.Fatalf("journal context for-prompt error = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "loaf journal log") {
		t.Fatalf("for-prompt stdout = %q, want journal-first log guidance", out)
	}
	if strings.Contains(out, "loaf session") {
		t.Fatalf("for-prompt stdout = %q, must not reference the deleted session command", out)
	}
}

func TestJournalContextForPromptExitsSilentlyForSubagent(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"agent_id":"sub"}`),
	}.Run([]string{"journal", "context", "for-prompt"})
	if err != nil {
		t.Fatalf("journal context for-prompt (subagent) error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("for-prompt (subagent) stdout = %q, want empty", stdout.String())
	}
}

func TestJournalContextForCompactInjectsFlushGuidance(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1"}`),
	}.Run([]string{"journal", "context", "for-compact"})
	if err != nil {
		t.Fatalf("journal context for-compact error = %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"COMPACTION IMMINENT", "loaf journal log", "wrap(scope):"} {
		if !strings.Contains(out, want) {
			t.Fatalf("for-compact stdout = %q, want %q", out, want)
		}
	}
	// Journal-first: no session-state snapshot language.
	for _, forbidden := range []string{"loaf session", "Current State", "session journal"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("for-compact stdout = %q, must not contain session-lifecycle language %q", out, forbidden)
		}
	}
}

// TestJournalContextFromHookDegradesOnMissingState verifies the SessionStart
// digest exits 0 with a single non-blocking diagnostic line (writing no error)
// when the state database is absent (SPEC-056 M2).
func TestJournalContextFromHookDegradesOnMissingState(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1"}`),
	}.Run([]string{"journal", "context", "--from-hook"})
	if err != nil {
		t.Fatalf("journal context --from-hook (missing state) error = %v, want nil (non-blocking)", err)
	}
	out := stdout.String()
	if strings.Contains(out, "SQLite state database is not initialized") {
		t.Fatalf("journal context --from-hook (missing state) stdout = %q, must not surface the raw error", out)
	}
	if !strings.Contains(out, "no journal yet") {
		t.Fatalf("journal context --from-hook (missing state) stdout = %q, want a single non-blocking diagnostic line", out)
	}
}

// TestJournalContextFromHookMissingStateStaysSilentForSubagent confirms the
// subagent guard still wins over the diagnostic line: subagents write nothing.
func TestJournalContextFromHookMissingStateStaysSilentForSubagent(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1","agent_id":"sub"}`),
	}.Run([]string{"journal", "context", "--from-hook"})
	if err != nil {
		t.Fatalf("journal context --from-hook (subagent, missing state) error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("journal context --from-hook (subagent, missing state) stdout = %q, want empty (silent exit)", stdout.String())
	}
}

// TestJournalContextForResumptionDegradesOnMissingState verifies PostCompact
// resumption exits 0 (non-blocking) when state is absent.
func TestJournalContextForResumptionDegradesOnMissingState(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1"}`),
	}.Run([]string{"journal", "context", "for-resumption"})
	if err != nil {
		t.Fatalf("journal context for-resumption (missing state) error = %v, want nil (non-blocking)", err)
	}
	if strings.Contains(stdout.String(), "SQLite state database is not initialized") {
		t.Fatalf("journal context for-resumption (missing state) stdout = %q, must not surface the raw error", stdout.String())
	}
}

// TestJournalLogFromHookWarnsOnMissingState verifies an explicitly supplied
// hook entry stays non-blocking and visible when state is absent (SPEC-056 M2).
func TestJournalLogFromHookWarnsOnMissingState(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"target":"claude-code","session_id":"s1"}`),
	}.Run([]string{"journal", "log", "--from-hook", "decision(hook): missing state"})
	if err != nil {
		t.Fatalf("journal log --from-hook (missing state) error = %v, want nil (non-blocking)", err)
	}
	var warning journalHookWarning
	if err := json.Unmarshal(stdout.Bytes(), &warning); err != nil {
		t.Fatalf("decode missing-state warning: %v\n%s", err, stdout.String())
	}
	if warning.Code != journalHookDiagnosticMissingState || !warning.NonBlocking {
		t.Fatalf("missing-state warning = %#v, want nonblocking missing-state diagnostic", warning)
	}
}

// TestJournalLogNonHookErrorsOnMissingState confirms an interactive (non-hook)
// journal log still errors when state is missing: M2 only relaxes hook paths.
func TestJournalLogNonHookErrorsOnMissingState(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"journal", "log", "decision(scope): interactive entry"})
	if err == nil {
		t.Fatal("journal log (non-hook, missing state) error = nil, want uninitialized-state error")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("journal log (non-hook, missing state) error = %v, want uninitialized-state sentinel", err)
	}
}

// TestJournalContextForPromptSurvivesMissingState confirms the UserPromptSubmit
// guidance emitter does not touch state and works on a fresh install.
func TestJournalContextForPromptSurvivesMissingState(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1"}`),
	}.Run([]string{"journal", "context", "for-prompt"})
	if err != nil {
		t.Fatalf("journal context for-prompt (missing state) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "loaf journal log") {
		t.Fatalf("for-prompt (missing state) stdout = %q, want guidance emitted", stdout.String())
	}
}

// TestJournalContextForCompactSurvivesMissingState confirms the PreCompact flush
// guidance emitter does not touch state and works on a fresh install.
func TestJournalContextForCompactSurvivesMissingState(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1"}`),
	}.Run([]string{"journal", "context", "for-compact"})
	if err != nil {
		t.Fatalf("journal context for-compact (missing state) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "COMPACTION IMMINENT") {
		t.Fatalf("for-compact (missing state) stdout = %q, want guidance emitted", stdout.String())
	}
}

func TestJournalContextForResumptionEmitsDigest(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	logRun := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}
	if err := logRun.Run([]string{"journal", "log", "wrap(hooks): resumption checkpoint"}); err != nil {
		t.Fatalf("journal log wrap error = %v", err)
	}
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1"}`),
	}.Run([]string{"journal", "context", "for-resumption"})
	if err != nil {
		t.Fatalf("journal context for-resumption error = %v", err)
	}
	if !strings.Contains(stdout.String(), "wrap(hooks): resumption checkpoint") {
		t.Fatalf("for-resumption stdout = %q, want the layered digest", stdout.String())
	}
}

func TestNormalizeJournalHookEnvelopeNamespacedIdentities(t *testing.T) {
	tests := []struct {
		name, payload, target, harness, version, event, session, agent string
	}{
		{"claude", `{"claude_code":{"version":"1.2.3","session_id":"claude-s","agent_id":"claude-a","hook_event_name":"PostToolUse"}}`, "claude-code", "claude-code", "1.2.3", "PostToolUse", "claude-s", "claude-a"},
		{"cursor", `{"target":"cursor","cursor":{"harness_version":"0.9","conversation_id":"cursor-s","event":"tool_result"}}`, "cursor", "cursor", "0.9", "tool_result", "cursor-s", ""},
		{"codex", `{"codex":{"version":"0.144.1","thread_id":"codex-s","event_name":"Stop"}}`, "codex", "codex", "0.144.1", "Stop", "codex-s", ""},
		{"opencode", `{"target":"opencode","opencode":{"runtime_version":"1.0","sessionID":"open-s","lifecycle_event":"compaction"}}`, "opencode", "opencode", "1.0", "compaction", "open-s", ""},
		{"amp", `{"amp":{"version":"2","session_id":"amp-s","is_subagent":true,"event":"session.start"}}`, "amp", "amp", "2", "session.start", "amp-s", "subagent"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal([]byte(test.payload), &raw); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			envelope := normalizeJournalHookEnvelope(journalHookInput{Raw: raw}, "")
			if envelope.Target != test.target || envelope.Harness != test.harness || envelope.HarnessVersion != test.version || envelope.Event != test.event || envelope.SessionID != test.session || envelope.AgentID != test.agent {
				t.Fatalf("envelope = %#v, want target=%q harness=%q version=%q event=%q session=%q agent=%q", envelope, test.target, test.harness, test.version, test.event, test.session, test.agent)
			}
		})
	}
}

func TestJournalHookCommandTextNeverCreatesCompletionEntries(t *testing.T) {
	fixtures := []string{
		`{"target":"claude-code","event":"PostToolUse","tool_input":{"command":"git commit -m 'failed'"},"success":false}`,
		`{"target":"cursor","event":"PostToolUse","tool_input":{"command":"git commit -m 'no-op'"},"success":true}`,
		`{"target":"codex","event":"PostToolUse","tool_input":{"command":"git commit --amend -m 'amend'"},"success":true}`,
		`{"target":"opencode","event":"PostToolUse","tool_input":{"command":"git commit -m 'success'"},"success":true}`,
		`{"target":"amp","event":"PostToolUse","tool_input":{"command":"gh pr create --title 'new'"},"success":true}`,
		`{"target":"claude-code","event":"PostToolUse","tool_input":{"command":"gh pr merge 42"},"success":true}`,
	}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			workingDir, stateHome := setupJournalHookRunner(t)
			var stdout bytes.Buffer
			if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(fixture)}).Run([]string{"journal", "log", "--from-hook"}); err != nil {
				t.Fatalf("journal log --from-hook error = %v\n%s", err, stdout.String())
			}
			var warning journalHookWarning
			if err := json.Unmarshal(stdout.Bytes(), &warning); err != nil {
				t.Fatalf("decode warning: %v\n%s", err, stdout.String())
			}
			if warning.Code != journalHookDiagnosticUnsupported || !warning.NonBlocking {
				t.Fatalf("warning = %#v, want unsupported nonblocking diagnostic", warning)
			}
			root, err := project.ResolveRoot(workingDir)
			if err != nil {
				t.Fatalf("ResolveRoot() error = %v", err)
			}
			databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
			if err != nil {
				t.Fatalf("DatabasePath() error = %v", err)
			}
			db := openCLITestDB(t, databasePath)
			defer closeCLITestDB(t, db)
			if got := sqliteCount(t, db, `SELECT COUNT(*) FROM journal_entries`); got != 0 {
				t.Fatalf("journal entries = %d, want zero for command-text-only payload", got)
			}
		})
	}
}

func TestJournalHookSyntheticProvenResultIsIdempotent(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	payload := `{"target":"synthetic","harness":"fixture","harness_version":"1","event":"PostToolUse","session_id":"s1","success":true,"outcome":"created","command_family":"git_commit","durable_result_kind":"git-commit","durable_result_id":"sha-123","message":"proven commit"}`
	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	envelope := normalizeJournalHookEnvelope(journalHookInput{Raw: raw}, workingDir)
	entry, ok, warning := journalHookEntryForEnvelope(envelope, raw, true)
	if !ok || warning != nil {
		t.Fatalf("trusted envelope entry=%q ok=%t warning=%#v", entry, ok, warning)
	}
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	origin := envelope.origin(state.ObservedGitBranch(workingDir))
	resolver := state.PathResolver{StateHome: stateHome}
	var results []state.JournalLogResult
	for attempt := 0; attempt < 2; attempt++ {
		result, logErr := state.LogJournal(context.Background(), root, resolver, state.JournalLogOptions{Entry: entry, HarnessSessionID: envelope.SessionID, ObservedWorktree: workingDir, Origin: &origin})
		if logErr != nil {
			t.Fatalf("attempt %d error = %v", attempt+1, logErr)
		}
		results = append(results, result)
		if result.ID == "" || result.EntryType != "commit" || result.Scope != "sha-123" {
			t.Fatalf("attempt %d result = %#v, want durable commit result", attempt+1, result)
		}
	}
	if results[0].ID != results[1].ID {
		t.Fatalf("replayed IDs = %q/%q, want same durable result", results[0].ID, results[1].ID)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	db := openCLITestDB(t, databasePath)
	defer closeCLITestDB(t, db)
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM journal_entries`); got != 1 {
		t.Fatalf("journal entries = %d, want one idempotent completion", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM journal_origins WHERE durable_result_kind = 'git-commit' AND durable_result_id = 'sha-123'`); got != 1 {
		t.Fatalf("durable result origins = %d, want one", got)
	}
}

func TestJournalHookTrustedPRCreateAndMergeUseDistinctReplayKeys(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	for _, test := range []struct {
		family, kind, entry string
	}{
		{"pr_create", "pull-request:create", "pr(create): PR (#42)"},
		{"pr_merge", "pull-request:merge", "pr(merge): #42"},
	} {
		outcome := "created"
		if test.family == "pr_merge" {
			outcome = "merged"
		}
		envelope := normalizedJournalHookEnvelope{ContractVersion: journalHookEnvelopeVersion, Target: "synthetic", Harness: "fixture", Event: "PostToolUse", Success: boolPtr(true), Outcome: outcome, CommandFamily: test.family, DurableResultKind: test.kind, DurableResultID: "42"}
		entry, ok, warning := journalHookEntryForEnvelope(envelope, map[string]any{"title": "PR"}, true)
		if !ok || warning != nil || entry != test.entry {
			t.Fatalf("%s entry=%q ok=%t warning=%#v", test.family, entry, ok, warning)
		}
		origin := envelope.origin("")
		first, logErr := state.LogJournal(context.Background(), root, resolver, state.JournalLogOptions{Entry: entry, Origin: &origin})
		if logErr != nil {
			t.Fatalf("%s first error = %v", test.family, logErr)
		}
		second, logErr := state.LogJournal(context.Background(), root, resolver, state.JournalLogOptions{Entry: entry, Origin: &origin})
		if logErr != nil || first.ID != second.ID {
			t.Fatalf("%s replay = %#v/%#v error=%v, want same ID", test.family, first, second, logErr)
		}
	}
}

func TestJournalHookTrustedOutcomeAndDurableIdentityMatrix(t *testing.T) {
	tests := []struct {
		name, family, outcome, kind, id string
		success                         bool
		wantEntry                       bool
	}{
		{name: "failed commit", family: "git_commit", outcome: "failed", kind: "git-commit", id: "sha-failed", success: false},
		{name: "unknown success", family: "git_commit", outcome: "unknown", kind: "git-commit", id: "sha-unknown", success: true},
		{name: "no-op commit", family: "git_commit", outcome: "no-op", kind: "git-commit", id: "sha-noop", success: true},
		{name: "amended commit", family: "git_commit", outcome: "amended", kind: "git-commit", id: "sha-amended", success: true, wantEntry: true},
		{name: "created commit", family: "git_commit", outcome: "created", kind: "git-commit", id: "sha-created", success: true, wantEntry: true},
		{name: "failed PR create", family: "pr_create", outcome: "failed", kind: "pull-request:create", id: "41", success: false},
		{name: "failed PR merge", family: "pr_merge", outcome: "failed", kind: "pull-request:merge", id: "41", success: false},
		{name: "created PR", family: "pr_create", outcome: "created", kind: "pull-request:create", id: "42", success: true, wantEntry: true},
		{name: "merged PR", family: "pr_merge", outcome: "merged", kind: "pull-request:merge", id: "42", success: true, wantEntry: true},
		{name: "completed task", family: "task_completed", outcome: "completed", kind: "task-completion", id: "task-42", success: true, wantEntry: true},
		{name: "completed task missing ID", family: "task_completed", outcome: "completed", kind: "task-completion", success: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			envelope := normalizedJournalHookEnvelope{
				ContractVersion:   journalHookEnvelopeVersion,
				Target:            "synthetic",
				Event:             "PostToolUse",
				Success:           boolPtr(test.success),
				Outcome:           test.outcome,
				CommandFamily:     test.family,
				DurableResultKind: test.kind,
				DurableResultID:   test.id,
				Message:           "matrix",
				TaskDescription:   "matrix task",
			}
			entry, ok, warning := journalHookEntryForEnvelope(envelope, nil, true)
			if ok != test.wantEntry {
				t.Fatalf("entry=%q ok=%t warning=%#v, want ok=%t", entry, ok, warning, test.wantEntry)
			}
			if test.wantEntry && warning != nil {
				t.Fatalf("successful matrix warning = %#v", warning)
			}
			if !test.wantEntry && warning == nil {
				t.Fatalf("rejected matrix case warning=nil, want structured diagnostic")
			}
		})
	}
}

func TestJournalHookTrustedDurableReplayIncludesTaskAndMissingState(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	envelope := normalizedJournalHookEnvelope{
		ContractVersion:   journalHookEnvelopeVersion,
		Target:            "synthetic",
		Event:             "TaskCompleted",
		Success:           boolPtr(true),
		Outcome:           "completed",
		CommandFamily:     "task_completed",
		DurableResultKind: "task-completion",
		DurableResultID:   "task-replay",
		TaskDescription:   "replayed task",
	}
	entry, ok, warning := journalHookEntryForEnvelope(envelope, nil, true)
	if !ok || warning != nil || entry != "task(completed): replayed task" {
		t.Fatalf("trusted task entry=%q ok=%t warning=%#v", entry, ok, warning)
	}
	origin := envelope.origin("")
	resolver := state.PathResolver{StateHome: stateHome}
	first, err := state.LogJournal(context.Background(), root, resolver, state.JournalLogOptions{Entry: entry, Origin: &origin})
	if err != nil {
		t.Fatalf("first task log error = %v", err)
	}
	second, err := state.LogJournal(context.Background(), root, resolver, state.JournalLogOptions{Entry: entry, Origin: &origin})
	if err != nil || first.ID != second.ID {
		t.Fatalf("task replay = %#v/%#v error=%v, want same canonical ID", first, second, err)
	}
	if first.EntryType != "task" || first.Scope != "completed" {
		t.Fatalf("task result = %#v, want task(completed)", first)
	}
	freshRoot, freshHome := freshHookRunnerDir(t)
	freshProject, err := project.ResolveRoot(freshRoot)
	if err != nil {
		t.Fatalf("fresh ResolveRoot() error = %v", err)
	}
	if _, err := state.LogJournal(context.Background(), freshProject, state.PathResolver{StateHome: freshHome}, state.JournalLogOptions{Entry: entry, Origin: &origin}); err == nil || !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("missing initialized state error = %v, want visible state error", err)
	}
}

func TestJournalHookNamespacedFixturesPersistExactOriginAndReload(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	fixtures := []struct {
		name, target, namespace, harness, version, session, event string
	}{
		{"claude", "claude-code", "claude_code", "claude-runtime", "1.2.3", "claude-session", "PostToolUse"},
		{"cursor", "cursor", "cursor", "cursor-runtime", "0.9.0", "cursor-session", "tool_result"},
		{"codex", "codex", "codex", "codex-runtime", "0.144.1", "codex-session", "Stop"},
		{"opencode", "opencode", "opencode", "opencode-runtime", "1.0.0", "open-session", "compaction"},
		{"amp", "amp", "amp", "amp-runtime", "2.0.0", "amp-session", "session.start"},
	}
	for index, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			namespace := map[string]any{
				"harness":           fixture.harness,
				"version":           fixture.version,
				"session_id":        fixture.session,
				"event":             fixture.event,
				"success":           true,
				"outcome":           "created",
				"command_family":    "git_commit",
				"durable_result_id": fmt.Sprintf("fixture-sha-%d", index),
				"message":           "namespaced message",
				"unknown":           "must not persist",
			}
			raw := map[string]any{
				"target":          fixture.target,
				"session_id":      "top-level-conflict",
				"message":         "top-level-conflict",
				fixture.namespace: namespace,
			}
			envelope := normalizeJournalHookEnvelope(journalHookInput{Raw: raw}, workingDir)
			if envelope.Target != fixture.target || envelope.Harness != fixture.harness || envelope.HarnessVersion != fixture.version || envelope.Event != fixture.event || envelope.SessionID != fixture.session || envelope.AgentID != "" || envelope.Worktree != workingDir {
				t.Fatalf("normalized envelope = %#v, want exact namespaced identity", envelope)
			}
			entry, ok, warning := journalHookEntryForEnvelope(envelope, raw, true)
			if !ok || warning != nil || !strings.Contains(entry, "namespaced message") || strings.Contains(entry, "top-level-conflict") {
				t.Fatalf("entry=%q ok=%t warning=%#v, want namespaced message only", entry, ok, warning)
			}
			origin := envelope.origin("")
			result, err := state.LogJournal(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.JournalLogOptions{Entry: entry, ObservedWorktree: workingDir, HarnessSessionID: envelope.SessionID, Origin: &origin})
			if err != nil {
				t.Fatalf("persist fixture error = %v", err)
			}
			show, err := state.ShowJournal(context.Background(), root, state.PathResolver{StateHome: stateHome}, result.ID)
			if err != nil {
				t.Fatalf("reload fixture error = %v", err)
			}
			if show.Entry.ID != result.ID || show.Entry.Message != "namespaced message" || show.Entry.ObservedWorktree != workingDir || show.Entry.HarnessSessionID != fixture.session {
				t.Fatalf("reloaded entry = %#v, want exact persisted fields", show.Entry)
			}
			if show.Origin == nil || show.Origin.ObservedHarness != fixture.harness || show.Origin.ObservedHarnessVersion != fixture.version || show.Origin.HarnessSessionID != fixture.session || show.Origin.AgentID != "" || show.Origin.SourceEvent != fixture.event || show.Origin.Worktree != workingDir || show.Origin.DurableResultID != fmt.Sprintf("fixture-sha-%d", index) {
				t.Fatalf("reloaded origin = %#v, want exact normalized origin", show.Origin)
			}
		})
	}
}

func TestNormalizeJournalHookEnvelopeDoesNotFabricateUnknownFields(t *testing.T) {
	envelope := normalizeJournalHookEnvelope(journalHookInput{Raw: map[string]any{
		"target": "claude-code", "unknown_session": "secret", "unknown_success": true,
	}}, "")
	if envelope.SessionID != "" || envelope.AgentID != "" || envelope.Success != nil || envelope.DurableResultID != "" {
		t.Fatalf("envelope = %#v, want unknown fields omitted", envelope)
	}
}

func TestNormalizeJournalHookEnvelopeIsolatesRecognizedNamespace(t *testing.T) {
	envelope := normalizeJournalHookEnvelope(journalHookInput{Raw: map[string]any{
		"target":     "cursor",
		"session_id": "top-session",
		"agent_id":   "top-agent",
		"version":    "top-version",
		"cursor": map[string]any{
			"session_id":        "cursor-session",
			"agent_id":          "cursor-agent",
			"version":           "cursor-version",
			"success":           true,
			"command_family":    "git_commit",
			"durable_result_id": "cursor-sha",
		},
	}}, "")
	if envelope.SessionID != "cursor-session" || envelope.AgentID != "cursor-agent" || envelope.HarnessVersion != "cursor-version" || envelope.DurableResultID != "cursor-sha" {
		t.Fatalf("isolated envelope = %#v, want only cursor namespace fields", envelope)
	}
}

func TestJournalLogFromHookRejectsSyntheticTargetFromStdin(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(`{"target":"synthetic","event":"PostToolUse","success":true,"command_family":"git_commit","durable_result_id":"sha-stdin"}`)}).Run([]string{"journal", "log", "--from-hook"})
	if err != nil {
		t.Fatalf("synthetic hook error = %v", err)
	}
	var warning journalHookWarning
	if err := json.Unmarshal(stdout.Bytes(), &warning); err != nil || warning.Code != journalHookDiagnosticUnsupported {
		t.Fatalf("synthetic warning = %#v decode=%v, want unsupported diagnostic", warning, err)
	}
}

func TestJournalLogFromHookMalformedPayloadIsNonBlockingDiagnostic(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(`{"unterminated"`)}).Run([]string{"journal", "log", "--from-hook"})
	if err != nil {
		t.Fatalf("malformed hook error = %v", err)
	}
	var warning journalHookWarning
	if err := json.Unmarshal(stdout.Bytes(), &warning); err != nil || warning.Code != journalHookDiagnosticInvalidEnvelope || !warning.NonBlocking {
		t.Fatalf("malformed warning = %#v decode=%v, want nonblocking invalid-envelope diagnostic", warning, err)
	}
}

func TestJournalLifecycleHooksUseNamespacedSubagentSuppression(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for _, command := range [][]string{{"journal", "context", "for-prompt"}, {"journal", "context", "for-compact"}, {"journal", "context", "for-resumption"}, {"journal", "context", "--from-hook"}} {
		var stdout bytes.Buffer
		err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(`{"cursor":{"session_id":"s","agent_id":"nested-subagent","event":"SessionStart"}}`)}).Run(command)
		if err != nil {
			t.Fatalf("%v error = %v", command, err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("%v stdout = %q, want silent nested subagent", command, stdout.String())
		}
	}
}

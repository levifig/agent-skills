package cli

import (
	"bytes"
	"strings"
	"testing"
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
	for _, want := range []string{"loaf journal context", "latest wrap:", "wrap(hooks): checkpoint one"} {
		if !strings.Contains(out, want) {
			t.Fatalf("journal context --from-hook stdout = %q, want %q", out, want)
		}
	}
}

func TestJournalLogFromHookDerivesTaskCompletedEntry(t *testing.T) {
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
	if !strings.Contains(stdout.String(), "task(completed): wire journal hooks") {
		t.Fatalf("journal log --from-hook (task) stdout = %q, want task(completed) entry", stdout.String())
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

// TestJournalLogFromHookDegradesOnMissingState verifies git/gh/task hook logging
// writes nothing and exits 0 when the state database is absent (SPEC-056 M2).
func TestJournalLogFromHookDegradesOnMissingState(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"s1","hook_event_name":"TaskCompleted","task_description":"x"}`),
	}.Run([]string{"journal", "log", "--from-hook"})
	if err != nil {
		t.Fatalf("journal log --from-hook (missing state) error = %v, want nil (non-blocking)", err)
	}
	if stdout.String() != "" {
		t.Fatalf("journal log --from-hook (missing state) stdout = %q, want empty (no write, no error)", stdout.String())
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

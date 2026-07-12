package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func TestJournalLogUnmatchedUnblockHumanAndJSON(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	runner := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}

	err := runner.Run([]string{"journal", "log", "unblock(human-key): never matched"})
	var unmatched *state.JournalUnmatchedUnblockError
	if !errors.As(err, &unmatched) || unmatched.Code != state.JournalUnmatchedUnblockCode || unmatched.Key != "human-key" {
		t.Fatalf("human journal log error = %T %#v, want typed unmatched error", err, err)
	}

	var blockOut bytes.Buffer
	if err := (Runner{Stdout: &blockOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "block(json-key): open", "--json"}); err != nil {
		t.Fatalf("block --json error = %v\n%s", err, blockOut.String())
	}
	var block state.JournalLogResult
	if err := json.Unmarshal(blockOut.Bytes(), &block); err != nil {
		t.Fatalf("decode block result: %v\n%s", err, blockOut.String())
	}
	var unblockOut bytes.Buffer
	if err := (Runner{Stdout: &unblockOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "unblock(json-key): resolved", "--json"}); err != nil {
		t.Fatalf("unblock --json error = %v\n%s", err, unblockOut.String())
	}
	var unblock state.JournalLogResult
	if err := json.Unmarshal(unblockOut.Bytes(), &unblock); err != nil {
		t.Fatalf("decode unblock result: %v\n%s", err, unblockOut.String())
	}

	var output bytes.Buffer
	err = (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "unblock(json-key): duplicate", "--json"})
	var exitErr interface {
		ExitCode() int
		Silent() bool
	}
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || !exitErr.Silent() {
		t.Fatalf("JSON duplicate error = %v, want silent exit 1", err)
	}
	var envelope journalUnmatchedUnblockJSON
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("decode unmatched envelope: %v\n%s", err, output.String())
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("unmatched JSON output = %q, want exactly one object (trailing decode error %v)", output.String(), err)
	}
	if envelope.ContractVersion != state.StateJSONContractVersion || envelope.Command != "journal log" || envelope.Code != state.JournalUnmatchedUnblockCode || envelope.Key != "json-key" || envelope.LatestSourceID != unblock.ID || envelope.LatestSourceType != "unblock" || envelope.Error == "" {
		t.Fatalf("unmatched envelope = %#v, want typed fields and latest unblock", envelope)
	}
	if block.ID == "" || unblock.ID == "" {
		t.Fatalf("successful IDs block=%q unblock=%q, want nonempty", block.ID, unblock.ID)
	}
}

func TestJournalLogFromHookUnmatchedUnblockWarnsWithoutWriting(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	log := func(entry string) state.JournalLogResult {
		t.Helper()
		var output bytes.Buffer
		if err := (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", entry, "--json"}); err != nil {
			t.Fatalf("journal log %q error = %v\n%s", entry, err, output.String())
		}
		var result state.JournalLogResult
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatalf("decode journal log %q: %v\n%s", entry, err, output.String())
		}
		return result
	}
	log("block(hook-key): open")
	resolved := log("unblock(hook-key): resolved")

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
	canonicalBefore := sqliteCount(t, db, `SELECT COUNT(*) FROM journal_entries`)
	searchBefore := sqliteCount(t, db, `SELECT COUNT(*) FROM journal_search`)
	originsBefore := sqliteCount(t, db, `SELECT COUNT(*) FROM journal_origins`)

	var output bytes.Buffer
	err = (Runner{
		Stdout:     &output,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"hook-session"}`),
	}).Run([]string{"journal", "log", "--from-hook", "unblock(hook-key): duplicate adapter event"})
	if err != nil {
		t.Fatalf("journal log --from-hook unmatched error = %v, want nonblocking nil\n%s", err, output.String())
	}
	var warning journalHookWarning
	if err := json.Unmarshal(output.Bytes(), &warning); err != nil {
		t.Fatalf("decode hook warning: %v\n%s", err, output.String())
	}
	if warning.ContractVersion != state.StateJSONContractVersion || warning.Kind != "journal-diagnostic" || warning.Severity != "warning" || warning.Code != state.JournalUnmatchedUnblockCode || warning.Key != "hook-key" || warning.LatestSourceID != resolved.ID || warning.LatestSourceType != "unblock" || !warning.NonBlocking || warning.Message == "" {
		t.Fatalf("hook warning = %#v, want structured nonblocking diagnostic", warning)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM journal_entries`); got != canonicalBefore {
		t.Fatalf("canonical rows after hook warning = %d, want %d", got, canonicalBefore)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM journal_search`); got != searchBefore {
		t.Fatalf("search rows after hook warning = %d, want %d", got, searchBefore)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM journal_origins`); got != originsBefore {
		t.Fatalf("origin rows after hook warning = %d, want %d", got, originsBefore)
	}
}

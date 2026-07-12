package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func TestParseSchemaUpgradeArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    schemaUpgradeOptions
		wantErr string
	}{
		{name: "dry run", args: []string{"--dry-run", "--json"}, want: schemaUpgradeOptions{dryRun: true, jsonOutput: true}},
		{name: "apply", args: []string{"--apply"}, want: schemaUpgradeOptions{apply: true}},
		{name: "mutually exclusive", args: []string{"--dry-run", "--apply"}, wantErr: "cannot combine"},
		{name: "required action", args: []string{"--json"}, wantErr: "requires --dry-run or --apply"},
		{name: "duplicate flag", args: []string{"--apply", "--apply"}, wantErr: "cannot repeat --apply"},
		{name: "unknown flag", args: []string{"--wat"}, wantErr: "unknown option"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSchemaUpgradeArgs(tt.args, "loaf state migrate schema")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseSchemaUpgradeArgs(%v) error = %v, want substring %q", tt.args, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSchemaUpgradeArgs(%v) error = %v", tt.args, err)
			}
			if got != tt.want {
				t.Fatalf("parseSchemaUpgradeArgs(%v) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}

func TestSchemaUpgradeHelpRoutesBothAliases(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "state alias", args: []string{"state", "migrate", "schema", "--help"}, want: "Usage: loaf state migrate schema [--dry-run|--apply] [--json]"},
		{name: "top level alias", args: []string{"migrate", "schema", "--help"}, want: "Usage: loaf migrate schema [--dry-run|--apply] [--json]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: t.TempDir()}).Run(tt.args)
			if err != nil {
				t.Fatalf("Run(%v) error = %v", tt.args, err)
			}
			if !strings.Contains(stdout.String(), tt.want) {
				t.Fatalf("Run(%v) stdout = %q, want %q", tt.args, stdout.String(), tt.want)
			}
		})
	}
}

func TestSchemaUpgradeCurrentDatabaseIsNoOp(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var dryRunOut bytes.Buffer
	err := (Runner{Stdout: &dryRunOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "schema", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("schema dry-run error = %v\n%s", err, dryRunOut.String())
	}
	var dryRun state.SchemaUpgradeResult
	if err := json.Unmarshal(dryRunOut.Bytes(), &dryRun); err != nil {
		t.Fatalf("decode dry-run JSON %q: %v", dryRunOut.String(), err)
	}
	if dryRun.Action != state.SchemaUpgradeActionAlreadyReady || dryRun.Applied || !dryRun.Verified || dryRun.BackupVerified || dryRun.SchemaVersion != state.CurrentSchemaVersion() {
		t.Fatalf("dry-run result = %#v, want already-current, unapplied, verified", dryRun)
	}
	if dryRun.BackupPath != "" {
		t.Fatalf("dry-run backup path = %q, want empty", dryRun.BackupPath)
	}

	var applyOut bytes.Buffer
	err = (Runner{Stdout: &applyOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"migrate", "schema", "--apply", "--json"})
	if err != nil {
		t.Fatalf("schema apply error = %v\n%s", err, applyOut.String())
	}
	var applied state.SchemaUpgradeResult
	if err := json.Unmarshal(applyOut.Bytes(), &applied); err != nil {
		t.Fatalf("decode apply JSON %q: %v", applyOut.String(), err)
	}
	if applied.Action != state.SchemaUpgradeActionAlreadyReady || applied.Applied || !applied.Verified || applied.BackupVerified || applied.SchemaVersion != state.CurrentSchemaVersion() {
		t.Fatalf("apply result = %#v, want already-current, unapplied, verified", applied)
	}
}

func TestSchemaUpgradeRequiredErrorJSONContract(t *testing.T) {
	err := &state.SchemaUpgradeRequiredError{
		Code:            state.SchemaUpgradeRequiredCode,
		DatabasePath:    "/tmp/loaf.sqlite",
		CurrentVersion:  10,
		RequiredVersion: 11,
		PendingVersions: []int{11},
		Command:         "loaf state migrate schema --apply",
	}
	var out bytes.Buffer
	returned := writeJSONCommandError(&out, "journal log", err)
	var payload map[string]any
	if decodeErr := json.Unmarshal(out.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode JSON %q: %v", out.String(), decodeErr)
	}
	for key, want := range map[string]any{
		"code":             state.SchemaUpgradeRequiredCode,
		"database_path":    "/tmp/loaf.sqlite",
		"current_version":  float64(10),
		"required_version": float64(11),
		"suggestion":       "loaf state migrate schema --apply",
	} {
		if payload[key] != want {
			t.Fatalf("JSON[%q] = %#v, want %#v", key, payload[key], want)
		}
	}
	pending, ok := payload["pending_versions"].([]any)
	if !ok || len(pending) != 1 || pending[0] != float64(11) {
		t.Fatalf("pending_versions = %#v, want [11]", payload["pending_versions"])
	}
	var exitErr ExitError
	if !errors.As(returned, &exitErr) {
		t.Fatalf("writeJSONCommandError() error = %v, want ExitError", returned)
	}
}

func setupCLISchema10State(t *testing.T) (workingDir, stateHome, databasePath, projectID string) {
	t.Helper()
	workingDir = realpath(t, t.TempDir())
	stateHome = t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	databasePath, err = resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	db, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	migrations := append([]state.SchemaMigration{}, state.SchemaMigrations()[:9]...)
	migrations = append(migrations, state.JournalFirstMigration())
	if err := state.ApplyMigrations(context.Background(), db, migrations); err != nil {
		db.Close()
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close() error = %v", err)
	}
	store, err := state.OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := store.UpsertProject(context.Background(), root); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	identity, err := store.LookupProjectIdentityForRoot(context.Background(), root)
	if closeErr := store.Close(); err != nil {
		t.Fatalf("LookupProjectIdentityForRoot() error = %v", err)
	} else if closeErr != nil {
		t.Fatalf("Store.Close() error = %v", closeErr)
	}
	return workingDir, stateHome, databasePath, identity.ID
}

func TestProjectMutationsRefuseSchema10WithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		args func(workingDir, projectID string) []string
	}{
		{name: "rename", args: func(_, _ string) []string {
			return []string{"project", "rename", "renamed", "--json"}
		}},
		{name: "move", args: func(workingDir, _ string) []string {
			return []string{"project", "move", "--from", workingDir, "--to", filepath.Join(workingDir, "moved"), "--json"}
		}},
		{name: "delete", args: func(_, projectID string) []string {
			return []string{"project", "delete", projectID, "--yes", "--json"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workingDir, stateHome, databasePath, projectID := setupCLISchema10State(t)
			before, err := os.ReadFile(databasePath)
			if err != nil {
				t.Fatalf("read schema10 database before %s: %v", tt.name, err)
			}
			beforeTables := schema10CLITableSnapshot(t, databasePath)
			var output bytes.Buffer
			err = (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run(tt.args(workingDir, projectID))
			if err == nil {
				t.Fatalf("%s schema10 mutation error = nil, want schema upgrade requirement", tt.name)
			}
			var payload struct {
				Code string `json:"code"`
			}
			if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
				t.Fatalf("decode %s error JSON %q: %v", tt.name, output.String(), decodeErr)
			}
			if payload.Code != state.SchemaUpgradeRequiredCode {
				t.Fatalf("%s error code = %q, want %q (error %v, output %q)", tt.name, payload.Code, state.SchemaUpgradeRequiredCode, err, output.String())
			}
			after, err := os.ReadFile(databasePath)
			if err != nil {
				t.Fatalf("read schema10 database after %s: %v", tt.name, err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("schema10 database changed after refused %s", tt.name)
			}
			afterTables := schema10CLITableSnapshot(t, databasePath)
			if !reflect.DeepEqual(beforeTables, afterTables) {
				t.Fatalf("schema10 tables changed after refused %s:\nbefore=%#v\nafter=%#v", tt.name, beforeTables, afterTables)
			}
		})
	}
}

type schema10ProjectPathRow struct {
	ID          string
	ProjectID   string
	Path        string
	Current     int
	FirstSeenAt string
	LastSeenAt  string
	CreatedAt   string
	UpdatedAt   string
}

type schema10CLISnapshot struct {
	TableRows    map[string]int
	ProjectPaths []schema10ProjectPathRow
}

func schema10CLITableSnapshot(t *testing.T, databasePath string) schema10CLISnapshot {
	t.Helper()
	db, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatalf("open schema10 database snapshot: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("list schema10 tables: %v", err)
	}
	defer rows.Close()
	snapshot := schema10CLISnapshot{TableRows: map[string]int{}}
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("scan schema10 table name: %v", err)
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM "` + strings.ReplaceAll(table, `"`, `""`) + `"`).Scan(&count); err != nil {
			t.Fatalf("count schema10 table %s: %v", table, err)
		}
		snapshot.TableRows[table] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema10 tables: %v", err)
	}

	pathRows, err := db.Query(`SELECT id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at FROM project_paths ORDER BY id`)
	if err != nil {
		t.Fatalf("query schema10 project paths: %v", err)
	}
	defer pathRows.Close()
	for pathRows.Next() {
		var row schema10ProjectPathRow
		if err := pathRows.Scan(&row.ID, &row.ProjectID, &row.Path, &row.Current, &row.FirstSeenAt, &row.LastSeenAt, &row.CreatedAt, &row.UpdatedAt); err != nil {
			t.Fatalf("scan schema10 project path: %v", err)
		}
		snapshot.ProjectPaths = append(snapshot.ProjectPaths, row)
	}
	if err := pathRows.Err(); err != nil {
		t.Fatalf("iterate schema10 project paths: %v", err)
	}
	return snapshot
}

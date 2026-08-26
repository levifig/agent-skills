package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"errors"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func TestRunnerRefusesSubstrateCommandWhenUnattached(t *testing.T) {
	workingDir, stateHome, projectID := setupAuthGateProject(t)
	seedSubstrateMode(t, stateHome)
	var stderr bytes.Buffer
	err := Runner{
		Stdout:     &bytes.Buffer{},
		Stderr:     &stderr,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"journal", "recent"})
	var exit ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("Run() = %v, want ExitError code 1", err)
	}
	if !strings.Contains(stderr.String(), "not attached") {
		t.Fatalf("stderr = %q, want unattached refusal", stderr.String())
	}
	_ = projectID
}

func TestRunnerAllowsAuthWhenUnattached(t *testing.T) {
	workingDir, stateHome, _ := setupAuthGateProject(t)
	err := Runner{
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"auth", "list", "--server-db", t.TempDir() + "/sync.sqlite"})
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "not attached") {
		t.Fatalf("Run() = %v, auth list should not be blocked by attach gate", err)
	}
}

func TestRunnerAllowsSubstrateCommandWhenAttached(t *testing.T) {
	workingDir, stateHome, projectID := setupAuthGateProject(t)
	seedSubstrateMode(t, stateHome)
	dataHome, err := (state.PathResolver{StateHome: stateHome}).ResolvedDataHome()
	if err != nil {
		t.Fatalf("ResolvedDataHome() error = %v", err)
	}
	if err := auth.SaveAttachRecord(dataHome, auth.AttachRecord{ProjectID: projectID, EnvID: "test-env", Endpoint: "http://127.0.0.1:8080"}); err != nil {
		t.Fatalf("SaveAttachRecord() error = %v", err)
	}
	err = Runner{
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"journal", "recent"})
	if err != nil {
		if strings.Contains(err.Error(), "not attached") {
			t.Fatalf("Run() = %v, want attached journal to pass attach gate", err)
		}
	}
}

func setupAuthGateProject(t *testing.T) (workingDir, stateHome, projectID string) {
	t.Helper()
	workingDir = t.TempDir()
	stateHome = t.TempDir()
	var initOut bytes.Buffer
	if err := (Runner{Stdout: &initOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init", "--json"}); err != nil {
		t.Fatalf("state init error = %v\n%s", err, initOut.String())
	}
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	store, err := state.OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	identity, err := store.ProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("ProjectIdentityForRoot() error = %v", err)
	}
	return workingDir, stateHome, identity.ID
}

func TestRunnerAttachGateJSONRefusal(t *testing.T) {
	workingDir, stateHome, _ := setupAuthGateProject(t)
	seedSubstrateMode(t, stateHome)
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stderr:     &stderr,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"journal", "recent", "--json"})
	if err == nil {
		t.Fatal("Run() error = nil, want refusal")
	}
	var payload commandErrorJSON
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("json.Unmarshal() error = %v\nstdout=%q stderr=%q", decodeErr, stdout.String(), stderr.String())
	}
	if payload.Code != auth.UnattachedCode {
		t.Fatalf("code = %q, want %q", payload.Code, auth.UnattachedCode)
	}
}

func TestRunnerSubstrateGateInactiveBeforeSetup(t *testing.T) {
	workingDir, stateHome, _ := setupAuthGateProject(t)
	err := Runner{
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"journal", "recent"})
	if err != nil {
		if strings.Contains(err.Error(), "not attached") {
			t.Fatalf("Run() = %v, gate should stay inactive before auth setup", err)
		}
	}
}

func seedSubstrateMode(t *testing.T, stateHome string) {
	t.Helper()
	dataHome, err := (state.PathResolver{StateHome: stateHome}).ResolvedDataHome()
	if err != nil {
		t.Fatalf("ResolvedDataHome() error = %v", err)
	}
	serverDB := filepath.Join(t.TempDir(), "sync.sqlite")
	if _, err := auth.SetupAccount(context.Background(), auth.AdminConfig{DataHome: dataHome, ServerDB: serverDB}, "http://127.0.0.1:8080"); err != nil {
		t.Fatalf("SetupAccount() error = %v", err)
	}
}

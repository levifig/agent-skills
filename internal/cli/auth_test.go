package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/state"
)

func testAuthStateHome(t *testing.T) string {
	t.Helper()
	stateHome := t.TempDir()
	authDir, err := state.PathResolver{StateHome: stateHome}.AuthDir()
	if err != nil {
		t.Fatalf("AuthDir() error = %v", err)
	}
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	return stateHome
}

func TestAttachGateRefusesJournalWhenConfiguredButUnattached(t *testing.T) {
	t.Parallel()
	stateHome := testAuthStateHome(t)
	authDir, err := state.PathResolver{StateHome: stateHome}.AuthDir()
	if err != nil {
		t.Fatalf("AuthDir() error = %v", err)
	}
	ctx := context.Background()
	serverDB := filepath.Join(t.TempDir(), "sync.sqlite")
	if _, err := auth.Setup(ctx, auth.NewStore(authDir), auth.SetupInput{
		Endpoint: "http://127.0.0.1:8080",
		ServerDB: serverDB,
	}); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	var stderr bytes.Buffer
	err = Runner{
		Stderr:     &stderr,
		WorkingDir: t.TempDir(),
		StateHome:  stateHome,
	}.Run([]string{"journal", "recent"})
	if err == nil {
		t.Fatal("Run() error = nil, want attach refusal")
	}
	var exit ExitError
	if !errorsAsExit(err, &exit) || exit.Code != 1 {
		t.Fatalf("Run() error = %v, want ExitError code 1", err)
	}
	if !strings.Contains(stderr.String(), "not attached") {
		t.Fatalf("stderr = %q, want unattached refusal", stderr.String())
	}
}

func TestAttachGateAllowsJournalBeforeAuthSetup(t *testing.T) {
	t.Parallel()
	stateHome := testAuthStateHome(t)
	var stderr bytes.Buffer
	err := Runner{
		Stderr:     &stderr,
		WorkingDir: t.TempDir(),
		StateHome:  stateHome,
	}.Run([]string{"state", "path"})
	if err != nil {
		t.Fatalf("Run(state path) error = %v", err)
	}
}

func TestAttachGateAllowsAuthSetupWhenUnattached(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := Runner{
		Stderr:     &stderr,
		WorkingDir: t.TempDir(),
		StateHome:  t.TempDir(),
	}.Run([]string{"auth", "setup", "--help"})
	if err != nil {
		t.Fatalf("Run(auth setup --help) error = %v", err)
	}
}

func errorsAsExit(err error, target *ExitError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(ExitError); ok {
		*target = e
		return true
	}
	return false
}

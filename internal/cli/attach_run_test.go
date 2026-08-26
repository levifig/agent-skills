package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/state"
)

func TestSessionStartAttachGateFailsClosedWhenConfiguredButUnattached(t *testing.T) {
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
	}.Run([]string{"journal", "context", "--cursor-hook", "--from-hook"})
	if err == nil {
		t.Fatal("Run() error = nil, want SessionStart attach refusal")
	}
	var exit ExitError
	if !errorsAsExit(err, &exit) || exit.Code != 1 {
		t.Fatalf("Run() error = %v, want ExitError code 1", err)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("not attached")) {
		t.Fatalf("stderr = %q, want unattached refusal", stderr.String())
	}
}

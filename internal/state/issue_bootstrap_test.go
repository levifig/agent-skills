package state

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestBootstrapTrackerSteeringTrackerlessGuidance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := projectRootInTempDir(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	got, err := BootstrapTrackerSteering(ctx, root, resolver)
	if err != nil {
		t.Fatalf("BootstrapTrackerSteering() error = %v", err)
	}
	if got.Mode != BootstrapTrackerModeTrackerless {
		t.Fatalf("Mode = %q, want %q", got.Mode, BootstrapTrackerModeTrackerless)
	}
	if !strings.Contains(got.Guidance, "branch:") || !strings.Contains(got.Guidance, "pr:") {
		t.Fatalf("Guidance = %q, want branch/pr guidance", got.Guidance)
	}
}

func TestBootstrapTrackerSteeringSteersLinear(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rootDir := t.TempDir()
	writeLinearBootstrapConfig(t, rootDir, "OPS")
	root, err := project.ResolveRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	got, err := BootstrapTrackerSteering(ctx, root, resolver)
	if err != nil {
		t.Fatalf("BootstrapTrackerSteering() error = %v", err)
	}
	if got.Mode != BootstrapTrackerModeLinear || !got.Applied {
		t.Fatalf("result = %#v, want linear applied", got)
	}
	if got.Authority != IssueAuthorityLinear || got.Prefix != "OPS" {
		t.Fatalf("authority/prefix = %q/%q, want linear/OPS", got.Authority, got.Prefix)
	}
}

func TestBootstrapTrackerSteeringRefusesGitHubAuthority(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rootDir := t.TempDir()
	writeIssueAuthorityConfig(t, rootDir, "github")
	root, err := project.ResolveRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	_, err = BootstrapTrackerSteering(ctx, root, resolver)
	var refused *UnsupportedIssueAuthorityBootstrapError
	if !errors.As(err, &refused) {
		t.Fatalf("BootstrapTrackerSteering() error = %v, want *UnsupportedIssueAuthorityBootstrapError", err)
	}
}

func TestRefuseUnsupportedIssueBootstrap(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	writeIssueAuthorityConfig(t, rootDir, "gitlab")
	err := RefuseUnsupportedIssueBootstrap(rootDir)
	if err == nil {
		t.Fatal("RefuseUnsupportedIssueBootstrap() = nil, want error")
	}
}

func writeLinearBootstrapConfig(t *testing.T, rootDir, prefix string) {
	t.Helper()
	agents := filepath.Join(rootDir, ".agents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"schema_version": "1.0.0",
		"integrations": map[string]any{
			"linear": map[string]any{"enabled": true},
		},
		"issue": map[string]any{"authority": "local", "prefix": prefix},
	}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(agents, "loaf.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeIssueAuthorityConfig(t *testing.T, rootDir, authority string) {
	t.Helper()
	agents := filepath.Join(rootDir, ".agents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{"schema_version": "1.0.0", "issue": map[string]any{"authority": authority}}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(agents, "loaf.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

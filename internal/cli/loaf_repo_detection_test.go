package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func TestDetectLoafRepoTiers(t *testing.T) {
	tests := []struct {
		name string
		// setup prepares the candidate directory; LOAF_DB is already isolated to
		// a fresh nonexistent path when it runs.
		setup     func(t *testing.T, dir string)
		wantTier  loafRepoTier
		wantBases []string
	}{
		{
			name:      "empty directory",
			setup:     func(*testing.T, string) {},
			wantTier:  loafRepoTierNone,
			wantBases: nil,
		},
		{
			name: "legacy artifact folders only",
			setup: func(t *testing.T, dir string) {
				mustMakeDetectionDirs(t, dir, ".agents/specs", ".agents/sessions")
			},
			wantTier:  loafRepoTierLegacy,
			wantBases: []string{"legacy Loaf artifact folders: .agents/sessions, .agents/specs"},
		},
		{
			name: "fenced marker in AGENTS.md",
			setup: func(t *testing.T, dir string) {
				mustWriteFencedAgentsFile(t, dir)
			},
			wantTier:  loafRepoTierStrong,
			wantBases: []string{"managed Loaf section in AGENTS.md"},
		},
		{
			name: "project config in .agents",
			setup: func(t *testing.T, dir string) {
				mustWriteLoafProjectConfig(t, dir)
			},
			wantTier:  loafRepoTierStrong,
			wantBases: []string{"Loaf project config at .agents/loaf.json"},
		},
		{
			name: "project record in state database",
			setup: func(t *testing.T, dir string) {
				mustRegisterDetectionProject(t, dir)
			},
			wantTier:  loafRepoTierAuthoritative,
			wantBases: []string{"project record proj_"},
		},
		{
			name: "mixed signals resolve to the strongest tier",
			setup: func(t *testing.T, dir string) {
				mustMakeDetectionDirs(t, dir, ".agents/drafts")
				mustWriteFencedAgentsFile(t, dir)
				mustWriteLoafProjectConfig(t, dir)
				mustRegisterDetectionProject(t, dir)
			},
			wantTier: loafRepoTierAuthoritative,
			wantBases: []string{
				"project record proj_",
				"managed Loaf section in AGENTS.md",
				"Loaf project config at .agents/loaf.json",
				"legacy Loaf artifact folders: .agents/drafts",
			},
		},
		{
			name: "moved repo keeps its marker but has no project record",
			setup: func(t *testing.T, dir string) {
				mustRegisterDetectionProject(t, t.TempDir())
				mustWriteFencedAgentsFile(t, dir)
			},
			wantTier:  loafRepoTierStrong,
			wantBases: []string{"managed Loaf section in AGENTS.md"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
			test.setup(t, dir)

			detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), "")

			if detection.Tier != test.wantTier {
				t.Fatalf("detectLoafRepo() tier = %s, want %s (bases %v)", detection.Tier, test.wantTier, detection.Bases)
			}
			assertDetectionBases(t, detection.Bases, test.wantBases)
		})
	}
}

func TestDetectLoafRepoDegradesOnUnreadableDatabase(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
	if err := os.WriteFile(databasePath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("LOAF_DB", databasePath)
	mustWriteFencedAgentsFile(t, dir)

	detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), "")

	if detection.Tier != loafRepoTierStrong {
		t.Fatalf("detectLoafRepo() tier = %s, want %s", detection.Tier, loafRepoTierStrong)
	}
	assertDetectionBases(t, detection.Bases, []string{"managed Loaf section in AGENTS.md"})
}

func TestDetectLoafRepoLeavesTheDirectoryUnchanged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	mustMakeDetectionDirs(t, dir, ".agents/specs")
	mustWriteFencedAgentsFile(t, dir)
	mustWriteLoafProjectConfig(t, dir)
	mustRegisterDetectionProject(t, dir)
	before := snapshotDetectionTree(t, dir)

	if detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), ""); detection.Tier != loafRepoTierAuthoritative {
		t.Fatalf("detectLoafRepo() tier = %s, want %s", detection.Tier, loafRepoTierAuthoritative)
	}

	if after := snapshotDetectionTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("detectLoafRepo() mutated the project directory:\nbefore = %v\nafter  = %v", before, after)
	}
}

func TestLoafRepoTierOrdering(t *testing.T) {
	if !(loafRepoTierNone < loafRepoTierLegacy && loafRepoTierLegacy < loafRepoTierStrong && loafRepoTierStrong < loafRepoTierAuthoritative) {
		t.Fatal("loafRepoTier constants must ascend with strength; commands compare them directly")
	}
	for tier, want := range map[loafRepoTier]string{
		loafRepoTierNone:          "none",
		loafRepoTierLegacy:        "legacy",
		loafRepoTierStrong:        "strong",
		loafRepoTierAuthoritative: "authoritative",
	} {
		if got := tier.String(); got != want {
			t.Errorf("loafRepoTier(%d).String() = %q, want %q", int(tier), got, want)
		}
	}
}

func assertDetectionBases(t *testing.T, bases []string, want []string) {
	t.Helper()
	if len(bases) != len(want) {
		t.Fatalf("detectLoafRepo() bases = %v, want %d matching %v", bases, len(want), want)
	}
	for i, fragment := range want {
		if !strings.Contains(bases[i], fragment) {
			t.Errorf("detectLoafRepo() bases[%d] = %q, want it to contain %q", i, bases[i], fragment)
		}
	}
}

func mustResolveDetectionRoot(t *testing.T, dir string) project.Root {
	t.Helper()
	root, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatalf("project.ResolveRoot() error = %v", err)
	}
	return root
}

func mustRegisterDetectionProject(t *testing.T, dir string) {
	t.Helper()
	if _, err := state.Initialize(context.Background(), mustResolveDetectionRoot(t, dir), state.PathResolver{}); err != nil {
		t.Fatalf("state.Initialize() error = %v", err)
	}
}

func mustWriteFencedAgentsFile(t *testing.T, dir string) {
	t.Helper()
	body := "# Project\n\nHouse rules.\n\n" + generateFencedContent() + "\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}
}

func mustWriteLoafProjectConfig(t *testing.T, dir string) {
	t.Helper()
	mustMakeDetectionDirs(t, dir, ".agents")
	if err := os.WriteFile(filepath.Join(dir, ".agents", "loaf.json"), []byte("{\"integrations\":{}}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(loaf.json) error = %v", err)
	}
}

func mustMakeDetectionDirs(t *testing.T, dir string, relativePaths ...string) {
	t.Helper()
	for _, relativePath := range relativePaths {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(relativePath)), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", relativePath, err)
		}
	}
}

func snapshotDetectionTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			snapshot[relativePath] = "dir"
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		snapshot[relativePath] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
	return snapshot
}

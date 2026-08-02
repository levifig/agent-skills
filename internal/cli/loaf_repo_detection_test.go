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

// TestDetectLoafRepoRequiresACompleteFencedSection pins the fragment rule: the
// start marker alone is not a managed section, so it cannot promote a directory
// to the tier that lets upgrade write without asking.
func TestDetectLoafRepoRequiresACompleteFencedSection(t *testing.T) {
	fragment := "# Project\n\n" + fencedStartMarker + " sha256=" + strings.Repeat("a", 64) + " -->\n<!-- Maintained by loaf -->\n## Loaf Framework\n"

	t.Run("fragment alone is no signal", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
		writeDetectionFile(t, filepath.Join(dir, "AGENTS.md"), fragment)

		detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), "")

		if detection.Tier != loafRepoTierNone {
			t.Fatalf("detectLoafRepo() tier = %s, want none for a start-marker fragment (bases %v)", detection.Tier, detection.Bases)
		}
	})

	t.Run("fragment never lifts legacy to strong", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
		mustMakeDetectionDirs(t, dir, ".agents/specs")
		writeDetectionFile(t, filepath.Join(dir, "AGENTS.md"), fragment)

		detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), "")

		if detection.Tier != loafRepoTierLegacy {
			t.Fatalf("detectLoafRepo() tier = %s, want legacy (bases %v)", detection.Tier, detection.Bases)
		}
		assertDetectionBases(t, detection.Bases, []string{"legacy Loaf artifact folders: .agents/specs"})
	})

	t.Run("complete section is strong", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
		mustWriteFencedAgentsFile(t, dir)

		if detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), ""); detection.Tier != loafRepoTierStrong {
			t.Fatalf("detectLoafRepo() tier = %s, want strong for a complete section", detection.Tier)
		}
	})
}

// TestDetectLoafRepoTreatsAMalformedFenceHeaderAsLegacy covers the state between
// the other two: both fences are present, so something wrote a managed section
// here, but the header does not parse, so it is not a section Loaf can vouch
// for. Answering "strong" would let upgrade write project files on the strength
// of a marker that has been tampered with or truncated; answering "none" would
// throw away real evidence. Legacy is the tier that asks.
func TestDetectLoafRepoTreatsAMalformedFenceHeaderAsLegacy(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		header string
	}{
		{name: "garbage in the header", header: "not-a-version garbage"},
		{name: "truncated fingerprint", header: "sha256=" + strings.Repeat("a", 12)},
		{name: "no header fields at all", header: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
			_, body, _ := strings.Cut(generateFencedContent(), "\n")
			writeDetectionFile(t, filepath.Join(dir, "AGENTS.md"), "# Project\n\n"+fencedStartMarker+" "+testCase.header+" -->\n"+body+"\n")

			detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), "")

			if detection.Tier != loafRepoTierLegacy {
				t.Fatalf("detectLoafRepo() tier = %s, want legacy for a paired fence with a malformed header (bases %v)", detection.Tier, detection.Bases)
			}
			assertDetectionBases(t, detection.Bases, []string{"tampered or malformed managed section in AGENTS.md"})
		})
	}

	// A stronger signal elsewhere still leads the evidence, so the line callers
	// print is the one that justifies the tier they acted on.
	t.Run("a stronger signal still leads the bases", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
		_, body, _ := strings.Cut(generateFencedContent(), "\n")
		writeDetectionFile(t, filepath.Join(dir, "AGENTS.md"), fencedStartMarker+" garbage -->\n"+body+"\n")
		mustWriteLoafProjectConfig(t, dir)

		detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), "")

		if detection.Tier != loafRepoTierStrong {
			t.Fatalf("detectLoafRepo() tier = %s, want strong from the project config", detection.Tier)
		}
		assertDetectionBases(t, detection.Bases, []string{
			"Loaf project config at .agents/loaf.json",
			"tampered or malformed managed section in AGENTS.md",
		})
	})
}

// TestDetectLoafRepoBoundsTheMarkerProbe proves the read is capped rather than
// trusted: a marker past the limit is simply not seen, and the probe never
// reads the whole file to find that out.
func TestDetectLoafRepoBoundsTheMarkerProbe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	padding := strings.Repeat("filler line to push the section past the read limit\n", (detectionReadLimit/52)+64)
	writeDetectionFile(t, filepath.Join(dir, "AGENTS.md"), padding+generateFencedContent()+"\n")

	detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), "")

	if detection.Tier != loafRepoTierNone {
		t.Fatalf("detectLoafRepo() tier = %s, want none when the section sits beyond the read limit", detection.Tier)
	}
}

// TestDetectLoafRepoIgnoresIrregularProbeTargets covers the file types a probe
// path can hold that are not files to read. A directory at either path and a
// symlink into nothing contribute nothing; the FIFO case, which is the one that
// would hang rather than mislead, has its own timeout-guarded test.
func TestDetectLoafRepoIgnoresIrregularProbeTargets(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{
			name: "directories at both probe paths",
			setup: func(t *testing.T, dir string) {
				mustMakeDetectionDirs(t, dir, "AGENTS.md", ".agents/loaf.json")
			},
		},
		{
			name: "symlinks pointing at nothing",
			setup: func(t *testing.T, dir string) {
				mustMakeDetectionDirs(t, dir, ".agents")
				mustSymlinkForDetection(t, filepath.Join(dir, "missing-instructions.md"), filepath.Join(dir, "AGENTS.md"))
				mustSymlinkForDetection(t, filepath.Join(dir, "missing-config.json"), filepath.Join(dir, ".agents", "loaf.json"))
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
			testCase.setup(t, dir)

			if detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), ""); detection.Tier != loafRepoTierNone {
				t.Fatalf("detectLoafRepo() tier = %s, want none (bases %v)", detection.Tier, detection.Bases)
			}
		})
	}
}

// TestDetectLoafRepoFollowsSymlinksToRegularFiles keeps the tightening honest:
// pointing AGENTS.md at another real file is a normal repo layout, and it still
// answers.
func TestDetectLoafRepoFollowsSymlinksToRegularFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	mustMakeDetectionDirs(t, dir, "docs")
	writeDetectionFile(t, filepath.Join(dir, "docs", "instructions.md"), generateFencedContent()+"\n")
	mustSymlinkForDetection(t, filepath.Join("docs", "instructions.md"), filepath.Join(dir, "AGENTS.md"))

	detection := detectLoafRepo(mustResolveDetectionRoot(t, dir), "")

	if detection.Tier != loafRepoTierStrong {
		t.Fatalf("detectLoafRepo() tier = %s, want strong through the symlink", detection.Tier)
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

func writeDetectionFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func mustSymlinkForDetection(t *testing.T, target string, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink(%s -> %s) error = %v", link, target, err)
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

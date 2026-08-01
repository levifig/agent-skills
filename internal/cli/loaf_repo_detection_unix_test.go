//go:build unix

package cli

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestDetectLoafRepoNeverOpensAFifoProbePath is the reason the probes settle a
// path's type before reading it. Opening a FIFO for reading blocks until a
// writer arrives, so a named pipe left at AGENTS.md would hang `loaf install`
// and `loaf upgrade` indefinitely rather than fail — the assertion that matters
// here is the deadline, not the tier.
func TestDetectLoafRepoNeverOpensAFifoProbePath(t *testing.T) {
	for _, probe := range []string{"AGENTS.md", filepath.Join(".agents", "loaf.json")} {
		t.Run(probe, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
			mustMakeDetectionDirs(t, dir, ".agents")
			if err := syscall.Mkfifo(filepath.Join(dir, probe), 0o600); err != nil {
				t.Skipf("Mkfifo(%s) unavailable here: %v", probe, err)
			}
			root := mustResolveDetectionRoot(t, dir)

			done := make(chan loafRepoDetection, 1)
			go func() { done <- detectLoafRepo(root, "") }()

			select {
			case detection := <-done:
				if detection.Tier != loafRepoTierNone {
					t.Fatalf("detectLoafRepo() tier = %s, want none for a FIFO at %s (bases %v)", detection.Tier, probe, detection.Bases)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("detectLoafRepo() blocked on the FIFO at %s; the probe must settle the file type before opening it", probe)
			}
		})
	}
}

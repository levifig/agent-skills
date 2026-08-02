//go:build unix

package cli

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestOpenDetectionFileSettlesTheTypeOnTheDescriptor is the fix for the window
// a path-based type check leaves open: between deciding a name is a regular
// file and opening it, the name can be replaced. The probe now opens first,
// without blocking, and asks the descriptor — the file actually being read —
// what it is.
func TestOpenDetectionFileSettlesTheTypeOnTheDescriptor(t *testing.T) {
	dir := t.TempDir()

	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("Mkfifo unavailable here: %v", err)
	}
	done := make(chan bool, 1)
	go func() {
		file, ok := openDetectionFile(fifo)
		if ok {
			file.Close()
		}
		done <- ok
	}()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("openDetectionFile() accepted a FIFO; only a descriptor that stats as a regular file may be read")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("openDetectionFile() blocked on a FIFO; the open must be non-blocking")
	}

	regular := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(regular, []byte("# Project\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, ok := openDetectionFile(regular)
	if !ok {
		t.Fatal("openDetectionFile() rejected a regular file")
	}
	body, err := io.ReadAll(file)
	file.Close()
	if err != nil || string(body) != "# Project\n" {
		t.Fatalf("read through the probe descriptor = %q, %v, want the file contents; O_NONBLOCK must not change a regular-file read", body, err)
	}
}

// TestDetectLoafRepoNeverOpensAFifoProbePath is the reason the probes never
// read a path whose descriptor is not a regular file. Opening a FIFO for
// reading blocks until a writer arrives, so a named pipe left at AGENTS.md
// would hang `loaf install` and `loaf upgrade` indefinitely rather than fail —
// the assertion that matters here is the deadline, not the tier.
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

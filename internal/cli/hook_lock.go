package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// hook_lock.go serializes the writers of one target's hooks file. Every writer
// — a reconcile during install or upgrade, an enable or disable — holds the
// lock from the moment it reads state until the moment it publishes the file,
// which is what makes the read-compute-record-project sequence atomic against
// another Loaf process rather than merely quick.
//
// The lock is a file beside the hooks file, taken by exclusive create. It is
// advisory and Loaf-only: a third-party writer that knows nothing about it is
// caught by the pre-rename comparison instead, and the write it can still lose
// in the window after that comparison is a named accepted residual, not
// something this lock pretends to cover.

const hookFileLockName = ".loaf-hooks.lock"

const (
	hookFileLockWait = 2 * time.Second
	hookFileLockPoll = 20 * time.Millisecond
)

type hookFileLock struct {
	path string
}

// acquireHookFileLock takes the lock for the hooks file at hooksPath, waiting
// briefly for a concurrent writer to finish. Contention past the wait is an
// actionable error rather than a longer wait: the other writer is either about
// to finish, in which case rerunning succeeds, or it is gone and left the lock
// behind, in which case only a person can say so.
func acquireHookFileLock(hooksPath string, wait time.Duration) (*hookFileLock, error) {
	if wait <= 0 {
		wait = hookFileLockWait
	}
	path := filepath.Join(filepath.Dir(hooksPath), hookFileLockName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, writeErr := fmt.Fprintf(file, "pid %d since %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				lock := &hookFileLock{path: path}
				return nil, fmt.Errorf("record hooks lock %s: %w", path, firstHookLockError(writeErr, closeErr, lock.release()))
			}
			return &hookFileLock{path: path}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("take hooks lock %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another Loaf process is reconciling %s (%s, %s); rerun in a moment, or delete that lock file if no Loaf process is running", hooksPath, path, describeHookFileLockHolder(path))
		}
		time.Sleep(hookFileLockPoll)
	}
}

func (l *hookFileLock) release() error {
	if l == nil {
		return nil
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("release hooks lock %s: %w", l.path, err)
	}
	return nil
}

func describeHookFileLockHolder(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return "holder unknown"
	}
	holder := strings.TrimSpace(string(body))
	if holder == "" {
		return "holder unknown"
	}
	return "held by " + holder
}

func firstHookLockError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

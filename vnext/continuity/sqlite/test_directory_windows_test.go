//go:build windows

package sqlite

import (
	"os"
	"testing"
)

func testTempDir(t *testing.T) string {
	t.Helper()

	root, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("resolve LocalAppData test root: %v", err)
	}
	directory, err := os.MkdirTemp(root, "loaf-continuity-test-")
	if err != nil {
		t.Fatalf("create LocalAppData test directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove LocalAppData test directory: %v", err)
		}
	})
	return directory
}

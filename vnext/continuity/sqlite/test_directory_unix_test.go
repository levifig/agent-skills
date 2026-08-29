//go:build !windows

package sqlite

import "testing"

func testTempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

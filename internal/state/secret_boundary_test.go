package state

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeSQLiteRuntimeDoesNotIntroduceSecretStorageTerms(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	stateDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(stateDir, "..", ".."))
	scanRoots := []string{
		filepath.Join(repoRoot, "internal", "state"),
		filepath.Join(repoRoot, "internal", "cli"),
		filepath.Join(repoRoot, "cmd", "loaf"),
	}
	forbidden := []string{
		"access_token",
		"refresh_token",
		"api_key",
		"api key",
		"password",
		"credential",
		"secret",
	}

	for _, root := range scanRoots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			lower := strings.ToLower(string(content))
			for _, term := range forbidden {
				if strings.Contains(lower, term) {
					rel, relErr := filepath.Rel(repoRoot, path)
					if relErr != nil {
						rel = path
					}
					t.Fatalf("%s contains forbidden secret-storage term %q", filepath.ToSlash(rel), term)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", root, err)
		}
	}
}

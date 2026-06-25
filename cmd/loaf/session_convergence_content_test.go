package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestSessionModelConvergenceContentGuards(t *testing.T) {
	root := repoRoot(t)

	searchRoots := []string{
		"content",
		"dist",
		filepath.ToSlash(filepath.Join("plugins", "loaf")),
		".agents/AGENTS.md",
		".claude/CLAUDE.md",
	}
	forbidden := []string{
		"loaf session housekeeping",
		".agents/transcripts/",
		"Transcript Archival",
		"MANDATORY: Create session file",
		"DO NOT PROCEED WITHOUT A SESSION FILE",
		"transcripts:",
	}

	var failures []string
	for _, rel := range searchRoots {
		path := filepath.Join(root, filepath.FromSlash(rel))
		failures = append(failures, forbiddenContentMatches(t, path, forbidden)...)
	}
	sort.Strings(failures)
	if len(failures) > 0 {
		t.Fatalf("session model convergence forbidden content remains:\n%s", strings.Join(failures, "\n"))
	}
}

func TestSessionModelConvergenceAnointsWrapInTopLevelGuidance(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{".agents/AGENTS.md", ".claude/CLAUDE.md"} {
		body := readTextFile(t, filepath.Join(root, filepath.FromSlash(rel)))
		for _, want := range []string{"canonical session model", "wrap", "loaf session start", "loaf session end --wrap"} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q in canonical session guidance", rel, want)
			}
		}
		if strings.Contains(body, "Create session file following") {
			t.Fatalf("%s still advertises hand-authored session file scaffolding", rel)
		}
	}
}

func forbiddenContentMatches(t *testing.T, path string, forbidden []string) []string {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if !info.IsDir() {
		return forbiddenMatchesInFile(t, path, forbidden)
	}
	var matches []string
	err = filepath.WalkDir(path, func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !isSessionConvergenceTextFile(file) {
			return nil
		}
		matches = append(matches, forbiddenMatchesInFile(t, file, forbidden)...)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s) error = %v", path, err)
	}
	return matches
}

func forbiddenMatchesInFile(t *testing.T, path string, forbidden []string) []string {
	t.Helper()
	body := readTextFile(t, path)
	lines := strings.Split(body, "\n")
	var matches []string
	for lineNumber, line := range lines {
		for _, phrase := range forbidden {
			if strings.Contains(line, phrase) {
				matches = append(matches, filepath.ToSlash(path)+":"+strconv.Itoa(lineNumber+1)+": "+phrase)
			}
		}
	}
	return matches
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(body)
}

func isSessionConvergenceTextFile(path string) bool {
	switch filepath.Ext(path) {
	case ".md", ".yaml", ".yml", ".sh", ".py", ".json":
		return true
	default:
		return false
	}
}

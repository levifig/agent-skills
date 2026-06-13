package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestNativeCutoverCommandSurfaceAuditCoversPublicCommands(t *testing.T) {
	root := repoRoot(t)
	reportPath := filepath.Join(root, "docs", "reports", "2026-06-10-native-go-cutover-test-map.md")
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", reportPath, err)
	}
	report := string(reportBytes)

	assertTypeScriptRuntimeRegistryRemoved(t, root)
	goCommands := nativeGoCommands(t, root)
	for _, command := range sortedKeys(goCommands) {
		wantStatus := "Native Go"
		row := regexp.MustCompile(`(?m)^\| ` + regexp.QuoteMeta("`"+command+"`") + ` \| ` + regexp.QuoteMeta(wantStatus) + ` \|`)
		if !row.MatchString(report) {
			t.Fatalf("audit report missing row for %q with status %q", command, wantStatus)
		}
	}
}

func assertTypeScriptRuntimeRegistryRemoved(t *testing.T, root string) {
	t.Helper()
	indexPath := filepath.Join(root, "cli", "index.ts")
	if _, err := os.Stat(indexPath); err == nil {
		t.Fatalf("%s still exists; TypeScript runtime registry should be removed after native cutover", indexPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v", indexPath, err)
	}
}

func nativeGoCommands(t *testing.T, root string) map[string]bool {
	t.Helper()
	cliPath := filepath.Join(root, "internal", "cli", "cli.go")
	body, err := os.ReadFile(cliPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", cliPath, err)
	}
	source := string(body)
	start := strings.Index(source, "func (r Runner) Run(args []string) error {")
	end := strings.Index(source, "func (r Runner) runHousekeeping")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("could not isolate Runner.Run dispatcher in %s", cliPath)
	}
	dispatcher := source[start:end]
	matches := regexp.MustCompile(`case "([^"]+)":\s*\n\s*return r\.run`).FindAllStringSubmatch(dispatcher, -1)
	if len(matches) == 0 {
		t.Fatalf("no Go-native command dispatch cases found in %s", cliPath)
	}
	commands := make(map[string]bool, len(matches))
	for _, match := range matches {
		if strings.HasPrefix(match[1], "-") {
			continue
		}
		commands[match[1]] = true
	}
	return commands
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

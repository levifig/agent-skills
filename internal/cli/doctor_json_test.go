package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func doctorFixtureProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir .agents: %v", err)
	}
	// A canonical AGENTS.md plus a stale legacy artifact so the fixture
	// produces a mix of pass and non-pass outcomes.
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Agents\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursorrules"), []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	return root
}

func hashDirectoryTree(t *testing.T, root string) string {
	t.Helper()
	digest := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintln(digest, relative)
		if entry.Type().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest.Write(content)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("hash fixture tree: %v", err)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func TestDoctorJSONMatchesHumanChecksAndNeverMutates(t *testing.T) {
	root := doctorFixtureProject(t)
	ctx := doctorContext{projectRoot: root}
	cliVersion := "2.0.0-test"

	before := hashDirectoryTree(t, root)
	jsonResult := runDoctorChecksJSON(ctx, cliVersion)
	after := hashDirectoryTree(t, root)
	if before != after {
		t.Fatal("JSON doctor mutated the project tree")
	}

	checks := doctorChecks(cliVersion)
	if len(jsonResult.Checks) != len(checks) {
		t.Fatalf("JSON checks = %d, want %d", len(jsonResult.Checks), len(checks))
	}
	tally := map[string]int{}
	for index, check := range checks {
		human := safeRunDoctorCheck(check, ctx)
		got := jsonResult.Checks[index]
		if got.Name != check.Name || got.Status != string(human.Status) || got.Message != human.Message || got.Fixable != human.Fixable {
			t.Fatalf("check %q JSON = %#v, want status %q message %q fixable %t", check.Name, got, human.Status, human.Message, human.Fixable)
		}
		tally[string(human.Status)]++
	}
	if jsonResult.Passes != tally["pass"] || jsonResult.Warnings != tally["warn"] || jsonResult.Failures != tally["fail"] || jsonResult.Skips != tally["skip"] {
		t.Fatalf("JSON tallies = %d/%d/%d/%d, want %v", jsonResult.Passes, jsonResult.Warnings, jsonResult.Failures, jsonResult.Skips, tally)
	}
	if jsonResult.Passed != (jsonResult.Failures == 0) {
		t.Fatalf("Passed = %t with %d failures", jsonResult.Passed, jsonResult.Failures)
	}
	if jsonResult.ContractVersion != 1 || jsonResult.Command != "doctor" {
		t.Fatalf("JSON envelope = %#v, want contract_version 1 command doctor", jsonResult)
	}

	// The human path over the same fixture reports the same outcome counts.
	var humanOut bytes.Buffer
	report := runDoctorChecks(&humanOut, ctx, doctorOptions{}, cliVersion, strings.NewReader(""))
	if report.Passes != jsonResult.Passes || report.Warnings != jsonResult.Warnings || report.Failures != jsonResult.Failures || report.Skips != jsonResult.Skips {
		t.Fatalf("human report = %+v, JSON = %d/%d/%d/%d; outcomes must match", report, jsonResult.Passes, jsonResult.Warnings, jsonResult.Failures, jsonResult.Skips)
	}
}

func TestDoctorJSONRejectsFixCombination(t *testing.T) {
	if _, err := parseLoafDoctorArgs([]string{"--json", "--fix"}); err == nil {
		t.Fatal("--json --fix accepted; JSON diagnosis must never reach repair code")
	}
	options, err := parseLoafDoctorArgs([]string{"--json"})
	if err != nil {
		t.Fatalf("parse --json error = %v", err)
	}
	if !options.jsonOutput {
		t.Fatal("--json flag not recorded")
	}
}

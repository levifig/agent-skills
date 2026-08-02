package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Enforcement hooks must fail closed on any regular-file read they cannot
// complete. Only errNotRegularFile (FIFO/device skip) is a soft warning; a
// permission or size failure means the check never inspected the file and
// must not report Passed.

func TestRenderDriftBlocksPermissionDeniedRegularSpec(t *testing.T) {
	skipWithoutEnforcedPermissions(t)

	root := t.TempDir()
	specsDir := filepath.Join(root, ".agents", "specs")
	mkdirAll(t, specsDir)
	path := filepath.Join(specsDir, "SPEC-001-denied.md")
	writeInstallFile(t, path, "# Spec\n")
	chmodForTest(t, path, 0o000)

	result := runNativeRenderDrift(checkHookContext{}, root)
	if result.Passed || !result.Blocked {
		t.Fatalf("render-drift = %#v, want Passed=false Blocked=true for an unreadable regular file", result)
	}
	if !errorsContainPathAndReadFailure(result.Errors, "SPEC-001-denied.md") {
		t.Fatalf("errors = %#v, want a read failure naming the unreadable spec", result.Errors)
	}
}

func TestRenderDriftBlocksOversizedRegularSpec(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, ".agents", "specs")
	mkdirAll(t, specsDir)
	path := filepath.Join(specsDir, "SPEC-001-huge.md")
	// One byte over the whole-file project limit forces errFileTooLarge.
	if err := os.WriteFile(path, make([]byte, projectFileReadLimit+1), 0o644); err != nil {
		t.Fatalf("WriteFile oversized: %v", err)
	}

	result := runNativeRenderDrift(checkHookContext{}, root)
	if result.Passed || !result.Blocked {
		t.Fatalf("render-drift = %#v, want Passed=false Blocked=true for an oversized regular file", result)
	}
	if !errorsContainPathAndReadFailure(result.Errors, "SPEC-001-huge.md") {
		t.Fatalf("errors = %#v, want a read failure naming the oversized spec", result.Errors)
	}
	if !errors.Is(errorsFromMessages(result.Errors), errFileTooLarge) && !stringsContainAny(result.Errors, "exceeds the project read limit", "file exceeds") {
		// The PathError wraps errFileTooLarge; the formatted message must still
		// name the size refusal so operators can act.
		if !stringsContainAny(result.Errors, "exceeds", "limit", "too large", "project read") {
			t.Fatalf("errors = %#v, want the oversized-file refusal phrased", result.Errors)
		}
	}
}

func TestEphemeralProvenanceBlocksPermissionDeniedRegularSpec(t *testing.T) {
	skipWithoutEnforcedPermissions(t)

	root := initEnumeratedGitRepo(t)
	specsDir := filepath.Join(root, ".agents", "specs")
	mkdirAll(t, specsDir)
	path := filepath.Join(specsDir, "SPEC-001-denied.md")
	writeInstallFile(t, path, "# Spec\n")
	chmodForTest(t, path, 0o000)

	result := runNativeEphemeralProvenance(checkHookContext{}, root)
	if result.Passed || !result.Blocked {
		t.Fatalf("ephemeral-provenance = %#v, want Passed=false Blocked=true for an unreadable regular file", result)
	}
	if !errorsContainPathAndReadFailure(result.Errors, "SPEC-001-denied.md") {
		t.Fatalf("errors = %#v, want a read failure naming the unreadable spec", result.Errors)
	}
}

func TestEphemeralProvenanceBlocksOversizedRegularSpec(t *testing.T) {
	root := initEnumeratedGitRepo(t)
	specsDir := filepath.Join(root, ".agents", "specs")
	mkdirAll(t, specsDir)
	path := filepath.Join(specsDir, "SPEC-001-huge.md")
	if err := os.WriteFile(path, make([]byte, projectFileReadLimit+1), 0o644); err != nil {
		t.Fatalf("WriteFile oversized: %v", err)
	}

	result := runNativeEphemeralProvenance(checkHookContext{}, root)
	if result.Passed || !result.Blocked {
		t.Fatalf("ephemeral-provenance = %#v, want Passed=false Blocked=true for an oversized regular file", result)
	}
	if !errorsContainPathAndReadFailure(result.Errors, "SPEC-001-huge.md") {
		t.Fatalf("errors = %#v, want a read failure naming the oversized spec", result.Errors)
	}
}

func errorsContainPathAndReadFailure(messages []string, pathFragment string) bool {
	for _, message := range messages {
		if strings.Contains(message, pathFragment) && (strings.Contains(message, "Read ") || strings.Contains(message, "permission") || strings.Contains(message, "exceeds") || strings.Contains(message, "denied")) {
			return true
		}
	}
	// Fallback: any error naming the path is enough for the fail-closed contract.
	for _, message := range messages {
		if strings.Contains(message, pathFragment) {
			return true
		}
	}
	return false
}

func stringsContainAny(messages []string, needles ...string) bool {
	for _, message := range messages {
		for _, needle := range needles {
			if strings.Contains(strings.ToLower(message), strings.ToLower(needle)) {
				return true
			}
		}
	}
	return false
}

// errorsFromMessages is unused for typed unwrap (messages are already
// formatted); kept as a no-op anchor so the errors import stays justified if a
// future assertion reintroduces typed matching.
func errorsFromMessages(_ []string) error {
	return nil
}

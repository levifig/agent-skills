package vnextboundary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependencyBoundaryScansInactiveTestSource(t *testing.T) {
	t.Parallel()

	root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
		"vnext/hidden_windows_test.go": `//go:build windows && arm64

package hidden

import _ "alternate.example/loaf/internal/state"
`,
	})

	assertBoundaryViolation(t, inspectFixtureBoundary(t, root), legacyImportRule)
}

func TestDependencyBoundaryDerivesAlternateModuleIdentity(t *testing.T) {
	t.Parallel()

	root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
		"vnext/cmd/loaf/main.go": `package main

import _ "alternate.example/loaf/vnext/internal/kernel"
`,
		"vnext/internal/kernel/kernel.go": "package kernel\n",
	})
	assertNoBoundaryViolations(t, inspectFixtureBoundary(t, root))

	hardCodedRoot := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
		"vnext/hidden.go": `package hidden

import _ "github.com/levifig/loaf/vnext/internal/kernel"
`,
	})
	assertBoundaryViolation(t, inspectFixtureBoundary(t, hardCodedRoot), thirdPartyImportRule)
}

func TestDependencyBoundaryRequiresAnExactModuleDirective(t *testing.T) {
	t.Parallel()

	if path, err := parseModulePath([]byte("modulex.example/loaf\n")); err == nil {
		t.Fatalf("parseModulePath() = %q, want missing-directive error", path)
	}
}

func TestDependencyBoundaryRejectsNestedModules(t *testing.T) {
	t.Parallel()

	root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
		"vnext/kernel.go":        "package vnext\n",
		"vnext/nested/go.mod":    "module escape.example/module\n\ngo 1.25\n",
		"vnext/nested/hidden.go": "package hidden\n",
	})

	assertBoundaryViolation(t, inspectFixtureBoundary(t, root), nestedModuleRule)
}

func TestDependencyBoundaryRejectsThirdPartyImports(t *testing.T) {
	t.Parallel()

	root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
		"vnext/dependency.go": `package vnext

import _ "dependency.example/client"
`,
	})

	assertBoundaryViolation(t, inspectFixtureBoundary(t, root), thirdPartyImportRule)
}

func TestDependencyBoundaryRejectsDirectBootstrapCapabilities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		rule string
	}{
		{name: "database sql", body: "package sample\n\nimport _ \"database/sql\"\n", rule: forbiddenImportRule},
		{name: "process execution", body: "package sample\n\nimport _ \"os/exec\"\n", rule: forbiddenImportRule},
		{name: "raw syscall", body: "package sample\n\nimport _ \"syscall\"\n", rule: forbiddenImportRule},
		{name: "runtime plugin", body: "package sample\n\nimport _ \"plugin\"\n", rule: forbiddenImportRule},
		{name: "cgo", body: "package sample\n\nimport \"C\"\n", rule: forbiddenImportRule},
		{
			name: "linkname",
			body: `package sample

import _ "unsafe"

//go:linkname hidden runtime.hidden
func hidden()
`,
			rule: forbiddenDirectiveRule,
		},
		{
			name: "start process alias",
			body: `package sample

import operatingSystem "os"

var start = operatingSystem.StartProcess
`,
			rule: forbiddenCapabilityRule,
		},
		{
			name: "dot imported os",
			body: `package sample

import . "os"

var start = StartProcess
`,
			rule: forbiddenCapabilityRule,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				"vnext/capability.go": test.body,
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), test.rule)
		})
	}
}

func inspectFixtureBoundary(t *testing.T, root string) []boundaryViolation {
	t.Helper()

	metadata, err := loadModuleMetadata(root)
	if err != nil {
		t.Fatalf("load fixture module metadata: %v", err)
	}
	violations, err := inspectVNextBoundary(metadata)
	if err != nil {
		t.Fatalf("inspect fixture boundary: %v", err)
	}
	return violations
}

func writeBoundaryFixture(t *testing.T, modulePath string, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	writeBoundaryFixtureFile(t, root, "go.mod", "module "+modulePath+"\n\ngo 1.25\n")
	for relativePath, contents := range files {
		writeBoundaryFixtureFile(t, root, relativePath, contents)
	}
	return root
}

func writeBoundaryFixtureFile(t *testing.T, root, relativePath, contents string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
}

func assertBoundaryViolation(t *testing.T, violations []boundaryViolation, rule string) {
	t.Helper()

	for _, violation := range violations {
		if violation.Rule == rule {
			return
		}
	}
	t.Fatalf("missing %q violation in:\n%s", rule, formatBoundaryViolations(violations))
}

func assertNoBoundaryViolations(t *testing.T, violations []boundaryViolation) {
	t.Helper()

	if len(violations) > 0 {
		t.Fatalf("unexpected boundary violations:\n%s", formatBoundaryViolations(violations))
	}
}

func formatBoundaryViolations(violations []boundaryViolation) string {
	if len(violations) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(violations))
	for _, violation := range violations {
		lines = append(lines, violation.String())
	}
	return strings.Join(lines, "\n")
}

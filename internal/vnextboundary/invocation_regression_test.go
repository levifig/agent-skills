package vnextboundary

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDependencyBoundaryNestedModuleCannotDisableCanonicalGate(t *testing.T) {
	policySource, err := os.ReadFile("policy.go")
	if err != nil {
		t.Fatalf("read authoritative policy source: %v", err)
	}

	root := t.TempDir()
	writeBoundaryFixtureFile(t, root, "go.mod", "module invocation.example/loaf\n\ngo 1.25\n")
	writeBoundaryFixtureFile(t, root, "internal/vnextboundary/policy.go", string(policySource))
	writeBoundaryFixtureFile(t, root, "internal/vnextboundary/gate_test.go", invocationFixtureGateTest)
	writeBoundaryFixtureFile(t, root, "vnext/go.mod", "module invocation.example/loaf/vnext\n\ngo 1.25\n")
	writeBoundaryFixtureFile(t, root, "vnext/kernel.go", "package vnext\n")

	command := exec.Command(
		"go", "test", "./internal/vnextboundary", "./vnext/...",
		"-count=1", "-run", "Kernel|DependencyBoundary",
	)
	command.Dir = root
	command.Env = offlineGoEnvironment(os.Environ())
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("canonical gate succeeded with nested vnext/go.mod:\n%s", output)
	}
	if !strings.Contains(string(output), nestedModuleRule) {
		t.Fatalf("canonical gate did not report the external nested-module policy:\n%s", output)
	}
}

func offlineGoEnvironment(environment []string) []string {
	overrides := []string{"GOFLAGS=", "GONOSUMDB=*", "GOPROXY=off", "GOSUMDB=off", "GOTELEMETRY=off", "GOTOOLCHAIN=local", "GOWORK=off"}
	filtered := make([]string, 0, len(environment)+len(overrides))
	for _, entry := range environment {
		keep := true
		for _, override := range overrides {
			if strings.HasPrefix(entry, override[:strings.IndexByte(override, '=')+1]) {
				keep = false
				break
			}
		}
		if keep {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, overrides...)
}

const invocationFixtureGateTest = `package vnextboundary

import (
	"os"
	"testing"
)

func TestDependencyBoundary(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	metadata, err := discoverModuleMetadata(workingDirectory)
	if err != nil {
		t.Fatalf("discover module metadata: %v", err)
	}
	violations, err := inspectVNextBoundary(metadata)
	if err != nil {
		t.Fatalf("inspect vNext boundary: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("unexpected boundary violations: %v", violations)
	}
}
`

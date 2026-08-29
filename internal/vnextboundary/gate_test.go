package vnextboundary

import (
	"os"
	"testing"
)

func TestDependencyBoundary(t *testing.T) {
	t.Parallel()

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
	assertNoBoundaryViolations(t, violations)
}

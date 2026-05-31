package state

import (
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestRuntimeExposesNameAndRootPath(t *testing.T) {
	root, err := project.ResolveWorkingDirectory(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveWorkingDirectory() error = %v", err)
	}

	runtime := NewRuntime(root)

	if runtime.Name() == "" {
		t.Fatal("Name() returned empty string")
	}
	if runtime.RootPath() != root.Path() {
		t.Fatalf("RootPath() = %q, want %q", runtime.RootPath(), root.Path())
	}
}

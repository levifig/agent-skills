package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestReadProjectConfRequiresProjectID(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	agents := filepath.Join(rootDir, ".agents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	confPath := filepath.Join(agents, "loaf.conf")
	if err := os.WriteFile(confPath, []byte(`{"conf_id":"conf_test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := project.ResolveRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = project.ReadProjectConf(root)
	if err == nil {
		t.Fatal("ReadProjectConf() error = nil, want missing project_id")
	}
}

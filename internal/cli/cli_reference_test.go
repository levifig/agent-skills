package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerGenerateCLIReferenceWritesSkillNatively(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"__generate-cli-ref"})
	if err != nil {
		t.Fatalf("__generate-cli-ref error = %v", err)
	}

	outputPath := filepath.Join(workingDir, "content", "skills", "cli-reference", "SKILL.md")
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", outputPath, err)
	}
	content := string(data)
	for _, want := range []string{
		"**Note:** This file is auto-generated from native CLI reference metadata.",
		"## Task Management",
		"`loaf task create`",
		"## Kb Management",
		"`loaf kb check`",
		"## Command Substitution Reference",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated CLI reference missing %q\n%s", want, content)
		}
	}
	if !strings.Contains(stdout.String(), outputPath) {
		t.Fatalf("stdout = %q, want generated path %q", stdout.String(), outputPath)
	}
}

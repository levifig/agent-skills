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
		"`-t, --target <name>` - Build a specific target only",
		"`loaf state repair legacy-project-database`",
		"`--dry-run` - Preview archive paths without writing",
		"`loaf state repair relationship-origin`",
		"`--origin <imported|manual>` - Provenance value to backfill",
		"`loaf state backup verify`",
		"`--json` - Output backup verification, restore guidance, schema version, and captured project identities as JSON",
		"`loaf state path`",
		"`--verbose` - Output command, scope, project root, and database path",
		"`loaf state backup` | Create a SQLite database backup under the global data-home backups directory",
		"`loaf state export`",
		"`--format <format>` - Output format for the selected export kind",
		"`loaf state export all`",
		"`--format <format>` - Output format: json",
		"`loaf state export release-readiness`",
		"`--format <format>` - Output format: markdown",
		"`loaf migrate worktree-storage`",
		"`--apply` - Perform the migration; dry-run is the default",
		"`--force-from-worktree` - On conflict, keep the worktree-local copy",
		"`--force-from-main` - On conflict, keep the main-worktree copy",
		"## Project Management",
		"`loaf project list`",
		"`--json` - Output database path, project IDs, friendly names, and current paths as JSON",
		"`loaf project show`",
		"`--json` - Output project ID, friendly name, current path, and database path as JSON",
		"`loaf project rename`",
		"`--dry-run` - Validate and preview without writing",
		"`--json` - Output project ID, friendly name, current path, database path, and applied status as JSON",
		"`loaf project move`",
		"## Docs Management",
		"`loaf docs index`",
		"`--rebuild` - Rebuild current worktree docs index before scanning",
		"`--json` - Output indexed docs, counts, global database scope, and project identity as JSON",
		"## Task Management",
		"`loaf task create`",
		"`--json` - Output created task, event, global database scope, and project identity as JSON",
		"`--status <status>` - Only show tasks with status: in_progress, blocked, todo, review, done, archived",
		"`--priority <level>` - Priority level: P0, P1, P2, P3",
		"`--status <status>` - New status: in_progress, blocked, todo, review, done",
		"`--priority <level>` - New priority: P0, P1, P2, P3",
		"`--json` - Output updated task, event, global database scope, and project identity as JSON",
		"`--json` - Output archive result, archived tasks, global database scope, and project identity as JSON",
		"`--json` - Output compatibility summary as JSON",
		"## Trace Management",
		"`loaf trace`",
		"`--json` - Output traced entity, sources, relationships, global database scope, and project identity as JSON",
		"`loaf brainstorm list`",
		"`--json` - Output brainstorms, global database scope, and project identity as JSON",
		"## Idea Management",
		"`loaf idea capture`",
		"`--title <title>` - Idea title",
		"`loaf idea resolve`",
		"`--by <entity>` - Resolving entity",
		"`--json` - Output resolution relationship, event, global database scope, and project identity as JSON",
		"## Spark Management",
		"`loaf spark capture`",
		"`--scope <scope>` - Spark scope",
		"`--text <text>` - Spark text",
		"## Bundle Management",
		"`loaf bundle create`",
		"`--tags <tags>` - Comma-separated tag query",
		"`loaf bundle update`",
		"`loaf bundle show`",
		"`--json` - Output bundle details, members, global database scope, and project identity as JSON",
		"## Link Management",
		"`loaf link create`",
		"`--from <entity>` - Source entity",
		"`--to <entity>` - Target entity",
		"`--type <type>` - Relationship type",
		"`--json` - Output relationship ID, source/target, global database scope, and project identity as JSON",
		"## Kb Management",
		"`loaf kb status`",
		"`--json` - Output knowledge file totals, coverage counts, stale count, review age, and directories as JSON",
		"`loaf kb check`",
		"`--json` - Output per-file staleness, coverage, commit, and review metadata as JSON",
		"`loaf kb init`",
		"`--json` - Output directory actions, config status, and QMD collections as JSON",
		"`loaf housekeeping`",
		"`--json` - Output housekeeping sections, cleanup candidates, signals, and SQLite-backed project identity when available as JSON",
		"`loaf check`",
		"`--json` - Output hook result, pass/block status, exit code, warnings, errors, and findings as JSON",
		"## Command Substitution Reference",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated CLI reference missing %q\n%s", want, content)
		}
	}
	if !strings.Contains(stdout.String(), outputPath) {
		t.Fatalf("stdout = %q, want generated path %q", stdout.String(), outputPath)
	}
	if !strings.Contains(content, "- `loaf report generate`:\n  - `--format <format>` - Output format: markdown") {
		t.Fatalf("generated CLI reference missing report generate markdown format guidance\n%s", content)
	}
	if !strings.Contains(content, "  - `--json` - Output contract, command, project context, and markdown content as JSON") {
		t.Fatalf("generated CLI reference missing report generate JSON guidance\n%s", content)
	}
	if !strings.Contains(content, "- `loaf state migrate markdown`:\n  - `--dry-run` - Preview import counts without creating a database\n  - `--apply` - Initialize SQLite and import Markdown artifacts\n  - `--resume` - Resume the Markdown import after an interrupted attempt\n  - `--backup` - Create SQLite and .agents rollback backups during apply or resume\n  - `--remove-source` - Remove ephemeral Markdown sources after a rollback backup\n  - `--rollback <manifest>` - Restore .agents files from a rollback manifest\n  - `--json` - Output migration contract, scope, project context, counts, and rollback fields as JSON") {
		t.Fatalf("generated CLI reference missing state migrate markdown JSON contract guidance\n%s", content)
	}
	if !strings.Contains(content, "- `loaf state migrate storage-home`:\n  - `--dry-run` - Preview the storage-home migration\n  - `--apply` - Copy the legacy database without deleting it\n  - `--json` - Output migration contract, global database paths, action, and project identity when available") {
		t.Fatalf("generated CLI reference missing state migrate storage-home JSON contract guidance\n%s", content)
	}
	if !strings.Contains(content, "- `loaf migrate markdown`:\n  - `--dry-run` - Preview import counts without creating a database\n  - `--apply` - Initialize SQLite and import Markdown artifacts\n  - `--resume` - Resume the Markdown import after an interrupted attempt\n  - `--backup` - Create SQLite and .agents rollback backups during apply or resume\n  - `--remove-source` - Remove ephemeral Markdown sources after a rollback backup\n  - `--rollback <manifest>` - Restore .agents files from a rollback manifest\n  - `--json` - Output migration contract, scope, project context, counts, and rollback fields as JSON") {
		t.Fatalf("generated CLI reference missing top-level migrate markdown JSON contract guidance\n%s", content)
	}
	if !strings.Contains(content, "- `loaf migrate storage-home`:\n  - `--dry-run` - Preview the storage-home migration\n  - `--apply` - Copy the legacy database without deleting it\n  - `--json` - Output migration contract, global database paths, action, and project identity when available") {
		t.Fatalf("generated CLI reference missing top-level migrate storage-home JSON contract guidance\n%s", content)
	}
	if !strings.Contains(content, "- `loaf report list`:\n  - `--type <type>` - Filter by report type\n  - `--status <status>` - Filter by status; Loaf lifecycle statuses: draft, done, archived") {
		t.Fatalf("generated CLI reference missing report list status guidance\n%s", content)
	}
	if !strings.Contains(content, "- `loaf task list`:\n  - `--json` - Output tasks, diagnostics, global database scope, and project identity as JSON") {
		t.Fatalf("generated CLI reference missing task list JSON contract guidance\n%s", content)
	}
	if !strings.Contains(content, "- `loaf spec list`:\n  - `--json` - Output specs, diagnostics, task counts, global database scope, and project identity as JSON") {
		t.Fatalf("generated CLI reference missing spec list JSON contract guidance\n%s", content)
	}
	if !strings.Contains(content, "- `loaf report list`:\n  - `--type <type>` - Filter by report type\n  - `--status <status>` - Filter by status; Loaf lifecycle statuses: draft, done, archived\n  - `--json` - Output reports, diagnostics, global database scope, and project identity as JSON") {
		t.Fatalf("generated CLI reference missing report list JSON contract guidance\n%s", content)
	}
}

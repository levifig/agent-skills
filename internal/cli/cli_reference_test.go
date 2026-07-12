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

	outputPath := filepath.Join(workingDir, "content", "skills", "loaf-reference", "SKILL.md")
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", outputPath, err)
	}
	content := string(data)
	for _, want := range []string{
		"name: loaf-reference",
		"Documents how agents operate the Loaf CLI",
		"**Note:** This file is auto-generated from native CLI reference metadata.",
		"## Operating Rules",
		"## Journal Context (contract v2)",
		"`loaf --help`",
		"`loaf <command> --help`",
		"`loaf config check --json`, `loaf state doctor --json`, `loaf change check --json`",
		"active-truth read model",
		"`project-synthesis`",
		"`scoped-checkpoint`",
		"`active-lineage`",
		"`unresolved-blockers`",
		"`deferred-intent`",
		"`active-changes`",
		"`branch-recency`",
		"`transitional-tasks`",
		"`wrap(project)`",
		"`source_available`",
		"`available_count`",
		"`shown_count`",
		"`truncated`",
		"`expand_command`",
		"`--limit` accepts 1 through 100 only with `--layer`",
		"`--cursor` also requires `--layer`",
		"Use `--branch` to select `branch-recency` scope and bind state cursors.",
		"active Change provenance or reasons, which always use the actual Git branch",
		"## Command Index",
		"| Command | Purpose | Subcommands |",
		"| `loaf build` | Build skill distributions for agent harnesses | — |",
		"| `loaf config` | Validate and refresh project Loaf config | check |",
		"| `loaf change` | Shape-first Change artifacts: git-canonical work context under docs/changes/ | init, check, list |",
		"| `loaf journal` | Record and read the project-scoped journal (the durable record across all conversations) | log, recent, search, show, context, export, defer |",
		"| `loaf state` | Manage native SQLite state | path, status, init, doctor, repair legacy-project-database,",
		"repair journal-search",
		"migrate schema",
		"| `loaf migrate` | Run native migration workflows | markdown, storage-home, schema, lifecycle-statuses, worktree-storage |",
		"| `loaf doctor` | Diagnose Loaf project alignment (symlinks, stale files, version drift) | — |",
		"## Topics",
		"[references/configuration.md](references/configuration.md)",
		"[references/command-routing.md](references/command-routing.md)",
		"[references/troubleshooting.md](references/troubleshooting.md)",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated CLI reference missing %q\n%s", want, content)
		}
	}
	// The thinned router drops the verbose per-command catalog, the command
	// substitution placeholders, and the global-command and decision-guide
	// prose that moved into references/.
	for _, unwanted := range []string{
		"{{",
		"## Global Commands",
		"## Command Substitution Reference",
		"## Quick Decision Guide",
		"## Build Management",
		"## State Management",
		"**Subcommands:**",
		"```bash",
		"latest wrap, recent branch entries, open tasks",
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("thinned CLI reference still contains %q\n%s", unwanted, content)
		}
	}
	if !strings.Contains(stdout.String(), outputPath) {
		t.Fatalf("stdout = %q, want generated path %q", stdout.String(), outputPath)
	}
}

func TestCLIReferenceSourceMatchesGeneratedContract(t *testing.T) {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	repositoryRoot := filepath.Clean(filepath.Join(workingDir, "..", ".."))
	data, err := os.ReadFile(filepath.Join(repositoryRoot, "content", "skills", "loaf-reference", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(source CLI reference) error = %v", err)
	}
	if got, want := string(data), generateCLIReferenceSkill(cliReferenceCommands()); got != want {
		t.Fatal("content/skills/loaf-reference/SKILL.md is stale; regenerate it with `loaf __generate-cli-ref`")
	}
}

func TestJournalContextReferenceMetadataDescribesContractV2(t *testing.T) {
	var journal *cliReferenceSubcommand
	for _, command := range cliReferenceCommands() {
		if command.Name != "journal" {
			continue
		}
		for index := range command.Subcommands {
			if command.Subcommands[index].Name == "context" {
				journal = &command.Subcommands[index]
				break
			}
		}
	}
	if journal == nil {
		t.Fatal("journal context reference metadata is missing")
	}
	if strings.Contains(journal.Description, "latest wrap") {
		t.Fatalf("journal context description = %q, must not describe the retired three-part digest", journal.Description)
	}
	for _, want := range []string{"--branch <branch>", "--layer <name>", "--limit <n>", "--cursor <token>", "--from-hook", "--json", "for-prompt|for-compact|for-resumption"} {
		found := false
		for _, option := range journal.Options {
			if option.Flags == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("journal context reference options = %#v, want %q", journal.Options, want)
		}
	}
	for _, option := range journal.Options {
		if option.Flags == "--branch <branch>" {
			for _, want := range []string{"branch-recency", "bind state cursors", "actual Git branch"} {
				if !strings.Contains(option.Description, want) {
					t.Fatalf("--branch description = %q, want %q", option.Description, want)
				}
			}
		}
		if option.Flags == "--layer <name>" {
			for _, layer := range []string{"project-synthesis", "scoped-checkpoint", "active-lineage", "unresolved-blockers", "deferred-intent", "active-changes", "branch-recency", "transitional-tasks"} {
				if !strings.Contains(option.Description, layer) {
					t.Fatalf("--layer description = %q, want canonical layer %q", option.Description, layer)
				}
			}
		}
	}
}

func TestJournalContextAgentHelpDescribesContractV2(t *testing.T) {
	var journal *agentHelpSubcommand
	for _, command := range agentHelpCommands() {
		if command.Name != "journal" {
			continue
		}
		for index := range command.Subcommands {
			if command.Subcommands[index].Name == "context" {
				journal = &command.Subcommands[index]
				break
			}
		}
	}
	if journal == nil {
		t.Fatal("journal context agent help is missing")
	}
	if !strings.Contains(journal.Description, "contract-v2 active-truth") {
		t.Fatalf("journal context agent help description = %q, want contract-v2 active truth", journal.Description)
	}
	for _, want := range []string{"--branch <branch>", "--layer <name>", "--limit <n>", "--cursor <token>", "--from-hook", "--json", "for-prompt|for-compact|for-resumption"} {
		found := false
		for _, option := range journal.Options {
			if option.Flags == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("journal context agent help options = %#v, want %q", journal.Options, want)
		}
	}
	for _, option := range journal.Options {
		if option.Flags != "--branch <branch>" {
			continue
		}
		for _, want := range []string{"branch-recency", "bind state cursors", "actual Git branch"} {
			if !strings.Contains(option.Description, want) {
				t.Fatalf("agent help --branch description = %q, want %q", option.Description, want)
			}
		}
	}
}

func TestJournalLogExecpolicySafeAppearsInPublicHelpMetadata(t *testing.T) {
	var referenceLog *cliReferenceSubcommand
	for _, command := range cliReferenceCommands() {
		if command.Name != "journal" {
			continue
		}
		for index := range command.Subcommands {
			if command.Subcommands[index].Name == "log" {
				referenceLog = &command.Subcommands[index]
				break
			}
		}
	}
	if referenceLog == nil {
		t.Fatal("journal log reference metadata is missing")
	}
	foundReference := false
	for _, option := range referenceLog.Options {
		if option.Flags == "--execpolicy-safe" {
			foundReference = true
			if !strings.Contains(option.Description, "registered project") || !strings.Contains(option.Description, "runtime or hook payload") {
				t.Fatalf("reference safe option description = %q, want trust boundary", option.Description)
			}
		}
	}
	if !foundReference {
		t.Fatalf("journal log reference options = %#v, want --execpolicy-safe", referenceLog.Options)
	}

	var agentLog *agentHelpSubcommand
	for _, command := range agentHelpCommands() {
		if command.Name != "journal" {
			continue
		}
		for index := range command.Subcommands {
			if command.Subcommands[index].Name == "log" {
				agentLog = &command.Subcommands[index]
				break
			}
		}
	}
	if agentLog == nil {
		t.Fatal("journal log agent help is missing")
	}
	foundAgent := false
	for _, option := range agentLog.Options {
		if option.Flags == "--execpolicy-safe" {
			foundAgent = true
			if !strings.Contains(option.Description, "registered project") || !strings.Contains(option.Description, "runtime or hook payload") {
				t.Fatalf("agent safe option description = %q, want trust boundary", option.Description)
			}
		}
	}
	if !foundAgent {
		t.Fatalf("journal log agent options = %#v, want --execpolicy-safe", agentLog.Options)
	}
}

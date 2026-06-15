package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerKbStatusJSONUsesNativeKnowledgeFiles(t *testing.T) {
	repo := writeKbStatusFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "status", "--json"})
	if err != nil {
		t.Fatalf("kb status --json error = %v", err)
	}

	var summary kbStatusSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if summary.TotalFiles != 2 || summary.FilesWithCovers != 1 || summary.FilesWithoutCovers != 1 || summary.Stale != 0 {
		t.Fatalf("summary = %#v, want two files with one covered fresh file", summary)
	}
	if summary.AvgReviewAgeDays != 0 {
		t.Fatalf("avg age = %d, want 0", summary.AvgReviewAgeDays)
	}
	if summary.Directories["docs/knowledge"] != 1 || summary.Directories["docs/decisions"] != 1 {
		t.Fatalf("directories = %#v, want docs/knowledge and docs/decisions counts", summary.Directories)
	}
}

func TestRunnerKbStatusHumanUsesNativeKnowledgeFiles(t *testing.T) {
	repo := writeKbStatusFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "status"})
	if err != nil {
		t.Fatalf("kb status error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{"loaf kb status", "Files:", "2", "Covers:", "1", "Stale:", "0", "docs/knowledge", "docs/decisions"} {
		if !strings.Contains(output, want) {
			t.Fatalf("kb status output = %q, want %q", output, want)
		}
	}
}

func TestRunnerKbStatusRequiresGitRepository(t *testing.T) {
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: t.TempDir(),
	}.Run([]string{"kb", "status", "--json"})
	if err == nil {
		t.Fatal("kb status error = nil, want git repository error")
	}
	assertSilentExitCode(t, err, 1)
	output := decodeCommandError(t, stdout.Bytes())
	if output.Command != "kb status" || !strings.Contains(output.Error, "not inside a git repository") {
		t.Fatalf("kb status JSON error = %#v, want git repository error", output)
	}
}

func TestRunnerKbValidateSucceedsForValidKnowledgeFiles(t *testing.T) {
	repo := writeKbValidateValidFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "validate"})
	if err != nil {
		t.Fatalf("kb validate error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{"loaf kb validate", "docs/knowledge/go.md", "files", "0", "errors", "warnings"} {
		if !strings.Contains(output, want) {
			t.Fatalf("kb validate output = %q, want %q", output, want)
		}
	}

	stdout.Reset()
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "validate", "--json"})
	if err != nil {
		t.Fatalf("kb validate --json error = %v", err)
	}
	var results []kbValidationResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if len(results) != 1 || results[0].Errors == nil || results[0].Warnings == nil {
		t.Fatalf("results = %#v, want empty error/warning arrays", results)
	}
}

func TestRunnerKbValidateJSONReportsErrorsAndWarnings(t *testing.T) {
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, "docs", "knowledge"))
	writeFile(t, filepath.Join(repo, "docs", "knowledge", "broken.md"), strings.Join([]string{
		"---",
		"last_reviewed: 2026-02-30",
		"covers: [missing/**]",
		"depends_on: [docs/knowledge/nope.md]",
		"implementation_status: banana",
		"---",
		"# Broken",
		"",
	}, "\n"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "validate", "--json"})
	if err == nil {
		t.Fatal("kb validate error = nil, want validation failure")
	}
	assertSilentExitCode(t, err, 1)

	var results []kbValidationResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %#v, want one broken knowledge file", results)
	}
	if !hasValidationIssue(results[0].Errors, "topics") || !hasValidationIssue(results[0].Errors, "last_reviewed") {
		t.Fatalf("errors = %#v, want topics and last_reviewed errors", results[0].Errors)
	}
	for _, field := range []string{"covers", "depends_on", "implementation_status"} {
		if !hasValidationIssue(results[0].Warnings, field) {
			t.Fatalf("warnings = %#v, want %s warning", results[0].Warnings, field)
		}
	}
}

func TestRunnerKbCheckJSONUsesNativeStalenessResults(t *testing.T) {
	repo := writeKbCheckFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "check", "--json"})
	if err != nil {
		t.Fatalf("kb check --json error = %v", err)
	}

	var results []kbStalenessResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %#v, want covered and uncovered knowledge files", results)
	}
	covered := findStalenessResult(results, "docs/knowledge/go.md")
	if covered == nil || !covered.IsStale || !covered.HasCoverage || covered.CommitCount == 0 || covered.LastCommitAuthor != "Loaf Test" {
		t.Fatalf("covered result = %#v, want stale covered file with git metadata", covered)
	}
	uncovered := findStalenessResult(results, "docs/knowledge/process.md")
	if uncovered == nil || uncovered.IsStale || uncovered.HasCoverage {
		t.Fatalf("uncovered result = %#v, want no-coverage file", uncovered)
	}
}

func TestRunnerKbCheckFileReverseLookupUsesNativeGlobMatching(t *testing.T) {
	repo := writeKbCheckFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "check", "--file", "src/nested/main.go", "--json"})
	if err != nil {
		t.Fatalf("kb check --file --json error = %v", err)
	}
	var results []kbStalenessResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if len(results) != 1 || results[0].File != "docs/knowledge/go.md" {
		t.Fatalf("results = %#v, want only covering go knowledge file", results)
	}

	stdout.Reset()
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "check", "--file", "README.md"})
	if err != nil {
		t.Fatalf("kb check --file human error = %v", err)
	}
	if !strings.Contains(stdout.String(), "No knowledge files cover this path") {
		t.Fatalf("stdout = %q, want no coverage message", stdout.String())
	}
}

func TestRunnerKbReviewUpdatesLastReviewedNatively(t *testing.T) {
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, "docs", "knowledge"))
	filePath := filepath.Join(repo, "docs", "knowledge", "go.md")
	writeFile(t, filePath, strings.Join([]string{
		"---",
		"topics:",
		"  - go",
		"  - cli",
		"last_reviewed: 2000-01-01",
		"covers:",
		"  - internal/**/*.go",
		"---",
		"# Go",
		"",
	}, "\n"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "review", "docs/knowledge/go.md", "--json"})
	if err != nil {
		t.Fatalf("kb review --json error = %v", err)
	}

	var output struct {
		Topics       []string `json:"topics"`
		LastReviewed string   `json:"last_reviewed"`
		Covers       []string `json:"covers"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	today := time.Now().Format("2006-01-02")
	if output.LastReviewed != today || strings.Join(output.Topics, ",") != "go,cli" || len(output.Covers) != 1 {
		t.Fatalf("output = %#v, want updated review date with existing arrays preserved", output)
	}
	body, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", filePath, err)
	}
	text := string(body)
	for _, want := range []string{"last_reviewed: " + today, "  - go", "  - cli", "# Go"} {
		if !strings.Contains(text, want) {
			t.Fatalf("reviewed file = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "2000-01-01") {
		t.Fatalf("reviewed file = %q, still contains old review date", text)
	}
}

func TestRunnerKbReviewInsertsMissingLastReviewed(t *testing.T) {
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, "docs", "knowledge"))
	filePath := filepath.Join(repo, "docs", "knowledge", "go.md")
	writeFile(t, filePath, strings.Join([]string{
		"---",
		"topics: [go]",
		"covers: [internal/**/*.go]",
		"---",
		"# Go",
		"",
	}, "\n"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "review", "docs/knowledge/go.md"})
	if err != nil {
		t.Fatalf("kb review error = %v", err)
	}

	today := time.Now().Format("2006-01-02")
	if !strings.Contains(stdout.String(), "last_reviewed: "+today) {
		t.Fatalf("stdout = %q, want inserted review date", stdout.String())
	}
	body, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", filePath, err)
	}
	text := string(body)
	for _, want := range []string{"topics: [go]", "covers: [internal/**/*.go]", "last_reviewed: " + today, "# Go"} {
		if !strings.Contains(text, want) {
			t.Fatalf("reviewed file = %q, want %q", text, want)
		}
	}
}

func TestRunnerKbReviewRejectsNonKnowledgeFile(t *testing.T) {
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, "docs"))
	filePath := filepath.Join(repo, "docs", "note.md")
	original := strings.Join([]string{
		"---",
		"topics: go",
		"last_reviewed: 2000-01-01",
		"---",
		"# Note",
		"",
	}, "\n")
	writeFile(t, filePath, original)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "review", "docs/note.md", "--json"})
	if err == nil {
		t.Fatal("kb review error = nil, want non-knowledge error")
	}
	assertSilentExitCode(t, err, 1)

	var output map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if !strings.Contains(output["error"], "Not a knowledge file") {
		t.Fatalf("json error = %#v, want non-knowledge message", output)
	}
	body, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", filePath, err)
	}
	if string(body) != original {
		t.Fatalf("file changed after rejected review:\n%s", string(body))
	}
}

func TestRunnerKbInitCreatesDirsAndConfigNatively(t *testing.T) {
	withNativeQMD(t, false, nil, nil)
	repo := initCLIGitRepo(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "init", "--json"})
	if err != nil {
		t.Fatalf("kb init --json error = %v", err)
	}

	var result kbInitResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if len(result.Directories) != 2 || result.Directories[0].Status != "created" || result.Directories[1].Status != "created" {
		t.Fatalf("directories = %#v, want two created directories", result.Directories)
	}
	if result.Config.Path != ".agents/loaf.json" || result.Config.Status != "created" {
		t.Fatalf("config = %#v, want created .agents/loaf.json", result.Config)
	}
	if result.QMD.Available || len(result.QMD.Collections) != 0 {
		t.Fatalf("qmd = %#v, want unavailable with no collections", result.QMD)
	}
	for _, dir := range []string{"docs/knowledge", "docs/decisions"} {
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(dir))); err != nil {
			t.Fatalf("Stat(%s) error = %v", dir, err)
		}
	}
	var config struct {
		Knowledge struct {
			Local                  []string `json:"local"`
			StalenessThresholdDays int      `json:"staleness_threshold_days"`
			Imports                []string `json:"imports"`
		} `json:"knowledge"`
	}
	body, err := os.ReadFile(filepath.Join(repo, ".agents", "loaf.json"))
	if err != nil {
		t.Fatalf("ReadFile(.agents/loaf.json) error = %v", err)
	}
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("Unmarshal config error = %v\n%s", err, string(body))
	}
	if strings.Join(config.Knowledge.Local, ",") != "docs/knowledge,docs/decisions" || config.Knowledge.StalenessThresholdDays != 30 || config.Knowledge.Imports == nil {
		t.Fatalf("config knowledge = %#v, want default KB config", config.Knowledge)
	}
}

func TestRunnerKbInitAddsKnowledgeSectionToExistingConfigNatively(t *testing.T) {
	withNativeQMD(t, false, nil, nil)
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, ".agents"))
	writeFile(t, filepath.Join(repo, ".agents", "loaf.json"), `{"integrations":{"linear":{"enabled":false}}}`+"\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "init"})
	if err != nil {
		t.Fatalf("kb init error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Added knowledge section to .agents/loaf.json") {
		t.Fatalf("stdout = %q, want knowledge section message", stdout.String())
	}

	var config map[string]any
	body, err := os.ReadFile(filepath.Join(repo, ".agents", "loaf.json"))
	if err != nil {
		t.Fatalf("ReadFile(.agents/loaf.json) error = %v", err)
	}
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("Unmarshal config error = %v\n%s", err, string(body))
	}
	if _, ok := config["integrations"].(map[string]any); !ok {
		t.Fatalf("config = %#v, want existing integrations preserved", config)
	}
	if _, ok := config["knowledge"].(map[string]any); !ok {
		t.Fatalf("config = %#v, want knowledge section added", config)
	}
}

func TestRunnerKbInitRegistersQMDCollectionsNatively(t *testing.T) {
	repo := initCLIGitRepo(t)
	repoName := filepath.Base(repo)
	var registered []kbInitQMDCollection
	withNativeQMD(t, true, []string{repoName + "-knowledge"}, func(name string, path string) error {
		registered = append(registered, kbInitQMDCollection{Collection: name, Path: path})
		return nil
	})
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "init", "--json"})
	if err != nil {
		t.Fatalf("kb init --json error = %v", err)
	}

	var result kbInitResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if !result.QMD.Available || len(result.QMD.Collections) != 2 {
		t.Fatalf("qmd = %#v, want available with two collections", result.QMD)
	}
	if result.QMD.Collections[0].Status != "exists" || result.QMD.Collections[1].Status != "registered" {
		t.Fatalf("collections = %#v, want existing knowledge and registered decisions", result.QMD.Collections)
	}
	if len(registered) != 1 || registered[0].Collection != repoName+"-decisions" || registered[0].Path != filepath.Join(repo, "docs", "decisions") {
		t.Fatalf("registered = %#v, want decisions collection", registered)
	}
}

func TestRunnerKbImportRequiresQMDNatively(t *testing.T) {
	withNativeQMD(t, false, nil, nil)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: t.TempDir(),
	}.Run([]string{"kb", "import", "other", "--json"})
	if err == nil {
		t.Fatal("kb import error = nil, want QMD required")
	}
	assertSilentExitCode(t, err, 1)

	var result kbImportResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if !strings.Contains(result.Error, "QMD is required") {
		t.Fatalf("result = %#v, want QMD error", result)
	}
}

func TestRunnerKbImportRejectsMalformedConfigBeforeRegistering(t *testing.T) {
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, ".agents"))
	writeFile(t, filepath.Join(repo, ".agents", "loaf.json"), "{not json\n")
	registered := false
	withNativeQMD(t, true, nil, func(name string, path string) error {
		registered = true
		return nil
	})
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "import", "other", "--json"})
	if err == nil {
		t.Fatal("kb import error = nil, want malformed config error")
	}
	assertSilentExitCode(t, err, 1)
	if registered {
		t.Fatalf("qmd register was called before config validation")
	}
	var result kbImportResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if !strings.Contains(result.Error, "Cannot parse .agents/loaf.json") {
		t.Fatalf("result = %#v, want malformed config error", result)
	}
}

func TestRunnerKbImportSkipsAlreadyImportedCollectionNatively(t *testing.T) {
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, ".agents"))
	writeFile(t, filepath.Join(repo, ".agents", "loaf.json"), strings.Join([]string{
		"{",
		`  "knowledge": {`,
		`    "local": ["docs/knowledge"],`,
		`    "imports": [{"name": "other"}]`,
		"  }",
		"}",
		"",
	}, "\n"))
	registered := false
	withNativeQMD(t, true, nil, func(name string, path string) error {
		registered = true
		return nil
	})
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "import", "other", "--json"})
	if err != nil {
		t.Fatalf("kb import duplicate error = %v", err)
	}
	if registered {
		t.Fatalf("qmd register was called for duplicate import")
	}
	var result kbImportResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if result.Name != "other" || result.Collection != "other-knowledge" || result.Status != "already_imported" {
		t.Fatalf("result = %#v, want already_imported", result)
	}
}

func TestRunnerKbImportRegistersCollectionAndUpdatesConfigNatively(t *testing.T) {
	repo := initCLIGitRepo(t)
	var registered []kbInitQMDCollection
	withNativeQMD(t, true, nil, func(name string, path string) error {
		registered = append(registered, kbInitQMDCollection{Collection: name, Path: path})
		return nil
	})
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "import", "other", "--path", "/tmp/other-kb", "--json"})
	if err != nil {
		t.Fatalf("kb import --json error = %v", err)
	}
	if len(registered) != 1 || registered[0].Collection != "other-knowledge" || registered[0].Path != "/tmp/other-kb" {
		t.Fatalf("registered = %#v, want custom path registration", registered)
	}

	var result kbImportResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if result.Name != "other" || result.Collection != "other-knowledge" || result.Status != "imported" {
		t.Fatalf("result = %#v, want imported", result)
	}
	var config struct {
		Knowledge struct {
			Local                  []string `json:"local"`
			StalenessThresholdDays int      `json:"staleness_threshold_days"`
			Imports                []struct {
				Name string `json:"name"`
			} `json:"imports"`
		} `json:"knowledge"`
	}
	body, err := os.ReadFile(filepath.Join(repo, ".agents", "loaf.json"))
	if err != nil {
		t.Fatalf("ReadFile(.agents/loaf.json) error = %v", err)
	}
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("Unmarshal config error = %v\n%s", err, string(body))
	}
	if strings.Join(config.Knowledge.Local, ",") != "docs/knowledge,docs/decisions" || config.Knowledge.StalenessThresholdDays != 30 {
		t.Fatalf("knowledge defaults = %#v, want default KB config", config.Knowledge)
	}
	if len(config.Knowledge.Imports) != 1 || config.Knowledge.Imports[0].Name != "other" {
		t.Fatalf("imports = %#v, want other import", config.Knowledge.Imports)
	}
}

func TestRunnerKbGlossaryUpsertCheckAndListNatively(t *testing.T) {
	repo := initCLIGitRepo(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "glossary", "upsert", "Module", "--definition", "A unit of code.", "--avoid", "Service, Component"})
	if err != nil {
		t.Fatalf("kb glossary upsert error = %v", err)
	}
	if !strings.Contains(stdout.String(), "canonical:") || !strings.Contains(stdout.String(), "Module") {
		t.Fatalf("stdout = %q, want canonical creation", stdout.String())
	}
	glossaryPath := filepath.Join(repo, "docs", "knowledge", "glossary.md")
	body, err := os.ReadFile(glossaryPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", glossaryPath, err)
	}
	for _, want := range []string{"type: glossary", "## Canonical Terms", "### Module", "A unit of code.", "_Avoid_: Service, Component"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("glossary = %q, want %q", string(body), want)
		}
	}

	stdout.Reset()
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "glossary", "check", "Service"})
	if err != nil {
		t.Fatalf("kb glossary check alias error = %v", err)
	}
	if !strings.Contains(stdout.String(), "avoided, use Module") {
		t.Fatalf("stdout = %q, want alias guidance", stdout.String())
	}

	stdout.Reset()
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "glossary", "list", "--canonical"})
	if err != nil {
		t.Fatalf("kb glossary list error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Module: A unit of code.") {
		t.Fatalf("stdout = %q, want canonical listing", stdout.String())
	}
}

func TestRunnerKbGlossaryProposeAndStabilizeNatively(t *testing.T) {
	repo := initCLIGitRepo(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "glossary", "propose", "Adapter", "--definition", "Candidate definition.", "--avoid", "Wrapper"})
	if err != nil {
		t.Fatalf("kb glossary propose error = %v", err)
	}
	if !strings.Contains(stdout.String(), "candidate:") {
		t.Fatalf("stdout = %q, want candidate proposal", stdout.String())
	}

	stdout.Reset()
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "glossary", "check", "Adapter"})
	if err != nil {
		t.Fatalf("kb glossary check candidate error = %v", err)
	}
	if !strings.Contains(stdout.String(), "candidate:") || !strings.Contains(stdout.String(), "Adapter") {
		t.Fatalf("stdout = %q, want candidate check", stdout.String())
	}

	stdout.Reset()
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "glossary", "stabilize", "Adapter", "--definition", "Canonical definition."})
	if err != nil {
		t.Fatalf("kb glossary stabilize error = %v", err)
	}
	if !strings.Contains(stdout.String(), "stabilized:") {
		t.Fatalf("stdout = %q, want stabilized output", stdout.String())
	}

	body, err := os.ReadFile(filepath.Join(repo, "docs", "knowledge", "glossary.md"))
	if err != nil {
		t.Fatalf("ReadFile(glossary) error = %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "## Canonical Terms\n\n### Adapter") || !strings.Contains(text, "Canonical definition.") {
		t.Fatalf("glossary = %q, want promoted canonical adapter", text)
	}
	if strings.Contains(text, "## Candidates\n\n### Adapter") {
		t.Fatalf("glossary = %q, adapter still appears in candidates", text)
	}
}

func TestRunnerKbGlossaryReadsDoNotCreateFile(t *testing.T) {
	repo := initCLIGitRepo(t)
	glossaryPath := filepath.Join(repo, "docs", "knowledge", "glossary.md")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "glossary", "list"})
	if err != nil {
		t.Fatalf("kb glossary list fresh error = %v", err)
	}
	if !strings.Contains(stdout.String(), "No glossary entries yet") {
		t.Fatalf("stdout = %q, want empty glossary message", stdout.String())
	}
	if _, err := os.Stat(glossaryPath); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want missing glossary after read", glossaryPath, err)
	}

	stdout.Reset()
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "glossary", "check", "Anything"})
	if err == nil || !strings.Contains(err.Error(), "unknown glossary term") {
		t.Fatalf("kb glossary check error = %v, want unknown term", err)
	}
	if !strings.Contains(stdout.String(), "unknown:") {
		t.Fatalf("stdout = %q, want unknown output", stdout.String())
	}
	if _, err := os.Stat(glossaryPath); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want missing glossary after check", glossaryPath, err)
	}
}

func TestRunnerKbGlossaryLinearNativeBlocksWrites(t *testing.T) {
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, ".agents"))
	writeFile(t, filepath.Join(repo, ".agents", "loaf.json"), `{"integrations":{"linear":{"enabled":true}}}`+"\n")
	glossaryPath := filepath.Join(repo, "docs", "knowledge", "glossary.md")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "glossary", "upsert", "Module", "--definition", "Nope."})
	if err == nil || err.Error() != glossaryLinearNativeMessage {
		t.Fatalf("kb glossary upsert error = %v, want exact linear-native message", err)
	}
	if _, err := os.Stat(glossaryPath); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want no glossary after blocked write", glossaryPath, err)
	}
}

func TestRunnerKbGlossaryStabilizeMissingDoesNotCreateFile(t *testing.T) {
	repo := initCLIGitRepo(t)
	glossaryPath := filepath.Join(repo, "docs", "knowledge", "glossary.md")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"kb", "glossary", "stabilize", "Missing"})
	if err == nil || !strings.Contains(err.Error(), "not in candidates") {
		t.Fatalf("kb glossary stabilize error = %v, want missing candidate", err)
	}
	if _, err := os.Stat(glossaryPath); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want no glossary after failed stabilize", glossaryPath, err)
	}
}

func TestNativeGlossaryMutationParity(t *testing.T) {
	repo := initCLIGitRepo(t)

	action, err := upsertNativeGlossaryTerm(repo, "Module", "v1", []string{"Service"})
	if err != nil {
		t.Fatalf("upsert created error = %v", err)
	}
	if action != "created" {
		t.Fatalf("action = %q, want created", action)
	}

	action, err = upsertNativeGlossaryTerm(repo, "Module", "v2", nil)
	if err != nil {
		t.Fatalf("upsert updated error = %v", err)
	}
	if action != "updated" {
		t.Fatalf("action = %q, want updated", action)
	}
	data, exists, err := loadNativeGlossaryForRead(repo)
	if err != nil || !exists {
		t.Fatalf("load glossary exists=%v err=%v, want existing glossary", exists, err)
	}
	if len(data.Canonical) != 1 || data.Canonical[0].Definition != "v2" || len(data.Canonical[0].Avoid) != 0 {
		t.Fatalf("canonical = %#v, want updated definition and cleared avoid list", data.Canonical)
	}

	if _, err := proposeNativeGlossaryTerm(repo, "Adapter", "candidate def", nil); err != nil {
		t.Fatalf("propose adapter error = %v", err)
	}
	if _, err := upsertNativeGlossaryTerm(repo, "Adapter", "canonical def", nil); err != nil {
		t.Fatalf("upsert candidate promotion error = %v", err)
	}
	data, exists, err = loadNativeGlossaryForRead(repo)
	if err != nil || !exists {
		t.Fatalf("load glossary exists=%v err=%v, want existing glossary", exists, err)
	}
	if len(data.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want promoted candidate removed", data.Candidates)
	}
	if findGlossaryTerm(data.Canonical, "Adapter") == nil {
		t.Fatalf("canonical = %#v, want promoted Adapter", data.Canonical)
	}
}

func TestNativeGlossaryRejectsConflictingWrites(t *testing.T) {
	repo := initCLIGitRepo(t)

	if _, err := upsertNativeGlossaryTerm(repo, "Module", "...", nil); err != nil {
		t.Fatalf("upsert module error = %v", err)
	}
	if _, err := upsertNativeGlossaryTerm(repo, "Boundary", "...", []string{"Module"}); err == nil || !strings.Contains(err.Error(), "already canonical") {
		t.Fatalf("conflicting alias error = %v, want already canonical", err)
	}
	if _, err := proposeNativeGlossaryTerm(repo, "Module", "...", nil); err == nil || !strings.Contains(err.Error(), "already canonical") {
		t.Fatalf("canonical proposal error = %v, want already canonical", err)
	}
}

func TestNativeGlossaryListAndReproposalParity(t *testing.T) {
	repo := initCLIGitRepo(t)

	if _, err := upsertNativeGlossaryTerm(repo, "Module", "canonical", nil); err != nil {
		t.Fatalf("upsert module error = %v", err)
	}
	if _, err := proposeNativeGlossaryTerm(repo, "Adapter", "v1", []string{"Wrapper"}); err != nil {
		t.Fatalf("propose adapter v1 error = %v", err)
	}
	action, err := proposeNativeGlossaryTerm(repo, "Adapter", "v2", nil)
	if err != nil {
		t.Fatalf("propose adapter v2 error = %v", err)
	}
	if action != "updated" {
		t.Fatalf("action = %q, want updated", action)
	}

	candidates, err := listNativeGlossaryTerms(repo, "candidates")
	if err != nil {
		t.Fatalf("list candidates error = %v", err)
	}
	if len(candidates.Canonical) != 0 || len(candidates.Candidates) != 1 || candidates.Candidates[0].Definition != "v2" || len(candidates.Candidates[0].Avoid) != 0 {
		t.Fatalf("candidates = %#v, want updated candidate only", candidates)
	}

	canonical, err := listNativeGlossaryTerms(repo, "canonical")
	if err != nil {
		t.Fatalf("list canonical error = %v", err)
	}
	if len(canonical.Candidates) != 0 || len(canonical.Canonical) != 1 || canonical.Canonical[0].Name != "Module" {
		t.Fatalf("canonical = %#v, want canonical-only module", canonical)
	}
}

func TestNativeGlossaryLinearNativeReReadsConfig(t *testing.T) {
	repo := initCLIGitRepo(t)
	configPath := filepath.Join(repo, ".agents", "loaf.json")
	mkdirAll(t, filepath.Dir(configPath))

	writeFile(t, configPath, `{"integrations":{"linear":{"enabled":false}}}`+"\n")
	if nativeGlossaryLinearNative(repo) {
		t.Fatalf("linear-native = true, want false")
	}
	writeFile(t, configPath, `{"integrations":{"linear":{"enabled":true}}}`+"\n")
	if !nativeGlossaryLinearNative(repo) {
		t.Fatalf("linear-native = false, want true")
	}
	writeFile(t, configPath, `{"integrations":{"linear":{"enabled":false}}}`+"\n")
	if nativeGlossaryLinearNative(repo) {
		t.Fatalf("linear-native = true after toggle back, want false")
	}
}

func TestRunnerKbHelpAndUnknownSubcommandAreNative(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"kb", "--help"})
	if err != nil {
		t.Fatalf("kb --help error = %v", err)
	}
	for _, want := range []string{"Usage: loaf kb <subcommand>", "status", "glossary"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}

	stdout.Reset()
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"kb", "legacy-tail", "--json"})
	if err == nil {
		t.Fatal("kb unknown error = nil, want native unknown subcommand")
	}
	assertSilentExitCode(t, err, 1)
	unknown := decodeCommandError(t, stdout.Bytes())
	if unknown.Command != "kb legacy-tail" || !strings.Contains(unknown.Error, `unknown loaf kb subcommand "legacy-tail"`) {
		t.Fatalf("kb unknown JSON error = %#v, want native unknown subcommand", unknown)
	}
}

func TestRunnerKbSubcommandHelpIsNative(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "status", args: []string{"kb", "status", "--help"}, want: "Usage: loaf kb status [--json]"},
		{name: "validate", args: []string{"kb", "validate", "--help"}, want: "Usage: loaf kb validate [--json]"},
		{name: "check", args: []string{"kb", "check", "--help"}, want: "Usage: loaf kb check [--file <path>] [--json]"},
		{name: "review", args: []string{"kb", "review", "--help"}, want: "Usage: loaf kb review <file> [--json]"},
		{name: "init", args: []string{"kb", "init", "--help"}, want: "Usage: loaf kb init [--json]"},
		{name: "import", args: []string{"kb", "import", "--help"}, want: "Usage: loaf kb import <name> --path <path> [--json]"},
		{name: "glossary", args: []string{"kb", "glossary", "--help"}, want: "Usage: loaf kb glossary <subcommand> [options]"},
		{name: "glossary list", args: []string{"kb", "glossary", "list", "--help"}, want: "Usage: loaf kb glossary list [--canonical|--candidates|--all]"},
		{name: "glossary upsert", args: []string{"kb", "glossary", "upsert", "--help"}, want: "Usage: loaf kb glossary upsert <term> --definition <text> [--avoid <terms>]"},
		{name: "glossary stabilize", args: []string{"kb", "glossary", "stabilize", "--help"}, want: "Usage: loaf kb glossary stabilize <term> --definition <text>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout := bytes.Buffer{}
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: workingDir,
			}.Run(tc.args)
			if err != nil {
				t.Fatalf("Run(%v) error = %v", tc.args, err)
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout = %q, want %q", stdout.String(), tc.want)
			}
		})
	}
}

func withNativeQMD(t *testing.T, available bool, existing []string, register func(name string, path string) error) {
	t.Helper()
	oldLookPath := qmdLookPath
	oldListCollections := qmdListCollections
	oldRegisterCollection := qmdRegisterCollection
	qmdLookPath = func(file string) (string, error) {
		if file != "qmd" {
			return "", errors.New("unexpected executable lookup")
		}
		if !available {
			return "", errors.New("qmd unavailable")
		}
		return "/test/bin/qmd", nil
	}
	qmdListCollections = func() []string {
		return append([]string(nil), existing...)
	}
	qmdRegisterCollection = func(name string, path string) error {
		if register == nil {
			return nil
		}
		return register(name, path)
	}
	t.Cleanup(func() {
		qmdLookPath = oldLookPath
		qmdListCollections = oldListCollections
		qmdRegisterCollection = oldRegisterCollection
	})
}

func writeKbStatusFixture(t *testing.T) string {
	t.Helper()
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, "docs", "knowledge"))
	mkdirAll(t, filepath.Join(repo, "docs", "decisions"))
	today := time.Now().Format("2006-01-02")
	writeFile(t, filepath.Join(repo, "docs", "knowledge", "go.md"), strings.Join([]string{
		"---",
		"topics:",
		"  - go",
		"last_reviewed: " + today,
		"covers:",
		"  - src/**/*.go",
		"---",
		"# Go",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(repo, "docs", "decisions", "adr.md"), strings.Join([]string{
		"---",
		"topics: [architecture]",
		"last_reviewed: " + today,
		"---",
		"# ADR",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(repo, "docs", "knowledge", "ignored.md"), "# no frontmatter\n")
	return repo
}

func writeKbValidateValidFixture(t *testing.T) string {
	t.Helper()
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, "src"))
	writeFile(t, filepath.Join(repo, "src", "main.go"), "package main\n")
	gitCLI(t, repo, "add", "src/main.go")
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "add go file")
	mkdirAll(t, filepath.Join(repo, "docs", "knowledge"))
	writeFile(t, filepath.Join(repo, "docs", "knowledge", "go.md"), strings.Join([]string{
		"---",
		"topics:",
		"  - go",
		"last_reviewed: " + time.Now().Format("2006-01-02"),
		"covers:",
		"  - src/main.go",
		"implementation_status: stable",
		"---",
		"# Go",
		"",
	}, "\n"))
	return repo
}

func hasValidationIssue(issues []kbValidationIssue, field string) bool {
	for _, issue := range issues {
		if issue.Field == field {
			return true
		}
	}
	return false
}

func writeKbCheckFixture(t *testing.T) string {
	t.Helper()
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, "src", "nested"))
	writeFile(t, filepath.Join(repo, "src", "nested", "main.go"), "package nested\n")
	gitCLI(t, repo, "add", "src/nested/main.go")
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "add nested go file")
	mkdirAll(t, filepath.Join(repo, "docs", "knowledge"))
	writeFile(t, filepath.Join(repo, "docs", "knowledge", "go.md"), strings.Join([]string{
		"---",
		"topics:",
		"  - go",
		"last_reviewed: 2000-01-01",
		"covers:",
		"  - src/**/*.go",
		"---",
		"# Go",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(repo, "docs", "knowledge", "process.md"), strings.Join([]string{
		"---",
		"topics:",
		"  - process",
		"last_reviewed: 2000-01-01",
		"---",
		"# Process",
		"",
	}, "\n"))
	return repo
}

func findStalenessResult(results []kbStalenessResult, file string) *kbStalenessResult {
	for i := range results {
		if results[i].File == file {
			return &results[i]
		}
	}
	return nil
}

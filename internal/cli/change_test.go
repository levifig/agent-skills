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

// changeDoc assembles a change.md body from frontmatter and section blocks so
// each test can isolate exactly one Verification-Contract clause.
func changeDoc(frontmatter string, sections ...string) string {
	var b strings.Builder
	b.WriteString(frontmatter)
	b.WriteString("\n# Title\n\n")
	for _, section := range sections {
		b.WriteString(section)
		b.WriteString("\n\n")
	}
	return b.String()
}

func changeFrontmatter(change, created, branch string) string {
	return strings.Join([]string{
		"---",
		"change: " + change,
		"created: " + created,
		"branch: " + branch,
		"---",
	}, "\n")
}

// productSections returns the five required Product Contract H2s, each with a
// line of body so they read as present and non-empty.
func productSections() []string {
	return []string{
		"## Problem\n\nThe friction.",
		"## Hypothesis\n\nThe bet.",
		"## Scope\n\nIn and out.",
		"## Observable Workflow\n\nWhat ships.",
		"## Rabbit Holes and No-Gos\n\nBoundaries.",
	}
}

// executableSections returns the four sections that drive derived executability.
func executableSections() []string {
	return []string{
		"## Planning Contract\n\n### Approach\n\nHow.",
		"## Implementation Units\n\n- U1 — do the thing.",
		"## Verification Contract\n\n- V1. command exits non-zero.",
		"## Definition of Done\n\n- Gates pass.",
	}
}

// writeChangeFolder materializes docs/changes/<folder>/change.md under root and
// returns the absolute folder path.
func writeChangeFolder(t *testing.T, root, folder, content string) string {
	t.Helper()
	dir := filepath.Join(root, "docs", "changes", folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "change.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(change.md) error = %v", err)
	}
	return dir
}

func runChangeCheckJSON(t *testing.T, repo string, args ...string) (changeCheckJSON, error) {
	t.Helper()
	var stdout bytes.Buffer
	runArgs := append([]string{"change", "check"}, args...)
	runArgs = append(runArgs, "--json")
	err := Runner{Stdout: &stdout, WorkingDir: repo}.Run(runArgs)
	var out changeCheckJSON
	if decodeErr := json.Unmarshal(stdout.Bytes(), &out); decodeErr != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), decodeErr)
	}
	return out, err
}

// --- V1: violations, exit non-zero -----------------------------------------

// V1(a): status-like frontmatter keys.
func TestChangeCheckV1RejectsStatusLikeKeys(t *testing.T) {
	for _, key := range []string{"readiness", "status", "state"} {
		t.Run(key, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			fm := strings.Join([]string{
				"---",
				"change: demo",
				"created: 2026-07-04",
				"branch: demo",
				key + ": whatever",
				"---",
			}, "\n")
			folder := writeChangeFolder(t, repo, "20260704-demo", changeDoc(fm, productSections()...))

			out, err := runChangeCheckJSON(t, repo, folder)
			var exitErr ExitError
			if !errors.As(err, &exitErr) || exitErr.Code == 0 {
				t.Fatalf("err = %v, want non-zero ExitError", err)
			}
			if out.Passed {
				t.Fatalf("passed = true, want false for status-like key %q", key)
			}
			if !findingsContain(out.Findings, key) {
				t.Fatalf("findings = %v, want mention of banned key %q", out.Findings, key)
			}
		})
	}
}

// V1(a): progress vocabulary as a frontmatter value in any field.
func TestChangeCheckV1RejectsProgressVocabularyValues(t *testing.T) {
	for _, value := range []string{"active", "in-progress", "done", "archived"} {
		t.Run(value, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			fm := strings.Join([]string{
				"---",
				"change: demo",
				"created: 2026-07-04",
				"branch: demo",
				"phase: " + value,
				"---",
			}, "\n")
			folder := writeChangeFolder(t, repo, "20260704-demo", changeDoc(fm, productSections()...))

			out, err := runChangeCheckJSON(t, repo, folder)
			var exitErr ExitError
			if !errors.As(err, &exitErr) || exitErr.Code == 0 {
				t.Fatalf("err = %v, want non-zero ExitError for progress value %q", value, err)
			}
			if out.Passed {
				t.Fatalf("passed = true, want false for progress value %q", value)
			}
			if !findingsContain(out.Findings, value) {
				t.Fatalf("findings = %v, want mention of progress value %q", out.Findings, value)
			}
		})
	}
}

// V1(a): the full canonical change-state vocabulary (Decision 22) plus released
// is banned as a frontmatter value under ANY key. This is external review round
// 4's exact probe set — each canonical state (and released) stored under an
// arbitrary key that is not itself status-like.
func TestChangeCheckV1RejectsCanonicalStateVocabularyUnderArbitraryKeys(t *testing.T) {
	cases := []struct{ key, value string }{
		{"phase", "shaping"},
		{"queue", "backlog"},
		{"lane", "todo"},
		{"review_phase", "review"},
		{"merge_state", "merged"},
		{"release_state", "released"},
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			fm := strings.Join([]string{
				"---",
				"change: demo",
				"created: 2026-07-04",
				"branch: demo",
				tc.key + ": " + tc.value,
				"---",
			}, "\n")
			folder := writeChangeFolder(t, repo, "20260704-demo", changeDoc(fm, productSections()...))

			out, err := runChangeCheckJSON(t, repo, folder)
			var exitErr ExitError
			if !errors.As(err, &exitErr) || exitErr.Code == 0 {
				t.Fatalf("err = %v, want non-zero ExitError for %s: %s", err, tc.key, tc.value)
			}
			if out.Passed {
				t.Fatalf("passed = true, want false for %q stored under key %q", tc.value, tc.key)
			}
			if !findingsContain(out.Findings, tc.value) || !findingsContain(out.Findings, tc.key) {
				t.Fatalf("findings = %v, want mention of banned value %q under key %q", out.Findings, tc.value, tc.key)
			}
		})
	}
}

// V1(a): matching is case-insensitive on the normalized value — underscores and
// spaces normalize to hyphens, so "In Progress" and "in_progress" both match the
// canonical "in-progress".
func TestChangeCheckV1RejectsStateVocabularyRegardlessOfCasingOrSeparator(t *testing.T) {
	for _, value := range []string{"In Progress", "in_progress", "IN-PROGRESS", "Merged", "BACKLOG"} {
		t.Run(value, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			fm := strings.Join([]string{
				"---",
				"change: demo",
				"created: 2026-07-04",
				"branch: demo",
				"phase: " + value,
				"---",
			}, "\n")
			folder := writeChangeFolder(t, repo, "20260704-demo", changeDoc(fm, productSections()...))

			out, err := runChangeCheckJSON(t, repo, folder)
			var exitErr ExitError
			if !errors.As(err, &exitErr) || exitErr.Code == 0 {
				t.Fatalf("err = %v, want non-zero ExitError for normalized state value %q", err, value)
			}
			if out.Passed {
				t.Fatalf("passed = true, want false for normalized state value %q", value)
			}
		})
	}
}

// V1(a) exemption: identity fields (change, created, branch) are exempt from the
// state-vocabulary ban — their semantics are checked elsewhere. A branch is a git
// ref that may legitimately be named after a state word, so branch: review passes.
func TestChangeCheckV1AllowsBranchNamedLikeState(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "review"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil; branch: review is an identity field, exempt from the state ban", err)
	}
	if !out.Passed {
		t.Fatalf("passed = false, want true; a branch named after a state word must not be a violation. findings = %v", out.Findings)
	}
}

// V1(b): frontmatter must open the file at byte one.
func TestChangeCheckV1RejectsFrontmatterNotAtByteOne(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", "\n"+body)

	out, err := runChangeCheckJSON(t, repo, folder)
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("err = %v, want non-zero ExitError", err)
	}
	if !findingsContain(out.Findings, "byte one") {
		t.Fatalf("findings = %v, want byte-one violation", out.Findings)
	}
}

// V1(c): malformed folder name.
func TestChangeCheckV1RejectsMalformedFolderName(t *testing.T) {
	cases := map[string]string{
		"no-date-prefix": "shape-first",
		"bad-date":       "2026-07-first",
		"uppercase-slug": "20260704-Demo",
		"underscore":     "20260704-de_mo",
	}
	for name, folderName := range cases {
		t.Run(name, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), productSections()...)
			folder := writeChangeFolder(t, repo, folderName, body)

			out, err := runChangeCheckJSON(t, repo, folder)
			var exitErr ExitError
			if !errors.As(err, &exitErr) || exitErr.Code == 0 {
				t.Fatalf("err = %v, want non-zero ExitError for folder %q", folderName, err)
			}
			if !findingsContain(out.Findings, "folder name") {
				t.Fatalf("findings = %v, want folder-name violation", out.Findings)
			}
		})
	}
}

// V1(d): identity mismatch — change: vs folder slug.
func TestChangeCheckV1RejectsSlugMismatch(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("other", "2026-07-04", "other"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("err = %v, want non-zero ExitError", err)
	}
	if !findingsContain(out.Findings, "change:") {
		t.Fatalf("findings = %v, want change/slug mismatch", out.Findings)
	}
}

// V1(d): identity mismatch — created: date vs folder date prefix.
func TestChangeCheckV1RejectsCreatedDateMismatch(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("demo", "2026-07-05", "demo"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("err = %v, want non-zero ExitError", err)
	}
	if !findingsContain(out.Findings, "created:") {
		t.Fatalf("findings = %v, want created/date mismatch", out.Findings)
	}
}

// V1(e): missing Product Contract sections.
func TestChangeCheckV1RejectsMissingProductSections(t *testing.T) {
	repo := initCLIGitRepo(t)
	// Only Problem present; the other four product sections missing.
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), "## Problem\n\nThe friction.")
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("err = %v, want non-zero ExitError", err)
	}
	for _, want := range []string{"Hypothesis", "Scope", "Observable Workflow", "Rabbit Holes and No-Gos"} {
		if !findingsContain(out.Findings, want) {
			t.Fatalf("findings = %v, want missing-section mention of %q", out.Findings, want)
		}
	}
}

// --- V2: report, exit zero for a valid shaping-stage document ----------------

// A document with only the product sections is valid (exit zero) but reported
// non-executable, with the missing tail sections listed as gaps.
func TestChangeCheckV2ShapingStagePassesButNotExecutable(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("shaping-stage check err = %v, want nil (exit zero)", err)
	}
	if !out.Passed {
		t.Fatalf("passed = false, want true for a valid shaping-stage document")
	}
	if out.Executable {
		t.Fatalf("executable = true, want false for product-only document")
	}
	for _, want := range []string{"Planning Contract", "Implementation Units", "Verification Contract", "Definition of Done"} {
		if !findingsContain(out.Gaps, want) {
			t.Fatalf("gaps = %v, want gap for %q", out.Gaps, want)
		}
	}
}

// V2: executability derivation with a single gap — an empty tail section counts
// as a gap even though the heading is present.
func TestChangeCheckV2ExecutabilityGapForEmptySection(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nHow.",
		"## Implementation Units\n\n- U1 — do the thing.",
		"## Verification Contract\n\n- V1. exits non-zero.",
		"## Definition of Done\n", // present but empty
	)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil (exit zero, empty section is a gap not a violation)", err)
	}
	if out.Executable {
		t.Fatalf("executable = true, want false when Definition of Done is empty")
	}
	if !findingsContain(out.Gaps, "Definition of Done") {
		t.Fatalf("gaps = %v, want Definition of Done gap", out.Gaps)
	}
	if findingsContain(out.Gaps, "Planning Contract") {
		t.Fatalf("gaps = %v, should not flag the non-empty Planning Contract", out.Gaps)
	}
}

// V2: a fully-populated document reports executable with no gaps.
func TestChangeCheckV2FullDocumentIsExecutable(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(), executableSections()...)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !out.Passed || !out.Executable {
		t.Fatalf("passed=%v executable=%v, want both true", out.Passed, out.Executable)
	}
	if len(out.Gaps) != 0 {
		t.Fatalf("gaps = %v, want none", out.Gaps)
	}
}

// --- V2 placeholder discounting: authored content, not scaffolding -----------

// TestChangeCheckFreshInitNotExecutable is the traceable resolution of the U3
// finding: a Change scaffolded by `loaf change init` used to read
// executable:yes because the template's bracket placeholders counted as content
// (the literal "present and non-empty" reading let a placeholder-only document
// satisfy the V3 gate). With V2 discounting placeholders and comments, a fresh
// Change reads not-executable — its tail is scaffolding, not authored content.
//
// Every tail section ships as scaffolding — bracket placeholders and HTML
// comments only, no bare labels — so all four executable sections read as gaps
// on a fresh init and none is executable until the author writes real content.
func TestChangeCheckFreshInitNotExecutable(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "fresh-demo"}); err != nil {
		t.Fatalf("change init error = %v", err)
	}
	today := time.Now().Format("20060102")
	folder := filepath.Join(repo, "docs", "changes", today+"-fresh-demo")

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("check on freshly-init'd change err = %v, want nil (shaping-stage is valid, exit zero)", err)
	}
	if !out.Passed {
		t.Fatalf("passed = false, want true; a freshly-init'd Change has no violations. findings = %v", out.Findings)
	}
	if out.Executable {
		t.Fatalf("executable = true, want false; a freshly-templated Change has no authored tail. gaps = %v", out.Gaps)
	}
	for _, want := range []string{"Planning Contract", "Implementation Units", "Verification Contract", "Definition of Done"} {
		if !findingsContain(out.Gaps, want) {
			t.Fatalf("gaps = %v, want scaffolding-only tail section %q listed as a gap", out.Gaps, want)
		}
	}
}

// A tail section mixing a placeholder line with one authored line is non-empty:
// discounting removes only the placeholder, and any real content keeps the
// section authored.
func TestChangeCheckSectionMixedPlaceholderIsAuthored(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nReal shaping prose.",
		"## Implementation Units\n\n- [What this Change delivers.]\n- Wire the token rotation job.",
		"## Verification Contract\n\n- V1. command exits non-zero.",
		"## Definition of Done\n\n- Gates pass.",
	)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if findingsContain(out.Gaps, "Implementation Units") {
		t.Fatalf("gaps = %v, must not flag Implementation Units; it carries an authored line beside the placeholder", out.Gaps)
	}
	if !out.Executable {
		t.Fatalf("executable = false, want true; every tail section is authored. gaps = %v", out.Gaps)
	}
}

// A tail section whose only content is an HTML comment reads as empty — comments
// are guidance, not authored content.
func TestChangeCheckSectionOnlyHTMLCommentIsEmpty(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nReal shaping prose.",
		"## Implementation Units\n\n- Ship the thing.",
		"## Verification Contract\n\n- V1. command exits non-zero.",
		"## Definition of Done\n\n<!-- fill this in before the draft to ready flip -->",
	)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if out.Executable {
		t.Fatalf("executable = true, want false; Definition of Done holds only a comment. gaps = %v", out.Gaps)
	}
	if !findingsContain(out.Gaps, "Definition of Done") {
		t.Fatalf("gaps = %v, want a Definition of Done gap for a comment-only section", out.Gaps)
	}
}

// Decision pinned: a surviving alphanumeric label (e.g. **U1**) after
// bracket-discounting is authored content. The V2 clause left "structure with
// only placeholder content" to implementation; the deterministic rule strips
// placeholder spans and comments, never bare labels, so a bullet like
// `- **U1 — [Unit name].** [What it delivers.]` reads as authored via its label.
func TestChangeCheckStructuralLabelCountsAsAuthored(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nReal shaping prose.",
		"## Implementation Units\n\n- **U1 — [Unit name].** [What it delivers.]",
		"## Verification Contract\n\n- V1. command exits non-zero.",
		"## Definition of Done\n\n- Gates pass.",
	)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if findingsContain(out.Gaps, "Implementation Units") {
		t.Fatalf("gaps = %v, Implementation Units should read authored: the **U1** label survives discounting", out.Gaps)
	}
	if !out.Executable {
		t.Fatalf("executable = false, want true; every tail section is authored. gaps = %v", out.Gaps)
	}
}

// --- V3: --require-executable turns the report into a gate -------------------

func TestChangeCheckV3RequireExecutableFailsOnShapingDoc(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder, "--require-executable")
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("err = %v, want non-zero ExitError with --require-executable on a shaping doc", err)
	}
	if out.Passed {
		t.Fatalf("passed = true, want false under --require-executable when not executable")
	}
}

func TestChangeCheckV3RequireExecutablePassesOnFullDoc(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(), executableSections()...)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder, "--require-executable")
	if err != nil {
		t.Fatalf("err = %v, want nil for an executable document under --require-executable", err)
	}
	if !out.Passed || !out.Executable {
		t.Fatalf("passed=%v executable=%v, want both true", out.Passed, out.Executable)
	}
}

// --- Branch mismatch is a warning, never a violation ------------------------

func TestChangeCheckBranchMismatchIsWarningNotViolation(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "some-other-branch"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil (branch mismatch is a warning)", err)
	}
	if !out.Passed {
		t.Fatalf("passed = false, want true; branch mismatch must not be a violation")
	}
	if len(out.Warnings) == 0 {
		t.Fatalf("warnings = %v, want a branch-mismatch warning", out.Warnings)
	}
	if !findingsContain(out.Warnings, "branch") {
		t.Fatalf("warnings = %v, want branch-mismatch mention", out.Warnings)
	}
}

// --- Folder resolution by branch --------------------------------------------

func TestChangeCheckResolvesByBranch(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "main"), productSections()...)
	writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo) // no positional path
	if err != nil {
		t.Fatalf("err = %v, want nil resolving by branch", err)
	}
	if !strings.Contains(out.Folder, "20260704-demo") {
		t.Fatalf("folder = %q, want the branch-matched folder", out.Folder)
	}
}

func TestChangeCheckByBranchErrorsWhenNoMatch(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "not-main"), productSections()...)
	writeChangeFolder(t, repo, "20260704-demo", body)

	var stdout, stderr bytes.Buffer
	err := Runner{Stdout: &stdout, Stderr: &stderr, WorkingDir: repo}.Run([]string{"change", "check"})
	if err == nil {
		t.Fatalf("err = nil, want an error telling the user to pass a path")
	}
}

func TestChangeCheckByBranchErrorsWhenManyMatch(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "main"), productSections()...)
	writeChangeFolder(t, repo, "20260704-demo", body)
	body2 := changeDoc(changeFrontmatter("other", "2026-07-04", "main"), productSections()...)
	writeChangeFolder(t, repo, "20260704-other", body2)

	err := Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "check"})
	if err == nil {
		t.Fatalf("err = nil, want an error when multiple folders match the branch")
	}
}

// The no-match error lists every discovered Change folder with its branch: value
// so the user can pick a path — a bare check on a branch with no matching Change
// (e.g. main right after init) is a dead end otherwise.
func TestChangeCheckByBranchNoMatchListsAvailableFolders(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	writeChangeFolder(t, repo, "20260704-demo",
		changeDoc(changeFrontmatter("demo", "2026-07-04", "feat-one"), productSections()...))
	writeChangeFolder(t, repo, "20260705-other",
		changeDoc(changeFrontmatter("other", "2026-07-05", "feat-two"), productSections()...))

	err := Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "check"})
	if err == nil {
		t.Fatalf("err = nil, want a no-match error listing available folders")
	}
	msg := err.Error()
	for _, want := range []string{
		"no change folder matches branch",
		"available change folders:",
		"20260704-demo", "branch: feat-one",
		"20260705-other", "branch: feat-two",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error = %q, want it to contain %q", msg, want)
		}
	}
}

// The ambiguous-match error lists the candidate folders too.
func TestChangeCheckByBranchManyMatchListsAvailableFolders(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	writeChangeFolder(t, repo, "20260704-demo",
		changeDoc(changeFrontmatter("demo", "2026-07-04", "main"), productSections()...))
	writeChangeFolder(t, repo, "20260704-other",
		changeDoc(changeFrontmatter("other", "2026-07-04", "main"), productSections()...))

	err := Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "check"})
	if err == nil {
		t.Fatalf("err = nil, want an ambiguous-match error listing available folders")
	}
	msg := err.Error()
	for _, want := range []string{
		"multiple change folders match branch",
		"available change folders:",
		"20260704-demo", "20260704-other",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error = %q, want it to contain %q", msg, want)
		}
	}
}

// --- init: happy path, refuse existing, bad slug ---------------------------

func TestChangeInitHappyPath(t *testing.T) {
	repo := initCLIGitRepo(t)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "init", "auth-token-rotation"}); err != nil {
		t.Fatalf("change init error = %v", err)
	}

	today := time.Now().Format("20060102")
	folder := filepath.Join(repo, "docs", "changes", today+"-auth-token-rotation")
	data, err := os.ReadFile(filepath.Join(folder, "change.md"))
	if err != nil {
		t.Fatalf("ReadFile(change.md) error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"change: auth-token-rotation",
		"created: " + time.Now().Format("2006-01-02"),
		"branch: auth-token-rotation",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("change.md = %q, want stamped %q", content, want)
		}
	}
	// Body placeholders remain for the human/skill to fill.
	if !strings.Contains(content, "[Change Title]") {
		t.Fatalf("change.md dropped body placeholders:\n%s", content)
	}
	// The freshly-stamped document passes the structural check (all sections
	// present in the template).
	if _, err := runChangeCheckJSON(t, repo, folder); err != nil {
		t.Fatalf("check on freshly-init'd change err = %v, want nil", err)
	}
}

// init's success output carries a next-steps hint: work happens on branch
// <slug>, so create/switch to it or pass the folder path to check explicitly.
// Without it, `loaf change init` on main followed by a bare `loaf change check`
// dead-ends on "no change folder matches branch main".
func TestChangeInitEmitsNextStepsHint(t *testing.T) {
	repo := initCLIGitRepo(t)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "init", "auth-token-rotation"}); err != nil {
		t.Fatalf("change init error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`branch "auth-token-rotation"`,
		"git switch -c auth-token-rotation",
		"loaf change check",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("init output = %q, want next-steps hint containing %q", output, want)
		}
	}
}

func TestChangeInitRefusesExistingFolder(t *testing.T) {
	repo := initCLIGitRepo(t)
	today := time.Now().Format("20060102")
	existing := filepath.Join(repo, "docs", "changes", today+"-demo")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "init", "demo"})
	if err == nil {
		t.Fatalf("err = nil, want refusal for an existing change folder")
	}
	if !strings.Contains(err.Error(), "exists") {
		t.Fatalf("err = %v, want an 'exists' message", err)
	}
}

func TestChangeInitRejectsBadSlug(t *testing.T) {
	for _, slug := range []string{"Bad", "under_score", "with space", "trailing-", "-leading", "double--hyphen", ""} {
		t.Run(slug, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			args := []string{"change", "init"}
			if slug != "" {
				args = append(args, slug)
			}
			err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run(args)
			if err == nil {
				t.Fatalf("err = nil, want rejection for slug %q", slug)
			}
		})
	}
}

// --- Embedded-template drift gate ------------------------------------------

func TestChangeTemplateMatchesCanonicalContent(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "content", "skills", "shape", "templates", "change.md"))
	if err != nil {
		t.Fatalf("ReadFile(canonical change template) error = %v", err)
	}
	if changeTemplate != string(canonical) {
		t.Fatalf("embedded change_template.md drifted from content/skills/shape/templates/change.md; re-copy the canonical file")
	}
}

// --- Pilot smoke: the change model's own pilot must pass --------------------

func TestChangeCheckPilotPasses(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	pilot := filepath.Join("docs", "changes", "20260704-shape-first-change-workflow")
	if _, err := os.Stat(filepath.Join(repoRoot, pilot, "change.md")); err != nil {
		t.Skipf("pilot change not present: %v", err)
	}

	out, err := runChangeCheckJSON(t, repoRoot, pilot)
	if err != nil {
		t.Fatalf("pilot check err = %v, want nil (no violations)", err)
	}
	if !out.Passed {
		t.Fatalf("pilot passed = false, findings = %v", out.Findings)
	}
	if !out.Executable {
		t.Fatalf("pilot executable = false, gaps = %v", out.Gaps)
	}
}

func findingsContain(findings []string, substr string) bool {
	for _, finding := range findings {
		if strings.Contains(finding, substr) {
			return true
		}
	}
	return false
}

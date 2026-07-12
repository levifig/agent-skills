package cli

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// changeTemplate is the canonical Change artifact template, embedded so
// `loaf change init` never depends on installed content. It must stay
// byte-identical to content/skills/shape/templates/change.md; the drift is
// gated by TestChangeTemplateMatchesCanonicalContent.
//
//go:embed change_template.md
var changeTemplate string

// changeSlugRE bounds a Change slug: lowercase letters and digits in
// hyphen-separated groups. No leading/trailing/doubled hyphens.
var changeSlugRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// changeFolderRE bounds a Change folder name: YYYYMMDD-slug.
var changeFolderRE = regexp.MustCompile(`^(\d{8})-([a-z0-9]+(?:-[a-z0-9]+)*)$`)

// changeHTMLCommentRE matches an HTML comment, including multi-line blocks.
var changeHTMLCommentRE = regexp.MustCompile(`(?s)<!--.*?-->`)

// changeBracketPlaceholderRE matches a bracket placeholder span (`[...]`). The
// class excludes brackets but not newlines, so a placeholder wrapping several
// lines is matched as one span.
var changeBracketPlaceholderRE = regexp.MustCompile(`\[[^\[\]]*\]`)

// changeProductSections are the required Product Contract H2s (V1e).
var changeProductSections = []string{
	"Problem",
	"Hypothesis",
	"Scope",
	"Observable Workflow",
	"Rabbit Holes and No-Gos",
}

// changeExecutableSections drive derived structural executability (V2).
var changeExecutableSections = []string{
	"Planning Contract",
	"Implementation Units",
	"Verification Contract",
	"Definition of Done",
}

// changeStatusKeys are the banned status-like frontmatter keys (V1a): readiness
// is derived from PR state and document structure, never declared.
var changeStatusKeys = map[string]bool{
	"readiness": true,
	"status":    true,
	"state":     true,
}

// changeBannedStateValues are banned as frontmatter values under any key (V1a):
// the full canonical change-state vocabulary (Decision 22) plus released and the
// legacy progress words. Change state is derived (loaf change state), never
// stored, so none of these words may live in a stored frontmatter value.
// Matching is on the normalized value (see normalizeChangeStateValue).
var changeBannedStateValues = map[string]bool{
	// Canonical change-state vocabulary (Decision 22).
	"backlog":     true,
	"shaping":     true,
	"todo":        true,
	"in-progress": true,
	"review":      true,
	"merged":      true,
	// released is a project-level event, never a change state, but is equally
	// banned as a stored value (Decision 22, Verification Contract V1).
	"released": true,
	// Legacy progress words, kept as regression insurance.
	"active":   true,
	"done":     true,
	"archived": true,
}

// changeIdentityKeys are the frontmatter fields whose values carry identity, not
// state, and are therefore exempt from the state-vocabulary ban. change: and
// created: are already checked against the folder name; branch: names a git
// branch that may legitimately equal a state word (a branch named "review").
// The status-key ban (readiness/status/state) still applies to every key.
var changeIdentityKeys = map[string]bool{
	"change":  true,
	"created": true,
	"branch":  true,
}

// changeStateSeparatorRE collapses underscores and whitespace runs to a single
// hyphen so "In Progress" and "in_progress" both normalize to "in-progress".
var changeStateSeparatorRE = regexp.MustCompile(`[_\s]+`)

type changeCheckOptions struct {
	path              string
	requireExecutable bool
	jsonOutput        bool
}

type changeCheckJSON struct {
	Command    string   `json:"command"`
	Folder     string   `json:"folder"`
	Passed     bool     `json:"passed"`
	Executable bool     `json:"executable"`
	ExitCode   int      `json:"exitCode"`
	Findings   []string `json:"findings"`
	Warnings   []string `json:"warnings"`
	Gaps       []string `json:"gaps"`
}

type changeFrontmatterField struct {
	Key   string
	Value string
}

type changeFrontmatterParse struct {
	Fields    []changeFrontmatterField
	AtByteOne bool
	Findings  []string
}

type changeCheckReport struct {
	Violations []string
	Warnings   []string
	Gaps       []string
	Executable bool
}

// changeNode is the git-canonical portion of a materialized Change. It is
// deliberately derived from retained files; no lineage state is persisted.
type changeNode struct {
	Slug         string `json:"slug"`
	Branch       string `json:"branch,omitempty"`
	Lineage      string `json:"lineage"`
	Predecessor  string `json:"predecessor,omitempty"`
	ReleaseAfter string `json:"releaseAfter,omitempty"`
	Folder       string `json:"folder"`
	ChangeFile   string `json:"-"`
	Content      string `json:"-"`
}

type changeListOptions struct {
	lineage    string
	jsonOutput bool
}

type changeListJSON struct {
	Command          string       `json:"command"`
	Lineage          string       `json:"lineage"`
	Nodes            []changeNode `json:"nodes"`
	Root             string       `json:"root,omitempty"`
	ReleaseAfter     string       `json:"releaseAfter,omitempty"`
	Findings         []string     `json:"findings"`
	Warnings         []string     `json:"warnings"`
	Gaps             []string     `json:"gaps"`
	JournalAvailable bool         `json:"journalAvailable"`
	LineageDecision  string       `json:"lineageDecision,omitempty"`
}

const (
	changeListProjectResolutionWarning = "journal-enrichment-project-resolution-failed: run change list from a resolvable project root"
	changeListJournalReadWarning       = "journal-enrichment-read-failed: inspect native state with `loaf state status`"
)

func (r Runner) runChange(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeChangeHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"init":  writeChangeInitHelp,
		"check": writeChangeCheckHelp,
		"list":  writeChangeListHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "init":
		return r.runChangeInit(args[1:], out, runtime.RootPath())
	case "check":
		return r.runChangeCheck(args[1:], out, runtime.RootPath())
	case "list":
		return r.runChangeList(args[1:], out, runtime)
	default:
		return unknownSubcommandError("change", args[0])
	}
}

func writeChangeHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf change <subcommand> [options]",
		"Shape-first Change artifacts: git-canonical work context under docs/changes/.",
		[]subcommandHelpItem{
			{Name: "init", Summary: "Scaffold a new Change folder from the template"},
			{Name: "check", Summary: "Validate a Change and report derived executability"},
			{Name: "list", Summary: "List a retained Change lineage without relying on branches"},
		})
}

func writeChangeListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf change list --lineage <key> [--json]",
		"List retained Change files for one lineage. Branch names are provenance only, so this remains usable after merge or branch deletion.",
		"--lineage  Required lineage key", "--json     Output the derived graph, gaps, and optional journal enrichment")
}

func writeChangeInitHelp(out io.Writer) {
	writeUsageHelp(out, "loaf change init <slug>",
		"Create docs/changes/<YYYYMMDD>-<slug>/change.md from the Change template. The slug uses lowercase letters, digits, and single hyphens.")
}

func writeChangeCheckHelp(out io.Writer) {
	writeUsageHelp(out, "loaf change check [folder] [--require-executable] [--json]",
		"Validate a Change and report derived structural executability, not implementation completion. Folder resolution: an "+
			"explicit [folder] path always wins; otherwise the current git branch is "+
			"matched against the branch: frontmatter across docs/changes/*/change.md.",
		"[folder]              Change folder (or change.md) path; resolves from the current branch when omitted",
		"--require-executable  Exit non-zero unless the Change is structurally executable (CI gate for non-draft PRs)",
		"--json                Output folder, passed, executable, findings, warnings, and gaps as JSON")
}

func (r Runner) runChangeInit(args []string, out io.Writer, rootPath string) error {
	if isHelpArg(args) {
		writeChangeInitHelp(out)
		return nil
	}
	slug := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown change init option %q", arg)
		}
		if slug != "" {
			return fmt.Errorf("change init accepts a single <slug> argument")
		}
		slug = arg
	}
	if slug == "" {
		return fmt.Errorf("change init requires a <slug> argument")
	}
	if !changeSlugRE.MatchString(slug) {
		return fmt.Errorf("invalid slug %q: use lowercase letters, digits, and single hyphens (e.g. auth-token-rotation)", slug)
	}
	if existing, err := findChangeSlug(rootPath, slug); err != nil {
		return err
	} else if existing != "" {
		return fmt.Errorf("change slug %q already exists in %s", slug, existing)
	}

	now := time.Now()
	folderName := now.Format("20060102") + "-" + slug
	folder := filepath.Join(rootPath, "docs", "changes", folderName)
	if info, err := os.Stat(folder); err == nil {
		_ = info
		return fmt.Errorf("change folder already exists: %s", relFromRoot(rootPath, folder))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat change folder: %w", err)
	}

	if err := os.MkdirAll(folder, 0o755); err != nil {
		return fmt.Errorf("create change folder: %w", err)
	}
	target := filepath.Join(folder, "change.md")
	if err := os.WriteFile(target, []byte(stampChangeTemplate(changeTemplate, slug, now)), 0o644); err != nil {
		return fmt.Errorf("write change.md: %w", err)
	}
	folderRel := relFromRoot(rootPath, folder)
	fmt.Fprintf(out, "Created change: %s\n", relFromRoot(rootPath, target))
	fmt.Fprintf(out, "\nNext: work on this change happens on branch %q.\n", slug)
	fmt.Fprintf(out, "  Create or switch to it:   git switch -c %s\n", slug)
	fmt.Fprintf(out, "  Then validate the change:  loaf change check\n")
	fmt.Fprintf(out, "  Or check it from any branch by passing the folder: loaf change check %s\n", folderRel)
	return nil
}

// stampChangeTemplate fills the frontmatter bracket placeholders only; body
// placeholders stay for the human or shape skill to complete.
func stampChangeTemplate(template string, slug string, now time.Time) string {
	return strings.NewReplacer(
		"change: [slug]", "change: "+slug,
		"created: [YYYY-MM-DD]", "created: "+now.Format("2006-01-02"),
		"branch: [slug]", "branch: "+slug,
	).Replace(template)
}

func (r Runner) runChangeCheck(args []string, out io.Writer, rootPath string) error {
	if isHelpArg(args) {
		writeChangeCheckHelp(out)
		return nil
	}
	options, err := parseChangeCheckArgs(args)
	if err != nil {
		return err
	}

	folder, changeFile, err := resolveChangeFolder(rootPath, options.path)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(changeFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", relFromRoot(rootPath, changeFile), err)
	}

	changePath := filepath.ToSlash(relFromRoot(rootPath, changeFile))
	report := evaluateChangeDocAtPath(string(content), filepath.Base(folder), currentChangeBranch(rootPath), changePath)
	nodes, indexErr := loadChangeNodes(rootPath)
	if indexErr != nil {
		return indexErr
	}
	report = applyLineageValidation(report, nodes, changePath, rootPath, options.requireExecutable)

	requireFail := options.requireExecutable && !report.Executable
	findings := append([]string{}, report.Violations...)
	if requireFail {
		findings = append(findings, "not structurally executable (--require-executable; implementation completion is not implied): missing "+strings.Join(report.Gaps, ", "))
	}
	exitCode := 0
	switch {
	case len(report.Violations) > 0:
		exitCode = 2
	case requireFail:
		exitCode = 1
	}
	passed := exitCode == 0

	result := changeCheckJSON{
		Command:    "change check",
		Folder:     relFromRoot(rootPath, folder),
		Passed:     passed,
		Executable: report.Executable,
		ExitCode:   exitCode,
		Findings:   findings,
		Warnings:   report.Warnings,
		Gaps:       report.Gaps,
	}

	if options.jsonOutput {
		if err := writeJSON(out, result); err != nil {
			return err
		}
	} else {
		writeChangeCheckText(out, result)
	}
	if exitCode != 0 {
		return ExitError{Code: exitCode}
	}
	return nil
}

func (r Runner) runChangeList(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseChangeListArgs(args)
	if err != nil {
		return err
	}
	nodes, err := loadChangeNodes(runtime.RootPath())
	if err != nil {
		return err
	}
	graph := deriveChangeGraph(nodes)
	result := changeListJSON{Command: "change list", Lineage: options.lineage, Nodes: []changeNode{}, Findings: graph.findingsForLineage(options.lineage), Warnings: []string{}, Gaps: graph.gapsForLineage(options.lineage)}
	for _, node := range nodes {
		if node.Lineage == options.lineage {
			result.Nodes = append(result.Nodes, node)
		}
	}
	if len(result.Nodes) == 0 {
		return fmt.Errorf("no retained Changes found for lineage %q", options.lineage)
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].Folder < result.Nodes[j].Folder })
	for _, node := range result.Nodes {
		if node.Predecessor == "" {
			if result.Root == "" {
				result.Root = node.Slug
			}
		}
		if node.ReleaseAfter != "" {
			if result.ReleaseAfter == "" {
				result.ReleaseAfter = node.ReleaseAfter
			}
		}
	}
	// Journal intent enriches this derived view when available. State is never a
	// prerequisite: missing/uninitialized state simply leaves it unavailable.
	if root, rootErr := project.ResolveRoot(runtime.RootPath()); rootErr != nil {
		result.Warnings = append(result.Warnings, changeListProjectResolutionWarning)
	} else {
		entry, found, available, recentErr := state.LatestJournalEntryForScope(context.Background(), root, state.PathResolver{StateHome: r.StateHome}, "decision", "lineage/"+options.lineage)
		if recentErr != nil {
			result.Warnings = append(result.Warnings, changeListJournalReadWarning)
		} else {
			result.JournalAvailable = available
			if found {
				result.LineageDecision = entry.Message
			}
		}
	}
	result.Warnings = sortedUnique(result.Warnings)
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "\n%s %s\n", ansiBold("change lineage"), result.Lineage)
	for _, node := range result.Nodes {
		fmt.Fprintf(out, "  %s  %s\n", node.Slug, filepath.ToSlash(node.Folder))
		if node.Predecessor != "" {
			fmt.Fprintf(out, "    predecessor: %s\n", node.Predecessor)
		}
	}
	if result.Root != "" {
		fmt.Fprintf(out, "root: %s\n", result.Root)
	}
	if result.ReleaseAfter != "" {
		fmt.Fprintf(out, "release after: %s\n", result.ReleaseAfter)
	}
	for _, gap := range result.Gaps {
		fmt.Fprintf(out, "gap: %s\n", gap)
	}
	for _, finding := range result.Findings {
		fmt.Fprintf(out, "finding: %s\n", finding)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "warning: %s\n", warning)
	}
	if result.JournalAvailable && result.LineageDecision != "" {
		fmt.Fprintf(out, "latest lineage decision: %s\n", result.LineageDecision)
	} else if !result.JournalAvailable {
		fmt.Fprintln(out, "lineage decision: unavailable (native state is not required)")
	}
	return nil
}

func parseChangeListArgs(args []string) (changeListOptions, error) {
	var options changeListOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--lineage":
			if i+1 >= len(args) {
				return options, fmt.Errorf("--lineage requires a value")
			}
			i++
			options.lineage = args[i]
		default:
			if strings.HasPrefix(args[i], "--lineage=") {
				options.lineage = strings.TrimPrefix(args[i], "--lineage=")
			} else {
				return options, fmt.Errorf("unknown change list option %q", args[i])
			}
		}
	}
	if options.lineage == "" {
		return options, fmt.Errorf("change list requires --lineage <key>")
	}
	return options, nil
}

func parseChangeCheckArgs(args []string) (changeCheckOptions, error) {
	var options changeCheckOptions
	for _, arg := range args {
		switch arg {
		case "--require-executable":
			options.requireExecutable = true
		case "--json":
			options.jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return changeCheckOptions{}, fmt.Errorf("unknown change check option %q", arg)
			}
			if options.path != "" {
				return changeCheckOptions{}, fmt.Errorf("change check accepts a single [folder] argument")
			}
			options.path = arg
		}
	}
	return options, nil
}

func findChangeSlug(rootPath, slug string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(rootPath, "docs", "changes", "*", "change.md"))
	if err != nil {
		return "", err
	}
	for _, changeFile := range matches {
		match := changeFolderRE.FindStringSubmatch(filepath.Base(filepath.Dir(changeFile)))
		if match != nil && match[2] == slug {
			return relFromRoot(rootPath, filepath.Dir(changeFile)), nil
		}
	}
	return "", nil
}

func loadChangeNodes(rootPath string) ([]changeNode, error) {
	matches, err := filepath.Glob(filepath.Join(rootPath, "docs", "changes", "*", "change.md"))
	if err != nil {
		return nil, err
	}
	nodes := make([]changeNode, 0, len(matches))
	for _, changeFile := range matches {
		content, err := os.ReadFile(changeFile)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", relFromRoot(rootPath, changeFile), err)
		}
		fields, _ := changeFrontmatterFields(string(content))
		folder := filepath.Dir(changeFile)
		nodes = append(nodes, changeNode{Slug: changeFieldValue(fields, "change"), Branch: changeFieldValue(fields, "branch"), Lineage: changeFieldValue(fields, "lineage"), Predecessor: changeFieldValue(fields, "predecessor"), ReleaseAfter: changeFieldValue(fields, "release-after"), Folder: relFromRoot(rootPath, folder), ChangeFile: relFromRoot(rootPath, changeFile), Content: string(content)})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ChangeFile < nodes[j].ChangeFile })
	return nodes, nil
}

// resolveChangeFolder returns the Change folder and its change.md path. An
// explicit path wins; otherwise the folder is resolved by matching the current
// git branch against branch: frontmatter across docs/changes/*/change.md.
func resolveChangeFolder(rootPath string, path string) (string, string, error) {
	if path != "" {
		abs := path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(rootPath, path)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return "", "", fmt.Errorf("change path not found: %s", path)
		}
		folder := abs
		changeFile := filepath.Join(abs, "change.md")
		if !info.IsDir() {
			changeFile = abs
			folder = filepath.Dir(abs)
		}
		if _, err := os.Stat(changeFile); err != nil {
			return "", "", fmt.Errorf("no change.md in %s", relFromRoot(rootPath, folder))
		}
		return folder, changeFile, nil
	}
	return resolveChangeFolderByBranch(rootPath)
}

func resolveChangeFolderByBranch(rootPath string) (string, string, error) {
	branch := currentChangeBranch(rootPath)
	if branch == "" {
		return "", "", fmt.Errorf("could not determine the current git branch; pass a change folder path")
	}
	matches, err := filepath.Glob(filepath.Join(rootPath, "docs", "changes", "*", "change.md"))
	if err != nil {
		return "", "", err
	}
	var folders []string
	var available []changeBranchEntry
	for _, changeFile := range matches {
		content, err := os.ReadFile(changeFile)
		if err != nil {
			continue
		}
		fields, atByteOne := changeFrontmatterFields(string(content))
		if !atByteOne {
			continue
		}
		fmBranch := changeFieldValue(fields, "branch")
		available = append(available, changeBranchEntry{
			folder: relFromRoot(rootPath, filepath.Dir(changeFile)),
			branch: fmBranch,
		})
		if fmBranch == branch {
			folders = append(folders, filepath.Dir(changeFile))
		}
	}
	switch len(folders) {
	case 1:
		return folders[0], filepath.Join(folders[0], "change.md"), nil
	case 0:
		return "", "", fmt.Errorf("no change folder matches branch %q; pass a change folder path.%s", branch, formatAvailableChanges(available))
	default:
		return "", "", fmt.Errorf("multiple change folders match branch %q; pass a change folder path.%s", branch, formatAvailableChanges(available))
	}
}

// changeBranchEntry pairs a Change folder with the branch declared in its
// frontmatter, for listing candidates when branch resolution is unambiguous.
type changeBranchEntry struct {
	folder string
	branch string
}

// formatAvailableChanges renders the discovered Change folders and their branch:
// values so a failed branch resolution tells the user exactly what they can pass.
func formatAvailableChanges(entries []changeBranchEntry) string {
	if len(entries) == 0 {
		return " (no change folders found under docs/changes/)"
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].folder < entries[j].folder })
	var b strings.Builder
	b.WriteString("\navailable change folders:")
	for _, entry := range entries {
		branch := entry.branch
		if branch == "" {
			branch = "(no branch: field)"
		}
		fmt.Fprintf(&b, "\n  %s  branch: %s", entry.folder, branch)
	}
	return b.String()
}

// evaluateChangeDoc runs the Verification Contract against one change.md.
func evaluateChangeDoc(content string, folderBase string, currentBranch string) changeCheckReport {
	return evaluateChangeDocAtPath(content, folderBase, currentBranch, "")
}

func evaluateChangeDocAtPath(content string, folderBase string, currentBranch string, changePath string) changeCheckReport {
	report := changeCheckReport{
		Violations: []string{},
		Warnings:   []string{},
		Gaps:       []string{},
	}

	parsed := parseChangeFrontmatter(content)
	fields, atByteOne := parsed.Fields, parsed.AtByteOne
	if !atByteOne {
		report.Violations = append(report.Violations, prefixChangeFinding(changePath, "frontmatter must open the file at byte one"))
	}
	for _, finding := range parsed.Findings {
		report.Violations = append(report.Violations, prefixChangeFinding(changePath, finding))
	}
	for _, key := range []string{"change", "created", "lineage", "predecessor", "release-after"} {
		if countChangeFields(fields, key) > 1 {
			report.Violations = append(report.Violations, prefixChangeFinding(changePath, fmt.Sprintf("duplicate frontmatter field %q", key)))
		}
	}

	// V1a: status-like keys and the canonical change-state vocabulary as values.
	for _, field := range fields {
		if changeStatusKeys[strings.ToLower(field.Key)] {
			report.Violations = append(report.Violations,
				fmt.Sprintf("status-like frontmatter key %q is banned; readiness is derived", field.Key))
			continue
		}
		if changeIdentityKeys[strings.ToLower(field.Key)] {
			continue
		}
		if changeBannedStateValues[normalizeChangeStateValue(field.Value)] {
			report.Violations = append(report.Violations,
				fmt.Sprintf("change-state vocabulary %q in frontmatter field %q is banned; state is derived", field.Value, field.Key))
		}
	}

	// V1c + V1d: folder-name shape and identity.
	folderMatch := changeFolderRE.FindStringSubmatch(folderBase)
	if folderMatch == nil {
		report.Violations = append(report.Violations,
			fmt.Sprintf("malformed change folder name %q (want YYYYMMDD-slug)", folderBase))
	} else if atByteOne {
		folderDate, folderSlug := folderMatch[1], folderMatch[2]
		if change := changeFieldValue(fields, "change"); change != folderSlug {
			report.Violations = append(report.Violations,
				fmt.Sprintf("identity mismatch: change: %q does not match folder slug %q", change, folderSlug))
		}
		created := changeFieldValue(fields, "created")
		if strings.ReplaceAll(created, "-", "") != folderDate {
			report.Violations = append(report.Violations,
				fmt.Sprintf("identity mismatch: created: %q does not match folder date %q", created, folderDate))
		}
	}

	// V1e: required Product Contract sections present.
	sections := changeSections(content)
	for _, name := range changeProductSections {
		if _, ok := sections[name]; !ok {
			report.Violations = append(report.Violations,
				fmt.Sprintf("missing Product Contract section: %s", name))
		}
	}

	// V2: derived executability — required tail sections present and non-empty.
	// Non-empty means authored content: bracket placeholders and comments are
	// scaffolding, not content, so a freshly-templated Change is not executable.
	for _, name := range changeExecutableSections {
		body, ok := sections[name]
		if !ok {
			report.Gaps = append(report.Gaps, fmt.Sprintf("%s (missing)", name))
			continue
		}
		if !changeSectionAuthored(body) {
			report.Gaps = append(report.Gaps, fmt.Sprintf("%s (empty)", name))
		}
	}
	report.Executable = len(report.Gaps) == 0

	// Branch mismatch is a warning, never a violation.
	if atByteOne && currentBranch != "" {
		if branch := changeFieldValue(fields, "branch"); branch != "" && branch != currentBranch {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("current branch %q does not match change branch %q", currentBranch, branch))
		}
	}

	return report
}

func prefixChangeFinding(changePath, finding string) string {
	if changePath == "" {
		return finding
	}
	return filepath.ToSlash(changePath) + ": " + finding
}

// changeFrontmatterFields parses the leading YAML frontmatter into ordered
// key/value fields. The second return reports whether frontmatter opens the
// file at byte one — parsers depend on it, so this is checkable on its own.
func changeFrontmatterFields(content string) ([]changeFrontmatterField, bool) {
	parsed := parseChangeFrontmatter(content)
	return parsed.Fields, parsed.AtByteOne
}

func parseChangeFrontmatter(content string) changeFrontmatterParse {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return changeFrontmatterParse{Fields: []changeFrontmatterField{}, Findings: []string{}}
	}
	result := changeFrontmatterParse{Fields: []changeFrontmatterField{}, AtByteOne: true, Findings: []string{}}
	lines := strings.Split(normalized, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		result.Findings = append(result.Findings, "frontmatter is not closed with ---")
		return result
	}
	for index, line := range lines[1:end] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			result.Findings = append(result.Findings, fmt.Sprintf("malformed frontmatter line %d: expected key: value", index+2))
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			result.Findings = append(result.Findings, fmt.Sprintf("malformed frontmatter line %d: key cannot be empty", index+2))
			continue
		}
		result.Fields = append(result.Fields, changeFrontmatterField{
			Key:   key,
			Value: cleanChangeScalar(strings.TrimSpace(value)),
		})
	}
	return result
}

// normalizeChangeStateValue lowercases, trims, and collapses underscore/space
// runs to hyphens so state words are matched regardless of casing or separator
// style ("In Progress", "in_progress", "in-progress" all match "in-progress").
func normalizeChangeStateValue(value string) string {
	return changeStateSeparatorRE.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-")
}

func changeFieldValue(fields []changeFrontmatterField, key string) string {
	for _, field := range fields {
		if strings.EqualFold(field.Key, key) {
			return field.Value
		}
	}
	return ""
}

func cleanChangeScalar(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// changeSections maps each H2 heading to its trimmed body text (H3 subsections
// included), so section presence and non-emptiness are both derivable.
func changeSections(content string) map[string]string {
	sections := map[string]string{}
	current := ""
	var body []string
	flush := func() {
		if current != "" {
			sections[current] = strings.TrimSpace(strings.Join(body, "\n"))
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			flush()
			current = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			body = nil
		case strings.HasPrefix(line, "# "):
			flush()
			current = ""
			body = nil
		default:
			if current != "" {
				body = append(body, line)
			}
		}
	}
	flush()
	return sections
}

// changeSectionAuthored reports whether a section body carries authored content
// once scaffolding is discounted (V2). HTML comments and bracket placeholder
// spans (`[...]`, including multi-line spans) are removed; if any letter or
// digit survives, the section is authored. Bare structural labels (e.g. a **U1**
// bullet left unfilled) survive discounting and therefore count as authored —
// the rule strips placeholders and comments, never labels.
func changeSectionAuthored(body string) bool {
	stripped := changeHTMLCommentRE.ReplaceAllString(body, "")
	stripped = changeBracketPlaceholderRE.ReplaceAllString(stripped, "")
	for _, r := range stripped {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func currentChangeBranch(root string) string {
	output, err := commandOutput(root, "git", "branch", "--show-current")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

func relFromRoot(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func writeChangeCheckText(out io.Writer, result changeCheckJSON) {
	fmt.Fprintf(out, "\n%s %s\n", ansiBold("change check"), result.Folder)
	if len(result.Findings) > 0 {
		fmt.Fprintf(out, "\n%s %d violation(s)\n", ansiRed("x"), len(result.Findings))
		for _, finding := range result.Findings {
			fmt.Fprintf(out, "   %s %s\n", ansiRed("-"), finding)
		}
	} else {
		fmt.Fprintf(out, "%s no violations\n", ansiGreen("ok"))
	}
	if result.Executable {
		fmt.Fprintf(out, "executable: %s\n", ansiGreen("yes"))
	} else {
		fmt.Fprintf(out, "executable: %s\n", ansiYellow("no"))
		for _, gap := range result.Gaps {
			fmt.Fprintf(out, "   %s %s\n", ansiGray("gap:"), gap)
		}
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "   %s %s\n", ansiYellow("warn:"), warning)
	}
}

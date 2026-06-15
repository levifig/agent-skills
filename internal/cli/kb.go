package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type kbConfig struct {
	Local                  []string
	StalenessThresholdDays int
}

type knowledgeFile struct {
	Path                 string
	RelativePath         string
	Topics               frontmatterField
	LastReviewed         frontmatterField
	Covers               frontmatterField
	DependsOn            frontmatterField
	ImplementationStatus frontmatterField
	HasFrontmatter       bool
}

type frontmatterField struct {
	Values []string
	Array  bool
	Set    bool
}

type kbValidationResult struct {
	File     string              `json:"file"`
	Errors   []kbValidationIssue `json:"errors"`
	Warnings []kbValidationIssue `json:"warnings"`
}

type kbValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type kbStalenessResult struct {
	File             string `json:"file"`
	IsStale          bool   `json:"isStale"`
	HasCoverage      bool   `json:"hasCoverage,omitempty"`
	CommitCount      int    `json:"commitCount"`
	LastCommitAuthor string `json:"lastCommitAuthor,omitempty"`
	LastCommitDate   string `json:"lastCommitDate,omitempty"`
	LastReviewed     string `json:"lastReviewed"`
}

type kbStatusSummary struct {
	TotalFiles         int            `json:"total_files"`
	FilesWithCovers    int            `json:"files_with_covers"`
	FilesWithoutCovers int            `json:"files_without_covers"`
	Stale              int            `json:"stale"`
	AvgReviewAgeDays   int            `json:"avg_review_age_days"`
	Directories        map[string]int `json:"directories"`
}

type kbStatusOptions struct {
	jsonOutput bool
}

type kbCheckOptions struct {
	file       string
	jsonOutput bool
}

type kbInitOptions struct {
	jsonOutput bool
}

type kbInitAction struct {
	Action string `json:"action"`
	Target string `json:"target"`
	Status string `json:"status"`
}

type kbInitQMDCollection struct {
	Collection string `json:"collection"`
	Path       string `json:"path"`
	Status     string `json:"status"`
}

type kbInitResult struct {
	Directories []kbInitAction `json:"directories"`
	Config      struct {
		Path   string `json:"path"`
		Status string `json:"status"`
	} `json:"config"`
	QMD struct {
		Available   bool                  `json:"available"`
		Collections []kbInitQMDCollection `json:"collections"`
	} `json:"qmd"`
}

type kbImportOptions struct {
	name       string
	path       string
	jsonOutput bool
}

type kbImportResult struct {
	Name       string `json:"name,omitempty"`
	Collection string `json:"collection,omitempty"`
	Status     string `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
}

type kbReviewOptions struct {
	file       string
	jsonOutput bool
}

type kbGlossaryOptions struct {
	subcommand string
	term       string
	definition string
	avoid      string
	mode       string
}

type glossaryTerm struct {
	Name       string
	Definition string
	Avoid      []string
}

type glossaryData struct {
	Canonical          []glossaryTerm
	Candidates         []glossaryTerm
	Relationships      string
	FlaggedAmbiguities string
}

const (
	glossaryRelativePath        = "docs/knowledge/glossary.md"
	glossaryLinearNativeMessage = "Linear-native glossary writes pending artifact-taxonomy spec — local mode only for now."
)

var (
	qmdLookPath           = exec.LookPath
	qmdListCollections    = nativeQMDListCollections
	qmdRegisterCollection = nativeQMDRegisterCollection
)

func (r Runner) runKb(args []string, out io.Writer, runtimeRoot string) error {
	if len(args) == 0 {
		writeKbHelp(out)
		return nil
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		writeKbHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"status":   writeKbStatusHelp,
		"validate": writeKbValidateHelp,
		"check":    writeKbCheckHelp,
		"review":   writeKbReviewHelp,
		"init":     writeKbInitHelp,
		"import":   writeKbImportHelp,
		"glossary": writeKbGlossaryHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "check":
		stderr := r.Stderr
		if stderr == nil {
			stderr = os.Stderr
		}
		return r.runKbCheck(args[1:], out, stderr, runtimeRoot)
	case "glossary":
		return r.runKbGlossary(args[1:], out, runtimeRoot)
	case "init":
		return r.runKbInit(args[1:], out, runtimeRoot)
	case "import":
		return r.runKbImport(args[1:], out, runtimeRoot)
	case "validate":
		stderr := r.Stderr
		if stderr == nil {
			stderr = os.Stderr
		}
		return r.runKbValidate(args[1:], out, stderr, runtimeRoot)
	case "review":
		return r.runKbReview(args[1:], out, runtimeRoot)
	case "status":
		stderr := r.Stderr
		if stderr == nil {
			stderr = os.Stderr
		}
		return r.runKbStatus(args[1:], out, stderr, runtimeRoot)
	default:
		return fmt.Errorf("unknown loaf kb subcommand %q", args[0])
	}
}

func writeKbHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf kb <subcommand> [options]",
		"",
		"Knowledge base management.",
		"",
		"Subcommands:",
		"  status      Show knowledge base overview",
		"  validate    Validate knowledge file frontmatter",
		"  check       Check knowledge file staleness against git history",
		"  review      Mark a knowledge file as reviewed today",
		"  init        Initialize knowledge base directories and QMD collections",
		"  import      Register an external QMD collection as a knowledge source",
		"  glossary    Domain glossary mutation and lookup",
	}, "\n"))
}

func writeKbStatusHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb status [--json]", "Show knowledge base overview.", "--json       Output knowledge file totals, coverage counts, stale count, review age, and directories as JSON")
}

func writeKbValidateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb validate [--json]", "Validate knowledge file frontmatter.", "--json       Output per-file frontmatter errors and warnings as JSON")
}

func writeKbCheckHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb check [--file <path>] [--json]", "Check knowledge file staleness against git history.", "--file       Reverse lookup: find knowledge files covering this path", "--json       Output per-file staleness, coverage, commit, and review metadata as JSON")
}

func writeKbReviewHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb review <file> [--json]", "Mark a knowledge file as reviewed today.", "--json       Output updated knowledge frontmatter as JSON")
}

func writeKbInitHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb init [--json]", "Initialize knowledge base directories and QMD collections.", "--json       Output directory actions, config status, and QMD collections as JSON")
}

func writeKbImportHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb import <name> --path <path> [--json]", "Register an external QMD collection as a knowledge source.", "--path       Path to the external project's knowledge directory", "--json       Output QMD import collection status or import error as JSON")
}

func writeKbGlossaryHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf kb glossary <subcommand> [options]",
		"",
		"Domain glossary mutation and lookup.",
		"",
		"Subcommands:",
		"  upsert     Create or update a canonical term",
		"  propose    Create or update a candidate term",
		"  check      Check one term",
		"  list       List glossary terms",
		"  stabilize  Promote a candidate term to canonical",
		"",
		"Options:",
		"  -h, --help  Show help",
	}, "\n"))
}

func writeKbGlossaryUpsertHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb glossary upsert <term> --definition <text> [--avoid <terms>]", "Create or update a canonical glossary term.", "--definition  Canonical definition", "--avoid       Comma-separated discouraged alternatives")
}

func writeKbGlossaryProposeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb glossary propose <term> --definition <text> [--avoid <terms>]", "Create or update a candidate glossary term.", "--definition  Candidate definition", "--avoid       Comma-separated discouraged alternatives")
}

func writeKbGlossaryCheckHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb glossary check <term>", "Check one glossary term.")
}

func writeKbGlossaryListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb glossary list [--canonical|--candidates|--all]", "List glossary terms.", "--canonical   Show canonical terms", "--candidates  Show candidate terms", "--all         Show canonical and candidate terms")
}

func writeKbGlossaryStabilizeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf kb glossary stabilize <term> --definition <text>", "Promote a candidate glossary term to canonical.", "--definition  Canonical definition")
}

func (r Runner) runKbGlossary(args []string, out io.Writer, runtimeRoot string) error {
	if len(args) == 0 || isHelpArg(args) {
		writeKbGlossaryHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"upsert":    writeKbGlossaryUpsertHelp,
		"propose":   writeKbGlossaryProposeHelp,
		"check":     writeKbGlossaryCheckHelp,
		"list":      writeKbGlossaryListHelp,
		"stabilize": writeKbGlossaryStabilizeHelp,
	}) {
		return nil
	}
	options, err := parseKbGlossaryArgs(args)
	if err != nil {
		return err
	}
	gitRoot, err := resolveGitRootForKB(runtimeRoot)
	if err != nil {
		return err
	}
	switch options.subcommand {
	case "upsert":
		action, err := upsertNativeGlossaryTerm(gitRoot, options.term, options.definition, parseCommaList(options.avoid))
		if err != nil {
			return err
		}
		verb := ansiGreen("created")
		if action == "updated" {
			verb = ansiCyan("updated")
		}
		fmt.Fprintf(out, "  %s canonical: %s\n", verb, ansiBold(options.term))
		return nil
	case "check":
		result, err := checkNativeGlossaryTerm(gitRoot, options.term)
		writeKbGlossaryCheck(out, result)
		return err
	case "list":
		result, err := listNativeGlossaryTerms(gitRoot, options.mode)
		if err != nil {
			return err
		}
		writeKbGlossaryList(out, result)
		return nil
	case "stabilize":
		if err := stabilizeNativeGlossaryTerm(gitRoot, options.term, options.definition); err != nil {
			return err
		}
		fmt.Fprintf(out, "  %s %s\n", ansiGreen("stabilized:"), ansiBold(options.term))
		return nil
	case "propose":
		action, err := proposeNativeGlossaryTerm(gitRoot, options.term, options.definition, parseCommaList(options.avoid))
		if err != nil {
			return err
		}
		verb := ansiGreen("proposed")
		if action == "updated" {
			verb = ansiCyan("updated")
		}
		fmt.Fprintf(out, "  %s candidate: %s\n", verb, ansiBold(options.term))
		return nil
	default:
		return fmt.Errorf("unknown kb glossary subcommand %q", options.subcommand)
	}
}

func (r Runner) runKbInit(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseKbInitArgs(args)
	if err != nil {
		return err
	}
	gitRoot, err := resolveGitRootForKB(runtimeRoot)
	if err != nil {
		return err
	}
	result, err := initializeNativeKB(gitRoot)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeKbInit(out, result)
	return nil
}

func (r Runner) runKbImport(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseKbImportArgs(args)
	if err != nil {
		return err
	}
	if !nativeQMDAvailable() {
		message := "QMD is required for importing external knowledge"
		if options.jsonOutput {
			if err := writeJSON(out, kbImportResult{Error: message}); err != nil {
				return err
			}
			return ExitError{Code: 1}
		}
		return fmt.Errorf("%s", message)
	}
	gitRoot, err := resolveGitRootForKB(runtimeRoot)
	if err != nil {
		return err
	}
	result, err := importNativeKB(gitRoot, options)
	if options.jsonOutput {
		if result.Error != "" {
			if writeErr := writeJSON(out, result); writeErr != nil {
				return writeErr
			}
		} else if result.Name != "" {
			if writeErr := writeJSON(out, result); writeErr != nil {
				return writeErr
			}
		}
	}
	if err != nil {
		if options.jsonOutput && result.Error != "" {
			return ExitError{Code: 1}
		}
		return err
	}
	if !options.jsonOutput {
		if result.Status == "already_imported" {
			fmt.Fprintf(out, "  Already imported: %s\n", ansiBold(options.name))
		} else {
			fmt.Fprintf(out, "  Imported: %s\n", ansiBold(options.name))
		}
	}
	return nil
}

func (r Runner) runKbStatus(args []string, out io.Writer, errOut io.Writer, runtimeRoot string) error {
	options, err := parseKbStatusArgs(args)
	if err != nil {
		return err
	}
	gitRoot, err := resolveGitRootForKB(runtimeRoot)
	if err != nil {
		return err
	}
	config := loadNativeKbConfig(gitRoot)
	files := loadNativeKnowledgeFiles(gitRoot, config, errOut, false)
	summary := summarizeKnowledgeFiles(context.Background(), gitRoot, files, time.Now())
	if options.jsonOutput {
		return writeJSON(out, summary)
	}
	writeKbStatus(out, summary)
	return nil
}

func (r Runner) runKbCheck(args []string, out io.Writer, errOut io.Writer, runtimeRoot string) error {
	options, err := parseKbCheckArgs(args)
	if err != nil {
		return err
	}
	gitRoot, err := resolveGitRootForKB(runtimeRoot)
	if err != nil {
		return err
	}
	config := loadNativeKbConfig(gitRoot)
	files := loadNativeKnowledgeFiles(gitRoot, config, errOut, false)
	if options.file != "" {
		filePath := filepath.ToSlash(options.file)
		if filepath.IsAbs(options.file) {
			rel, err := filepath.Rel(gitRoot, options.file)
			if err == nil {
				filePath = filepath.ToSlash(rel)
			}
		}
		files = filterKnowledgeFilesCovering(files, filePath)
		results := stalenessResults(context.Background(), gitRoot, files)
		if options.jsonOutput {
			return writeJSON(out, results)
		}
		writeKbCheckForFile(out, filePath, results)
		return nil
	}
	results := stalenessResults(context.Background(), gitRoot, files)
	if options.jsonOutput {
		return writeJSON(out, results)
	}
	writeKbCheck(out, results)
	return nil
}

func (r Runner) runKbValidate(args []string, out io.Writer, errOut io.Writer, runtimeRoot string) error {
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		return err
	}
	gitRoot, err := resolveGitRootForKB(runtimeRoot)
	if err != nil {
		return err
	}
	config := loadNativeKbConfig(gitRoot)
	files := loadNativeKnowledgeFiles(gitRoot, config, errOut, true)
	results := validateNativeKnowledgeFiles(gitRoot, files)
	if jsonOutput {
		if err := writeJSON(out, results); err != nil {
			return err
		}
	} else {
		writeKbValidation(out, results)
	}
	if countValidationErrors(results) > 0 {
		if jsonOutput {
			return ExitError{Code: 1}
		}
		return fmt.Errorf("kb validation failed: %d error(s)", countValidationErrors(results))
	}
	return nil
}

func (r Runner) runKbReview(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseKbReviewArgs(args)
	if err != nil {
		return err
	}
	gitRoot, err := resolveGitRootForKB(runtimeRoot)
	if err != nil {
		return err
	}
	absPath, relPath, err := resolveKBFilePath(gitRoot, options.file)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("file not found: %s", relPath)
	}
	frontmatter, ok := parseKnowledgeFrontmatter(body)
	if !ok || !frontmatter["topics"].Array || len(frontmatter["topics"].Values) == 0 {
		message := fmt.Sprintf("Not a knowledge file (missing topics field): %s", relPath)
		if options.jsonOutput {
			if err := writeJSON(out, map[string]string{"error": message}); err != nil {
				return err
			}
			return ExitError{Code: 1}
		}
		return fmt.Errorf("%s", message)
	}

	today := time.Now().Format("2006-01-02")
	updated, err := setFrontmatterScalar(body, "last_reviewed", today)
	if err != nil {
		return err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(absPath, updated, info.Mode().Perm()); err != nil {
		return err
	}
	updatedFrontmatter, ok := parseKnowledgeFrontmatter(updated)
	if !ok {
		return fmt.Errorf("updated frontmatter could not be parsed: %s", relPath)
	}
	if options.jsonOutput {
		return writeJSON(out, frontmatterJSON(updatedFrontmatter))
	}
	writeKbReview(out, relPath, today)
	return nil
}

func parseKbGlossaryArgs(args []string) (kbGlossaryOptions, error) {
	if len(args) == 0 {
		return kbGlossaryOptions{}, fmt.Errorf("kb glossary requires a subcommand")
	}
	options := kbGlossaryOptions{subcommand: args[0], mode: "all"}
	rest := args[1:]
	switch options.subcommand {
	case "upsert", "propose":
		if len(rest) == 0 {
			return kbGlossaryOptions{}, fmt.Errorf("kb glossary %s requires a term", options.subcommand)
		}
		options.term = rest[0]
		for i := 1; i < len(rest); i++ {
			switch rest[i] {
			case "--definition":
				value, err := consumeFlagValue(rest, &i, "--definition")
				if err != nil {
					return kbGlossaryOptions{}, err
				}
				options.definition = value
			case "--avoid":
				value, err := consumeFlagValue(rest, &i, "--avoid")
				if err != nil {
					return kbGlossaryOptions{}, err
				}
				options.avoid = value
			default:
				return kbGlossaryOptions{}, fmt.Errorf("unknown kb glossary %s option %q", options.subcommand, rest[i])
			}
		}
		if options.definition == "" {
			return kbGlossaryOptions{}, fmt.Errorf("kb glossary %s requires --definition", options.subcommand)
		}
	case "check":
		if len(rest) != 1 {
			return kbGlossaryOptions{}, fmt.Errorf("kb glossary check requires exactly one term")
		}
		options.term = rest[0]
	case "list":
		for _, arg := range rest {
			switch arg {
			case "--canonical":
				if options.mode != "all" && options.mode != "canonical" {
					options.mode = "all"
				} else {
					options.mode = "canonical"
				}
			case "--candidates":
				if options.mode != "all" && options.mode != "candidates" {
					options.mode = "all"
				} else {
					options.mode = "candidates"
				}
			case "--all":
				options.mode = "all"
			default:
				return kbGlossaryOptions{}, fmt.Errorf("unknown kb glossary list option %q", arg)
			}
		}
	case "stabilize":
		if len(rest) == 0 {
			return kbGlossaryOptions{}, fmt.Errorf("kb glossary stabilize requires a term")
		}
		options.term = rest[0]
		for i := 1; i < len(rest); i++ {
			switch rest[i] {
			case "--definition":
				value, err := consumeFlagValue(rest, &i, "--definition")
				if err != nil {
					return kbGlossaryOptions{}, err
				}
				options.definition = value
			default:
				return kbGlossaryOptions{}, fmt.Errorf("unknown kb glossary stabilize option %q", rest[i])
			}
		}
	default:
		return kbGlossaryOptions{}, fmt.Errorf("unknown kb glossary subcommand %q", options.subcommand)
	}
	return options, nil
}

func parseKbInitArgs(args []string) (kbInitOptions, error) {
	var options kbInitOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		default:
			return kbInitOptions{}, fmt.Errorf("unknown kb init option %q", arg)
		}
	}
	return options, nil
}

func parseKbImportArgs(args []string) (kbImportOptions, error) {
	var options kbImportOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--path":
			value, err := consumeFlagValue(args, &i, "--path")
			if err != nil {
				return kbImportOptions{}, err
			}
			options.path = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return kbImportOptions{}, fmt.Errorf("unknown kb import option %q", args[i])
			}
			if options.name != "" {
				return kbImportOptions{}, fmt.Errorf("kb import accepts exactly one name")
			}
			options.name = args[i]
		}
	}
	if options.name == "" {
		return kbImportOptions{}, fmt.Errorf("kb import requires a name")
	}
	return options, nil
}

func parseKbStatusArgs(args []string) (kbStatusOptions, error) {
	var options kbStatusOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		default:
			return kbStatusOptions{}, fmt.Errorf("unknown kb status option %q", arg)
		}
	}
	return options, nil
}

func parseKbCheckArgs(args []string) (kbCheckOptions, error) {
	var options kbCheckOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--file":
			value, err := consumeFlagValue(args, &i, "--file")
			if err != nil {
				return kbCheckOptions{}, err
			}
			options.file = value
		default:
			return kbCheckOptions{}, fmt.Errorf("unknown kb check option %q", args[i])
		}
	}
	return options, nil
}

func parseKbReviewArgs(args []string) (kbReviewOptions, error) {
	var options kbReviewOptions
	for _, arg := range args {
		switch {
		case arg == "--json":
			options.jsonOutput = true
		case strings.HasPrefix(arg, "-"):
			return kbReviewOptions{}, fmt.Errorf("unknown kb review option %q", arg)
		case options.file == "":
			options.file = arg
		default:
			return kbReviewOptions{}, fmt.Errorf("kb review accepts exactly one file")
		}
	}
	if options.file == "" {
		return kbReviewOptions{}, fmt.Errorf("kb review requires a file")
	}
	return options, nil
}

func resolveGitRootForKB(start string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = start
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository")
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", fmt.Errorf("not inside a git repository")
	}
	return root, nil
}

func resolveKBFilePath(gitRoot string, filePath string) (string, string, error) {
	absPath := filepath.Clean(filePath)
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(gitRoot, filepath.FromSlash(filePath))
	}
	relPath, err := filepath.Rel(gitRoot, absPath)
	if err != nil {
		return "", "", err
	}
	return absPath, filepath.ToSlash(relPath), nil
}

func initializeNativeKB(gitRoot string) (kbInitResult, error) {
	var result kbInitResult
	for _, dir := range []string{"docs/knowledge", "docs/decisions"} {
		fullPath := filepath.Join(gitRoot, filepath.FromSlash(dir))
		status := "exists"
		if _, err := os.Stat(fullPath); err != nil {
			if !os.IsNotExist(err) {
				return kbInitResult{}, err
			}
			if err := os.MkdirAll(fullPath, 0o755); err != nil {
				return kbInitResult{}, err
			}
			status = "created"
		}
		result.Directories = append(result.Directories, kbInitAction{Action: "directory", Target: dir, Status: status})
	}

	configStatus, err := initializeNativeKBConfig(gitRoot)
	if err != nil {
		return kbInitResult{}, err
	}
	result.Config.Path = ".agents/loaf.json"
	result.Config.Status = configStatus

	result.QMD.Available = nativeQMDAvailable()
	result.QMD.Collections = []kbInitQMDCollection{}
	if result.QMD.Available {
		repoName := filepath.Base(gitRoot)
		existing := stringSet(qmdListCollections())
		collections := []kbInitQMDCollection{
			{Collection: repoName + "-knowledge", Path: filepath.Join(gitRoot, "docs", "knowledge")},
			{Collection: repoName + "-decisions", Path: filepath.Join(gitRoot, "docs", "decisions")},
		}
		for _, collection := range collections {
			if existing[collection.Collection] {
				collection.Status = "exists"
			} else {
				if err := qmdRegisterCollection(collection.Collection, collection.Path); err != nil {
					return kbInitResult{}, err
				}
				collection.Status = "registered"
			}
			result.QMD.Collections = append(result.QMD.Collections, collection)
		}
	}

	return result, nil
}

func initializeNativeKBConfig(gitRoot string) (string, error) {
	configPath := filepath.Join(gitRoot, ".agents", "loaf.json")
	if _, err := os.Stat(configPath); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return "", err
		}
		body, err := json.MarshalIndent(map[string]any{"knowledge": defaultNativeKBConfigJSON()}, "", "  ")
		if err != nil {
			return "", err
		}
		return "created", os.WriteFile(configPath, append(body, '\n'), 0o644)
	}

	body, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "exists", nil
	}
	object, ok := parsed.(map[string]any)
	if !ok {
		return "exists", nil
	}
	if knowledge, ok := object["knowledge"].(map[string]any); ok && knowledge != nil {
		return "exists", nil
	}
	object["knowledge"] = defaultNativeKBConfigJSON()
	updated, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, append(updated, '\n'), 0o644); err != nil {
		return "", err
	}
	return "updated", nil
}

func importNativeKB(gitRoot string, options kbImportOptions) (kbImportResult, error) {
	collectionName := options.name + "-knowledge"
	result := kbImportResult{Name: options.name, Collection: collectionName}
	configPath := filepath.Join(gitRoot, ".agents", "loaf.json")
	config, err := readNativeKBImportConfig(configPath)
	if err != nil {
		message := "Cannot parse .agents/loaf.json — fix or remove it before importing"
		result.Error = message
		return result, fmt.Errorf("%s", message)
	}
	if nativeKBImportExists(config, options.name) {
		result.Status = "already_imported"
		return result, nil
	}

	collectionPath := options.path
	if collectionPath == "" {
		collectionPath = options.name
	}
	if !stringSet(qmdListCollections())[collectionName] {
		if err := qmdRegisterCollection(collectionName, collectionPath); err != nil {
			message := "Failed to register collection: " + err.Error()
			result.Error = message
			return result, fmt.Errorf("%s", message)
		}
	}

	if err := writeNativeKBImportConfig(configPath, config, options.name); err != nil {
		return result, err
	}
	result.Status = "imported"
	return result, nil
}

func readNativeKBImportConfig(configPath string) (map[string]any, error) {
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	config, ok := parsed.(map[string]any)
	if !ok || config == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return config, nil
}

func nativeKBImportExists(config map[string]any, name string) bool {
	knowledge, ok := config["knowledge"].(map[string]any)
	if !ok {
		return false
	}
	imports, ok := knowledge["imports"].([]any)
	if !ok {
		return false
	}
	for _, entry := range imports {
		object, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if entryName, ok := object["name"].(string); ok && entryName == name {
			return true
		}
	}
	return false
}

func writeNativeKBImportConfig(configPath string, config map[string]any, name string) error {
	knowledge, ok := config["knowledge"].(map[string]any)
	if !ok || knowledge == nil {
		knowledge = defaultNativeKBConfigJSON()
		config["knowledge"] = knowledge
	}
	imports, ok := knowledge["imports"].([]any)
	if !ok {
		imports = []any{}
	}
	knowledge["imports"] = append(imports, map[string]any{"name": name})
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, append(body, '\n'), 0o644)
}

func glossaryPath(gitRoot string) string {
	return filepath.Join(gitRoot, filepath.FromSlash(glossaryRelativePath))
}

func loadNativeGlossaryForRead(gitRoot string) (glossaryData, bool, error) {
	body, err := os.ReadFile(glossaryPath(gitRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return glossaryData{}, false, nil
		}
		return glossaryData{}, false, err
	}
	data, err := parseNativeGlossary(string(body))
	return data, true, err
}

func loadOrCreateNativeGlossary(gitRoot string) (glossaryData, error) {
	data, exists, err := loadNativeGlossaryForRead(gitRoot)
	if err != nil {
		return glossaryData{}, err
	}
	if exists {
		return data, nil
	}
	data = glossaryData{}
	if err := writeNativeGlossary(gitRoot, data); err != nil {
		return glossaryData{}, err
	}
	return data, nil
}

func parseNativeGlossary(text string) (glossaryData, error) {
	fields, content, ok := splitNativeGlossaryFrontmatter(text)
	if !ok || firstFieldValue(fields["type"]) != "glossary" {
		return glossaryData{}, fmt.Errorf("glossary frontmatter must have `type: glossary`")
	}
	sections, err := splitNativeGlossarySections(content)
	if err != nil {
		return glossaryData{}, err
	}
	required := []string{"Canonical Terms", "Candidates", "Relationships", "Flagged ambiguities"}
	for _, section := range required {
		if _, ok := sections[section]; !ok {
			return glossaryData{}, fmt.Errorf("glossary missing required section: %q", section)
		}
	}
	return glossaryData{
		Canonical:          parseNativeGlossaryTerms(sections["Canonical Terms"]),
		Candidates:         parseNativeGlossaryTerms(sections["Candidates"]),
		Relationships:      trimBlankLines(sections["Relationships"]),
		FlaggedAmbiguities: trimBlankLines(sections["Flagged ambiguities"]),
	}, nil
}

func splitNativeGlossaryFrontmatter(text string) (map[string]frontmatterField, string, bool) {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return nil, "", false
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, "", false
	}
	fields, ok := parseKnowledgeFrontmatter([]byte(strings.Join(lines[:end+1], "\n") + "\n"))
	if !ok {
		return nil, "", false
	}
	return fields, strings.Join(lines[end+1:], "\n"), true
}

func splitNativeGlossarySections(content string) (map[string]string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	sections := map[string]string{}
	current := ""
	var buffer []string
	inFence := false
	known := stringSet([]string{"Canonical Terms", "Candidates", "Relationships", "Flagged ambiguities"})
	flush := func() {
		sections[current] = strings.Join(buffer, "\n")
	}
	for _, line := range lines {
		if isMarkdownFenceLine(line) {
			inFence = !inFence
			buffer = append(buffer, line)
			continue
		}
		if !inFence && strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "### ") {
			flush()
			current = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if !known[current] {
				return nil, fmt.Errorf("unknown glossary section: %q", current)
			}
			buffer = nil
			continue
		}
		buffer = append(buffer, line)
	}
	flush()
	if strings.TrimSpace(sections[""]) != "" {
		return nil, fmt.Errorf("glossary body must start with a `## ` section header")
	}
	return sections, nil
}

func isMarkdownFenceLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func parseNativeGlossaryTerms(body string) []glossaryTerm {
	lines := strings.Split(body, "\n")
	var terms []glossaryTerm
	var currentName string
	var currentLines []string
	inFence := false
	flush := func() {
		if currentName == "" {
			return
		}
		terms = append(terms, nativeGlossaryTermFromLines(currentName, currentLines))
		currentName = ""
		currentLines = nil
	}
	for _, line := range lines {
		if isMarkdownFenceLine(line) {
			inFence = !inFence
			if currentName != "" {
				currentLines = append(currentLines, line)
			}
			continue
		}
		if !inFence && strings.HasPrefix(line, "### ") {
			flush()
			currentName = strings.TrimSpace(strings.TrimPrefix(line, "### "))
			continue
		}
		if currentName != "" {
			currentLines = append(currentLines, line)
		}
	}
	flush()
	return terms
}

func nativeGlossaryTermFromLines(name string, lines []string) glossaryTerm {
	var definition []string
	var avoid []string
	for _, line := range lines {
		if strings.HasPrefix(line, "_Avoid_:") {
			avoid = parseCommaList(strings.TrimSpace(strings.TrimPrefix(line, "_Avoid_:")))
			continue
		}
		definition = append(definition, line)
	}
	return glossaryTerm{Name: name, Definition: trimBlankLines(strings.Join(definition, "\n")), Avoid: avoid}
}

func writeNativeGlossary(gitRoot string, data glossaryData) error {
	path := glossaryPath(gitRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(serializeNativeGlossary(data)), 0o644)
}

func serializeNativeGlossary(data glossaryData) string {
	var builder strings.Builder
	builder.WriteString("---\n")
	builder.WriteString("type: glossary\n")
	builder.WriteString("topics:\n")
	builder.WriteString("  - glossary\n")
	builder.WriteString("last_reviewed: " + time.Now().Format("2006-01-02") + "\n")
	builder.WriteString("---\n")
	builder.WriteString("## Canonical Terms\n\n")
	builder.WriteString(renderNativeGlossaryTerms(data.Canonical))
	builder.WriteString("\n## Candidates\n\n")
	builder.WriteString(renderNativeGlossaryTerms(data.Candidates))
	builder.WriteString("\n## Relationships\n\n")
	if strings.TrimSpace(data.Relationships) != "" {
		builder.WriteString(trimBlankLines(data.Relationships) + "\n")
	}
	builder.WriteString("\n## Flagged ambiguities\n\n")
	if strings.TrimSpace(data.FlaggedAmbiguities) != "" {
		builder.WriteString(trimBlankLines(data.FlaggedAmbiguities) + "\n")
	}
	return builder.String()
}

func renderNativeGlossaryTerms(terms []glossaryTerm) string {
	if len(terms) == 0 {
		return ""
	}
	var blocks []string
	for _, term := range terms {
		var lines []string
		lines = append(lines, "### "+term.Name, "")
		if term.Definition != "" {
			lines = append(lines, term.Definition)
		}
		if len(term.Avoid) > 0 {
			if term.Definition != "" {
				lines = append(lines, "")
			}
			lines = append(lines, "_Avoid_: "+strings.Join(term.Avoid, ", "))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

func upsertNativeGlossaryTerm(gitRoot string, term string, definition string, avoid []string) (string, error) {
	if nativeGlossaryLinearNative(gitRoot) {
		return "", fmt.Errorf("%s", glossaryLinearNativeMessage)
	}
	data, err := loadOrCreateNativeGlossary(gitRoot)
	if err != nil {
		return "", err
	}
	data.Candidates = removeGlossaryTerm(data.Candidates, term)
	for _, alias := range avoid {
		if conflict := findGlossaryTerm(data.Canonical, alias); conflict != nil && !strings.EqualFold(conflict.Name, term) {
			return "", fmt.Errorf("alias %q is already canonical (term: %s)", alias, conflict.Name)
		}
	}
	action := "created"
	next := glossaryTerm{Name: term, Definition: definition, Avoid: avoid}
	if index := glossaryTermIndex(data.Canonical, term); index >= 0 {
		data.Canonical[index] = next
		action = "updated"
	} else {
		data.Canonical = append(data.Canonical, next)
	}
	if err := writeNativeGlossary(gitRoot, data); err != nil {
		return "", err
	}
	return action, nil
}

func proposeNativeGlossaryTerm(gitRoot string, term string, definition string, avoid []string) (string, error) {
	if nativeGlossaryLinearNative(gitRoot) {
		return "", fmt.Errorf("%s", glossaryLinearNativeMessage)
	}
	data, err := loadOrCreateNativeGlossary(gitRoot)
	if err != nil {
		return "", err
	}
	if findGlossaryTerm(data.Canonical, term) != nil {
		return "", fmt.Errorf("%q is already canonical; use upsert to update it", term)
	}
	action := "created"
	next := glossaryTerm{Name: term, Definition: definition, Avoid: avoid}
	if index := glossaryTermIndex(data.Candidates, term); index >= 0 {
		data.Candidates[index] = next
		action = "updated"
	} else {
		data.Candidates = append(data.Candidates, next)
	}
	if err := writeNativeGlossary(gitRoot, data); err != nil {
		return "", err
	}
	return action, nil
}

func stabilizeNativeGlossaryTerm(gitRoot string, term string, definition string) error {
	if nativeGlossaryLinearNative(gitRoot) {
		return fmt.Errorf("%s", glossaryLinearNativeMessage)
	}
	data, exists, err := loadNativeGlossaryForRead(gitRoot)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("not in candidates: %q", term)
	}
	index := glossaryTermIndex(data.Candidates, term)
	if index < 0 {
		return fmt.Errorf("not in candidates: %q", term)
	}
	candidate := data.Candidates[index]
	if definition != "" {
		candidate.Definition = definition
	}
	data.Candidates = append(data.Candidates[:index], data.Candidates[index+1:]...)
	if canonicalIndex := glossaryTermIndex(data.Canonical, term); canonicalIndex >= 0 {
		data.Canonical[canonicalIndex] = candidate
	} else {
		data.Canonical = append(data.Canonical, candidate)
	}
	return writeNativeGlossary(gitRoot, data)
}

type glossaryCheckResult struct {
	Kind      string
	Query     string
	Term      *glossaryTerm
	Canonical *glossaryTerm
}

func checkNativeGlossaryTerm(gitRoot string, term string) (glossaryCheckResult, error) {
	data, _, err := loadNativeGlossaryForRead(gitRoot)
	if err != nil {
		return glossaryCheckResult{}, err
	}
	if canonical := findGlossaryTerm(data.Canonical, term); canonical != nil {
		return glossaryCheckResult{Kind: "canonical", Term: canonical}, nil
	}
	if candidate := findGlossaryTerm(data.Candidates, term); candidate != nil {
		return glossaryCheckResult{Kind: "candidate", Term: candidate}, nil
	}
	for i := range data.Canonical {
		for _, alias := range data.Canonical[i].Avoid {
			if strings.EqualFold(alias, term) {
				return glossaryCheckResult{Kind: "alias", Query: term, Canonical: &data.Canonical[i]}, nil
			}
		}
	}
	return glossaryCheckResult{Kind: "unknown", Query: term}, fmt.Errorf("unknown glossary term: %s", term)
}

func listNativeGlossaryTerms(gitRoot string, mode string) (glossaryData, error) {
	data, _, err := loadNativeGlossaryForRead(gitRoot)
	if err != nil {
		return glossaryData{}, err
	}
	switch mode {
	case "canonical":
		data.Candidates = nil
	case "candidates":
		data.Canonical = nil
	}
	return data, nil
}

func nativeGlossaryLinearNative(gitRoot string) bool {
	body, err := os.ReadFile(filepath.Join(gitRoot, ".agents", "loaf.json"))
	if err != nil {
		return false
	}
	var config struct {
		Integrations struct {
			Linear struct {
				Enabled bool `json:"enabled"`
			} `json:"linear"`
		} `json:"integrations"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return false
	}
	return config.Integrations.Linear.Enabled
}

func findGlossaryTerm(terms []glossaryTerm, name string) *glossaryTerm {
	for i := range terms {
		if strings.EqualFold(terms[i].Name, name) {
			return &terms[i]
		}
	}
	return nil
}

func glossaryTermIndex(terms []glossaryTerm, name string) int {
	for i := range terms {
		if strings.EqualFold(terms[i].Name, name) {
			return i
		}
	}
	return -1
}

func removeGlossaryTerm(terms []glossaryTerm, name string) []glossaryTerm {
	filtered := terms[:0]
	for _, term := range terms {
		if !strings.EqualFold(term.Name, name) {
			filtered = append(filtered, term)
		}
	}
	return filtered
}

func parseCommaList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func trimBlankLines(value string) string {
	value = strings.TrimLeft(value, "\n")
	value = strings.TrimRight(value, "\n")
	return value
}

func defaultNativeKBConfigJSON() map[string]any {
	return map[string]any{
		"local":                    []string{"docs/knowledge", "docs/decisions"},
		"staleness_threshold_days": float64(30),
		"imports":                  []string{},
	}
}

func nativeQMDAvailable() bool {
	_, err := qmdLookPath("qmd")
	return err == nil
}

func nativeQMDListCollections() []string {
	output, err := exec.Command("qmd", "collection", "list").Output()
	if err != nil {
		return []string{}
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	collections := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			collections = append(collections, line)
		}
	}
	return collections
}

func nativeQMDRegisterCollection(name string, path string) error {
	cmd := exec.Command("qmd", "collection", "add", path, "--name", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to register QMD collection %q: %s", name, strings.TrimSpace(string(output)))
	}
	return nil
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func loadNativeKbConfig(gitRoot string) kbConfig {
	defaultConfig := kbConfig{
		Local:                  []string{"docs/knowledge", "docs/decisions"},
		StalenessThresholdDays: 30,
	}
	body, err := os.ReadFile(filepath.Join(gitRoot, ".agents", "loaf.json"))
	if err != nil {
		return defaultConfig
	}
	var parsed struct {
		Knowledge struct {
			Local                  []string `json:"local"`
			StalenessThresholdDays int      `json:"staleness_threshold_days"`
		} `json:"knowledge"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return defaultConfig
	}
	if len(parsed.Knowledge.Local) > 0 {
		defaultConfig.Local = append([]string(nil), parsed.Knowledge.Local...)
	}
	if parsed.Knowledge.StalenessThresholdDays > 0 {
		defaultConfig.StalenessThresholdDays = parsed.Knowledge.StalenessThresholdDays
	}
	return defaultConfig
}

func loadNativeKnowledgeFiles(gitRoot string, config kbConfig, errOut io.Writer, includeInvalid bool) []knowledgeFile {
	var files []knowledgeFile
	for _, dir := range config.Local {
		absDir := filepath.Join(gitRoot, filepath.FromSlash(dir))
		entries, err := os.ReadDir(absDir)
		if err != nil {
			fmt.Fprintf(errOut, "  %swarn:%s KB directory not found: %s\n", ansiYellowStart(), ansiReset(), dir)
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			absPath := filepath.Join(absDir, entry.Name())
			file, ok := loadNativeKnowledgeFile(gitRoot, absPath, errOut, includeInvalid)
			if ok {
				files = append(files, file)
			}
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
	return files
}

func loadNativeKnowledgeFile(gitRoot string, absPath string, errOut io.Writer, includeInvalid bool) (knowledgeFile, bool) {
	body, err := os.ReadFile(absPath)
	rel, relErr := filepath.Rel(gitRoot, absPath)
	if relErr != nil {
		rel = absPath
	}
	rel = filepath.ToSlash(rel)
	if err != nil {
		fmt.Fprintf(errOut, "  %swarn:%s Failed to parse %s: %v\n", ansiYellowStart(), ansiReset(), rel, err)
		return knowledgeFile{}, false
	}
	frontmatter, ok := parseKnowledgeFrontmatter(body)
	if !ok {
		return knowledgeFile{}, false
	}
	if !includeInvalid && len(frontmatter["topics"].Values) == 0 {
		return knowledgeFile{}, false
	}
	return knowledgeFile{
		Path:                 absPath,
		RelativePath:         rel,
		Topics:               frontmatter["topics"],
		LastReviewed:         frontmatter["last_reviewed"],
		Covers:               frontmatter["covers"],
		DependsOn:            frontmatter["depends_on"],
		ImplementationStatus: frontmatter["implementation_status"],
		HasFrontmatter:       true,
	}, true
}

func parseKnowledgeFrontmatter(body []byte) (map[string]frontmatterField, bool) {
	if !bytes.HasPrefix(body, []byte("---\n")) && !bytes.HasPrefix(body, []byte("---\r\n")) {
		return nil, false
	}
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return nil, false
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, false
	}
	values := map[string]frontmatterField{}
	currentKey := ""
	for _, line := range lines[1:end] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && currentKey != "" {
			field := values[currentKey]
			field.Values = append(field.Values, cleanYamlScalar(strings.TrimPrefix(trimmed, "- ")))
			field.Array = true
			field.Set = true
			values[currentKey] = field
			continue
		}
		key, raw, ok := strings.Cut(trimmed, ":")
		if !ok {
			currentKey = ""
			continue
		}
		key = strings.TrimSpace(key)
		currentKey = key
		raw = strings.TrimSpace(raw)
		if raw == "" {
			values[key] = frontmatterField{Array: true, Set: true}
			continue
		}
		values[key] = parseYamlScalarOrArray(raw)
	}
	return values, true
}

func parseYamlScalarOrArray(raw string) frontmatterField {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]"))
		if inner == "" {
			return frontmatterField{Array: true, Set: true}
		}
		parts := strings.Split(inner, ",")
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			values = append(values, cleanYamlScalar(part))
		}
		return frontmatterField{Values: values, Array: true, Set: true}
	}
	return frontmatterField{Values: []string{cleanYamlScalar(raw)}, Set: true}
}

func cleanYamlScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return value
}

func firstFieldValue(field frontmatterField) string {
	if len(field.Values) == 0 {
		return ""
	}
	return field.Values[0]
}

func setFrontmatterScalar(body []byte, key string, value string) ([]byte, error) {
	if !bytes.HasPrefix(body, []byte("---\n")) && !bytes.HasPrefix(body, []byte("---\r\n")) {
		return nil, fmt.Errorf("frontmatter not found")
	}
	newline := "\n"
	text := string(body)
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return nil, fmt.Errorf("frontmatter not found")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("frontmatter not found")
	}
	replacement := fmt.Sprintf("%s: %s", key, value)
	for i := 1; i < end; i++ {
		field, _, ok := strings.Cut(strings.TrimSpace(lines[i]), ":")
		if ok && strings.TrimSpace(field) == key {
			lines[i] = replacement
			updated := strings.Join(lines, "\n")
			return []byte(strings.ReplaceAll(updated, "\n", newline)), nil
		}
	}
	lines = append(lines[:end], append([]string{replacement}, lines[end:]...)...)
	updated := strings.Join(lines, "\n")
	return []byte(strings.ReplaceAll(updated, "\n", newline)), nil
}

func frontmatterJSON(fields map[string]frontmatterField) map[string]any {
	output := make(map[string]any, len(fields))
	for key, field := range fields {
		if field.Array {
			output[key] = append([]string(nil), field.Values...)
			continue
		}
		output[key] = firstFieldValue(field)
	}
	return output
}

func summarizeKnowledgeFiles(ctx context.Context, gitRoot string, files []knowledgeFile, now time.Time) kbStatusSummary {
	summary := kbStatusSummary{
		TotalFiles:  len(files),
		Directories: map[string]int{},
	}
	totalAgeDays := 0
	reviewedCount := 0
	for _, file := range files {
		dir := filepath.Dir(file.RelativePath)
		if dir == "." {
			dir = "."
		}
		summary.Directories[dir]++
		if len(file.Covers.Values) > 0 {
			summary.FilesWithCovers++
			if stalenessForKnowledgeFile(ctx, gitRoot, file).IsStale {
				summary.Stale++
			}
		}
		if reviewed, ok := parseReviewedDate(firstFieldValue(file.LastReviewed)); ok {
			totalAgeDays += int(now.Sub(reviewed).Hours() / 24)
			reviewedCount++
		}
	}
	summary.FilesWithoutCovers = summary.TotalFiles - summary.FilesWithCovers
	if reviewedCount > 0 {
		summary.AvgReviewAgeDays = int(float64(totalAgeDays)/float64(reviewedCount) + 0.5)
	}
	return summary
}

func parseReviewedDate(value string) (time.Time, bool) {
	reviewed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	return reviewed, true
}

func stalenessResults(ctx context.Context, gitRoot string, files []knowledgeFile) []kbStalenessResult {
	results := make([]kbStalenessResult, 0, len(files))
	for _, file := range files {
		results = append(results, stalenessForKnowledgeFile(ctx, gitRoot, file))
	}
	return results
}

func stalenessForKnowledgeFile(ctx context.Context, gitRoot string, file knowledgeFile) kbStalenessResult {
	result := kbStalenessResult{
		File:         file.RelativePath,
		LastReviewed: firstFieldValue(file.LastReviewed),
	}
	if len(file.Covers.Values) == 0 {
		return result
	}
	result.HasCoverage = true
	lastReviewed := firstFieldValue(file.LastReviewed)
	if _, ok := parseReviewedDate(lastReviewed); !ok {
		result.IsStale = true
		return result
	}
	args := []string{"log", "--since=" + lastReviewed, "--format=%H%n%an%n%aI", "--"}
	for _, cover := range file.Covers.Values {
		args = append(args, ":(glob)"+cover)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = gitRoot
	output, err := cmd.Output()
	if err != nil {
		return result
	}
	commits, author, date := parseKbGitLog(output)
	result.CommitCount = commits
	result.LastCommitAuthor = author
	result.LastCommitDate = date
	result.IsStale = commits > 0
	return result
}

func parseKbGitLog(output []byte) (int, string, string) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return 0, "", ""
	}
	lines := strings.Split(trimmed, "\n")
	count := len(lines) / 3
	if count == 0 {
		return 0, "", ""
	}
	return count, lines[1], lines[2]
}

func filterKnowledgeFilesCovering(files []knowledgeFile, filePath string) []knowledgeFile {
	var matched []knowledgeFile
	for _, file := range files {
		for _, pattern := range file.Covers.Values {
			if kbGlobMatches(pattern, filePath) {
				matched = append(matched, file)
				break
			}
		}
	}
	return matched
}

func kbGlobMatches(pattern string, path string) bool {
	regex, err := regexp.Compile(kbGlobToRegexp(pattern))
	if err != nil {
		return false
	}
	return regex.MatchString(filepath.ToSlash(path))
}

func kbGlobToRegexp(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	runes := []rune(filepath.ToSlash(pattern))
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '*':
			if i+1 < len(runes) && runes[i+1] == '*' {
				i++
				if i+1 < len(runes) && runes[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(runes[i])))
		}
	}
	b.WriteString("$")
	return b.String()
}

func validateNativeKnowledgeFiles(gitRoot string, files []knowledgeFile) []kbValidationResult {
	results := make([]kbValidationResult, 0, len(files))
	for _, file := range files {
		result := kbValidationResult{
			File:     file.RelativePath,
			Errors:   []kbValidationIssue{},
			Warnings: []kbValidationIssue{},
		}
		if !file.Topics.Set {
			result.Errors = append(result.Errors, kbValidationIssue{Field: "topics", Message: "Missing required field"})
		} else if !file.Topics.Array {
			result.Errors = append(result.Errors, kbValidationIssue{Field: "topics", Message: fmt.Sprintf("Must be an array, got string: %q", firstFieldValue(file.Topics))})
		} else if len(file.Topics.Values) == 0 {
			result.Errors = append(result.Errors, kbValidationIssue{Field: "topics", Message: "Must contain at least one topic"})
		}
		if !file.LastReviewed.Set || firstFieldValue(file.LastReviewed) == "" {
			result.Errors = append(result.Errors, kbValidationIssue{Field: "last_reviewed", Message: "Missing required field"})
		} else if !validReviewedDate(firstFieldValue(file.LastReviewed)) {
			result.Errors = append(result.Errors, kbValidationIssue{Field: "last_reviewed", Message: fmt.Sprintf("Invalid date format (expected YYYY-MM-DD): %q", firstFieldValue(file.LastReviewed))})
		}
		for _, glob := range file.Covers.Values {
			if !globMatchesTrackedFiles(gitRoot, glob) {
				result.Warnings = append(result.Warnings, kbValidationIssue{Field: "covers", Message: fmt.Sprintf("Glob pattern matches no tracked files: %q", glob)})
			}
		}
		for _, dep := range file.DependsOn.Values {
			fromRoot := filepath.Join(gitRoot, filepath.FromSlash(dep))
			fromFileDir := filepath.Join(filepath.Dir(file.Path), filepath.FromSlash(dep))
			if !pathExists(fromRoot) && !pathExists(fromFileDir) {
				result.Warnings = append(result.Warnings, kbValidationIssue{Field: "depends_on", Message: fmt.Sprintf("Referenced file does not exist: %q", dep)})
			}
		}
		if file.ImplementationStatus.Set && len(file.ImplementationStatus.Values) > 0 {
			status := firstFieldValue(file.ImplementationStatus)
			if status != "in-progress" && status != "stable" && status != "deprecated" {
				result.Warnings = append(result.Warnings, kbValidationIssue{Field: "implementation_status", Message: fmt.Sprintf("Unrecognized value: %q", status)})
			}
		}
		results = append(results, result)
	}
	return results
}

func validReviewedDate(value string) bool {
	reviewed, err := time.Parse("2006-01-02", value)
	return err == nil && reviewed.Format("2006-01-02") == value
}

func globMatchesTrackedFiles(gitRoot string, glob string) bool {
	cmd := exec.Command("git", "ls-files", "--", glob)
	cmd.Dir = gitRoot
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) != ""
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func countValidationErrors(results []kbValidationResult) int {
	count := 0
	for _, result := range results {
		count += len(result.Errors)
	}
	return count
}

func countValidationWarnings(results []kbValidationResult) int {
	count := 0
	for _, result := range results {
		count += len(result.Warnings)
	}
	return count
}

func writeKbValidation(out io.Writer, results []kbValidationResult) {
	fmt.Fprintf(out, "\n  %s\n\n", ansiBold("loaf kb validate"))
	for _, result := range results {
		if len(result.Errors) == 0 && len(result.Warnings) == 0 {
			fmt.Fprintf(out, "  %s %s\n", ansiGreen("✓"), result.File)
			continue
		}
		fmt.Fprintf(out, "  %s\n", result.File)
		for _, issue := range result.Errors {
			fmt.Fprintf(out, "    %s %s — %s\n", ansiRed("error:"), issue.Field, issue.Message)
		}
		for _, issue := range result.Warnings {
			fmt.Fprintf(out, "    %s %s — %s\n", ansiYellow("warn:"), issue.Field, issue.Message)
		}
	}
	fmt.Fprintf(out, "\n  %s files, %s errors, %s warnings\n\n", ansiBold(fmt.Sprint(len(results))), ansiRed(fmt.Sprint(countValidationErrors(results))), ansiYellow(fmt.Sprint(countValidationWarnings(results))))
}

func writeKbCheck(out io.Writer, results []kbStalenessResult) {
	fmt.Fprintf(out, "\n  %s\n\n", ansiBold("loaf kb check"))
	stale, fresh, noCoverage := splitKbStaleness(results)
	if len(stale) > 0 {
		fmt.Fprintf(out, "  %s\n", ansiRed(ansiBold("Stale")))
		for _, result := range stale {
			fmt.Fprintf(out, "    %s %s\n", ansiRed("✗"), result.File)
			fmt.Fprintf(out, "      %d commit%s since %s\n", result.CommitCount, pluralSuffix(result.CommitCount), result.LastReviewed)
			if result.LastCommitAuthor != "" {
				fmt.Fprintf(out, "      last by: %s (%s)\n", result.LastCommitAuthor, result.LastCommitDate)
			}
		}
		fmt.Fprintln(out)
	}
	if len(fresh) > 0 {
		fmt.Fprintf(out, "  %s\n", ansiGreen(ansiBold("Fresh")))
		for _, result := range fresh {
			fmt.Fprintf(out, "    %s %s  %s\n", ansiGreen("✓"), result.File, ansiGray("reviewed "+result.LastReviewed))
		}
		fmt.Fprintln(out)
	}
	if len(noCoverage) > 0 {
		fmt.Fprintf(out, "  %s\n", ansiGray(ansiBold("No coverage")))
		for _, result := range noCoverage {
			fmt.Fprintf(out, "    %s %s\n", ansiGray("-"), ansiGray(result.File))
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "  %s stale, %s fresh, %s without coverage\n\n", ansiRed(fmt.Sprint(len(stale))), ansiGreen(fmt.Sprint(len(fresh))), ansiGray(fmt.Sprint(len(noCoverage))))
}

func writeKbCheckForFile(out io.Writer, filePath string, results []kbStalenessResult) {
	fmt.Fprintf(out, "\n  %s --file %s\n\n", ansiBold("loaf kb check"), filePath)
	if len(results) == 0 {
		fmt.Fprintf(out, "  %s\n\n", ansiGray("No knowledge files cover this path"))
		return
	}
	for _, result := range results {
		label := ansiGreen("fresh")
		if result.IsStale {
			label = ansiRed("stale")
		}
		fmt.Fprintf(out, "  %s  %s\n", label, result.File)
		fmt.Fprintf(out, "    last_reviewed: %s\n", result.LastReviewed)
		if result.IsStale {
			fmt.Fprintf(out, "    %d commit%s since review\n", result.CommitCount, pluralSuffix(result.CommitCount))
			if result.LastCommitAuthor != "" {
				fmt.Fprintf(out, "    last by: %s (%s)\n", result.LastCommitAuthor, result.LastCommitDate)
			}
		}
	}
	fmt.Fprintln(out)
}

func writeKbInit(out io.Writer, result kbInitResult) {
	fmt.Fprintf(out, "\n  %s\n\n", ansiBold("loaf kb init"))
	for _, action := range result.Directories {
		if action.Status == "created" {
			fmt.Fprintf(out, "  %s Created %s\n", ansiGreen("+"), action.Target)
		} else {
			fmt.Fprintf(out, "  %s Already exists: %s\n", ansiGray("-"), action.Target)
		}
	}
	configActionStatus := result.Config.Status
	if configActionStatus == "updated" {
		configActionStatus = "created"
	}
	if configActionStatus == "created" {
		fmt.Fprintf(out, "  %s Created %s\n", ansiGreen("+"), result.Config.Path)
	} else {
		fmt.Fprintf(out, "  %s Already exists: %s\n", ansiGray("-"), result.Config.Path)
	}
	if result.Config.Status == "updated" {
		fmt.Fprintf(out, "  %s Added knowledge section to .agents/loaf.json\n", ansiGreen("+"))
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %s\n", ansiBold("QMD"))
	if result.QMD.Available {
		for _, collection := range result.QMD.Collections {
			if collection.Status == "registered" {
				fmt.Fprintf(out, "  %s Registered collection: %s\n", ansiGreen("+"), ansiCyan(collection.Collection))
			} else {
				fmt.Fprintf(out, "  %s Collection exists: %s\n", ansiGray("-"), ansiCyan(collection.Collection))
			}
		}
	} else {
		fmt.Fprintf(out, "  %s QMD not found. Install QMD for knowledge retrieval:\n", ansiYellow("info:"))
		fmt.Fprintf(out, "        %s\n", ansiCyan("https://github.com/tobi/qmd"))
	}
	fmt.Fprintf(out, "\n  %s Knowledge base initialized\n\n", ansiGreen("✓"))
}

func writeKbGlossaryCheck(out io.Writer, result glossaryCheckResult) {
	switch result.Kind {
	case "canonical":
		fmt.Fprintf(out, "  %s %s\n", ansiGreen("canonical:"), ansiBold(result.Term.Name))
		if result.Term.Definition != "" {
			fmt.Fprintf(out, "    %s\n", result.Term.Definition)
		}
		if len(result.Term.Avoid) > 0 {
			fmt.Fprintf(out, "    %s %s\n", ansiGray("avoid:"), strings.Join(result.Term.Avoid, ", "))
		}
	case "candidate":
		fmt.Fprintf(out, "  %s %s\n", ansiYellow("candidate:"), ansiBold(result.Term.Name))
		if result.Term.Definition != "" {
			fmt.Fprintf(out, "    %s\n", result.Term.Definition)
		}
	case "alias":
		fmt.Fprintf(out, "avoided, use %s\n", result.Canonical.Name)
		if result.Canonical.Definition != "" {
			fmt.Fprintf(out, "    %s\n", result.Canonical.Definition)
		}
	default:
		fmt.Fprintf(out, "  %s %s\n", ansiGray("unknown:"), result.Query)
	}
}

func writeKbGlossaryList(out io.Writer, data glossaryData) {
	if len(data.Canonical)+len(data.Candidates) == 0 {
		fmt.Fprintln(out, "No glossary entries yet")
		return
	}
	for _, term := range data.Canonical {
		fmt.Fprintf(out, "%s: %s\n", term.Name, truncateOneLine(term.Definition, 80))
	}
	for _, term := range data.Candidates {
		fmt.Fprintf(out, "%s: %s\n", term.Name, truncateOneLine(term.Definition, 80))
	}
}

func truncateOneLine(text string, max int) string {
	firstLine := strings.Split(text, "\n")[0]
	if len(firstLine) <= max {
		return firstLine
	}
	if max <= 3 {
		return firstLine[:max]
	}
	return firstLine[:max-3] + "..."
}

func writeKbReview(out io.Writer, relPath string, today string) {
	fmt.Fprintf(out, "\n  %s Updated last_reviewed for %s\n", ansiGreen("✓"), ansiBold(relPath))
	fmt.Fprintf(out, "    last_reviewed: %s\n\n", today)
}

func splitKbStaleness(results []kbStalenessResult) ([]kbStalenessResult, []kbStalenessResult, []kbStalenessResult) {
	var stale []kbStalenessResult
	var fresh []kbStalenessResult
	var noCoverage []kbStalenessResult
	for _, result := range results {
		switch {
		case result.IsStale:
			stale = append(stale, result)
		case result.HasCoverage:
			fresh = append(fresh, result)
		default:
			noCoverage = append(noCoverage, result)
		}
	}
	return stale, fresh, noCoverage
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func writeKbStatus(out io.Writer, summary kbStatusSummary) {
	fmt.Fprintf(out, "\n  %s\n\n", ansiBold("loaf kb status"))
	fmt.Fprintf(out, "  Files:    %s\n", ansiBold(fmt.Sprint(summary.TotalFiles)))
	fmt.Fprintf(out, "  Covers:   %s with %s without\n", ansiGreen(fmt.Sprint(summary.FilesWithCovers)), ansiGray(fmt.Sprint(summary.FilesWithoutCovers)))
	if summary.Stale > 0 {
		fmt.Fprintf(out, "  Stale:    %s\n", ansiRed(fmt.Sprint(summary.Stale)))
	} else {
		fmt.Fprintf(out, "  Stale:    %s\n", ansiGreen("0"))
	}
	fmt.Fprintf(out, "  Avg age:  %s days since last review\n\n", ansiBold(fmt.Sprint(summary.AvgReviewAgeDays)))
	fmt.Fprintf(out, "  %s\n", ansiBold("Directories"))
	for _, dir := range sortedCountKeys(summary.Directories) {
		fmt.Fprintf(out, "    %s: %d files\n", ansiCyan(dir), summary.Directories[dir])
	}
	fmt.Fprintln(out)
}

func ansiGreen(value string) string {
	return "\x1b[32m" + value + "\x1b[0m"
}

func ansiRed(value string) string {
	return "\x1b[31m" + value + "\x1b[0m"
}

func ansiYellow(value string) string {
	return "\x1b[33m" + value + "\x1b[0m"
}

func ansiCyan(value string) string {
	return "\x1b[36m" + value + "\x1b[0m"
}

func ansiYellowStart() string {
	return "\x1b[33m"
}

func ansiReset() string {
	return "\x1b[0m"
}

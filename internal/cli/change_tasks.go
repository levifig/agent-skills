package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	changeTaskFileRE   = regexp.MustCompile(`(?i)^TASK-(\d+)-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$`)
	changeTaskIDRE     = regexp.MustCompile(`(?i)^TASK-(\d+)$`)
	changeTaskCheckbox = regexp.MustCompile(`(?m)^[ \t]*- \[([ xX])\]`)
)

var changeTaskAllowedKeys = map[string]bool{
	"change":     true,
	"id":         true,
	"title":      true,
	"parent":     true,
	"blocks":     true,
	"blocked-by": true,
	"relates-to": true,
}

var changeTaskBannedKeys = map[string]bool{
	"readiness":  true,
	"status":     true,
	"state":      true,
	"completion": true,
	"done":       true,
	"assignee":   true,
	"estimate":   true,
	"priority":   true,
	"progress":   true,
	"lifecycle":  true,
}

type changeTaskRelationKind string

const (
	changeTaskRelParent    changeTaskRelationKind = "parent"
	changeTaskRelBlocks    changeTaskRelationKind = "blocks"
	changeTaskRelBlockedBy changeTaskRelationKind = "blocked-by"
	changeTaskRelRelates   changeTaskRelationKind = "relates-to"
)

type changeTask struct {
	ID            string   `json:"id"`
	Number        int      `json:"number"`
	Title         string   `json:"title"`
	File          string   `json:"file"`
	Parent        string   `json:"parent,omitempty"`
	Blocks        []string `json:"blocks,omitempty"`
	BlockedBy     []string `json:"blockedBy,omitempty"`
	RelatesTo     []string `json:"relatesTo,omitempty"`
	Children      []string `json:"children,omitempty"`
	BlockedByInv  []string `json:"blockedByDerived,omitempty"`
	BlocksInv     []string `json:"blocksDerived,omitempty"`
	Complete      bool     `json:"complete"`
	CheckboxTotal int      `json:"checkboxTotal"`
	CheckboxDone  int      `json:"checkboxDone"`
	Findings      []string `json:"-"`
	Warnings      []string `json:"-"`
}

type changeTasksJSON struct {
	Command  string       `json:"command"`
	Change   string       `json:"change"`
	Folder   string       `json:"folder"`
	Layout   string       `json:"layout"`
	Tasks    []changeTask `json:"tasks"`
	Findings []string     `json:"findings"`
	Warnings []string     `json:"warnings"`
}

type changeShowJSON struct {
	Command       string   `json:"command"`
	Change        string   `json:"change"`
	Folder        string   `json:"folder"`
	Layout        string   `json:"layout"`
	Branch        string   `json:"branch,omitempty"`
	TargetRelease string   `json:"targetRelease,omitempty"`
	State         string   `json:"state"`
	CapturedOnly  bool     `json:"capturedOnly"`
	Executable    bool     `json:"executable"`
	PRs           []int    `json:"prs"`
	Findings      []string `json:"findings"`
	Warnings      []string `json:"warnings"`
}

func (r Runner) runChangeTasks(args []string, out io.Writer, rootPath string) error {
	if isHelpArg(args) {
		writeChangeTasksHelp(out)
		return nil
	}
	path := ""
	jsonOutput := false
	for _, arg := range args {
		switch {
		case arg == "--json":
			jsonOutput = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown change tasks option %q", arg)
		case path != "":
			return fmt.Errorf("change tasks accepts a single [folder] argument")
		default:
			path = arg
		}
	}
	if !jsonOutput {
		// Projection is JSON-first; text mode prints a compact index.
		jsonOutput = false
	}
	folder, _, err := resolveChangeFolder(rootPath, path)
	if err != nil {
		return err
	}
	node, err := assembleChangeNodeFromFolder(rootPath, folder)
	if err != nil {
		return err
	}
	tasks, findings, warnings := loadChangeTasks(rootPath, folder, node, changeTaskContentWorkingTree, commandOutput)
	result := changeTasksJSON{
		Command:  "change tasks",
		Change:   node.Slug,
		Folder:   relFromRoot(rootPath, folder),
		Layout:   node.Layout,
		Tasks:    tasks,
		Findings: findings,
		Warnings: warnings,
	}
	if jsonOutput || true {
		// Always emit JSON for the stable machine projection (V6).
		_ = jsonOutput
		return writeJSON(out, result)
	}
	return nil
}

func writeChangeTasksHelp(out io.Writer) {
	writeUsageHelp(out, "loaf change tasks [folder] [--json]",
		"Project the stable-ID task index for a Change (parent/children, relations, derived completion). Always emits JSON.",
		"[folder]  Change folder path; resolves from the current branch when omitted",
		"--json    Explicit JSON (default)")
}

func (r Runner) runChangeShow(args []string, out io.Writer, rootPath string) error {
	if isHelpArg(args) {
		writeChangeShowHelp(out)
		return nil
	}
	path := ""
	jsonOutput := false
	for _, arg := range args {
		switch {
		case arg == "--json":
			jsonOutput = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown change show option %q", arg)
		case path != "":
			return fmt.Errorf("change show accepts a single [folder] argument")
		default:
			path = arg
		}
	}
	folder, _, err := resolveChangeFolder(rootPath, path)
	if err != nil {
		return err
	}
	node, err := assembleChangeNodeFromFolder(rootPath, folder)
	if err != nil {
		return err
	}
	report := evaluateChangeNode(node, currentChangeBranch(rootPath))
	_, findings, warnings := loadChangeTasks(rootPath, folder, node, changeTaskContentWorkingTree, commandOutput)
	prs := deriveChangePRSet(rootPath, folder)
	state, stateWarnings := deriveChangeStateDetailed(rootPath, node, changeEvidenceGitOutput)
	result := changeShowJSON{
		Command:       "change show",
		Change:        node.Slug,
		Folder:        relFromRoot(rootPath, folder),
		Layout:        node.Layout,
		Branch:        node.Branch,
		TargetRelease: node.TargetRelease,
		State:         state,
		CapturedOnly:  node.CapturedOnly,
		Executable:    report.Executable,
		PRs:           prs,
		Findings:      append(append([]string{}, report.Violations...), findings...),
		Warnings:      append(append(append([]string{}, report.Warnings...), warnings...), stateWarnings...),
	}
	if node.CapturedOnly {
		result.Warnings = append(result.Warnings, "captured, not shaped (brief-only)")
	}
	result.Findings = sortedUnique(result.Findings)
	result.Warnings = sortedUnique(result.Warnings)
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "\n%s %s\n", ansiBold("change"), result.Change)
	fmt.Fprintf(out, "  folder:   %s\n", result.Folder)
	fmt.Fprintf(out, "  layout:   %s\n", result.Layout)
	if result.Branch != "" {
		fmt.Fprintf(out, "  branch:   %s\n", result.Branch)
	}
	if result.TargetRelease != "" {
		fmt.Fprintf(out, "  target:   %s\n", result.TargetRelease)
	}
	fmt.Fprintf(out, "  state:    %s\n", result.State)
	if len(result.PRs) == 0 {
		fmt.Fprintf(out, "  prs:      (none derived from squash subjects)\n")
	} else {
		fmt.Fprintf(out, "  prs:      ")
		for i, pr := range result.PRs {
			if i > 0 {
				fmt.Fprint(out, ", ")
			}
			fmt.Fprintf(out, "#%d", pr)
		}
		fmt.Fprintln(out)
	}
	for _, w := range result.Warnings {
		fmt.Fprintf(out, "  %s %s\n", ansiYellow("warn:"), w)
	}
	for _, f := range result.Findings {
		fmt.Fprintf(out, "  %s %s\n", ansiRed("x"), f)
	}
	return nil
}

func writeChangeShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf change show [folder] [--json]",
		"Show a Change's derived view: layout, target, derived state ladder, and PR set from squash subjects (#N).",
		"[folder]  Change folder path; resolves from the current branch when omitted",
		"--json    Output as JSON")
}

// changeTaskContentSource selects where structural task-file reads come from.
// check uses the working tree (author feedback); gate and verified-state use
// committed HEAD (evidence) — never a silent filesystem fallback on the evidence path.
type changeTaskContentSource int

const (
	changeTaskContentWorkingTree changeTaskContentSource = iota
	changeTaskContentHEAD
)

func loadChangeTasks(rootPath, folderAbs string, node changeNode, source changeTaskContentSource, outputCommand changeGitOutput) ([]changeTask, []string, []string) {
	if node.Layout != changeLayoutNew {
		return nil, nil, nil
	}
	folderRel := relFromRoot(rootPath, folderAbs)
	names, bodies, listFindings := listChangeTaskFileContents(rootPath, folderAbs, folderRel, source, outputCommand)
	if listFindings != nil && len(names) == 0 {
		return nil, listFindings, nil
	}
	byID := map[string]*changeTask{}
	var findings []string
	var warnings []string
	findings = append(findings, listFindings...)
	seenNumbers := map[int]string{}
	for _, name := range names {
		match := changeTaskFileRE.FindStringSubmatch(name)
		if match == nil {
			findings = append(findings, fmt.Sprintf("tasks/%s: filename must be TASK-NNN-slug.md", name))
			continue
		}
		num := 0
		fmt.Sscanf(match[1], "%d", &num)
		id := fmt.Sprintf("TASK-%03d", num)
		rel := filepath.ToSlash(filepath.Join(folderRel, "tasks", name))
		if prev, ok := seenNumbers[num]; ok {
			findings = append(findings, fmt.Sprintf("duplicate task number %d: %s and %s", num, prev, name))
		}
		seenNumbers[num] = name

		body, ok := bodies[name]
		if !ok {
			findings = append(findings, fmt.Sprintf("%s: missing content", rel))
			continue
		}
		task := parseChangeTaskFile(body, id, num, rel, node.Slug)
		if task.CheckboxTotal == 0 {
			task.Warnings = append(task.Warnings, fmt.Sprintf("%s: zero checkboxes (coordination parents still want one closing box)", rel))
		}
		byID[id] = &task
		findings = append(findings, task.Findings...)
		warnings = append(warnings, task.Warnings...)
	}

	// Derive inverses and validate relations.
	for id, task := range byID {
		if task.Parent != "" {
			if task.Parent == id {
				findings = append(findings, fmt.Sprintf("%s: parent cannot be self", task.File))
			} else if parent, ok := byID[task.Parent]; ok {
				parent.Children = append(parent.Children, id)
			} else if looksLikeExternalTaskRef(task.Parent) {
				findings = append(findings, fmt.Sprintf("%s: cross-change relation parent %q forbidden", task.File, task.Parent))
			} else {
				findings = append(findings, fmt.Sprintf("%s: dangling parent %q", task.File, task.Parent))
			}
		}
		for _, target := range task.Blocks {
			if target == id {
				findings = append(findings, fmt.Sprintf("%s: blocks cannot be self", task.File))
				continue
			}
			if other, ok := byID[target]; ok {
				other.BlockedByInv = appendUniqueSorted(other.BlockedByInv, id)
			} else if looksLikeExternalTaskRef(target) {
				findings = append(findings, fmt.Sprintf("%s: cross-change relation blocks %q forbidden", task.File, target))
			} else {
				findings = append(findings, fmt.Sprintf("%s: dangling blocks %q", task.File, target))
			}
		}
		for _, target := range task.BlockedBy {
			if target == id {
				findings = append(findings, fmt.Sprintf("%s: blocked-by cannot be self", task.File))
				continue
			}
			if other, ok := byID[target]; ok {
				other.BlocksInv = appendUniqueSorted(other.BlocksInv, id)
			} else if looksLikeExternalTaskRef(target) {
				findings = append(findings, fmt.Sprintf("%s: cross-change relation blocked-by %q forbidden", task.File, target))
			} else {
				findings = append(findings, fmt.Sprintf("%s: dangling blocked-by %q", task.File, target))
			}
		}
		for _, target := range task.RelatesTo {
			if target == id {
				findings = append(findings, fmt.Sprintf("%s: relates-to cannot be self", task.File))
				continue
			}
			if _, ok := byID[target]; !ok {
				if looksLikeExternalTaskRef(target) {
					findings = append(findings, fmt.Sprintf("%s: cross-change relation relates-to %q forbidden", task.File, target))
				} else {
					findings = append(findings, fmt.Sprintf("%s: dangling relates-to %q", task.File, target))
				}
			}
		}
	}

	// Parent-chain and blocking-graph cycles.
	findings = append(findings, detectChangeTaskCycles(byID)...)

	tasks := make([]changeTask, 0, len(byID))
	for _, task := range byID {
		task.Children = sortedUnique(task.Children)
		task.BlockedByInv = sortedUnique(task.BlockedByInv)
		task.BlocksInv = sortedUnique(task.BlocksInv)
		tasks = append(tasks, *task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Number < tasks[j].Number })
	return tasks, sortedUnique(findings), sortedUnique(warnings)
}

func listChangeTaskFileContents(rootPath, folderAbs, folderRel string, source changeTaskContentSource, outputCommand changeGitOutput) (names []string, bodies map[string]string, findings []string) {
	bodies = map[string]string{}
	if source == changeTaskContentHEAD {
		if outputCommand == nil {
			outputCommand = commandOutput
		}
		tasksDir := filepath.ToSlash(filepath.Join(folderRel, "tasks"))
		listOutput, err := outputCommand(rootPath, "git", "ls-tree", "-r", "--name-only", "HEAD", "--", tasksDir)
		if err != nil {
			return nil, nil, []string{fmt.Sprintf("read tasks/ at HEAD: %v", err)}
		}
		prefix := tasksDir + "/"
		for _, path := range strings.Split(listOutput, "\n") {
			path = filepath.ToSlash(strings.TrimSpace(path))
			if path == "" || !strings.HasPrefix(path, prefix) {
				continue
			}
			relRest := strings.TrimPrefix(path, prefix)
			if relRest == "" || strings.Contains(relRest, "/") || strings.HasPrefix(relRest, ".") {
				continue
			}
			content, found, readErr := readCommittedOptional(rootPath, "HEAD", path, outputCommand)
			if readErr != nil {
				findings = append(findings, fmt.Sprintf("%s: %v", path, readErr))
				continue
			}
			if !found {
				continue
			}
			names = append(names, relRest)
			bodies[relRest] = content
		}
		sort.Strings(names)
		return names, bodies, findings
	}

	tasksDir := filepath.Join(folderAbs, "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, bodies, nil
		}
		return nil, nil, []string{fmt.Sprintf("read tasks/: %v", err)}
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		body, readErr := readRegularFile(filepath.Join(tasksDir, entry.Name()), projectFileReadLimit)
		if readErr != nil {
			findings = append(findings, fmt.Sprintf("%s: %v", filepath.ToSlash(filepath.Join(folderRel, "tasks", entry.Name())), readErr))
			continue
		}
		names = append(names, entry.Name())
		bodies[entry.Name()] = string(body)
	}
	sort.Strings(names)
	return names, bodies, findings
}

// parseChangeTaskFrontmatter accepts scalar key: value pairs and YAML sequence
// forms (key: followed by - item lines) used by task relation lists.
func parseChangeTaskFrontmatter(content string) ([]changeFrontmatterField, []string) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, nil
	}
	lines := strings.Split(normalized, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, []string{"frontmatter is not closed with ---"}
	}
	var fields []changeFrontmatterField
	var findings []string
	i := 1
	for i < end {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		i++
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			findings = append(findings, fmt.Sprintf("malformed frontmatter line %d: list item without a key", i))
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			findings = append(findings, fmt.Sprintf("malformed frontmatter line %d: expected key: value", i))
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			findings = append(findings, fmt.Sprintf("malformed frontmatter line %d: key cannot be empty", i))
			continue
		}
		if value == "" {
			var items []string
			for i < end {
				next := strings.TrimSpace(lines[i])
				if strings.HasPrefix(next, "- ") {
					items = append(items, strings.TrimSpace(strings.TrimPrefix(next, "- ")))
					i++
					continue
				}
				break
			}
			if len(items) > 0 {
				value = strings.Join(items, ", ")
			}
		}
		fields = append(fields, changeFrontmatterField{Key: key, Value: cleanChangeScalar(value)})
	}
	return fields, findings
}

func parseChangeTaskFile(content, id string, num int, rel, changeSlug string) changeTask {
	task := changeTask{ID: id, Number: num, File: rel, Blocks: []string{}, BlockedBy: []string{}, RelatesTo: []string{}}
	fields, findings := parseChangeTaskFrontmatter(content)
	for _, finding := range findings {
		task.Findings = append(task.Findings, fmt.Sprintf("%s: %s", rel, finding))
	}
	seenKeys := map[string]bool{}
	for _, field := range fields {
		lower := strings.ToLower(field.Key)
		if changeTaskBannedKeys[lower] {
			task.Findings = append(task.Findings,
				fmt.Sprintf("%s: status-like or tracker-parity task frontmatter key %q is banned", rel, field.Key))
			continue
		}
		if !changeTaskAllowedKeys[lower] {
			task.Findings = append(task.Findings,
				fmt.Sprintf("%s: unknown task frontmatter key %q; schema is closed", rel, field.Key))
			continue
		}
		seenKeys[lower] = true
		switch lower {
		case "change":
			if field.Value != "" && field.Value != changeSlug {
				task.Findings = append(task.Findings,
					fmt.Sprintf("%s: change %q does not match owning change %q", rel, field.Value, changeSlug))
			}
		case "id":
			normalized := normalizeChangeTaskID(field.Value)
			if normalized != "" && normalized != id {
				task.Findings = append(task.Findings,
					fmt.Sprintf("%s: id %q does not match filename %s", rel, field.Value, id))
			}
		case "title":
			task.Title = field.Value
		case "parent":
			task.Parent = normalizeChangeTaskID(field.Value)
		case "blocks":
			task.Blocks = appendUniqueSorted(task.Blocks, parseChangeTaskIDList(field.Value)...)
		case "blocked-by":
			task.BlockedBy = appendUniqueSorted(task.BlockedBy, parseChangeTaskIDList(field.Value)...)
		case "relates-to":
			task.RelatesTo = appendUniqueSorted(task.RelatesTo, parseChangeTaskIDList(field.Value)...)
		}
	}
	if task.Title == "" {
		// Fall back to first H1.
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "# ") {
				task.Title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "# "))
				break
			}
		}
	}
	body := content
	if idx := strings.Index(content, "\n---"); idx >= 0 && strings.HasPrefix(content, "---") {
		rest := content[idx+4:]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			body = rest[end+4:]
		}
	}
	// Strip fenced code blocks before counting checkboxes.
	body = stripMarkdownCodeFences(body)
	matches := changeTaskCheckbox.FindAllStringSubmatch(body, -1)
	task.CheckboxTotal = len(matches)
	for _, m := range matches {
		if strings.EqualFold(m[1], "x") {
			task.CheckboxDone++
		}
	}
	task.Complete = task.CheckboxTotal > 0 && task.CheckboxDone == task.CheckboxTotal
	_ = seenKeys
	return task
}

func normalizeChangeTaskID(value string) string {
	value = strings.TrimSpace(value)
	match := changeTaskIDRE.FindStringSubmatch(value)
	if match == nil {
		return value
	}
	num := 0
	fmt.Sscanf(match[1], "%d", &num)
	return fmt.Sprintf("TASK-%03d", num)
}

func parseChangeTaskIDList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	// Support YAML inline list "[TASK-001, TASK-002]" or comma/space separated.
	value = strings.Trim(value, "[]")
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, `"'`)
		if part == "" || part == "-" {
			continue
		}
		out = append(out, normalizeChangeTaskID(part))
	}
	return out
}

func looksLikeExternalTaskRef(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "/") || strings.Contains(lower, "spec-") ||
		strings.HasPrefix(lower, "chg-") || strings.Contains(lower, "#")
}

func detectChangeTaskCycles(byID map[string]*changeTask) []string {
	var findings []string
	// Parent chain cycles.
	for start := range byID {
		seen := map[string]bool{}
		cur := start
		for cur != "" {
			if seen[cur] {
				findings = append(findings, fmt.Sprintf("parent-chain cycle involving %s", start))
				break
			}
			seen[cur] = true
			next := byID[cur]
			if next == nil {
				break
			}
			cur = next.Parent
		}
	}
	// Blocking graph cycles (blocks edges).
	adj := map[string][]string{}
	for id, task := range byID {
		adj[id] = append(adj[id], task.Blocks...)
		for _, blocker := range task.BlockedBy {
			adj[blocker] = append(adj[blocker], id)
		}
	}
	state := map[string]int{} // 0=unseen 1=stack 2=done
	var visit func(string) bool
	visit = func(id string) bool {
		state[id] = 1
		for _, next := range adj[id] {
			if _, ok := byID[next]; !ok {
				continue
			}
			switch state[next] {
			case 1:
				return true
			case 0:
				if visit(next) {
					return true
				}
			}
		}
		state[id] = 2
		return false
	}
	for id := range byID {
		if state[id] == 0 && visit(id) {
			findings = append(findings, fmt.Sprintf("blocking-graph cycle involving %s", id))
			break
		}
	}
	return findings
}

func stripMarkdownCodeFences(body string) string {
	lines := strings.Split(body, "\n")
	var out []string
	inFence := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func appendUniqueSorted(list []string, values ...string) []string {
	list = append(list, values...)
	return sortedUnique(list)
}

var changePRSubjectRE = regexp.MustCompile(`\(#(\d+)\)`)

func deriveChangePRSet(rootPath, folderAbs string) []int {
	folderRel := filepath.ToSlash(relFromRoot(rootPath, folderAbs))
	output, err := commandOutput(rootPath, "git", "log", "--format=%s", "--", folderRel)
	if err != nil {
		return nil
	}
	seen := map[int]bool{}
	var prs []int
	for _, line := range strings.Split(output, "\n") {
		for _, match := range changePRSubjectRE.FindAllStringSubmatch(line, -1) {
			n := 0
			fmt.Sscanf(match[1], "%d", &n)
			if n > 0 && !seen[n] {
				seen[n] = true
				prs = append(prs, n)
			}
		}
	}
	sort.Ints(prs)
	return prs
}

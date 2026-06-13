package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const worktreeBackPointerFile = ".moved-to"
const worktreePartialSuffix = ".partial.loaf-migrate"
const debugResolveEnv = "LOAF_DEBUG_RESOLVE"

const preA3RefusalMessageNative = `This worktree has unmigrated agentic state under .agents/.
SPEC-036 centralizes .agents/ to the main worktree, so this command is refused
until you run:

    loaf migrate worktree-storage         # dry-run preview
    loaf migrate worktree-storage --apply # perform the migration

(Tip: set LOAF_DEBUG_RESOLVE=1 to see git probe diagnostics if the refusal seems unexpected.)`

type worktreeConflictPolicy string

const (
	worktreeConflictNewer    worktreeConflictPolicy = "newer"
	worktreeConflictMain     worktreeConflictPolicy = "main"
	worktreeConflictWorktree worktreeConflictPolicy = "worktree"
)

type worktreeMigrationOptions struct {
	apply          bool
	conflictPolicy worktreeConflictPolicy
	help           bool
}

type worktreeMigrationMove struct {
	from             string
	to               string
	rel              string
	conflict         bool
	resolution       string
	resolutionReason string
}

type worktreeMigrationPlan struct {
	worktreeAgents  string
	mainAgents      string
	mainRoot        string
	moves           []worktreeMigrationMove
	backPointerPath string
}

type worktreeMigrationResult struct {
	status   string
	message  string
	plan     *worktreeMigrationPlan
	partials []string
}

func (r Runner) runMigrateWorktreeStorage(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseWorktreeMigrationArgs(args)
	if err != nil {
		return err
	}
	if options.help {
		writeWorktreeMigrationHelp(out)
		return nil
	}
	result := runWorktreeStorageMigration(runtimeRoot, options)
	switch result.status {
	case "not-in-git", "main-missing", "partial-leftover", "symlink-unsupported":
		return fmt.Errorf("%s", result.message)
	default:
		fmt.Fprintln(out, formatWorktreeMigrationResult(result, options.apply))
		return nil
	}
}

func parseWorktreeMigrationArgs(args []string) (worktreeMigrationOptions, error) {
	options := worktreeMigrationOptions{conflictPolicy: worktreeConflictNewer}
	for _, arg := range args {
		switch arg {
		case "--apply":
			options.apply = true
		case "--force-from-worktree":
			if options.conflictPolicy == worktreeConflictMain {
				return worktreeMigrationOptions{}, fmt.Errorf("--force-from-worktree and --force-from-main are mutually exclusive")
			}
			options.conflictPolicy = worktreeConflictWorktree
		case "--force-from-main":
			if options.conflictPolicy == worktreeConflictWorktree {
				return worktreeMigrationOptions{}, fmt.Errorf("--force-from-worktree and --force-from-main are mutually exclusive")
			}
			options.conflictPolicy = worktreeConflictMain
		case "--help", "-h":
			options.help = true
		default:
			return worktreeMigrationOptions{}, fmt.Errorf("unknown migrate worktree-storage option %q", arg)
		}
	}
	return options, nil
}

func writeWorktreeMigrationHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf migrate worktree-storage [options]",
		"",
		"Move a linked worktree's local .agents/ into the main worktree (SPEC-036).",
		"dry-run by default; pass --apply to mutate.",
		"",
		"Options:",
		"  --apply                Perform the migration",
		"  --force-from-worktree  On conflict, always keep the worktree-local copy",
		"  --force-from-main      On conflict, always keep the main-worktree copy",
		"  -h, --help             Show help",
		"",
		"Set LOAF_DEBUG_RESOLVE=1 to surface git probe diagnostics.",
	}, "\n"))
}

func runWorktreeStorageMigration(cwd string, options worktreeMigrationOptions) worktreeMigrationResult {
	mainRoot := findMainWorktreeRootNative(cwd)
	if mainRoot == "" {
		wtRoot := firstNonEmpty(findWorktreeRootNative(cwd), cwd)
		if pointerMainRoot := readGitdirPointerMainRootNative(wtRoot); pointerMainRoot != "" {
			exists, isDir := probeDirectory(pointerMainRoot)
			if !exists || !isDir {
				return worktreeMigrationResult{status: "main-missing", message: buildMainMissingMessageNative(pointerMainRoot, exists)}
			}
		}
		if !isInGitContextNative(cwd) {
			return worktreeMigrationResult{
				status:  "not-in-git",
				message: "loaf migrate worktree-storage: not in a git repository - this command is only meaningful inside a linked git worktree.",
			}
		}
		return worktreeMigrationResult{status: "not-in-worktree", message: "Nothing to migrate - already in the main worktree."}
	}
	exists, isDir := probeDirectory(mainRoot)
	if !exists || !isDir {
		return worktreeMigrationResult{status: "main-missing", message: buildMainMissingMessageNative(mainRoot, exists)}
	}

	wtRoot := firstNonEmpty(findWorktreeRootNative(cwd), cwd)
	localAgents := filepath.Join(wtRoot, ".agents")
	mainAgents := filepath.Join(mainRoot, ".agents")

	partials := findWorktreePartialPaths(mainAgents)
	if len(partials) > 0 {
		lines := []string{
			"Refusing to migrate: found leftover staging paths from a previous interrupted run.",
			"Resolve each path manually (delete it, or rename it into place if you trust the staged copy),",
			"then re-run the migrate command:",
			"",
		}
		for _, partial := range partials {
			lines = append(lines, "  "+partial)
		}
		lines = append(lines,
			"",
			fmt.Sprintf("These paths end in '%s' and were created by an EXDEV cross-filesystem", worktreePartialSuffix),
			"stage that did not complete the atomic rename to the final destination.",
		)
		return worktreeMigrationResult{status: "partial-leftover", message: strings.Join(lines, "\n"), partials: partials}
	}

	pointer := readBackPointerNative(localAgents)
	if pointer == mainRoot && !worktreeAgentsHasContentNative(localAgents) {
		return worktreeMigrationResult{status: "already-migrated", message: "Nothing to do - already migrated."}
	}
	if !worktreeAgentsHasContentNative(localAgents) {
		return worktreeMigrationResult{status: "no-local-agents", message: "Nothing to migrate - worktree has no local .agents/ content."}
	}

	symlinks := findWorktreeSymlinkPaths(localAgents)
	if len(symlinks) > 0 {
		lines := []string{
			"Refusing to migrate: found symlinks under worktree-local .agents/.",
			"Handle these paths manually, then re-run the migrate command:",
			"",
		}
		for _, symlink := range symlinks {
			lines = append(lines, "  "+symlink)
		}
		return worktreeMigrationResult{status: "symlink-unsupported", message: strings.Join(lines, "\n")}
	}

	plan := &worktreeMigrationPlan{
		worktreeAgents:  localAgents,
		mainAgents:      mainAgents,
		mainRoot:        mainRoot,
		moves:           planWorktreeMoves(localAgents, mainAgents, options.conflictPolicy),
		backPointerPath: filepath.Join(localAgents, worktreeBackPointerFile),
	}
	if !options.apply {
		return worktreeMigrationResult{status: "planned", message: "Dry run - re-run with --apply to perform the migration.", plan: plan}
	}

	if err := os.MkdirAll(mainAgents, 0o755); err != nil {
		return worktreeMigrationResult{status: "apply-error", message: err.Error(), plan: plan}
	}
	for _, move := range plan.moves {
		if err := applyWorktreeMove(move); err != nil {
			return worktreeMigrationResult{status: "apply-error", message: err.Error(), plan: plan}
		}
	}
	if err := os.MkdirAll(filepath.Dir(plan.backPointerPath), 0o755); err != nil {
		return worktreeMigrationResult{status: "apply-error", message: err.Error(), plan: plan}
	}
	if err := os.WriteFile(plan.backPointerPath, []byte(mainRoot+"\n"), 0o644); err != nil {
		return worktreeMigrationResult{status: "apply-error", message: err.Error(), plan: plan}
	}
	return worktreeMigrationResult{
		status:  "applied",
		message: fmt.Sprintf("Migrated %d file(s) to %s.", len(plan.moves), mainAgents),
		plan:    plan,
	}
}

func shouldRefuseCommandNative(args []string, cwd string) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args {
		switch arg {
		case "--help", "-h", "--version", "-v":
			return false
		}
	}
	switch args[0] {
	case "migrate", "help":
		return false
	default:
		return detectPreA3StateNative(cwd)
	}
}

func unknownTopLevelCommandNative(args []string) string {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") || args[0] == "help" {
		return ""
	}
	if knownTopLevelCommandNative(args[0]) {
		return ""
	}
	return args[0]
}

func knownTopLevelCommandNative(command string) bool {
	switch command {
	case "brainstorm",
		"build",
		"bundle",
		"check",
		"doctor",
		"housekeeping",
		"idea",
		"init",
		"install",
		"kb",
		"link",
		"migrate",
		"release",
		"report",
		"session",
		"setup",
		"spark",
		"spec",
		"state",
		"tag",
		"task",
		"trace",
		"version":
		return true
	default:
		return false
	}
}

func detectMainMissingForRefusalNative(startDir string) string {
	wtRoot := findWorktreeRootNative(startDir)
	if wtRoot == "" {
		return ""
	}
	if mainRoot := findMainWorktreeRootNative(wtRoot); mainRoot != "" {
		exists, isDir := probeDirectory(mainRoot)
		if !exists || !isDir {
			return buildMainMissingMessageNative(mainRoot, exists)
		}
		return ""
	}
	pointerMainRoot := readGitdirPointerMainRootNative(wtRoot)
	if pointerMainRoot == "" {
		return ""
	}
	exists, isDir := probeDirectory(pointerMainRoot)
	if !exists || !isDir {
		return buildMainMissingMessageNative(pointerMainRoot, exists)
	}
	return ""
}

func detectPreA3StateNative(startDir string) bool {
	wtRoot := findWorktreeRootNative(startDir)
	if wtRoot == "" {
		return false
	}
	localAgents := filepath.Join(wtRoot, ".agents")
	if _, err := os.Stat(localAgents); err != nil {
		return false
	}
	mainRoot := findMainWorktreeRootNative(wtRoot)
	if mainRoot == "" {
		return false
	}
	if !worktreeAgentsHasContentNative(localAgents) {
		return false
	}
	pointer := readBackPointerNative(localAgents)
	if pointer == "" {
		return true
	}
	if _, err := os.Stat(pointer); err != nil {
		return true
	}
	return normalizePathForComparisonNative(pointer) != normalizePathForComparisonNative(mainRoot)
}

func formatWorktreeMigrationResult(result worktreeMigrationResult, apply bool) string {
	var out []string
	if result.plan != nil {
		mode := "(dry-run)"
		if apply {
			mode = "--apply"
		}
		out = append(out,
			fmt.Sprintf("loaf migrate worktree-storage %s", mode),
			"",
			fmt.Sprintf("  from %s", result.plan.worktreeAgents),
			fmt.Sprintf("  to   %s", result.plan.mainAgents),
			"",
		)
		if len(result.plan.moves) == 0 {
			out = append(out, "  ok Nothing to move.")
		} else {
			for _, move := range result.plan.moves {
				if move.conflict {
					out = append(out, fmt.Sprintf("  [conflict->%s] %s", strings.TrimPrefix(move.resolution, "keep-"), move.rel))
					out = append(out, fmt.Sprintf("      %s", move.resolutionReason))
				} else {
					out = append(out, fmt.Sprintf("  -> %s", move.rel))
				}
			}
		}
		out = append(out, "", fmt.Sprintf("  back-pointer: %s", result.plan.backPointerPath), "")
	}
	prefix := "->"
	if result.status == "applied" {
		prefix = "ok"
	}
	out = append(out, fmt.Sprintf("  %s %s", prefix, result.message))
	return strings.Join(out, "\n")
}

func findMainWorktreeRootNative(startDir string) string {
	gitDir, err := gitOutput(startDir, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		debugResolveNative(err)
		return ""
	}
	commonDir, err := gitOutput(startDir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		debugResolveNative(err)
		return ""
	}
	gitAbs := absoluteGitProbePath(startDir, strings.TrimSpace(gitDir))
	commonAbs := absoluteGitProbePath(startDir, strings.TrimSpace(commonDir))
	if normalizePathForComparisonNative(gitAbs) == normalizePathForComparisonNative(commonAbs) {
		return ""
	}
	commonCanonical := realpathOrSelfNative(commonAbs)
	if filepath.Base(commonCanonical) == ".git" {
		return filepath.Dir(commonCanonical)
	}
	return ""
}

func absoluteGitProbePath(startDir string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(filepath.Join(startDir, path))
	if err != nil {
		return filepath.Join(startDir, path)
	}
	return abs
}

func gitOutput(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}

func debugResolveNative(err error) {
	if !debugResolveEnabledNative() {
		return
	}
	fmt.Fprintf(os.Stderr, "%s: findMainWorktreeRoot fell back to parent-walk (error: %v)\n", debugResolveEnv, err)
}

func debugResolveEnabledNative() bool {
	switch strings.ToLower(os.Getenv(debugResolveEnv)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizePathForComparisonNative(path string) string {
	return realpathOrSelfNative(path)
}

func realpathOrSelfNative(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

func readGitdirPointerMainRootNative(wtRoot string) string {
	body, err := os.ReadFile(filepath.Join(wtRoot, ".git"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "gitdir:") {
			continue
		}
		gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
		needle := string(filepath.Separator) + ".git" + string(filepath.Separator) + "worktrees" + string(filepath.Separator)
		idx := strings.LastIndex(gitdir, needle)
		if idx <= 0 {
			return ""
		}
		return gitdir[:idx]
	}
	return ""
}

func probeDirectory(path string) (exists bool, isDirectory bool) {
	info, err := os.Stat(path)
	if err != nil {
		return false, false
	}
	return true, info.IsDir()
}

func buildMainMissingMessageNative(mainPath string, exists bool) string {
	cause := fmt.Sprintf("Main worktree at %s not found.", mainPath)
	if exists {
		cause = fmt.Sprintf("Main worktree at %s is not a directory.", mainPath)
	}
	return strings.Join([]string{
		cause,
		"",
		"The .agents/ migration target is unreachable. This usually means the main",
		"worktree was removed (`git worktree remove`) or its directory was deleted.",
		"Migration cannot proceed without a valid target.",
		"",
		"To resolve:",
		"- Restore the main worktree, OR",
		"- Check `git worktree list` and re-initialize the project layout you expect",
	}, "\n")
}

func findWorktreeRootNative(startDir string) string {
	current := startDir
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func isInGitContextNative(startDir string) bool {
	return findWorktreeRootNative(startDir) != ""
}

func readBackPointerNative(worktreeAgentsDir string) string {
	body, err := os.ReadFile(filepath.Join(worktreeAgentsDir, worktreeBackPointerFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func worktreeAgentsHasContentNative(worktreeAgentsDir string) bool {
	files := enumerateWorktreeAgentFiles(worktreeAgentsDir)
	return len(files) > 0
}

func enumerateWorktreeAgentFiles(dir string) []string {
	var files []string
	var walk func(string, string)
	walk = func(current string, rel string) {
		entries, err := os.ReadDir(current)
		if err != nil {
			return
		}
		for _, entry := range entries {
			relPath := entry.Name()
			if rel != "" {
				relPath = filepath.Join(rel, entry.Name())
			}
			if rel == "" && entry.Name() == worktreeBackPointerFile {
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 {
				files = append(files, filepath.ToSlash(relPath))
				continue
			}
			if entry.IsDir() {
				walk(filepath.Join(current, entry.Name()), relPath)
				continue
			}
			if entry.Type().IsRegular() {
				files = append(files, filepath.ToSlash(relPath))
			}
		}
	}
	walk(dir, "")
	sort.Strings(files)
	return files
}

func findWorktreeSymlinkPaths(dir string) []string {
	var paths []string
	var walk func(string, string)
	walk = func(current string, rel string) {
		entries, err := os.ReadDir(current)
		if err != nil {
			return
		}
		for _, entry := range entries {
			relPath := entry.Name()
			if rel != "" {
				relPath = filepath.Join(rel, entry.Name())
			}
			if rel == "" && entry.Name() == worktreeBackPointerFile {
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 {
				paths = append(paths, filepath.ToSlash(relPath))
				continue
			}
			if entry.IsDir() {
				walk(filepath.Join(current, entry.Name()), relPath)
			}
		}
	}
	walk(dir, "")
	sort.Strings(paths)
	return paths
}

func findWorktreePartialPaths(dir string) []string {
	var paths []string
	var walk func(string)
	walk = func(current string) {
		entries, err := os.ReadDir(current)
		if err != nil {
			return
		}
		for _, entry := range entries {
			abs := filepath.Join(current, entry.Name())
			if strings.HasSuffix(entry.Name(), worktreePartialSuffix) {
				paths = append(paths, abs)
				continue
			}
			if entry.IsDir() {
				walk(abs)
			}
		}
	}
	walk(dir)
	sort.Strings(paths)
	return paths
}

func planWorktreeMoves(worktreeAgents string, mainAgents string, policy worktreeConflictPolicy) []worktreeMigrationMove {
	relFiles := enumerateWorktreeAgentFiles(worktreeAgents)
	moves := make([]worktreeMigrationMove, 0, len(relFiles))
	for _, rel := range relFiles {
		from := filepath.Join(worktreeAgents, filepath.FromSlash(rel))
		to := filepath.Join(mainAgents, filepath.FromSlash(rel))
		move := worktreeMigrationMove{from: from, to: to, rel: rel}
		if _, err := os.Stat(to); err == nil {
			move.conflict = true
			switch {
			case filesHaveSameContent(from, to):
				move.resolution = "keep-main"
				move.resolutionReason = "identical content"
			case policy == worktreeConflictWorktree:
				move.resolution = "keep-worktree"
				move.resolutionReason = "forced by --force-from-worktree"
			case policy == worktreeConflictMain:
				move.resolution = "keep-main"
				move.resolutionReason = "forced by --force-from-main"
			default:
				fromMtime := fileModTimeUnixNano(from)
				toMtime := fileModTimeUnixNano(to)
				if fromMtime > toMtime {
					move.resolution = "keep-worktree"
					move.resolutionReason = "worktree mtime > main mtime"
				} else {
					move.resolution = "keep-main"
					move.resolutionReason = "main mtime >= worktree mtime"
				}
			}
		}
		moves = append(moves, move)
	}
	return moves
}

func filesHaveSameContent(a string, b string) bool {
	left, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	right, err := os.ReadFile(b)
	if err != nil {
		return false
	}
	return bytes.Equal(left, right)
}

func fileModTimeUnixNano(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

func applyWorktreeMove(move worktreeMigrationMove) error {
	if move.conflict {
		if move.resolution == "keep-main" {
			return os.Remove(move.from)
		}
		if err := os.Remove(move.to); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(move.to), 0o755); err != nil {
		return err
	}
	if err := os.Rename(move.from, move.to); err != nil {
		if !isCrossDeviceRename(err) {
			return err
		}
		partial := move.to + worktreePartialSuffix
		if err := os.RemoveAll(partial); err != nil {
			return err
		}
		if err := copyFilePreservingMode(move.from, partial); err != nil {
			return err
		}
		if err := os.Rename(partial, move.to); err != nil {
			return err
		}
		if err := os.Remove(move.from); err != nil {
			return err
		}
	}
	return nil
}

func isCrossDeviceRename(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return errors.Is(linkErr.Err, syscall.EXDEV)
	}
	return false
}

func copyFilePreservingMode(from string, to string) error {
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	target, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return err
	}
	return target.Close()
}

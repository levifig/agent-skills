package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

type issueStartResult struct {
	ContractVersion    int         `json:"contract_version"`
	DatabaseScope      string      `json:"database_scope"`
	DatabasePath       string      `json:"database_path"`
	ProjectID          string      `json:"project_id"`
	ProjectName        string      `json:"project_name"`
	ProjectCurrentPath string      `json:"project_current_path"`
	Issue              state.Issue `json:"issue"`
	Requested          string      `json:"requested"`
	Joined             bool        `json:"joined"`
	Branch             string      `json:"branch"`
	Worktree           string      `json:"worktree"`
	Base               string      `json:"base"`
}

type issueStopResult struct {
	ContractVersion    int         `json:"contract_version"`
	DatabaseScope      string      `json:"database_scope"`
	DatabasePath       string      `json:"database_path"`
	ProjectID          string      `json:"project_id"`
	ProjectName        string      `json:"project_name"`
	ProjectCurrentPath string      `json:"project_current_path"`
	Issue              state.Issue `json:"issue"`
	Branch             string      `json:"branch"`
	Worktree           string      `json:"worktree"`
	AlreadyGone        bool        `json:"already_gone"`
}

type issueStopOptions struct {
	jsonOutput bool
	force      bool
	ref        string
}

func writeIssueStartHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue start <ref> [--json]", "Start or join the shippable root's branch and worktree. Walks parent_id to the root; only the root records started_branch and started_worktree. A descendant becomes active without its own worktree.",
		"--json       Output the root issue, requested ref, joined flag, branch, worktree, base, global database scope, and project identity as JSON")
}

func writeIssueStopHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue stop <ref> [--force] [--json]", "Remove the issue worktree and clear the started workspace on the row. Keeps the branch. Does not change status. Descendants that do not own a worktree must stop the root.",
		"--force      Remove a dirty worktree",
		"--json       Output the stopped issue, branch, worktree, already-gone flag, global database scope, and project identity as JSON")
}

func (r Runner) runIssueStart(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("issue start", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue start", runtime)
	if err != nil {
		return err
	}
	if providerQualifiedRefCommand(ref) {
		return r.runIssueStartContract(context.Background(), projectRoot, ref, jsonOutput, out)
	}
	repoRoot := projectRoot.Path()
	if !gitRepoAt(repoRoot) {
		return fmt.Errorf("issue start requires a git repository")
	}

	resolver := state.PathResolver{StateHome: r.StateHome}
	ctx := context.Background()
	requested, err := state.GetIssue(ctx, projectRoot, resolver, ref)
	if err != nil {
		return err
	}
	if err := refuseIssueStart(requested); err != nil {
		return err
	}
	owner, err := state.IssueRoot(ctx, projectRoot, resolver, requested.ID)
	if err != nil {
		return err
	}

	if requested.ID != owner.ID && issueIsStarted(owner) {
		if err := refuseIssueStartLifecycle(owner); err != nil {
			return err
		}
		if err := refuseMissingStartedWorktree(owner); err != nil {
			return err
		}
		if err := activateIssue(ctx, projectRoot, resolver, requested); err != nil {
			return err
		}
		base, err := resolveIssueStartBase(ctx, projectRoot, resolver, owner, repoRoot)
		if err != nil {
			return err
		}
		return writeIssueStartResult(out, projectRoot, resolver, owner.ID, requested, owner.StartedBranch, owner.StartedWorktree, base, true, jsonOutput)
	}

	if requested.ID != owner.ID {
		if err := refuseIssueStart(owner); err != nil {
			return err
		}
	}

	base, err := resolveIssueStartBase(ctx, projectRoot, resolver, owner, repoRoot)
	if err != nil {
		return err
	}
	listed, err := state.ListIssues(ctx, projectRoot, resolver, state.IssueListOptions{Archived: true})
	if err != nil {
		return err
	}
	branch, err := resolveIssueStartBranch(owner, listed.Issues, repoRoot)
	if err != nil {
		return err
	}
	worktree := issueWorktreePath(repoRoot, branch)
	if _, err := os.Stat(worktree); err == nil {
		return fmt.Errorf("worktree path %s already exists", worktree)
	}

	createdBranch, err := addIssueWorktree(repoRoot, worktree, branch, base)
	if err != nil {
		return err
	}

	if _, err := state.UpdateIssue(ctx, projectRoot, resolver, state.IssueUpdateOptions{
		Ref:             owner.ID,
		Status:          state.IssueStatusActive,
		SetStatus:       true,
		StartedBranch:   branch,
		StartedWorktree: worktree,
		SetStarted:      true,
	}); err != nil {
		return wrapIssueStartUpdateError(err, rollbackIssueWorktree(repoRoot, worktree, branch, createdBranch), worktree, branch)
	}
	if requested.ID != owner.ID {
		if err := activateIssue(ctx, projectRoot, resolver, requested); err != nil {
			return err
		}
	}

	return writeIssueStartResult(out, projectRoot, resolver, owner.ID, requested, branch, worktree, base, false, jsonOutput)
}

func activateIssue(ctx context.Context, root project.Root, resolver state.PathResolver, issue state.Issue) error {
	_, err := state.UpdateIssue(ctx, root, resolver, state.IssueUpdateOptions{
		Ref:       issue.ID,
		Status:    state.IssueStatusActive,
		SetStatus: true,
	})
	return err
}

func writeIssueStartResult(out io.Writer, root project.Root, resolver state.PathResolver, ownerRef string, requested state.Issue, branch, worktree, base string, joined, jsonOutput bool) error {
	shown, err := state.ShowIssue(context.Background(), root, resolver, ownerRef)
	if err != nil {
		return err
	}
	result := issueStartResult{
		ContractVersion:    shown.ContractVersion,
		DatabaseScope:      shown.DatabaseScope,
		DatabasePath:       shown.DatabasePath,
		ProjectID:          shown.ProjectID,
		ProjectName:        shown.ProjectName,
		ProjectCurrentPath: shown.ProjectCurrentPath,
		Issue:              shown.Issue,
		Requested:          issueDisplayRef(requested),
		Joined:             joined,
		Branch:             branch,
		Worktree:           worktree,
		Base:               base,
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	if joined {
		fmt.Fprintf(out, "joined issue %s\n", issueDisplayRef(result.Issue))
	} else {
		fmt.Fprintf(out, "started issue %s\n", issueDisplayRef(result.Issue))
	}
	if requested.ID != result.Issue.ID {
		fmt.Fprintf(out, "activated: %s\n", issueDisplayRef(requested))
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "branch: %s\n", result.Branch)
	fmt.Fprintf(out, "worktree: %s\n", result.Worktree)
	fmt.Fprintf(out, "base: %s\n", result.Base)
	fmt.Fprintf(out, "status: %s\n", result.Issue.Status)
	return nil
}

func (r Runner) runIssueStop(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueStopArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue stop", runtime)
	if err != nil {
		return err
	}
	if providerQualifiedRefCommand(options.ref) {
		return r.runIssueStopContract(context.Background(), projectRoot, options.ref, options.force, options.jsonOutput, out)
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	issue, err := state.GetIssue(context.Background(), projectRoot, resolver, options.ref)
	if err != nil {
		return err
	}
	if !issueIsStarted(issue) {
		rootIssue, err := state.IssueRoot(context.Background(), projectRoot, resolver, issue.ID)
		if err != nil {
			return err
		}
		if rootIssue.ID != issue.ID {
			return fmt.Errorf("issue %s does not own a worktree; stop %s", issueDisplayRef(issue), issueDisplayRef(rootIssue))
		}
		return fmt.Errorf("issue %s is not started", issueDisplayRef(issue))
	}

	savedBranch := issue.StartedBranch
	savedWorktree := issue.StartedWorktree

	cleared, err := state.UpdateIssue(context.Background(), projectRoot, resolver, state.IssueUpdateOptions{
		Ref:        issue.ID,
		SetStarted: true,
	})
	if err != nil {
		return err
	}

	alreadyGone, err := removeIssueWorktreeFn(projectRoot.Path(), savedWorktree, options.force)
	if err != nil {
		if _, restoreErr := state.UpdateIssue(context.Background(), projectRoot, resolver, state.IssueUpdateOptions{
			Ref:             issue.ID,
			StartedBranch:   savedBranch,
			StartedWorktree: savedWorktree,
			SetStarted:      true,
		}); restoreErr != nil {
			return fmt.Errorf("remove worktree: %w (also failed to restore started fields: %v)", err, restoreErr)
		}
		return err
	}

	shown, err := state.ShowIssue(context.Background(), projectRoot, resolver, cleared.ID)
	if err != nil {
		return err
	}
	result := issueStopResult{
		ContractVersion:    shown.ContractVersion,
		DatabaseScope:      shown.DatabaseScope,
		DatabasePath:       shown.DatabasePath,
		ProjectID:          shown.ProjectID,
		ProjectName:        shown.ProjectName,
		ProjectCurrentPath: shown.ProjectCurrentPath,
		Issue:              shown.Issue,
		Branch:             issue.StartedBranch,
		Worktree:           issue.StartedWorktree,
		AlreadyGone:        alreadyGone,
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "stopped issue %s\n", issueDisplayRef(result.Issue))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if alreadyGone {
		fmt.Fprintf(out, "worktree already gone: %s\n", result.Worktree)
	} else {
		fmt.Fprintf(out, "removed worktree: %s\n", result.Worktree)
	}
	fmt.Fprintf(out, "kept branch: %s\n", result.Branch)
	fmt.Fprintf(out, "status: %s\n", result.Issue.Status)
	return nil
}

func parseIssueStopArgs(args []string) (issueStopOptions, error) {
	var options issueStopOptions
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		case "--force":
			options.force = true
		default:
			if strings.HasPrefix(arg, "-") {
				return issueStopOptions{}, fmt.Errorf("unknown option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) != 1 {
		return issueStopOptions{}, fmt.Errorf("issue stop requires an issue ref")
	}
	options.ref = positional[0]
	return options, nil
}

func refuseIssueStart(issue state.Issue) error {
	if issueIsStarted(issue) {
		return fmt.Errorf("issue %s is already started at %s", issueDisplayRef(issue), firstNonEmpty(issue.StartedWorktree, issue.StartedBranch))
	}
	return refuseIssueStartLifecycle(issue)
}

func refuseIssueStartLifecycle(issue state.Issue) error {
	if issue.ArchivedAt != "" {
		return fmt.Errorf("issue %s is archived", issueDisplayRef(issue))
	}
	if issueStartRefusedStatus(issue.Status) {
		return fmt.Errorf("issue %s is %s; start refuses terminal statuses", issueDisplayRef(issue), issue.Status)
	}
	return nil
}

func refuseMissingStartedWorktree(issue state.Issue) error {
	path := strings.TrimSpace(issue.StartedWorktree)
	ref := issueDisplayRef(issue)
	if path == "" {
		return fmt.Errorf("issue %s is started but has no worktree path; stop %s", ref, ref)
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("issue %s is started at %s but the worktree is missing; stop %s", ref, path, ref)
		}
		return fmt.Errorf("stat worktree %s: %w", path, err)
	}
	return nil
}

func issueIsStarted(issue state.Issue) bool {
	return strings.TrimSpace(issue.StartedBranch) != "" || strings.TrimSpace(issue.StartedWorktree) != ""
}

func issueStartRefusedStatus(status string) bool {
	switch status {
	case state.IssueStatusDone, state.IssueStatusCancelled, state.IssueStatusDuplicate:
		return true
	default:
		return false
	}
}

func resolveIssueStartBase(ctx context.Context, root project.Root, resolver state.PathResolver, issue state.Issue, repoRoot string) (string, error) {
	ancestor, found, err := state.NearestStartedAncestor(ctx, root, resolver, issue.ID)
	if err != nil {
		return "", err
	}
	if found {
		if strings.TrimSpace(ancestor.StartedBranch) == "" {
			return "", fmt.Errorf("started ancestor %s has no started_branch", issueDisplayRef(ancestor))
		}
		return ancestor.StartedBranch, nil
	}
	base := resolveReleaseDefaultBranch(repoRoot)
	if base == "" {
		return "", fmt.Errorf("could not resolve repository default branch")
	}
	return qualifyIssueStartBase(repoRoot, base), nil
}

func qualifyIssueStartBase(repoRoot, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	if gitRefExists(repoRoot, "refs/heads/"+base) {
		return base
	}
	remote := "origin/" + base
	if gitRefExists(repoRoot, "refs/remotes/"+remote) {
		return remote
	}
	return base
}

func issueStartSlug(issue state.Issue) string {
	name := strings.TrimSpace(issue.Alias)
	if name == "" {
		name = issue.ID
	}
	return strings.ToLower(name)
}

func issueStartBranch(issue state.Issue) string {
	return "issue/" + issueStartSlug(issue)
}

func issueStartBranchSuffix(id string) string {
	_, rest, ok := strings.Cut(id, "_")
	if ok && len(rest) >= 8 {
		return rest[:8]
	}
	if len(id) >= 8 {
		return id[len(id)-8:]
	}
	return id
}

func issueStartBranchDisambiguated(issue state.Issue) string {
	return issueStartBranch(issue) + "-" + issueStartBranchSuffix(issue.ID)
}

func resolveIssueStartBranch(issue state.Issue, issues []state.Issue, repoRoot string) (string, error) {
	preferred := issueStartBranch(issue)
	if live := startedBranchClaimant(issues, preferred, issue.ID); live != nil {
		return requireUnclaimedIssueBranch(issueStartBranchDisambiguated(issue), issue, issues)
	}
	if gitRefExists(repoRoot, "refs/heads/"+preferred) {
		if owner := issueSharingStartSlug(issue, issues); owner != nil {
			return requireUnclaimedIssueBranch(issueStartBranchDisambiguated(issue), issue, issues)
		}
	}
	return preferred, nil
}

func requireUnclaimedIssueBranch(branch string, issue state.Issue, issues []state.Issue) (string, error) {
	if live := startedBranchClaimant(issues, branch, issue.ID); live != nil {
		return "", fmt.Errorf("branch %s collides for issues %s and %s", branch, issueDisplayRef(*live), issueDisplayRef(issue))
	}
	return branch, nil
}

func startedBranchClaimant(issues []state.Issue, branch, exceptID string) *state.Issue {
	for i := range issues {
		if issues[i].ID == exceptID {
			continue
		}
		if strings.TrimSpace(issues[i].StartedBranch) == branch {
			return &issues[i]
		}
	}
	return nil
}

func issueSharingStartSlug(issue state.Issue, issues []state.Issue) *state.Issue {
	slug := issueStartSlug(issue)
	for i := range issues {
		if issues[i].ID == issue.ID {
			continue
		}
		if issueStartSlug(issues[i]) == slug {
			return &issues[i]
		}
	}
	return nil
}

func issueWorktreePath(repoRoot, branch string) string {
	slug := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(filepath.Dir(repoRoot), filepath.Base(repoRoot)+"-wt", slug)
}

func gitRepoAt(root string) bool {
	_, err := gitOutput(root, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

func gitRefExists(root, ref string) bool {
	_, err := gitOutput(root, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func addIssueWorktree(repoRoot, worktree, branch, base string) (createdBranch bool, err error) {
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		return false, fmt.Errorf("create worktree parent: %w", err)
	}
	if gitRefExists(repoRoot, "refs/heads/"+branch) {
		if _, err := gitRun(repoRoot, "worktree", "add", worktree, branch); err != nil {
			return false, fmt.Errorf("git worktree add: %w", err)
		}
		return false, nil
	}
	if _, err := gitRun(repoRoot, "worktree", "add", worktree, "-b", branch, base); err != nil {
		return false, fmt.Errorf("git worktree add: %w", err)
	}
	return true, nil
}

func rollbackIssueWorktree(repoRoot, worktree, branch string, createdBranch bool) error {
	var cleanupErr error
	note := func(err error) {
		if err == nil {
			return
		}
		if cleanupErr == nil {
			cleanupErr = err
			return
		}
		cleanupErr = fmt.Errorf("%w; %v", cleanupErr, err)
	}
	if _, err := gitRun(repoRoot, "worktree", "remove", "--force", worktree); err != nil {
		note(fmt.Errorf("git worktree remove: %w", err))
		if rmErr := os.RemoveAll(worktree); rmErr != nil {
			note(fmt.Errorf("remove leftover directory: %w", rmErr))
		}
	} else {
		_ = os.RemoveAll(worktree)
	}
	if _, err := gitRun(repoRoot, "worktree", "prune", "--expire", "now"); err != nil {
		note(fmt.Errorf("git worktree prune: %w", err))
	}
	if createdBranch {
		if _, err := gitRun(repoRoot, "branch", "-D", branch); err != nil {
			note(fmt.Errorf("git branch -D: %w", err))
		}
	}
	if cleanupErr != nil {
		return fmt.Errorf("leftover worktree %s branch %s: %w", worktree, branch, cleanupErr)
	}
	return nil
}

func wrapIssueStartUpdateError(updateErr, cleanupErr error, worktree, branch string) error {
	if cleanupErr == nil {
		return updateErr
	}
	return fmt.Errorf("%w (also failed to clean up leftover worktree %s branch %s: %v)", updateErr, worktree, branch, cleanupErr)
}

var removeIssueWorktreeFn = removeIssueWorktree

func removeIssueWorktree(repoRoot, worktree string, force bool) (alreadyGone bool, err error) {
	if strings.TrimSpace(worktree) == "" {
		return true, nil
	}
	if _, statErr := os.Stat(worktree); os.IsNotExist(statErr) {
		if _, pruneErr := gitRun(repoRoot, "worktree", "prune", "--expire", "now"); pruneErr != nil {
			return false, fmt.Errorf("git worktree prune: %w", pruneErr)
		}
		return true, nil
	}
	if !force && gitWorktreeDirty(worktree) {
		return false, fmt.Errorf("worktree %s is dirty; pass --force to remove it", worktree)
	}
	args := []string{"worktree", "remove", worktree}
	if force {
		args = []string{"worktree", "remove", "--force", worktree}
	}
	if _, err := gitRun(repoRoot, args...); err != nil {
		return false, fmt.Errorf("git worktree remove: %w", err)
	}
	return false, nil
}

func gitRun(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed != "" {
			return trimmed, fmt.Errorf("%w: %s", err, trimmed)
		}
		return trimmed, err
	}
	return trimmed, nil
}

func gitWorktreeDirty(worktree string) bool {
	out, err := gitOutput(worktree, "status", "--porcelain")
	if err != nil {
		return true
	}
	return strings.TrimSpace(out) != ""
}

func markStartedWorktreeLiveness(issues []state.Issue) {
	for i := range issues {
		path := strings.TrimSpace(issues[i].StartedWorktree)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			issues[i].WorktreeMissing = true
		}
	}
}

func writeIssueStartedList(out io.Writer, result state.IssueListResult) {
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Issues) == 0 {
		fmt.Fprintln(out, "no started issues")
		return
	}
	for _, issue := range result.Issues {
		line := fmt.Sprintf("%s\t%s\t%s\t%s", issueDisplayRef(issue), issue.Title, issue.StartedBranch, issue.StartedWorktree)
		if issue.WorktreeMissing {
			line += "\t(missing)"
		}
		fmt.Fprintln(out, line)
	}
}

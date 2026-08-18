package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

type issueNewOptions struct {
	jsonOutput bool
	status     string
	create     state.IssueCreateOptions
	body       bodyInputOptions
}

type issueListOptions struct {
	jsonOutput bool
	filters    state.IssueListOptions
}

type issueEditOptions struct {
	jsonOutput bool
	ref        string
	body       bodyInputOptions
}

type issueStatusOptions struct {
	jsonOutput  bool
	ref         string
	status      string
	duplicateOf string
}

type issueDodAddOptions struct {
	jsonOutput bool
	ref        string
	input      state.IssueCriterionInput
}

type issueLinkOptions struct {
	jsonOutput       bool
	from             string
	to               string
	relationshipType string
	remove           bool
}

func (r Runner) runIssue(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeIssueHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"new":       writeIssueNewHelp,
		"absorb":    writeIssueAbsorbHelp,
		"show":      writeIssueShowHelp,
		"list":      writeIssueListHelp,
		"tree":      writeIssueTreeHelp,
		"frontier":  writeIssueFrontierHelp,
		"start":     writeIssueStartHelp,
		"stop":      writeIssueStopHelp,
		"edit":      writeIssueEditHelp,
		"retitle":   writeIssueRetitleHelp,
		"status":    writeIssueStatusHelp,
		"dod":       writeIssueDodHelp,
		"promote":   writeIssuePromoteHelp,
		"check":     writeIssueCheckHelp,
		"verify":    writeIssueVerifyHelp,
		"bucket":    writeIssueBucketHelp,
		"link":      writeIssueLinkHelp,
		"render":    writeIssueRenderHelp,
		"identity":  writeIssueIdentityHelp,
		"export":    writeIssueExportHelp,
		"pull":      writeIssuePullHelp,
		"push":      writeIssuePushHelp,
		"reconcile": writeIssueReconcileHelp,
	}) {
		return nil
	}
	if args[0] == "dod" && writeNestedHelp(out, args[1:], map[string]func(io.Writer){
		"add":     writeIssueDodAddHelp,
		"list":    writeIssueDodListHelp,
		"remove":  writeIssueDodRemoveHelp,
		"claim":   writeIssueDodClaimHelp,
		"unclaim": writeIssueDodUnclaimHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "new":
		return r.runIssueNew(args[1:], out, runtime)
	case "absorb":
		return r.runIssueAbsorb(args[1:], out, runtime)
	case "show":
		return r.runIssueShow(args[1:], out, runtime)
	case "list":
		return r.runIssueList(args[1:], out, runtime)
	case "tree":
		return r.runIssueTree(args[1:], out, runtime)
	case "frontier":
		return r.runIssueFrontier(args[1:], out, runtime)
	case "start":
		return r.runIssueStart(args[1:], out, runtime)
	case "stop":
		return r.runIssueStop(args[1:], out, runtime)
	case "edit":
		return r.runIssueEdit(args[1:], out, runtime)
	case "retitle":
		return r.runIssueRetitle(args[1:], out, runtime)
	case "status":
		return r.runIssueStatus(args[1:], out, runtime)
	case "dod":
		return r.runIssueDod(args[1:], out, runtime)
	case "promote":
		return r.runIssuePromote(args[1:], out, runtime)
	case "check":
		return r.runIssueCheck(args[1:], out, runtime)
	case "verify":
		return r.runIssueVerify(args[1:], out, runtime)
	case "bucket":
		return r.runIssueBucket(args[1:], out, runtime)
	case "link":
		return r.runIssueLink(args[1:], out, runtime)
	case "render":
		return r.runIssueRender(args[1:], out, runtime)
	case "identity":
		return r.runIssueIdentity(args[1:], out, runtime)
	case "export":
		return r.runIssueExport(args[1:], out, runtime)
	case "pull":
		return r.runIssuePull(args[1:], out, runtime)
	case "push":
		return r.runIssuePush(args[1:], out, runtime)
	case "reconcile":
		return r.runIssueReconcile(args[1:], out, runtime)
	default:
		return unknownSubcommandError("issue", args[0])
	}
}

func writeIssueHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf issue <subcommand> [options]", "Manage issues in native SQLite state.", []subcommandHelpItem{
		{Name: "new", Summary: "Create an issue"},
		{Name: "absorb", Summary: "Mint an issue from leftover SQLite work, or dismiss the source"},
		{Name: "show", Summary: "Show one issue"},
		{Name: "list", Summary: "List project issues"},
		{Name: "tree", Summary: "Print a recursive issue tree"},
		{Name: "frontier", Summary: "List unblocked pick-up-next issues"},
		{Name: "start", Summary: "Start or join the shippable root workspace"},
		{Name: "stop", Summary: "Remove a started worktree; descendants must stop the root"},
		{Name: "edit", Summary: "Replace an issue body"},
		{Name: "retitle", Summary: "Replace an issue title"},
		{Name: "status", Summary: "Set an issue status"},
		{Name: "dod", Summary: "Manage definition-of-done criteria"},
		{Name: "promote", Summary: "Promote a criterion into a child issue"},
		{Name: "check", Summary: "Derive readiness from the issue row"},
		{Name: "verify", Summary: "Run V-tier criteria from the repository root"},
		{Name: "bucket", Summary: "Set an advisory Now/Next/Later label"},
		{Name: "link", Summary: "Create or remove an issue relationship"},
		{Name: "render", Summary: "Emit a paste-ready PR body"},
		{Name: "identity", Summary: "Show, define, or align the local issue prefix"},
		{Name: "export", Summary: "Export issues, identity, criteria, claims, and relationships as JSON"},
		{Name: "pull", Summary: "Adopt an existing Linear issue"},
		{Name: "push", Summary: "Write the local render and status to Linear"},
		{Name: "reconcile", Summary: "Compare local and Linear and surface conflicts"},
	})
}

func writeIssueNewHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue new <title> [--body <text>|--body -|--body-file <path>|--message <text>] [--kind delivery|decision] [--parent <ref>] [--fog <text>] [--status <status>] [--json]", "Create an issue in SQLite state.",
		"--body        Inline issue body, or '-' to read from stdin",
		"--body-file   Read the issue body from a UTF-8 file",
		"--message     Inline issue body; lower precedence than --body-file and --body -",
		"--kind        Issue kind: delivery (default) or decision",
		"--parent      Parent issue ref",
		"--fog         Questions not yet sharp enough to be issues",
		"--status      Write status after create: "+strings.Join(state.IssueWriteStatuses(), ", ")+"; still records the initial triage event",
		"--json        Output the created issue, global database scope, and project identity as JSON")
}

func writeIssueShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue show <ref> [--json]", "Show one issue by alias or opaque id.", "--json       Output issue details, parent, children, bucket, global database scope, and project identity as JSON")
}

func writeIssueListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue list [--status <status>] [--kind delivery|decision] [--archived] [--started] [--json]", "List project issues. Archived issues are hidden by default.",
		"--status     Filter by status: triage, backlog, todo, active, done, cancelled, duplicate",
		"--kind       Filter by kind: delivery or decision",
		"--archived   Include archived issues",
		"--started    List issues with a recorded started worktree",
		"--json       Output issues, global database scope, and project identity as JSON")
}

func writeIssueTreeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue tree [<ref>] [--archived] [--json]", "Print a recursive issue tree from a ref, or the whole project when omitted.",
		"--archived   Include archived issues",
		"--json       Output the tree, global database scope, and project identity as JSON")
}

func writeIssueFrontierHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue frontier [--json]", "List non-archived triage/backlog/todo issues that are not blocked. Derived at read time.",
		"--json       Output frontier issues, global database scope, and project identity as JSON")
}

func writeIssueEditHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue edit <ref> [options]", "Replace an issue body through the shared body-edit path.",
		"--body-file  Read the issue body from a file",
		"--body -     Read the issue body from stdin",
		"--message    Use the given text as the issue body",
		"--json       Output the edited issue, global database scope, and project identity as JSON")
}

func writeIssueStatusHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue status <ref> <status> [--duplicate-of <ref>] [--json]", "Set issue status. Write statuses (triage, backlog, todo, active, done) update in place; cancelled and duplicate archive through the remove path.",
		"--duplicate-of  Surviving issue required when status is duplicate",
		"--json          Output the updated issue, global database scope, and project identity as JSON")
}

func writeIssueDodHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf issue dod <subcommand> [options]", "Manage definition-of-done criteria on an issue.", []subcommandHelpItem{
		{Name: "add", Summary: "Add a criterion"},
		{Name: "list", Summary: "List criteria"},
		{Name: "remove", Summary: "Remove a criterion by position"},
		{Name: "claim", Summary: "Claim a child criterion against a parent criterion"},
		{Name: "unclaim", Summary: "Remove a child-to-parent criterion claim"},
	})
}

func writeIssueDodAddHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue dod add <ref> <text> [--command <cmd>] [--expect <expect>] [--tier V|H] [--serves <parent-position>] [--json]", "Add a definition-of-done criterion. V tier is used when --command is present, otherwise H, unless --tier overrides.",
		"--command    Verification command (implies tier V)",
		"--expect     Verification expect grammar (exit N, contains <text>)",
		"--tier       Override criterion tier: V or H",
		"--serves     Parent criterion position this child criterion claims",
		"--json       Output the updated issue, global database scope, and project identity as JSON")
}

func writeIssueDodClaimHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue dod claim <child> <child-position> <parent-position> [--json]", "Record that the child's criterion at child-position serves the parent's criterion at parent-position.",
		"--json       Output the updated child issue, global database scope, and project identity as JSON")
}

func writeIssueDodUnclaimHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue dod unclaim <child> <child-position> <parent-position> [--json]", "Remove the claim from the child's criterion to the parent's criterion.",
		"--json       Output the updated child issue, global database scope, and project identity as JSON")
}

func writeIssueDodListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue dod list <ref> [--json]", "List definition-of-done criteria for one issue.", "--json       Output the issue and criteria, global database scope, and project identity as JSON")
}

func writeIssueDodRemoveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue dod remove <ref> <position> [--json]", "Remove the criterion at the 1-based position.", "--json       Output the updated issue, global database scope, and project identity as JSON")
}

func writeIssuePromoteHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue promote <ref> <position> [--json]", "Promote the criterion at the 1-based position into a child delivery issue. The parent criterion stays in place.",
		"--json       Output the new child issue, global database scope, and project identity as JSON")
}

func writeIssueBucketHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue bucket <ref> now|next|later|none [--json]", "Set an advisory Now/Next/Later label. Buckets are labels only and are never read as a constraint.",
		"--json       Output the issue and bucket, global database scope, and project identity as JSON")
}

func writeIssueLinkHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue link <from> blocks|relates-to <to> | loaf issue link <from> remove <type> <to> [--json]", "Create or remove an issue relationship. Stored types are blocks and relates_to.",
		"--json       Output the relationship mutation, global database scope, and project identity as JSON")
}

func writeIssueRenderHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue render <ref> [--json]", "Emit markdown suitable to paste as a PR body with no manual editing.",
		"--json       Output the markdown, issue, global database scope, and project identity as JSON")
}

func writeIssueExportHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue export [--json]", "Export the project's issues, identity, criteria, claims, and issue relationships as JSON.",
		"--json       Output the export snapshot (default)")
}

func (r Runner) requireIssueSQLiteState(command string, runtime state.Runtime) (project.Root, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return project.Root{}, err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return project.Root{}, err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return project.Root{}, sqliteStateRequiredError(command)
	case state.ModeInvalid:
		return project.Root{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	return projectRoot, nil
}

func (r Runner) runIssueNew(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueNewArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue new", runtime)
	if err != nil {
		return err
	}
	body, ok, err := r.resolveBodyInput("issue new", options.body, false)
	if err != nil {
		return err
	}
	if ok {
		options.create.Body = body
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	created, err := r.createIssueWithIdentity(projectRoot, resolver, options.create)
	if err != nil {
		return err
	}
	return r.finishIssueNew(out, projectRoot, resolver, created.ID, options)
}

type mintedLinearIssue struct {
	Identifier string
	URL        string
}

func (r Runner) mintIssueIdentity(projectRoot project.Root, resolver state.PathResolver, create state.IssueCreateOptions) (alias string, minted *mintedLinearIssue, err error) {
	identity, ok, err := state.LookupIssueIdentity(context.Background(), projectRoot, resolver)
	if err != nil {
		return "", nil, err
	}
	if !ok || identity.Authority != state.IssueAuthorityLinear {
		return "", nil, nil
	}
	client, err := state.LinearClientFromEnv()
	if err != nil {
		return "", nil, &state.LinearMintError{Err: err}
	}
	issue, err := state.MintLinearIssue(context.Background(), projectRoot, resolver, client, create)
	if err != nil {
		return "", nil, err
	}
	return issue.Identifier, &mintedLinearIssue{Identifier: issue.Identifier, URL: issue.URL}, nil
}

func (r Runner) bindMintedLinearIssue(projectRoot project.Root, resolver state.PathResolver, issueID string, minted *mintedLinearIssue) error {
	if minted == nil {
		return nil
	}
	if err := state.BindLinearIssue(context.Background(), projectRoot, resolver, issueID, minted.Identifier, minted.URL); err != nil {
		return &state.LinearOrphanError{Identifier: minted.Identifier, URL: minted.URL, Err: err}
	}
	return nil
}

func (r Runner) createIssueWithIdentity(projectRoot project.Root, resolver state.PathResolver, create state.IssueCreateOptions) (state.Issue, error) {
	alias, minted, err := r.mintIssueIdentity(projectRoot, resolver, create)
	if err != nil {
		return state.Issue{}, err
	}
	if alias != "" {
		create.Alias = alias
	}
	created, err := state.CreateIssue(context.Background(), projectRoot, resolver, create)
	if err != nil {
		if minted != nil {
			return state.Issue{}, &state.LinearOrphanError{Identifier: minted.Identifier, URL: minted.URL, Err: err}
		}
		return state.Issue{}, err
	}
	if err := r.bindMintedLinearIssue(projectRoot, resolver, created.ID, minted); err != nil {
		return state.Issue{}, err
	}
	return created, nil
}

func (r Runner) finishIssueNew(out io.Writer, projectRoot project.Root, resolver state.PathResolver, issueID string, options issueNewOptions) error {
	createdID := issueID
	if options.status != "" && options.status != state.IssueStatusTriage {
		updated, err := state.UpdateIssue(context.Background(), projectRoot, resolver, state.IssueUpdateOptions{
			Ref:       createdID,
			Status:    options.status,
			SetStatus: true,
		})
		if err != nil {
			return err
		}
		createdID = updated.ID
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, resolver, createdID)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeIssueCreated(out, result)
	return nil
}

func (r Runner) runIssueShow(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("issue show", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue show", runtime)
	if err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeIssueShow(out, result)
	return nil
}

func (r Runner) runIssueList(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueListArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue list", runtime)
	if err != nil {
		return err
	}
	result, err := state.ListIssues(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	if options.filters.Started {
		markStartedWorktreeLiveness(result.Issues)
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	if options.filters.Started {
		writeIssueStartedList(out, result)
		return nil
	}
	writeIssueList(out, result)
	return nil
}

func (r Runner) runIssueTree(args []string, out io.Writer, runtime state.Runtime) error {
	ref, archived, jsonOutput, err := parseIssueTreeArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue tree", runtime)
	if err != nil {
		return err
	}
	result, err := state.IssueTree(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref, archived)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeIssueTree(out, result)
	return nil
}

func (r Runner) runIssueFrontier(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue frontier", runtime)
	if err != nil {
		return err
	}
	result, err := state.ListIssueFrontier(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeIssueFrontier(out, result)
	return nil
}

func (r Runner) runIssueEdit(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueEditArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue edit", runtime)
	if err != nil {
		return err
	}
	body, ok, err := r.resolveBodyInput("issue edit", options.body, false)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("issue edit requires body content via --body-file, --body -, or --message")
	}
	updated, err := state.UpdateIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.IssueUpdateOptions{
		Ref:     options.ref,
		Body:    body,
		SetBody: true,
	})
	if err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, updated.ID)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "edited issue %s\n", issueDisplayRef(result.Issue))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runIssueStatus(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueStatusArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue status", runtime)
	if err != nil {
		return err
	}
	var updated state.Issue
	switch options.status {
	case state.IssueStatusCancelled, state.IssueStatusDuplicate:
		updated, err = state.RemoveIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.IssueRemoveOptions{
			Ref:         options.ref,
			Status:      options.status,
			DuplicateOf: options.duplicateOf,
		})
	default:
		if options.duplicateOf != "" {
			return fmt.Errorf("issue status --duplicate-of is only valid with status duplicate")
		}
		updated, err = state.UpdateIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.IssueUpdateOptions{
			Ref:       options.ref,
			Status:    options.status,
			SetStatus: true,
		})
	}
	if err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, updated.ID)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeIssueStatus(out, result)
	return nil
}

func (r Runner) runIssueDod(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeIssueDodHelp(out)
		return nil
	}
	switch args[0] {
	case "add":
		return r.runIssueDodAdd(args[1:], out, runtime)
	case "list":
		return r.runIssueDodList(args[1:], out, runtime)
	case "remove":
		return r.runIssueDodRemove(args[1:], out, runtime)
	case "claim":
		return r.runIssueDodClaim(args[1:], out, runtime)
	case "unclaim":
		return r.runIssueDodUnclaim(args[1:], out, runtime)
	default:
		return unknownSubcommandError("issue dod", args[0])
	}
}

func (r Runner) runIssueDodAdd(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueDodAddArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue dod add", runtime)
	if err != nil {
		return err
	}
	updated, err := state.AddIssueCriterion(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.ref, options.input)
	if err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, updated.ID)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "added criterion %d on %s\n", result.Issue.Criteria[len(result.Issue.Criteria)-1].Position, issueDisplayRef(result.Issue))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runIssueDodList(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("issue dod list", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue dod list", runtime)
	if err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeIssueDodList(out, result)
	return nil
}

func (r Runner) runIssueDodRemove(args []string, out io.Writer, runtime state.Runtime) error {
	ref, position, jsonOutput, err := parseIssueRefPositionArgs("issue dod remove", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue dod remove", runtime)
	if err != nil {
		return err
	}
	updated, err := state.RemoveIssueCriterion(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref, position)
	if err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, updated.ID)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "removed criterion %d from %s\n", position, issueDisplayRef(result.Issue))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runIssueDodClaim(args []string, out io.Writer, runtime state.Runtime) error {
	ref, childPosition, parentPosition, jsonOutput, err := parseIssueDodClaimArgs("issue dod claim", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue dod claim", runtime)
	if err != nil {
		return err
	}
	updated, err := state.ClaimIssueCriterion(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref, childPosition, parentPosition)
	if err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, updated.ID)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "claimed criterion %d on %s as serving parent criterion %d\n", childPosition, issueDisplayRef(result.Issue), parentPosition)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runIssueDodUnclaim(args []string, out io.Writer, runtime state.Runtime) error {
	ref, childPosition, parentPosition, jsonOutput, err := parseIssueDodClaimArgs("issue dod unclaim", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue dod unclaim", runtime)
	if err != nil {
		return err
	}
	updated, err := state.UnclaimIssueCriterion(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref, childPosition, parentPosition)
	if err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, updated.ID)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "unclaimed criterion %d on %s from parent criterion %d\n", childPosition, issueDisplayRef(result.Issue), parentPosition)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runIssuePromote(args []string, out io.Writer, runtime state.Runtime) error {
	ref, position, jsonOutput, err := parseIssueRefPositionArgs("issue promote", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue promote", runtime)
	if err != nil {
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	parent, err := state.GetIssue(context.Background(), projectRoot, resolver, ref)
	if err != nil {
		return err
	}
	var criterion *state.IssueCriterion
	for i := range parent.Criteria {
		if parent.Criteria[i].Position == position {
			criterion = &parent.Criteria[i]
			break
		}
	}
	if criterion == nil {
		return fmt.Errorf("issue %s has no criterion at position %d", issueDisplayRef(parent), position)
	}
	alias, minted, err := r.mintIssueIdentity(projectRoot, resolver, state.IssueCreateOptions{
		Title:  criterion.Text,
		Parent: firstNonEmpty(parent.Alias, parent.ID),
	})
	if err != nil {
		return err
	}
	child, err := state.PromoteIssueCriterion(context.Background(), projectRoot, resolver, ref, position, alias)
	if err != nil {
		if minted != nil {
			return &state.LinearOrphanError{Identifier: minted.Identifier, URL: minted.URL, Err: err}
		}
		return err
	}
	if err := r.bindMintedLinearIssue(projectRoot, resolver, child.ID, minted); err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, child.ID)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "promoted criterion %d to %s\n", position, issueDisplayRef(result.Issue))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "title: %s\n", result.Issue.Title)
	if result.Parent != nil {
		fmt.Fprintf(out, "parent: %s\n", firstNonEmpty(result.Parent.Alias, result.Parent.ID))
	}
	return nil
}

func (r Runner) runIssueBucket(args []string, out io.Writer, runtime state.Runtime) error {
	ref, bucket, jsonOutput, err := parseIssueBucketArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue bucket", runtime)
	if err != nil {
		return err
	}
	result, err := state.SetIssueBucket(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref, bucket)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	if result.Bucket == "" {
		fmt.Fprintf(out, "cleared bucket on %s\n", issueDisplayRef(result.Issue))
	} else {
		fmt.Fprintf(out, "set bucket %s on %s\n", result.Bucket, issueDisplayRef(result.Issue))
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runIssueLink(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueLinkArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue link", runtime)
	if err != nil {
		return err
	}
	var result state.LinkMutationResult
	if options.remove {
		result, err = state.RemoveIssueLink(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.from, options.relationshipType, options.to)
	} else {
		result, err = state.CreateIssueLink(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.from, options.relationshipType, options.to)
	}
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	if options.remove {
		fmt.Fprintf(out, "removed link %s %s %s\n", firstNonEmpty(result.From.Alias, result.From.ID), result.Type, firstNonEmpty(result.To.Alias, result.To.ID))
	} else {
		fmt.Fprintf(out, "linked %s %s %s\n", firstNonEmpty(result.From.Alias, result.From.ID), result.Type, firstNonEmpty(result.To.Alias, result.To.ID))
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runIssueRender(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("issue render", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue render", runtime)
	if err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	markdown := renderIssueMarkdown(result)
	if jsonOutput {
		return writeJSON(out, map[string]any{
			"contract_version":     result.ContractVersion,
			"database_scope":       result.DatabaseScope,
			"database_path":        result.DatabasePath,
			"project_id":           result.ProjectID,
			"project_name":         result.ProjectName,
			"project_current_path": result.ProjectCurrentPath,
			"issue":                result.Issue,
			"markdown":             markdown,
		})
	}
	fmt.Fprint(out, markdown)
	return nil
}

func (r Runner) runIssueExport(args []string, out io.Writer, runtime state.Runtime) error {
	if _, err := parseJSONOnly(args); err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue export", runtime)
	if err != nil {
		return err
	}
	result, err := state.ExportIssues(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	return writeJSON(out, result)
}

func parseIssueNewArgs(args []string) (issueNewOptions, error) {
	var options issueNewOptions
	var positional []string
	endOfOptions := false
	for i := 0; i < len(args); i++ {
		if endOfOptions {
			positional = append(positional, args[i])
			continue
		}
		switch args[i] {
		case "--":
			endOfOptions = true
		case "--json":
			options.jsonOutput = true
		case "--kind":
			value, err := consumeFlagValue(args, &i, "--kind")
			if err != nil {
				return issueNewOptions{}, err
			}
			options.create.Kind = value
		case "--parent":
			value, err := consumeFlagValue(args, &i, "--parent")
			if err != nil {
				return issueNewOptions{}, err
			}
			options.create.Parent = value
		case "--fog":
			value, err := consumeFlagValue(args, &i, "--fog")
			if err != nil {
				return issueNewOptions{}, err
			}
			options.create.Fog = value
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return issueNewOptions{}, err
			}
			status := strings.TrimSpace(value)
			valid := false
			for _, candidate := range state.IssueWriteStatuses() {
				if status == candidate {
					valid = true
					break
				}
			}
			if !valid {
				return issueNewOptions{}, fmt.Errorf("issue new --status must be one of %s", strings.Join(state.IssueWriteStatuses(), ", "))
			}
			options.status = status
		case "--body":
			value, err := consumeFlagValue(args, &i, "--body")
			if err != nil {
				return issueNewOptions{}, err
			}
			if value == "-" {
				options.body.body = "-"
			} else {
				options.body.message = value
			}
		case "--body-file":
			value, err := consumeFlagValue(args, &i, "--body-file")
			if err != nil {
				return issueNewOptions{}, err
			}
			options.body.bodyFile = value
		case "--message":
			value, err := consumeFlagValue(args, &i, "--message")
			if err != nil {
				return issueNewOptions{}, err
			}
			if options.body.message == "" {
				options.body.message = value
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				return issueNewOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return issueNewOptions{}, fmt.Errorf("issue new requires a title")
	}
	options.create.Title = positional[0]
	return options, nil
}

func parseIssueListArgs(args []string) (issueListOptions, error) {
	var options issueListOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--archived":
			options.filters.Archived = true
		case "--started":
			options.filters.Started = true
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return issueListOptions{}, err
			}
			options.filters.Status = value
		case "--kind":
			value, err := consumeFlagValue(args, &i, "--kind")
			if err != nil {
				return issueListOptions{}, err
			}
			options.filters.Kind = value
		default:
			return issueListOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseIssueTreeArgs(args []string) (string, bool, bool, error) {
	ref := ""
	archived := false
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--archived":
			archived = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, false, fmt.Errorf("unknown option %q", arg)
			}
			if ref != "" {
				return "", false, false, fmt.Errorf("issue tree accepts at most one ref")
			}
			ref = arg
		}
	}
	return ref, archived, jsonOutput, nil
}

func parseIssueEditArgs(args []string) (issueEditOptions, error) {
	var options issueEditOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		if ok, err := parseBodyInputFlag(args, &i, &options.body); ok || err != nil {
			if err != nil {
				return issueEditOptions{}, err
			}
			continue
		}
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return issueEditOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return issueEditOptions{}, fmt.Errorf("issue edit requires exactly one issue ref")
	}
	options.ref = positional[0]
	return options, nil
}

func parseIssueStatusArgs(args []string) (issueStatusOptions, error) {
	var options issueStatusOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--duplicate-of":
			value, err := consumeFlagValue(args, &i, "--duplicate-of")
			if err != nil {
				return issueStatusOptions{}, err
			}
			options.duplicateOf = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return issueStatusOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 2 {
		return issueStatusOptions{}, fmt.Errorf("issue status requires an issue ref and a status")
	}
	options.ref = positional[0]
	options.status = positional[1]
	return options, nil
}

func parseIssueDodAddArgs(args []string) (issueDodAddOptions, error) {
	var options issueDodAddOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--command":
			value, err := consumeFlagValue(args, &i, "--command")
			if err != nil {
				return issueDodAddOptions{}, err
			}
			options.input.Command = value
		case "--expect":
			value, err := consumeFlagValue(args, &i, "--expect")
			if err != nil {
				return issueDodAddOptions{}, err
			}
			options.input.Expect = value
		case "--tier":
			value, err := consumeFlagValue(args, &i, "--tier")
			if err != nil {
				return issueDodAddOptions{}, err
			}
			options.input.Tier = value
		case "--serves":
			value, err := consumeFlagValue(args, &i, "--serves")
			if err != nil {
				return issueDodAddOptions{}, err
			}
			position, err := strconv.Atoi(value)
			if err != nil || position < 1 {
				return issueDodAddOptions{}, fmt.Errorf("issue dod add --serves must be a positive integer")
			}
			options.input.ServesParentPosition = position
		default:
			if strings.HasPrefix(args[i], "-") {
				return issueDodAddOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) < 2 {
		return issueDodAddOptions{}, fmt.Errorf("issue dod add requires an issue ref and criterion text")
	}
	options.ref = positional[0]
	options.input.Text = strings.Join(positional[1:], " ")
	return options, nil
}

func parseIssueRefPositionArgs(command string, args []string) (string, int, bool, error) {
	var positional []string
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", 0, false, fmt.Errorf("unknown option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return "", 0, false, fmt.Errorf("%s requires an issue ref and a position", command)
	}
	position, err := strconv.Atoi(positional[1])
	if err != nil || position < 1 {
		return "", 0, false, fmt.Errorf("%s position must be a positive integer", command)
	}
	return positional[0], position, jsonOutput, nil
}

func parseIssueDodClaimArgs(command string, args []string) (string, int, int, bool, error) {
	var positional []string
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", 0, 0, false, fmt.Errorf("unknown option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) != 3 {
		return "", 0, 0, false, fmt.Errorf("%s requires a child ref, child position, and parent position", command)
	}
	childPosition, err := strconv.Atoi(positional[1])
	if err != nil || childPosition < 1 {
		return "", 0, 0, false, fmt.Errorf("%s child position must be a positive integer", command)
	}
	parentPosition, err := strconv.Atoi(positional[2])
	if err != nil || parentPosition < 1 {
		return "", 0, 0, false, fmt.Errorf("%s parent position must be a positive integer", command)
	}
	return positional[0], childPosition, parentPosition, jsonOutput, nil
}

func parseIssueBucketArgs(args []string) (string, string, bool, error) {
	var positional []string
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", false, fmt.Errorf("unknown option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return "", "", false, fmt.Errorf("issue bucket requires an issue ref and now|next|later|none")
	}
	return positional[0], positional[1], jsonOutput, nil
}

func parseIssueLinkArgs(args []string) (issueLinkOptions, error) {
	var options issueLinkOptions
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return issueLinkOptions{}, fmt.Errorf("unknown option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	switch len(positional) {
	case 3:
		options.from = positional[0]
		options.relationshipType = positional[1]
		options.to = positional[2]
	case 4:
		if positional[1] != "remove" {
			return issueLinkOptions{}, fmt.Errorf("issue link remove requires <from> remove <type> <to>")
		}
		options.from = positional[0]
		options.remove = true
		options.relationshipType = positional[2]
		options.to = positional[3]
	default:
		return issueLinkOptions{}, fmt.Errorf("issue link requires <from> blocks|relates-to <to> or <from> remove <type> <to>")
	}
	return options, nil
}

func writeIssueCreated(out io.Writer, result state.IssueResult) {
	fmt.Fprintf(out, "created issue %s\n", issueDisplayRef(result.Issue))
	if result.Issue.Alias == "" {
		fmt.Fprintln(out, "note: no local alias is minted under a tracker authority")
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "title: %s\n", result.Issue.Title)
	fmt.Fprintf(out, "kind: %s\n", result.Issue.Kind)
	fmt.Fprintf(out, "status: %s\n", result.Issue.Status)
}

func writeIssueShow(out io.Writer, result state.IssueResult) {
	issue := result.Issue
	fmt.Fprintf(out, "issue %s\n", issueDisplayRef(issue))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "id: %s\n", issue.ID)
	if issue.Alias != "" {
		fmt.Fprintf(out, "alias: %s\n", issue.Alias)
	}
	fmt.Fprintf(out, "title: %s\n", issue.Title)
	fmt.Fprintf(out, "kind: %s\n", issue.Kind)
	fmt.Fprintf(out, "status: %s\n", issue.Status)
	if issue.StartedBranch != "" {
		fmt.Fprintf(out, "started_branch: %s\n", issue.StartedBranch)
	}
	if issue.StartedWorktree != "" {
		fmt.Fprintf(out, "started_worktree: %s\n", issue.StartedWorktree)
	}
	if result.Parent != nil {
		fmt.Fprintf(out, "parent: %s\n", firstNonEmpty(result.Parent.Alias, result.Parent.ID))
	} else {
		fmt.Fprintln(out, "parent: none")
	}
	if issue.Fog != "" {
		fmt.Fprintf(out, "fog: %s\n", issue.Fog)
	}
	if result.Bucket != "" {
		fmt.Fprintf(out, "bucket: %s\n", result.Bucket)
	}
	if issue.ArchivedAt != "" {
		fmt.Fprintf(out, "archived: %s\n", issue.ArchivedAt)
	} else {
		fmt.Fprintln(out, "archived: no")
	}
	fmt.Fprintln(out, "body:")
	fmt.Fprintln(out, issue.Body)
	fmt.Fprintln(out, "definition of done:")
	if len(issue.Criteria) == 0 {
		fmt.Fprintln(out, "  none")
	} else {
		for _, criterion := range issue.Criteria {
			fmt.Fprintf(out, "  %d. [%s] %s", criterion.Position, criterion.Tier, criterion.Text)
			if criterion.Command != "" {
				fmt.Fprintf(out, "  command=%s", criterion.Command)
			}
			if criterion.Expect != "" {
				fmt.Fprintf(out, "  expect=%s", criterion.Expect)
			}
			fmt.Fprintln(out)
		}
	}
	if len(result.Children) > 0 {
		fmt.Fprintln(out, "children:")
		for _, child := range result.Children {
			fmt.Fprintf(out, "  %s  %s  %s\n", firstNonEmpty(child.Alias, child.ID), child.Status, child.Title)
		}
	}
}

func writeIssueList(out io.Writer, result state.IssueListResult) {
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Issues) == 0 {
		fmt.Fprintln(out, "no issues found")
		return
	}
	for _, issue := range result.Issues {
		fmt.Fprintf(out, "%-10s %-10s %-10s %s\n", issueDisplayRef(issue), issue.Status, issue.Kind, issue.Title)
	}
}

func writeIssueTree(out io.Writer, result state.IssueTreeResult) {
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Roots) == 0 {
		fmt.Fprintln(out, "no issues found")
		return
	}
	for _, root := range result.Roots {
		writeIssueTreeNode(out, root, 0)
	}
}

func writeIssueTreeNode(out io.Writer, node state.IssueTreeNode, depth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(out, "%s%s  %s  %s\n", indent, firstNonEmpty(node.Alias, node.ID), node.Status, node.Title)
	for _, child := range node.Children {
		writeIssueTreeNode(out, child, depth+1)
	}
}

func writeIssueFrontier(out io.Writer, result state.IssueFrontierResult) {
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Issues) == 0 {
		fmt.Fprintln(out, "no frontier issues")
		return
	}
	for _, issue := range result.Issues {
		fmt.Fprintf(out, "%-10s %-10s %s\n", firstNonEmpty(issue.Alias, issue.ID), issue.Status, issue.Title)
	}
}

func writeIssueStatus(out io.Writer, result state.IssueResult) {
	issue := result.Issue
	switch issue.Status {
	case state.IssueStatusCancelled:
		fmt.Fprintf(out, "cancelled issue %s\n", issueDisplayRef(issue))
	case state.IssueStatusDuplicate:
		fmt.Fprintf(out, "marked issue %s duplicate\n", issueDisplayRef(issue))
	default:
		fmt.Fprintf(out, "updated issue %s\n", issueDisplayRef(issue))
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "status: %s\n", issue.Status)
	if issue.ArchivedAt != "" {
		fmt.Fprintf(out, "archived: %s\n", issue.ArchivedAt)
	}
}

func writeIssueDodList(out io.Writer, result state.IssueResult) {
	fmt.Fprintf(out, "criteria for %s\n", issueDisplayRef(result.Issue))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Issue.Criteria) == 0 {
		fmt.Fprintln(out, "no criteria")
		return
	}
	for _, criterion := range result.Issue.Criteria {
		fmt.Fprintf(out, "%d. [%s] %s", criterion.Position, criterion.Tier, criterion.Text)
		if criterion.Command != "" {
			fmt.Fprintf(out, "  command=%s", criterion.Command)
		}
		if criterion.Expect != "" {
			fmt.Fprintf(out, "  expect=%s", criterion.Expect)
		}
		fmt.Fprintln(out)
	}
}

func renderIssueMarkdown(result state.IssueResult) string {
	return state.RenderIssueMarkdown(result)
}

func issueDisplayRef(issue state.Issue) string {
	return firstNonEmpty(issue.Alias, issue.ID)
}

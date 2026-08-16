package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

type releaseTrackOptions struct {
	help       bool
	jsonOutput bool
	dryRun     bool
	base       string
	bump       string
	noTag      bool
	noGh       bool
	includes   []string
}

type releaseTrackCommit struct {
	Hash     string `json:"hash"`
	Subject  string `json:"subject"`
	Body     string `json:"body,omitempty"`
	Type     string `json:"type,omitempty"`
	Breaking bool   `json:"breaking,omitempty"`
}

type releaseTrackLandedIssue struct {
	ID      string               `json:"id"`
	Alias   string               `json:"alias,omitempty"`
	Title   string               `json:"title"`
	Status  string               `json:"status"`
	Commits []releaseTrackCommit `json:"commits"`
}

type releaseTrackMissingChild struct {
	ID     string `json:"id"`
	Alias  string `json:"alias,omitempty"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type releaseTrackPartialParent struct {
	ID      string                     `json:"id"`
	Alias   string                     `json:"alias,omitempty"`
	Title   string                     `json:"title"`
	Status  string                     `json:"status"`
	Missing []releaseTrackMissingChild `json:"missing"`
}

type releaseTrackBucketRow struct {
	ID     string `json:"id"`
	Alias  string `json:"alias,omitempty"`
	Title  string `json:"title"`
	Bucket string `json:"bucket"`
	Landed bool   `json:"landed"`
}

type releaseTrackBuckets struct {
	Planned         []releaseTrackBucketRow `json:"planned"`
	UnplannedLanded []releaseTrackBucketRow `json:"unplanned_landed"`
}

type releaseTrackSuggestion struct {
	Base             string                      `json:"base,omitempty"`
	CurrentVersion   string                      `json:"current_version,omitempty"`
	SuggestedBump    string                      `json:"suggested_bump"`
	SuggestedVersion string                      `json:"suggested_version,omitempty"`
	BumpEvidence     string                      `json:"bump_evidence"`
	Landed           []releaseTrackLandedIssue   `json:"landed"`
	PartiallyLanded  []releaseTrackPartialParent `json:"partially_landed"`
	Unattributed     []releaseTrackCommit        `json:"unattributed"`
	Buckets          releaseTrackBuckets         `json:"buckets"`
	Notes            string                      `json:"notes"`
}

var releaseTrackMergeSubjectRE = regexp.MustCompile(`(?i)^merge\b`)

func (r Runner) runReleaseSuggest(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseReleaseTrackArgs(args, false)
	if err != nil {
		return err
	}
	if options.help {
		writeReleaseSuggestHelp(out)
		return nil
	}
	suggestion, err := r.computeReleaseTrackSuggestion(runtimeRoot, options.base)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, suggestion)
	}
	writeReleaseTrackSuggestion(out, suggestion)
	return nil
}

func (r Runner) runReleaseCut(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseReleaseTrackArgs(args, true)
	if err != nil {
		return err
	}
	if options.help {
		writeReleaseCutHelp(out)
		return nil
	}
	if !releaseIsGitRepo(runtimeRoot) {
		return fmt.Errorf("Not a git repository")
	}
	suggestion, err := r.computeReleaseTrackSuggestion(runtimeRoot, options.base)
	if err != nil {
		return err
	}
	if options.bump != "" {
		if !releaseValidBumps[options.bump] {
			return fmt.Errorf("Invalid bump type %q. Must be one of: major, minor, patch, prerelease, release", options.bump)
		}
		suggestion.SuggestedBump = options.bump
		suggestion.BumpEvidence = "overridden by --bump " + options.bump
		if suggestion.CurrentVersion != "" {
			suggestion.SuggestedVersion = bumpReleaseVersion(suggestion.CurrentVersion, options.bump)
		}
		suggestion.Notes = draftReleaseTrackNotes(suggestion.SuggestedVersion, time.Now().UTC().Format("2006-01-02"), suggestion.Landed, suggestion.Unattributed)
	}
	if suggestion.SuggestedVersion == "" {
		return fmt.Errorf("could not compute a version to cut")
	}

	projectRoot, resolver, err := r.releaseTrackState(runtimeRoot)
	if err != nil {
		return err
	}
	var included []state.Release
	for _, ref := range options.includes {
		release, err := state.GetRelease(context.Background(), projectRoot, resolver, ref)
		if err != nil {
			return fmt.Errorf("--includes %q: %w", ref, err)
		}
		included = append(included, release)
	}

	if tagName, commit, version, ok := findUnrecordedReleaseCommit(runtimeRoot, projectRoot, resolver); ok {
		suggestion.SuggestedVersion = version
		if options.dryRun {
			fmt.Fprintf(out, "--dry-run: would record existing tag %s at %s; nothing written.\n", tagName, commit)
			return nil
		}
		return r.completeReleaseCutRecord(out, projectRoot, resolver, suggestion, included, tagName, commit, options)
	}

	tagName := "v" + suggestion.SuggestedVersion

	existingTagCommit, tagExists := releaseTrackLookupTag(runtimeRoot, tagName)
	existingRelease, releaseExists := releaseTrackLookupRecorded(projectRoot, resolver, tagName)

	if releaseExists && tagExists && existingRelease.TaggedCommit == existingTagCommit {
		fmt.Fprintf(out, "Release %s already recorded at %s; nothing to cut.\n", existingRelease.Tag, existingRelease.TaggedCommit)
		return r.finishReleaseCut(out, projectRoot, resolver, existingRelease, suggestion, options)
	}
	if releaseExists && !tagExists {
		return fmt.Errorf("release %s is recorded at %s but tag %s is missing; restore the tag or delete the row before cutting", existingRelease.Version, existingRelease.TaggedCommit, tagName)
	}
	if tagExists && releaseExists && existingRelease.TaggedCommit != existingTagCommit {
		return fmt.Errorf("tag %s points at %s but release row records %s", tagName, existingTagCommit, existingRelease.TaggedCommit)
	}

	if options.noTag {
		if !tagExists {
			return fmt.Errorf("cut --no-tag requires tag %s to already exist; create it first or omit --no-tag", tagName)
		}
	} else if tagExists && !releaseExists {
		// Resume after a partial cut: do not rewrite files or retag.
		return r.completeReleaseCutRecord(out, projectRoot, resolver, suggestion, included, tagName, existingTagCommit, options)
	}

	if options.dryRun {
		writeReleaseTrackSuggestion(out, suggestion)
		fmt.Fprintln(out, "Includes:")
		if len(included) == 0 {
			fmt.Fprintln(out, "  (none)")
		}
		for _, release := range included {
			fmt.Fprintf(out, "  %s (%s) — reference only\n", release.Tag, release.Version)
		}
		fmt.Fprintf(out, "\n--dry-run: would cut %s (tag %s) with %d issue member(s); nothing written.\n",
			suggestion.SuggestedVersion, tagName, len(suggestion.Landed))
		return nil
	}

	if err := releaseTrackWorktreeClean(runtimeRoot); err != nil {
		return err
	}

	versionFiles, err := detectReleaseVersionFiles(runtimeRoot, nil)
	if err != nil {
		return err
	}
	if len(versionFiles) == 0 {
		return fmt.Errorf("No version files found")
	}
	if err := rejectDisagreeingReleaseVersionFiles(versionFiles); err != nil {
		return err
	}
	updates, err := prepareReleaseVersionUpdates(runtimeRoot, versionFiles, suggestion.SuggestedVersion)
	if err != nil {
		return fmt.Errorf("Failed to update version files: %w", err)
	}

	written := make([]string, 0, len(updates)+1)
	for _, update := range updates {
		if err := os.WriteFile(update.path, []byte(update.content), 0o644); err != nil {
			releaseTrackRestorePaths(runtimeRoot, written)
			return fmt.Errorf("Failed to update %s: %w", update.relativePath, err)
		}
		written = append(written, update.relativePath)
		fmt.Fprintf(out, "Updated %s (%s → %s)\n", update.relativePath, update.oldVersion, suggestion.SuggestedVersion)
	}
	if err := writeReleaseChangelog(runtimeRoot, suggestion.Notes); err != nil {
		releaseTrackRestorePaths(runtimeRoot, written)
		return fmt.Errorf("Failed to update CHANGELOG.md: %w", err)
	}
	written = append(written, "CHANGELOG.md")
	fmt.Fprintln(out, "Updated CHANGELOG.md")

	if err := releaseCommandRun(runtimeRoot, "git", "add", "-A"); err != nil {
		releaseTrackRestorePaths(runtimeRoot, written)
		return fmt.Errorf("Failed to stage release artifacts: %w", err)
	}
	if err := releaseCommandRun(runtimeRoot, "git", "commit", "-m", "chore: release "+tagName); err != nil {
		releaseTrackRestorePaths(runtimeRoot, written)
		return fmt.Errorf("Failed to commit release: %w", err)
	}
	fmt.Fprintln(out, "Committed release artifacts")

	if !options.noTag {
		if err := releaseCommandRun(runtimeRoot, "git", "tag", "-a", tagName, "-m", "Release "+suggestion.SuggestedVersion); err != nil {
			return fmt.Errorf("committed release artifacts but failed to create tag %s: %w; delete the release commit or create the tag, then re-run loaf release cut --no-tag", tagName, err)
		}
		fmt.Fprintf(out, "Created tag %s\n", tagName)
	} else {
		fmt.Fprintln(out, "Git tag skipped (--no-tag)")
	}

	taggedCommit, err := releaseTrackTagCommit(runtimeRoot, tagName)
	if err != nil {
		return fmt.Errorf("committed release artifacts but tag %s is missing: %w; create the tag, then re-run loaf release cut --no-tag", tagName, err)
	}
	return r.completeReleaseCutRecord(out, projectRoot, resolver, suggestion, included, tagName, taggedCommit, options)
}

func (r Runner) completeReleaseCutRecord(out io.Writer, projectRoot project.Root, resolver state.PathResolver, suggestion releaseTrackSuggestion, included []state.Release, tagName, taggedCommit string, options releaseTrackOptions) error {
	issueIDs := make([]string, 0, len(suggestion.Landed))
	for _, landed := range suggestion.Landed {
		issueIDs = append(issueIDs, landed.ID)
	}
	includedIDs := make([]string, 0, len(included))
	for _, release := range included {
		includedIDs = append(includedIDs, release.ID)
	}
	recorded, err := recordReleaseFn(context.Background(), projectRoot, resolver, state.RecordReleaseOptions{
		Version:      suggestion.SuggestedVersion,
		Tag:          tagName,
		TaggedCommit: taggedCommit,
		Notes:        suggestion.Notes,
		IssueIDs:     issueIDs,
		IncludedIDs:  includedIDs,
	})
	if err != nil {
		return fmt.Errorf("created tag %s at %s but failed to record the release: %w; re-run loaf release cut --no-tag to complete the record", tagName, taggedCommit, err)
	}
	fmt.Fprintf(out, "Recorded release %s at %s (%d member(s))\n", recorded.Tag, recorded.TaggedCommit, len(recorded.Members))
	return r.finishReleaseCut(out, projectRoot, resolver, recorded, suggestion, options)
}

func (r Runner) finishReleaseCut(out io.Writer, projectRoot project.Root, resolver state.PathResolver, recorded state.Release, suggestion releaseTrackSuggestion, options releaseTrackOptions) error {
	r.pushLinearReleaseOnCut(out, projectRoot, resolver, recorded)

	if options.noGh {
		fmt.Fprintln(out, "GitHub release skipped (--no-gh)")
		return nil
	}
	if !releaseGhAvailable() {
		return r.warnReleaseTrackGitHubFailure(out, recorded, suggestion.Notes, fmt.Errorf("gh not found"))
	}
	ghArgs := []string{"release", "create", recorded.Tag, "--draft", "--title", recorded.Tag, "--notes", suggestion.Notes}
	if releaseVersionIsPrerelease(recorded.Version) {
		ghArgs = append(ghArgs, "--prerelease")
	}
	if err := verifyConfiguredGitHubAccount(projectRoot.Path(), out); err != nil {
		return r.warnReleaseTrackGitHubFailure(out, recorded, suggestion.Notes, err)
	}
	if err := releaseCommandRun(projectRoot.Path(), "gh", ghArgs...); err != nil {
		return r.warnReleaseTrackGitHubFailure(out, recorded, suggestion.Notes, err)
	}
	fmt.Fprintln(out, "Created GitHub release draft")
	return nil
}

var recordReleaseFn = state.RecordRelease

func findUnrecordedReleaseCommit(root string, projectRoot project.Root, resolver state.PathResolver) (tagName, commit, version string, ok bool) {
	subject := strings.TrimSpace(releaseCommandOutput(root, "git", "log", "-1", "--pretty=%s"))
	if !strings.HasPrefix(subject, "chore: release v") {
		return "", "", "", false
	}
	tagName = strings.TrimPrefix(subject, "chore: release ")
	commit, err := releaseTrackTagCommit(root, tagName)
	if err != nil {
		return "", "", "", false
	}
	if _, exists := releaseTrackLookupRecorded(projectRoot, resolver, tagName); exists {
		return "", "", "", false
	}
	return tagName, commit, strings.TrimPrefix(tagName, "v"), true
}

func releaseTrackLookupTag(root, tagName string) (string, bool) {
	commit, err := releaseTrackTagCommit(root, tagName)
	if err != nil {
		return "", false
	}
	return commit, true
}

func releaseTrackLookupRecorded(projectRoot project.Root, resolver state.PathResolver, ref string) (state.Release, bool) {
	recorded, err := state.GetRelease(context.Background(), projectRoot, resolver, ref)
	if err != nil {
		return state.Release{}, false
	}
	return recorded, true
}

func releaseTrackRestorePaths(root string, paths []string) {
	if len(paths) == 0 {
		return
	}
	var restore []string
	for _, path := range paths {
		if releaseTrackPathExistsAtHEAD(root, path) {
			restore = append(restore, path)
			continue
		}
		_ = releaseCommandRun(root, "git", "rm", "-f", "--ignore-unmatch", "--", path)
		_ = os.Remove(filepath.Join(root, path))
	}
	if len(restore) == 0 {
		return
	}
	args := append([]string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}, restore...)
	_ = releaseCommandRun(root, "git", args...)
}

func releaseTrackPathExistsAtHEAD(root, path string) bool {
	return releaseCommandRun(root, "git", "cat-file", "-e", "HEAD:"+filepath.ToSlash(path)) == nil
}

func (r Runner) pushLinearReleaseOnCut(out io.Writer, projectRoot project.Root, resolver state.PathResolver, recorded state.Release) {
	identity, ok, err := state.LookupIssueIdentity(context.Background(), projectRoot, resolver)
	if err != nil {
		r.warnLinearReleasePublication(out, recorded, err.Error(), nil)
		return
	}
	if !ok || identity.Authority != state.IssueAuthorityLinear {
		return
	}
	client, err := state.LinearClientFromEnv()
	if err != nil {
		r.warnLinearReleasePublication(out, recorded, err.Error(), nil)
		return
	}
	result, err := state.PushLinearRelease(context.Background(), projectRoot, resolver, client, recorded)
	if err != nil {
		r.warnLinearReleasePublication(out, recorded, err.Error(), result.Unmapped)
		return
	}
	if result.Skipped != "" {
		if result.Skipped == state.LinearReleaseUnsupportedSkip {
			return
		}
		r.warnLinearReleasePublication(out, recorded, result.Skipped, result.Unmapped)
		return
	}
	if result.Supported && result.Release.ID != "" {
		fmt.Fprintf(out, "Recorded Linear release %s (%d issue(s))\n", result.Release.Name, len(result.Release.IssueKeys))
	}
	if len(result.Unmapped) > 0 {
		r.warnLinearReleasePublication(out, recorded, "", result.Unmapped)
	}
}

func (r Runner) warnLinearReleasePublication(out io.Writer, recorded state.Release, reason string, unmapped []string) {
	warnOut := r.Stderr
	if warnOut == nil {
		warnOut = out
	}
	switch {
	case strings.TrimSpace(reason) != "" && len(unmapped) > 0:
		fmt.Fprintf(warnOut, "warning: recorded release %s at %s but Linear publication failed: %s (unmapped members: %s)\n", recorded.Tag, recorded.TaggedCommit, reason, strings.Join(unmapped, ", "))
	case strings.TrimSpace(reason) != "":
		fmt.Fprintf(warnOut, "warning: recorded release %s at %s but Linear publication failed: %s\n", recorded.Tag, recorded.TaggedCommit, reason)
	case len(unmapped) > 0:
		fmt.Fprintf(warnOut, "warning: recorded release %s at %s but Linear publication omitted unmapped members: %s\n", recorded.Tag, recorded.TaggedCommit, strings.Join(unmapped, ", "))
	}
}

func (r Runner) warnReleaseTrackGitHubFailure(out io.Writer, recorded state.Release, notes string, err error) error {
	warnOut := r.Stderr
	if warnOut == nil {
		warnOut = out
	}
	fmt.Fprintf(warnOut, "warning: recorded release %s at %s but GitHub release failed: %v\n", recorded.Tag, recorded.TaggedCommit, err)
	fmt.Fprintf(warnOut, "retry: %s\n", formatReleaseTrackRetryCommand(recorded, notes))
	return nil
}

func formatReleaseTrackRetryCommand(recorded state.Release, notes string) string {
	args := []string{"gh", "release", "create", recorded.Tag, "--draft", "--title", recorded.Tag, "--notes", notes}
	if releaseVersionIsPrerelease(recorded.Version) {
		args = append(args, "--prerelease")
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = state.PosixSingleQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func releaseTrackTagCommit(root, tagName string) (string, error) {
	commit := releaseCommandOutput(root, "git", "rev-parse", "refs/tags/"+tagName+"^{commit}")
	if commit == "" {
		return "", fmt.Errorf("tag %s does not exist", tagName)
	}
	return commit, nil
}

func parseReleaseTrackArgs(args []string, cut bool) (releaseTrackOptions, error) {
	var options releaseTrackOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h" || arg == "help":
			options.help = true
		case arg == "--json":
			options.jsonOutput = true
		case arg == "--dry-run":
			options.dryRun = true
		case arg == "--no-tag":
			options.noTag = true
		case arg == "--no-gh":
			options.noGh = true
		case arg == "--bump":
			value, err := consumeFlagValue(args, &i, "--bump")
			if err != nil {
				return releaseTrackOptions{}, err
			}
			options.bump = value
		case strings.HasPrefix(arg, "--bump="):
			options.bump = strings.TrimPrefix(arg, "--bump=")
			if options.bump == "" {
				return releaseTrackOptions{}, fmt.Errorf("--bump requires a value")
			}
		case arg == "--base":
			value, err := consumeFlagValue(args, &i, "--base")
			if err != nil {
				return releaseTrackOptions{}, err
			}
			options.base = value
		case strings.HasPrefix(arg, "--base="):
			options.base = strings.TrimPrefix(arg, "--base=")
			if options.base == "" {
				return releaseTrackOptions{}, fmt.Errorf("--base requires a value")
			}
		case arg == "--includes":
			value, err := consumeFlagValue(args, &i, "--includes")
			if err != nil {
				return releaseTrackOptions{}, err
			}
			options.includes = append(options.includes, value)
		case strings.HasPrefix(arg, "--includes="):
			value := strings.TrimPrefix(arg, "--includes=")
			if value == "" {
				return releaseTrackOptions{}, fmt.Errorf("--includes requires a value")
			}
			options.includes = append(options.includes, value)
		default:
			return releaseTrackOptions{}, fmt.Errorf("unknown release option %q", arg)
		}
	}
	if options.bump != "" && !releaseValidBumps[options.bump] {
		return releaseTrackOptions{}, fmt.Errorf("Invalid bump type %q. Must be one of: major, minor, patch, prerelease, release", options.bump)
	}
	if !cut {
		if options.dryRun {
			return releaseTrackOptions{}, fmt.Errorf("suggest is read-only; --dry-run is not valid")
		}
		if options.noTag || options.noGh || len(options.includes) > 0 || options.bump != "" {
			return releaseTrackOptions{}, fmt.Errorf("suggest does not accept cut-only flags")
		}
	} else if options.jsonOutput {
		return releaseTrackOptions{}, fmt.Errorf("cut does not accept --json")
	}
	return options, nil
}

func (r Runner) computeReleaseTrackSuggestion(runtimeRoot, baseFlag string) (releaseTrackSuggestion, error) {
	if !releaseIsGitRepo(runtimeRoot) {
		return releaseTrackSuggestion{}, fmt.Errorf("Not a git repository")
	}
	baseRef, err := resolveReleaseTrackBase(runtimeRoot, baseFlag)
	if err != nil {
		return releaseTrackSuggestion{}, err
	}
	commits := collectReleaseTrackCommits(runtimeRoot, baseRef)
	projectRoot, resolver, err := r.releaseTrackState(runtimeRoot)
	if err != nil {
		return releaseTrackSuggestion{}, err
	}
	ctx := context.Background()
	identity, ok, err := state.LookupIssueIdentity(ctx, projectRoot, resolver)
	if err != nil {
		return releaseTrackSuggestion{}, err
	}
	listed, err := state.ListIssues(ctx, projectRoot, resolver, state.IssueListOptions{Archived: true})
	if err != nil {
		return releaseTrackSuggestion{}, err
	}
	journal, err := state.ListCommitJournalEntries(ctx, projectRoot, resolver)
	if err != nil {
		return releaseTrackSuggestion{}, err
	}
	buckets, err := state.ListIssueBuckets(ctx, projectRoot, resolver)
	if err != nil {
		return releaseTrackSuggestion{}, err
	}
	doneAt, err := state.ListLatestIssueDoneAt(ctx, projectRoot, resolver)
	if err != nil {
		return releaseTrackSuggestion{}, err
	}

	prefix := state.DefaultIssuePrefix
	if ok && identity.Prefix != "" {
		prefix = identity.Prefix
	}
	byID := map[string]state.Issue{}
	byAlias := map[string]state.Issue{}
	childrenOf := map[string][]state.Issue{}
	for _, issue := range listed.Issues {
		byID[issue.ID] = issue
		if issue.Alias != "" {
			byAlias[strings.ToUpper(issue.Alias)] = issue
		}
		if issue.ParentID != "" {
			childrenOf[issue.ParentID] = append(childrenOf[issue.ParentID], issue)
		}
	}

	journalByHash := resolveReleaseTrackJournalAliases(commits, prefix, journal)
	landedCommits := map[string][]releaseTrackCommit{}
	var unattributed []releaseTrackCommit
	for _, commit := range commits {
		issues := attributeReleaseTrackCommit(commit, prefix, byAlias, journalByHash[commit.Hash])
		if len(issues) == 0 {
			unattributed = append(unattributed, commit)
			continue
		}
		for _, issue := range issues {
			landedCommits[issue.ID] = append(landedCommits[issue.ID], commit)
		}
	}

	landedIDs := map[string]bool{}
	var landed []releaseTrackLandedIssue
	for id, commitList := range landedCommits {
		issue := byID[id]
		landedIDs[id] = true
		landed = append(landed, releaseTrackLandedIssue{
			ID:      issue.ID,
			Alias:   issue.Alias,
			Title:   issue.Title,
			Status:  issue.Status,
			Commits: commitList,
		})
	}
	sort.Slice(landed, func(i, j int) bool {
		left := firstNonEmptyString(landed[i].Alias, landed[i].ID)
		right := firstNonEmptyString(landed[j].Alias, landed[j].ID)
		return left < right
	})

	var partial []releaseTrackPartialParent
	seenParent := map[string]bool{}
	for _, item := range landed {
		issue := byID[item.ID]
		if issue.ParentID == "" || seenParent[issue.ParentID] {
			continue
		}
		seenParent[issue.ParentID] = true
		parent := byID[issue.ParentID]
		var missing []releaseTrackMissingChild
		for _, child := range childrenOf[parent.ID] {
			if child.Status != state.IssueStatusDone {
				missing = append(missing, releaseTrackMissingChild{
					ID:     child.ID,
					Alias:  child.Alias,
					Title:  child.Title,
					Status: child.Status,
				})
			}
		}
		if len(missing) == 0 {
			continue
		}
		partial = append(partial, releaseTrackPartialParent{
			ID:      parent.ID,
			Alias:   parent.Alias,
			Title:   parent.Title,
			Status:  parent.Status,
			Missing: missing,
		})
	}
	sort.Slice(partial, func(i, j int) bool {
		return firstNonEmptyString(partial[i].Alias, partial[i].ID) < firstNonEmptyString(partial[j].Alias, partial[j].ID)
	})

	bump, evidence := deriveReleaseTrackBump(commits, byID, childrenOf, landedIDs, doneAt, releaseTrackCommitterTime(runtimeRoot, baseRef))
	currentVersion, err := releaseTrackCurrentVersion(runtimeRoot, baseRef)
	if err != nil {
		return releaseTrackSuggestion{}, err
	}
	suggestedVersion := ""
	if currentVersion != "" {
		suggestedVersion = bumpReleaseVersion(currentVersion, bump)
	}
	notes := draftReleaseTrackNotes(suggestedVersion, time.Now().UTC().Format("2006-01-02"), landed, unattributed)

	var planned []releaseTrackBucketRow
	plannedIDs := map[string]bool{}
	for id, bucket := range buckets {
		issue := byID[id]
		plannedIDs[id] = true
		planned = append(planned, releaseTrackBucketRow{
			ID:     issue.ID,
			Alias:  issue.Alias,
			Title:  issue.Title,
			Bucket: bucket,
			Landed: landedIDs[id],
		})
	}
	sort.Slice(planned, func(i, j int) bool {
		return firstNonEmptyString(planned[i].Alias, planned[i].ID) < firstNonEmptyString(planned[j].Alias, planned[j].ID)
	})
	var unplanned []releaseTrackBucketRow
	for _, item := range landed {
		if plannedIDs[item.ID] {
			continue
		}
		unplanned = append(unplanned, releaseTrackBucketRow{
			ID:     item.ID,
			Alias:  item.Alias,
			Title:  item.Title,
			Landed: true,
		})
	}

	return releaseTrackSuggestion{
		Base:             baseRef,
		CurrentVersion:   currentVersion,
		SuggestedBump:    bump,
		SuggestedVersion: suggestedVersion,
		BumpEvidence:     evidence,
		Landed:           landed,
		PartiallyLanded:  partial,
		Unattributed:     unattributed,
		Buckets:          releaseTrackBuckets{Planned: planned, UnplannedLanded: unplanned},
		Notes:            notes,
	}, nil
}

func (r Runner) releaseTrackState(runtimeRoot string) (project.Root, state.PathResolver, error) {
	projectRoot, err := project.ResolveRoot(runtimeRoot)
	if err != nil {
		return project.Root{}, state.PathResolver{}, err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	status, err := state.Inspect(projectRoot, resolver)
	if err != nil {
		return project.Root{}, state.PathResolver{}, err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return project.Root{}, state.PathResolver{}, sqliteStateRequiredError("release")
	case state.ModeInvalid:
		return project.Root{}, state.PathResolver{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	return projectRoot, resolver, nil
}

func resolveReleaseTrackBase(root, baseFlag string) (string, error) {
	if strings.TrimSpace(baseFlag) != "" {
		resolved, err := validateReleaseBaseRef(root, baseFlag)
		if err != nil {
			return "", err
		}
		return resolved, nil
	}
	if tag := releaseLastTag(root); tag != "" {
		return tag, nil
	}
	return "", nil
}

func collectReleaseTrackCommits(root, base string) []releaseTrackCommit {
	format := "%h%x00%s%x00%B%x00"
	args := []string{"log", "--format=" + format}
	if base != "" {
		args = []string{"log", base + "..HEAD", "--format=" + format}
	}
	output := releaseCommandOutput(root, "git", args...)
	if strings.TrimSpace(output) == "" {
		return nil
	}
	var commits []releaseTrackCommit
	for _, chunk := range strings.Split(output, "\x00\n") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		parts := strings.Split(chunk, "\x00")
		if len(parts) < 2 {
			continue
		}
		hash := strings.TrimSpace(parts[0])
		subject := strings.TrimSpace(parts[1])
		body := ""
		if len(parts) > 2 {
			body = strings.TrimSpace(parts[2])
		}
		if hash == "" {
			continue
		}
		parsed := parseReleaseCommit(hash, subject, body)
		commits = append(commits, releaseTrackCommit{
			Hash:     parsed.Hash,
			Subject:  parsed.Raw,
			Body:     body,
			Type:     parsed.Type,
			Breaking: parsed.Breaking,
		})
	}
	return commits
}

func attributeReleaseTrackCommit(commit releaseTrackCommit, prefix string, byAlias map[string]state.Issue, journalAliases []string) []state.Issue {
	if issues := resolveReleaseTrackAliases(extractReleaseTrackAliases(commit.Subject+"\n"+commit.Body, prefix, false), byAlias); len(issues) > 0 {
		return issues
	}
	// Branch rung: merge-commit subjects ("Merge pull request #N from owner/branch")
	// plus any alias found anywhere in the body. Squash subjects like
	// "feat: add auth (#42)" carry no branch name; the original commit list
	// in the body usually does. Do not call gh/network. An alias that lives
	// only in a deleted branch name is unattributable locally and lands in
	// the unattributed report.
	var branchText []string
	if releaseTrackMergeSubjectRE.MatchString(commit.Subject) {
		branchText = append(branchText, extractReleaseTrackAliases(commit.Subject, prefix, true)...)
	}
	if commit.Body != "" {
		branchText = append(branchText, extractReleaseTrackAliases(commit.Body, prefix, true)...)
	}
	if issues := resolveReleaseTrackAliases(branchText, byAlias); len(issues) > 0 {
		return issues
	}
	return resolveReleaseTrackAliases(journalAliases, byAlias)
}

var (
	releaseTrackURLRE      = regexp.MustCompile(`https?://\S+`)
	releaseTrackCodeSpanRE = regexp.MustCompile("`[^`]*`")
)

func extractReleaseTrackAliases(text, prefix string, insensitive bool) []string {
	if prefix == "" || strings.TrimSpace(text) == "" {
		return nil
	}
	text = stripReleaseTrackAliasNoise(text)
	pattern := `\b` + regexp.QuoteMeta(prefix) + `-\d+\b`
	flags := ""
	if insensitive {
		flags = `(?i)`
	}
	re := regexp.MustCompile(flags + pattern)
	return uniqueNonEmptyStrings(re.FindAllString(text, -1))
}

func stripReleaseTrackAliasNoise(text string) string {
	text = releaseTrackURLRE.ReplaceAllString(text, "")
	return releaseTrackCodeSpanRE.ReplaceAllString(text, "")
}

func resolveReleaseTrackJournalAliases(commits []releaseTrackCommit, prefix string, journal []state.JournalEntryRecord) map[string][]string {
	byHash := map[string][]string{}
	for _, entry := range journal {
		var matched []string
		for _, commit := range commits {
			if releaseTrackHashMatches(commit.Hash, entry.Scope) {
				matched = append(matched, commit.Hash)
			}
		}
		if len(matched) != 1 {
			continue
		}
		aliases := uniqueNonEmptyStrings(append(
			extractReleaseTrackAliases(entry.Message, prefix, false),
			extractReleaseTrackAliases(entry.Message, prefix, true)...,
		))
		if len(aliases) == 0 {
			continue
		}
		byHash[matched[0]] = uniqueNonEmptyStrings(append(byHash[matched[0]], aliases...))
	}
	return byHash
}

func releaseTrackHashMatches(commitHash, scope string) bool {
	commitHash = strings.ToLower(strings.TrimSpace(commitHash))
	scope = strings.ToLower(strings.TrimSpace(scope))
	if commitHash == "" || scope == "" {
		return false
	}
	return commitHash == scope || strings.HasPrefix(commitHash, scope) || strings.HasPrefix(scope, commitHash)
}

func resolveReleaseTrackAliases(aliases []string, byAlias map[string]state.Issue) []state.Issue {
	seen := map[string]bool{}
	var issues []state.Issue
	for _, alias := range aliases {
		issue, ok := byAlias[strings.ToUpper(alias)]
		if !ok || seen[issue.ID] {
			continue
		}
		seen[issue.ID] = true
		issues = append(issues, issue)
	}
	return issues
}

func deriveReleaseTrackBump(commits []releaseTrackCommit, byID map[string]state.Issue, childrenOf map[string][]state.Issue, landedIDs map[string]bool, doneAt map[string]string, baselineAt time.Time) (string, string) {
	for _, commit := range commits {
		if commit.Breaking {
			return "major", "breaking marker in " + commit.Hash
		}
	}
	var fullyLanded []string
	seen := map[string]bool{}
	for id := range landedIDs {
		issue := byID[id]
		parentID := issue.ParentID
		if parentID == "" {
			parentID = issue.ID
		}
		if seen[parentID] {
			continue
		}
		seen[parentID] = true
		parent := byID[parentID]
		children := childrenOf[parent.ID]
		if parent.Status != state.IssueStatusDone || len(children) < 2 {
			continue
		}
		if !baselineAt.IsZero() {
			parentDone, ok := parseReleaseTrackTime(doneAt[parent.ID])
			if !ok || !parentDone.After(baselineAt) {
				continue
			}
		}
		allLanded := true
		for _, child := range children {
			if child.Status != state.IssueStatusDone || !landedIDs[child.ID] {
				allLanded = false
				break
			}
		}
		if allLanded {
			fullyLanded = append(fullyLanded, firstNonEmptyString(parent.Alias, parent.ID))
		}
	}
	if len(fullyLanded) > 0 {
		sort.Strings(fullyLanded)
		return "minor", "closed multi-child parent " + fullyLanded[0] + " fully landed"
	}
	for _, commit := range commits {
		if commit.Type == "feat" {
			return "minor", "feat commit " + commit.Hash
		}
	}
	return "patch", "fix/other commits only"
}

func releaseTrackCurrentVersion(root, baseRef string) (string, error) {
	if err := rejectDirtyReleaseVersionFiles(root); err != nil {
		return "", err
	}
	files, err := detectReleaseVersionFiles(root, nil)
	if err != nil {
		return "", err
	}
	if len(files) > 0 {
		if err := rejectDisagreeingReleaseVersionFiles(files); err != nil {
			return "", err
		}
	}
	tag := strings.TrimPrefix(baseRef, "v")
	if _, ok := parseReleaseSemver(tag); ok {
		if err := rejectVersionFilesDisagreeingWithBaseline(files, tag); err != nil {
			return "", err
		}
		return tag, nil
	}
	ref := strings.TrimSpace(baseRef)
	if ref == "" {
		ref = "HEAD"
	}
	if version, err := releaseTrackCommittedVersion(root, ref); err != nil {
		return "", err
	} else if version != "" {
		return version, nil
	}
	return "", nil
}

func rejectVersionFilesDisagreeingWithBaseline(files []releaseVersionFile, baseline string) error {
	if strings.TrimSpace(baseline) == "" {
		return nil
	}
	var parts []string
	disagree := false
	for _, file := range files {
		if file.CurrentVersion == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", file.RelativePath, file.CurrentVersion))
		if file.CurrentVersion != baseline {
			disagree = true
		}
	}
	if !disagree {
		return nil
	}
	return fmt.Errorf("version files disagree with baseline %s: %s", baseline, strings.Join(parts, ", "))
}

func rejectDirtyReleaseVersionFiles(root string) error {
	files, err := detectReleaseVersionFiles(root, nil)
	if err != nil {
		return err
	}
	var dirty []string
	for _, file := range files {
		status := releaseCommandOutput(root, "git", "status", "--porcelain", "--", file.RelativePath)
		if strings.TrimSpace(status) != "" {
			dirty = append(dirty, file.RelativePath)
		}
	}
	if len(dirty) == 0 {
		return nil
	}
	return fmt.Errorf("version file %s has uncommitted modifications; commit or revert it before suggesting a release", strings.Join(dirty, ", "))
}

func releaseTrackCommittedVersion(root, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", nil
	}
	files, err := detectReleaseVersionFiles(root, nil)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", nil
	}
	var loaded []releaseVersionFile
	for _, file := range files {
		body := releaseCommandOutput(root, "git", "show", ref+":"+file.RelativePath)
		if strings.TrimSpace(body) == "" {
			continue
		}
		version, _, err := parseReleaseVersion(file.RelativePath, []byte(body+"\n"))
		if err != nil || version == "" {
			continue
		}
		copy := file
		copy.CurrentVersion = version
		loaded = append(loaded, copy)
	}
	if len(loaded) == 0 {
		return "", nil
	}
	if err := rejectDisagreeingReleaseVersionFiles(loaded); err != nil {
		return "", err
	}
	return loaded[0].CurrentVersion, nil
}

func rejectDisagreeingReleaseVersionFiles(files []releaseVersionFile) error {
	if len(files) < 2 {
		return nil
	}
	version := files[0].CurrentVersion
	agree := true
	for _, file := range files[1:] {
		if file.CurrentVersion != version {
			agree = false
			break
		}
	}
	if agree {
		return nil
	}
	parts := make([]string, 0, len(files))
	for _, file := range files {
		parts = append(parts, fmt.Sprintf("%s=%s", file.RelativePath, file.CurrentVersion))
	}
	return fmt.Errorf("version files disagree: %s", strings.Join(parts, ", "))
}

func releaseTrackCommitterTime(root, ref string) time.Time {
	if strings.TrimSpace(ref) == "" {
		return time.Time{}
	}
	raw := releaseCommandOutput(root, "git", "log", "-1", "--format=%cI", ref)
	parsed, ok := parseReleaseTrackTime(raw)
	if !ok {
		return time.Time{}
	}
	return parsed
}

func parseReleaseTrackTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func draftReleaseTrackNotes(version, date string, landed []releaseTrackLandedIssue, unattributed []releaseTrackCommit) string {
	if version == "" {
		version = "unreleased"
	}
	lines := []string{fmt.Sprintf("## [%s] - %s", version, date)}
	for _, item := range landed {
		heading := firstNonEmptyString(item.Alias, item.ID)
		if item.Title != "" {
			heading += " — " + item.Title
		}
		lines = append(lines, "", "### "+heading)
		for _, commit := range item.Commits {
			lines = append(lines, fmt.Sprintf("- %s (%s)", commit.Subject, commit.Hash))
		}
	}
	if len(unattributed) > 0 {
		lines = append(lines, "", "### Unattributed")
		for _, commit := range unattributed {
			message := commit.Subject
			if parsed := parseReleaseCommit(commit.Hash, commit.Subject, commit.Body); parsed.Message != "" {
				message = capitalizeReleaseMessage(parsed.Message)
			}
			lines = append(lines, fmt.Sprintf("- %s (%s)", message, commit.Hash))
		}
	}
	return strings.Join(lines, "\n")
}

func writeReleaseTrackSuggestion(out io.Writer, suggestion releaseTrackSuggestion) {
	fmt.Fprintf(out, "Base: %s\n", firstNonEmptyString(suggestion.Base, "(none)"))
	fmt.Fprintf(out, "Suggested bump: %s → %s\n", suggestion.SuggestedBump, firstNonEmptyString(suggestion.SuggestedVersion, "(unknown)"))
	fmt.Fprintf(out, "Evidence: %s\n\n", suggestion.BumpEvidence)

	fmt.Fprintln(out, "Landed:")
	if len(suggestion.Landed) == 0 {
		fmt.Fprintln(out, "  (none)")
	}
	for _, item := range suggestion.Landed {
		fmt.Fprintf(out, "  %s — %s\n", firstNonEmptyString(item.Alias, item.ID), item.Title)
		for _, commit := range item.Commits {
			fmt.Fprintf(out, "    %s %s\n", commit.Hash, commit.Subject)
		}
	}

	fmt.Fprintln(out, "\nPartially landed:")
	if len(suggestion.PartiallyLanded) == 0 {
		fmt.Fprintln(out, "  (none)")
	}
	for _, item := range suggestion.PartiallyLanded {
		fmt.Fprintf(out, "  %s — %s\n", firstNonEmptyString(item.Alias, item.ID), item.Title)
		for _, child := range item.Missing {
			fmt.Fprintf(out, "    missing %s (%s) — %s\n", firstNonEmptyString(child.Alias, child.ID), child.Status, child.Title)
		}
	}

	fmt.Fprintln(out, "\nUnattributed:")
	if len(suggestion.Unattributed) == 0 {
		fmt.Fprintln(out, "  (none)")
	}
	for _, commit := range suggestion.Unattributed {
		fmt.Fprintf(out, "  %s %s\n", commit.Hash, commit.Subject)
	}

	fmt.Fprintln(out, "\nBuckets (advisory):")
	if len(suggestion.Buckets.Planned) == 0 && len(suggestion.Buckets.UnplannedLanded) == 0 {
		fmt.Fprintln(out, "  (none)")
	}
	for _, row := range suggestion.Buckets.Planned {
		stateLabel := "not landed"
		if row.Landed {
			stateLabel = "landed"
		}
		fmt.Fprintf(out, "  bucket:%s %s — %s (%s)\n", row.Bucket, firstNonEmptyString(row.Alias, row.ID), row.Title, stateLabel)
	}
	for _, row := range suggestion.Buckets.UnplannedLanded {
		fmt.Fprintf(out, "  unplanned %s — %s (landed)\n", firstNonEmptyString(row.Alias, row.ID), row.Title)
	}

	fmt.Fprintln(out, "\nDrafted notes:")
	fmt.Fprintln(out, suggestion.Notes)
}

func releaseTrackWorktreeClean(root string) error {
	status := releaseCommandOutput(root, "git", "status", "--porcelain=v1")
	if status != "" {
		return fmt.Errorf("working tree is not clean")
	}
	return nil
}

func writeReleaseSuggestHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf release suggest [options]",
		"",
		"Report landed work since the last version tag. Writes nothing.",
		"",
		"Options:",
		"  --base <ref>   Use commits since <ref> instead of last tag",
		"  --json         Output the suggestion as JSON",
		"  -h, --help     Show help",
	}, "\n"))
}

func writeReleaseCutHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf release cut [options]",
		"",
		"Cut a retroactive release from landed work. Records members as facts.",
		"",
		"Options:",
		"  --base <ref>              Use commits since <ref> instead of last tag",
		"  --bump <type>             Override the suggested bump",
		"  --includes <version|tag>  Reference a prior release (repeatable)",
		"  --no-tag                  Skip git tag creation (tag v<version> must already exist)",
		"  --no-gh                   Skip GitHub release draft",
		"  --dry-run                 Print the plan and write nothing",
		"  -h, --help                Show help",
	}, "\n"))
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToUpper(value)] {
			continue
		}
		seen[strings.ToUpper(value)] = true
		out = append(out, value)
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

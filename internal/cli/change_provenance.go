package cli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Execution provenance grades (Planning Contract / TASK-004).
// Path grade: a commit modifies tasks/ (or legacy change.md) plus a path
// outside docs/changes/ entirely — feeds derived display.
// Flip grade: that commit's diff also flips `- [ ]`→`- [x]` outside fences —
// the first grading path for cohort members.
// Receipt grade: a fresh verify receipt over a folder whose every committed task
// box is checked — the content-bound floor, because squash, rebase, and every
// cleanup-then-merge hybrid rewrite the flip while none of them can rewrite the
// tree a receipt binds.

var (
	changeFlipUncheckedRE = regexp.MustCompile(`(?i)^\s*- \[ \]\s*(.*)$`)
	changeFlipCheckedRE   = regexp.MustCompile(`(?i)^\s*- \[x\]\s*(.*)$`)
	changeDiffHunkRE      = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)
)

type changeExecutionStatus struct {
	PathExecuted bool
	FlipExecuted bool
}

// changeMemberEvidence is everything the gate knows about one cohort member's
// execution: the history-shape grade, the receipt's freshness verdict, and
// whether the committed packets are fully checked. One value, gathered once, so
// freshness is computed a single time and the refusal can name what is missing.
type changeMemberEvidence struct {
	FolderRel       string
	Status          changeExecutionStatus
	Verdict         changeReceiptVerdict
	AllTasksChecked bool
}

// executed is the gate's execution grade: a flip transition somewhere in
// ancestry, or a receipt that vouches for a fully checked folder. The disjunct
// is what makes the grade merge-strategy-proof — the flip path stays correct
// wherever history preserves it, and the receipt path holds where it does not.
func (e changeMemberEvidence) executed() bool {
	return e.Status.FlipExecuted || (e.Verdict.OK && e.AllTasksChecked)
}

// changeMemberExecutionEvidence gathers both grading inputs for one member.
func changeMemberExecutionEvidence(rootPath string, node changeNode, outputCommand changeGitOutput) (changeMemberEvidence, error) {
	status, err := changeFolderExecuted(rootPath, node.Folder, node.Layout, outputCommand)
	if err != nil {
		return changeMemberEvidence{}, err
	}
	return changeMemberEvidence{
		FolderRel:       filepath.ToSlash(node.Folder),
		Status:          status,
		Verdict:         changeReceiptStatus(rootPath, node.Folder, node, outputCommand),
		AllTasksChecked: changeFolderTasksAllChecked(rootPath, node, outputCommand),
	}, nil
}

// changeFolderTasksAllChecked reports whether the folder carries at least one
// task checkbox and every one of them is checked. It reads committed HEAD, not
// the working tree: this half of the execution grade is evidence, so a box
// checked in a dirty checkout must not open the gate. Unreadable or absent
// tasks/ yields no boxes, which fails the grade rather than passing it.
func changeFolderTasksAllChecked(rootPath string, node changeNode, outputCommand changeGitOutput) bool {
	if node.Layout != changeLayoutNew {
		return false
	}
	folderAbs := filepath.Join(rootPath, filepath.FromSlash(node.Folder))
	tasks, _, _ := loadChangeTasks(rootPath, folderAbs, node, changeTaskContentHEAD, outputCommand)
	total, done := 0, 0
	for _, task := range tasks {
		total += task.CheckboxTotal
		done += task.CheckboxDone
	}
	return total > 0 && done == total
}

func changeFolderExecuted(rootPath, folderRel string, layout string, outputCommand changeGitOutput) (changeExecutionStatus, error) {
	if outputCommand == nil {
		outputCommand = commandOutput
	}
	commits, err := outputCommand(rootPath, "git", "log", "--format=%H", "HEAD", "--", folderRel)
	if err != nil {
		return changeExecutionStatus{}, err
	}
	status := changeExecutionStatus{}
	for _, commit := range strings.Split(strings.TrimSpace(commits), "\n") {
		commit = strings.TrimSpace(commit)
		if commit == "" {
			continue
		}
		pathsOut, err := outputCommand(rootPath, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", commit)
		if err != nil {
			return changeExecutionStatus{}, err
		}
		paths := strings.Split(strings.TrimSpace(pathsOut), "\n")
		hasTaskSurface := false
		hasOutside := false
		var taskPaths []string
		for _, p := range paths {
			p = filepath.ToSlash(strings.TrimSpace(p))
			if p == "" {
				continue
			}
			if !strings.HasPrefix(p, "docs/changes/") {
				hasOutside = true
				continue
			}
			if !strings.HasPrefix(p, folderRel+"/") && p != folderRel {
				continue
			}
			switch layout {
			case changeLayoutNew:
				if strings.Contains(p, "/tasks/") || strings.HasSuffix(p, "/tasks") {
					hasTaskSurface = true
					taskPaths = append(taskPaths, p)
				}
			default:
				if strings.HasSuffix(p, "/change.md") || filepath.Base(p) == "change.md" {
					hasTaskSurface = true
					taskPaths = append(taskPaths, p)
				}
			}
		}
		if !(hasTaskSurface && hasOutside) {
			continue
		}
		status.PathExecuted = true
		if layout == changeLayoutNew {
			flipped, err := commitFlipsTaskCheckboxes(rootPath, commit, taskPaths, outputCommand)
			if err != nil {
				return changeExecutionStatus{}, err
			}
			if flipped {
				status.FlipExecuted = true
				return status, nil
			}
		} else {
			// Legacy path grade only for display; flip grade never satisfied on legacy.
		}
	}
	return status, nil
}

func commitFlipsTaskCheckboxes(rootPath, commit string, taskPaths []string, outputCommand changeGitOutput) (bool, error) {
	if outputCommand == nil {
		outputCommand = commandOutput
	}
	parent, hasParent, err := changeCommitFirstParent(rootPath, commit, outputCommand)
	if err != nil {
		return false, err
	}
	for _, path := range taskPaths {
		// Fence state is a whole-file property: an opening fence can sit any
		// distance above the flipped line, so it is derived from the complete
		// pre-image and post-image rather than inferred from the patch window.
		var preFenced, postFenced changeFencedLines
		if hasParent {
			pre, exists, err := readCommittedOptional(rootPath, parent, path, outputCommand)
			if err != nil {
				return false, err
			}
			if exists {
				preFenced = markdownFencedLines(pre)
			}
		}
		post, exists, err := readCommittedOptional(rootPath, commit, path, outputCommand)
		if err != nil {
			return false, err
		}
		if exists {
			postFenced = markdownFencedLines(post)
		}
		// unified=3 fixes the hunk grouping the flip grammar reads (same-hunk,
		// same-normalized-label); it no longer carries fence context.
		diff, err := outputCommand(rootPath, "git", "show", "--format=", "--unified=3", commit, "--", path)
		if err != nil {
			return false, err
		}
		if diffContainsCheckboxFlip(diff, preFenced, postFenced) {
			return true, nil
		}
	}
	return false, nil
}

// changeCommitFirstParent resolves the pre-image ref for a commit. A root commit
// reports no parent rather than failing, so a change whose first commit creates
// its task files is scored, not errored.
func changeCommitFirstParent(rootPath, commit string, outputCommand changeGitOutput) (string, bool, error) {
	out, err := outputCommand(rootPath, "git", "log", "--max-count=1", "--format=%P", commit)
	if err != nil {
		return "", false, fmt.Errorf("read parents of %s: %w", shortChangeCommit(commit), err)
	}
	parents := strings.Fields(out)
	if len(parents) == 0 {
		return "", false, nil
	}
	return parents[0], true, nil
}

func normalizeCheckboxLabel(label string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(label)), " ")
}

func isMarkdownFenceMarker(body string) bool {
	return strings.HasPrefix(strings.TrimSpace(body), "```")
}

// changeFencedLines records, by 1-based line number, which lines of one file
// image sit inside a fenced region (marker lines included). A nil map means no
// image existed — file creation or deletion — and nothing is fenced.
type changeFencedLines map[int]bool

func (f changeFencedLines) fenced(line int) bool {
	if f == nil {
		return false
	}
	return f[line]
}

func markdownFencedLines(content string) changeFencedLines {
	fenced := changeFencedLines{}
	inFence := false
	for index, line := range strings.Split(content, "\n") {
		marker := isMarkdownFenceMarker(line)
		if marker || inFence {
			fenced[index+1] = true
		}
		if marker {
			inFence = !inFence
		}
	}
	return fenced
}

// diffContainsCheckboxFlip scores a single-file patch: it requires a same-hunk
// `- [ ]`→`- [x]` pair with the same normalized label whose removed line is
// unfenced in the pre-image and whose added line is unfenced in the post-image.
// Hunk headers supply the file positions; an unparseable header ends the hunk
// without crediting a flip, keeping the failure direction on non-events.
func diffContainsCheckboxFlip(diff string, preFenced, postFenced changeFencedLines) bool {
	var removed map[string]struct{}
	var added map[string]struct{}
	inHunk := false
	oldLine, newLine := 0, 0

	hunkHasFlip := func() bool {
		for label := range removed {
			if _, ok := added[label]; ok {
				return true
			}
		}
		return false
	}

	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "@@") {
			if inHunk && hunkHasFlip() {
				return true
			}
			match := changeDiffHunkRE.FindStringSubmatch(line)
			if match == nil {
				inHunk = false
				continue
			}
			oldStart, oldErr := strconv.Atoi(match[1])
			newStart, newErr := strconv.Atoi(match[2])
			if oldErr != nil || newErr != nil {
				inHunk = false
				continue
			}
			oldLine, newLine = oldStart, newStart
			inHunk = true
			removed = make(map[string]struct{})
			added = make(map[string]struct{})
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		body := line[1:]
		switch line[0] {
		case ' ':
			oldLine++
			newLine++
		case '-':
			if !preFenced.fenced(oldLine) {
				if m := changeFlipUncheckedRE.FindStringSubmatch(body); m != nil {
					removed[normalizeCheckboxLabel(m[1])] = struct{}{}
				}
			}
			oldLine++
		case '+':
			if !postFenced.fenced(newLine) {
				if m := changeFlipCheckedRE.FindStringSubmatch(body); m != nil {
					added[normalizeCheckboxLabel(m[1])] = struct{}{}
				}
			}
			newLine++
		}
	}
	return inHunk && hunkHasFlip()
}

// formatChangeExecutionBlock renders the execution refusal. The squash branch
// exists because that refusal is otherwise unactionable: checked packets plus
// landed code plus no vouching receipt is exactly what a squash merge leaves
// behind, and the operator needs the cause and the one command that fixes it.
func formatChangeExecutionBlock(slug, target string, layout string, evidence changeMemberEvidence, materialized bool) string {
	if !materialized {
		return fmt.Sprintf("release blocked: change %q targets %s but is not materialized", slug, target)
	}
	if layout == changeLayoutLegacy {
		return fmt.Sprintf("release blocked: change %q targets %s but is legacy layout — convert first", slug, target)
	}
	if evidence.executed() {
		return ""
	}
	if evidence.AllTasksChecked && evidence.Status.PathExecuted {
		return fmt.Sprintf("release blocked: change %q targets %s but is not executed: every task box is checked and code landed outside docs/changes/, but no receipt vouches for this tree (%s) — a squash merge rewrites the checkbox flips the first grading path reads. Run: loaf change verify %s, then commit the receipt",
			slug, target, evidence.Verdict.Cause(), evidence.FolderRel)
	}
	return fmt.Sprintf("release blocked: change %q targets %s but is not executed", slug, target)
}

// formatChangeReceiptBlock renders a cohort receipt failure from a typed
// verdict. Every block names the folder, the cause, and a copy-pasteable remedy
// — preflight never runs criteria.
func formatChangeReceiptBlock(slug, target string, verdict changeReceiptVerdict, folder string) string {
	folder = filepath.ToSlash(folder)
	cause := verdict.Cause()
	if verdict.Reason == changeReceiptFailingResults {
		return fmt.Sprintf("change %q targets %s but %s. Fix the failing criteria, then run: loaf change verify %s and commit the receipt", slug, target, cause, folder)
	}
	if verdict.Reason == changeReceiptEvidenceUnavailable {
		return fmt.Sprintf("change %q targets %s but %s. Verification cannot proceed until git reads succeed — inspect the repository (git fsck) or re-clone", slug, target, cause)
	}
	remedy := fmt.Sprintf("Run: loaf change verify %s, then commit the receipt", folder)
	return fmt.Sprintf("change %q targets %s but %s. %s", slug, target, cause, remedy)
}

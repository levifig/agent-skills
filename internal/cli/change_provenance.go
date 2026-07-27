package cli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Execution provenance grades (Planning Contract / TASK-004).
// Path grade: a commit modifies tasks/ (or legacy change.md) plus a path
// outside docs/changes/ entirely — feeds derived display.
// Flip grade: that commit's diff also flips `- [ ]`→`- [x]` outside fences —
// what gates cohort members.

var (
	changeFlipUncheckedRE = regexp.MustCompile(`(?i)^\s*- \[ \]\s*(.*)$`)
	changeFlipCheckedRE   = regexp.MustCompile(`(?i)^\s*- \[x\]\s*(.*)$`)
)

type changeExecutionStatus struct {
	PathExecuted bool
	FlipExecuted bool
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
	for _, path := range taskPaths {
		// unified=3 keeps nearby fence markers as context so fence state is trackable.
		diff, err := outputCommand(rootPath, "git", "show", "--format=", "--unified=3", commit, "--", path)
		if err != nil {
			return false, err
		}
		if diffContainsCheckboxFlip(diff) {
			return true, nil
		}
	}
	return false, nil
}

func normalizeCheckboxLabel(label string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(label)), " ")
}

func isMarkdownFenceMarker(body string) bool {
	return strings.HasPrefix(strings.TrimSpace(body), "```")
}

func diffContainsCheckboxFlip(diff string) bool {
	// Require a same-hunk `- [ ]`→`- [x]` pair with the same normalized label,
	// ignoring checkboxes inside fenced regions on either side of the patch.
	inFenceOld := false
	inFenceNew := false
	var removed map[string]struct{}
	var added map[string]struct{}

	hunkHasFlip := func() bool {
		for label := range removed {
			if _, ok := added[label]; ok {
				return true
			}
		}
		return false
	}

	startHunk := func() {
		removed = make(map[string]struct{})
		added = make(map[string]struct{})
	}

	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		if strings.HasPrefix(line, "@@") {
			if removed != nil && hunkHasFlip() {
				return true
			}
			startHunk()
			continue
		}
		if removed == nil || line == "" {
			continue
		}
		prefix := line[0]
		if prefix != '+' && prefix != '-' && prefix != ' ' {
			continue
		}
		body := line[1:]
		switch prefix {
		case ' ':
			if isMarkdownFenceMarker(body) {
				inFenceOld = !inFenceOld
				inFenceNew = !inFenceNew
			}
		case '-':
			if isMarkdownFenceMarker(body) {
				inFenceOld = !inFenceOld
				continue
			}
			if inFenceOld {
				continue
			}
			if m := changeFlipUncheckedRE.FindStringSubmatch(body); m != nil {
				removed[normalizeCheckboxLabel(m[1])] = struct{}{}
			}
		case '+':
			if isMarkdownFenceMarker(body) {
				inFenceNew = !inFenceNew
				continue
			}
			if inFenceNew {
				continue
			}
			if m := changeFlipCheckedRE.FindStringSubmatch(body); m != nil {
				added[normalizeCheckboxLabel(m[1])] = struct{}{}
			}
		}
	}
	return removed != nil && hunkHasFlip()
}

func formatChangeExecutionBlock(slug, target string, layout string, status changeExecutionStatus, materialized bool) string {
	if !materialized {
		return fmt.Sprintf("release blocked: change %q targets %s but is not materialized", slug, target)
	}
	if layout == changeLayoutLegacy {
		return fmt.Sprintf("release blocked: change %q targets %s but is legacy layout — convert first", slug, target)
	}
	if !status.FlipExecuted {
		return fmt.Sprintf("release blocked: change %q targets %s but is not executed", slug, target)
	}
	return ""
}

// formatChangeReceiptBlock renders a cohort receipt failure. Failing criteria
// are stated plainly; freshness failures keep the "not current" framing.
func formatChangeReceiptBlock(slug, target, reason string) string {
	if strings.HasPrefix(reason, "receipt records failing criteria") {
		return fmt.Sprintf("change %q targets %s but %s", slug, target, reason)
	}
	return fmt.Sprintf("change %q targets %s but receipt is not current (%s)", slug, target, reason)
}

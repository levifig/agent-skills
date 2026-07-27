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
	changeFlipAddedRE      = regexp.MustCompile(`(?m)^\+\s*- \[x\]`)
	changeFlipRemovedRE    = regexp.MustCompile(`(?m)^-\s*- \[ \]`)
	changeFlipAddedLoose   = regexp.MustCompile(`(?mi)^\+\s*- \[x\]`)
	changeFlipRemovedLoose = regexp.MustCompile(`(?mi)^-\s*- \[ \]`)
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
		diff, err := outputCommand(rootPath, "git", "show", "--format=", "--unified=0", commit, "--", path)
		if err != nil {
			return false, err
		}
		if diffContainsCheckboxFlip(diff) {
			return true, nil
		}
	}
	return false, nil
}

func diffContainsCheckboxFlip(diff string) bool {
	// Strip diff headers and fenced regions inside added/removed content is hard;
	// operate line-wise on the patch and ignore lines that look like fence markers.
	hasAdd := false
	hasRemove := false
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "@@") {
			continue
		}
		body := line
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			body = line[1:]
		}
		trim := strings.TrimSpace(body)
		if strings.HasPrefix(trim, "```") {
			continue
		}
		if changeFlipAddedLoose.MatchString(line) {
			hasAdd = true
		}
		if changeFlipRemovedLoose.MatchString(line) {
			hasRemove = true
		}
	}
	return hasAdd && hasRemove
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

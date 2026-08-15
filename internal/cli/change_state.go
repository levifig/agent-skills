package cli

import (
	"fmt"
	"path/filepath"
)

// changeEvidenceGitOutput is the git seam for verified-rung evidence reads.
// Production points at commandOutput; tests may redirect it to simulate load failures.
// Callers pass it (or another seam) into deriveChangeState*; nil falls back here.
var changeEvidenceGitOutput changeGitOutput = commandOutput

// deriveChangeState returns the single derived-state ladder shared by list,
// show, and check: captured → shaped → executable → executing → complete.
//
// complete means every task checkbox is checked. Verification authority is
// ship, not a receipt rung on this ladder.
func deriveChangeState(rootPath string, node changeNode, outputCommand changeGitOutput) string {
	state, _ := deriveChangeStateDetailed(rootPath, node, outputCommand)
	return state
}

// deriveChangeStateDetailed returns the ladder state plus warnings for evidence
// evaluation failures that demoted the member (fail-closed stays; silence goes).
func deriveChangeStateDetailed(rootPath string, node changeNode, outputCommand changeGitOutput) (string, []string) {
	if outputCommand == nil {
		outputCommand = changeEvidenceGitOutput
	}
	var warnings []string
	if node.CapturedOnly {
		return "captured", warnings
	}
	report := evaluateChangeNode(node, "")
	if !report.Executable {
		return "shaped", warnings
	}
	status, err := changeFolderExecuted(rootPath, node.Folder, node.Layout, outputCommand)
	if err != nil {
		warnings = append(warnings, "execution provenance failed: "+err.Error())
		return "executable", warnings
	}
	if !status.PathExecuted {
		return "executable", warnings
	}
	if changeAllTaskCheckboxesChecked(rootPath, node) {
		return "complete", warnings
	}
	return "executing", warnings
}

// changeStructurallyCleanForState reports whether the committed HEAD node is
// structurally clean. Evidence is committed HEAD only: nodes and task files are
// never read from the working tree here, so a dirty checkout cannot flip the
// verdict either way. On load/evaluation error it returns false with a reason
// (fail-closed, not silent).
func changeStructurallyCleanForState(rootPath string, node changeNode, outputCommand changeGitOutput) (bool, string) {
	if outputCommand == nil {
		outputCommand = changeEvidenceGitOutput
	}
	nodes, err := loadChangeNodesAtHEADWithOutput(rootPath, outputCommand)
	if err != nil {
		return false, err.Error()
	}
	headNode, found := changeNodeForFolder(nodes, node.Folder)
	if !found {
		headNode, found = changeNodeForSlug(nodes, node.Slug)
	}
	if !found {
		return false, fmt.Sprintf("change %q missing from committed HEAD", node.Slug)
	}
	folderAbs := filepath.Join(rootPath, filepath.FromSlash(headNode.Folder))
	report, reportErr := composeChangeCheckReport(evaluateChangeNode(headNode, ""), rootPath, folderAbs, headNode, nodes, outputCommand, false, changeTaskContentHEAD)
	if reportErr != nil {
		return false, reportErr.Error()
	}
	return len(report.Violations) == 0 && report.Executable, ""
}

func changeNodeForFolder(nodes []changeNode, folder string) (changeNode, bool) {
	folder = filepath.ToSlash(folder)
	for _, n := range nodes {
		if filepath.ToSlash(n.Folder) == folder {
			return n, true
		}
	}
	return changeNode{}, false
}

func changeNodeForSlug(nodes []changeNode, slug string) (changeNode, bool) {
	for _, n := range nodes {
		if n.Slug == slug {
			return n, true
		}
	}
	return changeNode{}, false
}

// changeAllTaskCheckboxesChecked reports whether the change has at least one
// checkbox and every checkbox across its task files is checked.
func changeAllTaskCheckboxesChecked(rootPath string, node changeNode) bool {
	if node.Layout != changeLayoutNew {
		return false
	}
	folderAbs := filepath.Join(rootPath, filepath.FromSlash(node.Folder))
	tasks, _, _ := loadChangeTasks(rootPath, folderAbs, node, changeTaskContentWorkingTree, commandOutput)
	total := 0
	done := 0
	for _, task := range tasks {
		total += task.CheckboxTotal
		done += task.CheckboxDone
	}
	return total > 0 && done == total
}

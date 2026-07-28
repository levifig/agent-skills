package cli

import (
	"fmt"
	"path/filepath"
)

// changeEvidenceGitOutput is the git seam for verified-rung evidence reads.
// Production points at commandOutput; tests may redirect it to simulate load failures.
var changeEvidenceGitOutput changeGitOutput = commandOutput

// deriveChangeState returns the single derived-state ladder shared by list,
// show, and check: captured → shaped → executable → executing → complete,
// plus verified for changes that declare a target_release.
//
// complete means every task checkbox is checked. verified requires a fresh
// receipt whose criteria all passed AND the same structural composite the
// cohort gate applies (lineage-inclusive) — a structurally rejected member
// never displays verified where the gate would refuse. Unchecked boxes on a
// verified cohort member remain legal descoped work (Decision 15).
func deriveChangeState(rootPath string, node changeNode, outputCommand changeGitOutput) string {
	state, _ := deriveChangeStateDetailed(rootPath, node, outputCommand)
	return state
}

// deriveChangeStateDetailed returns the ladder state plus warnings for evidence
// evaluation failures that demoted the member (fail-closed stays; silence goes).
func deriveChangeStateDetailed(rootPath string, node changeNode, outputCommand changeGitOutput) (string, []string) {
	if outputCommand == nil {
		outputCommand = commandOutput
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
	if err != nil || !status.PathExecuted {
		return "executable", warnings
	}
	if node.TargetRelease != "" {
		ok, _, receiptErr := changeReceiptStatus(rootPath, node.Folder, node, outputCommand)
		clean, evalErr := changeStructurallyCleanForState(rootPath, node, changeEvidenceGitOutput)
		if evalErr != "" {
			warnings = append(warnings, "structural evaluation failed: "+evalErr)
		}
		if receiptErr == nil && ok && clean {
			return "verified", warnings
		}
	}
	if changeAllTaskCheckboxesChecked(rootPath, node) {
		return "complete", warnings
	}
	return "executing", warnings
}

// changeStructurallyCleanForState reports whether the gate's structural
// composite is clean for this node — the verified rung must agree with the gate.
// Evidence is committed HEAD only: nodes and task files are never read from the
// working tree here, so a dirty checkout cannot flip the verdict either way.
// On load/evaluation error it returns false with a reason (fail-closed, not silent).
func changeStructurallyCleanForState(rootPath string, node changeNode, outputCommand changeGitOutput) (bool, string) {
	if outputCommand == nil {
		outputCommand = changeEvidenceGitOutput
	}
	nodes, err := loadChangeNodesAtHEADWithOutput(rootPath, outputCommand)
	if err != nil {
		return false, err.Error()
	}
	headNode, ok := changeNodeForFolder(nodes, node.Folder)
	if !ok {
		headNode, ok = changeNodeForSlug(nodes, node.Slug)
	}
	if !ok {
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

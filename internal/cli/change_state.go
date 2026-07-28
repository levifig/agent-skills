package cli

import (
	"path/filepath"
)

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
	if outputCommand == nil {
		outputCommand = commandOutput
	}
	if node.CapturedOnly {
		return "captured"
	}
	report := evaluateChangeNode(node, "")
	if !report.Executable {
		return "shaped"
	}
	status, err := changeFolderExecuted(rootPath, node.Folder, node.Layout, outputCommand)
	if err != nil || !status.PathExecuted {
		return "executable"
	}
	if node.TargetRelease != "" {
		ok, _, receiptErr := changeReceiptStatus(rootPath, node.Folder, node, outputCommand)
		if receiptErr == nil && ok && changeStructurallyCleanForState(rootPath, node, outputCommand) {
			return "verified"
		}
	}
	if changeAllTaskCheckboxesChecked(rootPath, node) {
		return "complete"
	}
	return "executing"
}

// changeStructurallyCleanForState reports whether the gate's structural
// composite is clean for this node — the verified rung must agree with the gate.
func changeStructurallyCleanForState(rootPath string, node changeNode, outputCommand changeGitOutput) bool {
	nodes, err := loadChangeNodes(rootPath)
	if err != nil {
		return false
	}
	folderAbs := filepath.Join(rootPath, filepath.FromSlash(node.Folder))
	report, reportErr := composeChangeCheckReport(evaluateChangeNode(node, ""), rootPath, folderAbs, node, nodes, outputCommand, false)
	if reportErr != nil {
		return false
	}
	return len(report.Violations) == 0 && report.Executable
}

// changeAllTaskCheckboxesChecked reports whether the change has at least one
// checkbox and every checkbox across its task files is checked.
func changeAllTaskCheckboxesChecked(rootPath string, node changeNode) bool {
	if node.Layout != changeLayoutNew {
		return false
	}
	folderAbs := filepath.Join(rootPath, filepath.FromSlash(node.Folder))
	tasks, _, _ := loadChangeTasks(rootPath, folderAbs, node)
	total := 0
	done := 0
	for _, task := range tasks {
		total += task.CheckboxTotal
		done += task.CheckboxDone
	}
	return total > 0 && done == total
}

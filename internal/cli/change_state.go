package cli

import (
	"fmt"
	"path/filepath"
	"strings"
)

// changeEvidenceGitOutput is the git seam for verified-rung evidence reads.
// Production points at commandOutput; tests may redirect it to simulate load failures.
// Callers pass it (or another seam) into deriveChangeState*; nil falls back here.
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
	if node.TargetRelease != "" {
		_, evidenceGit, pinErr := pinEvidenceAtHEAD(rootPath, outputCommand)
		if pinErr != nil {
			warnings = append(warnings, "evidence pin failed: "+pinErr.Error())
		} else {
			ok, receiptErr, clean, evalErr := evaluateVerifiedRungAtCommit(rootPath, node, evidenceGit)
			if evalErr != "" {
				warnings = append(warnings, "structural evaluation failed: "+evalErr)
			}
			if receiptErr != nil {
				warnings = append(warnings, "receipt evaluation failed: "+receiptErr.Error())
			}
			if receiptErr == nil && ok && clean {
				return "verified", warnings
			}
		}
	}
	if changeAllTaskCheckboxesChecked(rootPath, node) {
		return "complete", warnings
	}
	return "executing", warnings
}

// pinEvidenceAtHEAD resolves HEAD to a SHA once and returns a git seam that
// rewrites every symbolic HEAD token to that SHA, so one derivation cannot split
// node / task / receipt reads across a commit that lands mid-flight.
func pinEvidenceAtHEAD(rootPath string, outputCommand changeGitOutput) (string, changeGitOutput, error) {
	if outputCommand == nil {
		outputCommand = changeEvidenceGitOutput
	}
	sha, err := outputCommand(rootPath, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", nil, err
	}
	sha = strings.TrimSpace(sha)
	return sha, rewriteHEADRef(sha, outputCommand), nil
}

func rewriteHEADRef(sha string, inner changeGitOutput) changeGitOutput {
	return func(cwd, name string, args ...string) (string, error) {
		out := make([]string, len(args))
		for i, arg := range args {
			out[i] = rewriteHEADToken(arg, sha)
		}
		return inner(cwd, name, out...)
	}
}

func rewriteHEADToken(arg, sha string) string {
	switch {
	case arg == "HEAD":
		return sha
	case strings.HasPrefix(arg, "HEAD:"):
		return sha + arg[len("HEAD"):]
	case strings.HasSuffix(arg, "..HEAD"):
		return strings.TrimSuffix(arg, "HEAD") + sha
	case strings.HasSuffix(arg, "...HEAD"):
		return strings.TrimSuffix(arg, "HEAD") + sha
	case strings.HasPrefix(arg, "HEAD.."):
		return sha + arg[len("HEAD"):]
	case strings.HasPrefix(arg, "HEAD..."):
		return sha + arg[len("HEAD"):]
	default:
		return arg
	}
}

// evaluateVerifiedRungAtCommit loads the committed node once and feeds that same
// node (folder + content) into both the structural composite and the receipt
// check — never the working-tree node the ladder received.
func evaluateVerifiedRungAtCommit(rootPath string, node changeNode, outputCommand changeGitOutput) (ok bool, receiptErr error, clean bool, evalErr string) {
	if outputCommand == nil {
		outputCommand = changeEvidenceGitOutput
	}
	nodes, err := loadChangeNodesAtHEADWithOutput(rootPath, outputCommand)
	if err != nil {
		return false, nil, false, err.Error()
	}
	headNode, found := changeNodeForFolder(nodes, node.Folder)
	if !found {
		headNode, found = changeNodeForSlug(nodes, node.Slug)
	}
	if !found {
		return false, nil, false, fmt.Sprintf("change %q missing from committed HEAD", node.Slug)
	}
	folderAbs := filepath.Join(rootPath, filepath.FromSlash(headNode.Folder))
	report, reportErr := composeChangeCheckReport(evaluateChangeNode(headNode, ""), rootPath, folderAbs, headNode, nodes, outputCommand, false, changeTaskContentHEAD)
	if reportErr != nil {
		return false, nil, false, reportErr.Error()
	}
	clean = len(report.Violations) == 0 && report.Executable
	verdict, receiptErr := changeReceiptStatus(rootPath, headNode.Folder, headNode, outputCommand)
	ok = verdict.OK
	return ok, receiptErr, clean, ""
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
	_, _, clean, evalErr := evaluateVerifiedRungAtCommit(rootPath, node, outputCommand)
	return clean, evalErr
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

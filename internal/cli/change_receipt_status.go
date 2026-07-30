package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// changeReceiptReason is the typed freshness verdict. Rendering derives from the
// type; consumers must not parse prose (ADR-024).
type changeReceiptReason int

const (
	changeReceiptOK changeReceiptReason = iota
	changeReceiptMissing
	changeReceiptUncommitted
	changeReceiptUnreadable
	changeReceiptUnsupportedSchema
	changeReceiptCriteriaMismatch
	changeReceiptContentDrift
	changeReceiptBoundaryChanged
	changeReceiptResultsGap
	changeReceiptFailingResults
	changeReceiptDirtyExecution
	changeReceiptEvidenceUnavailable
)

// changeReceiptVerdict is the pure freshness outcome for one change folder.
type changeReceiptVerdict struct {
	OK              bool
	Reason          changeReceiptReason
	DriftedSections []string
	FailedIDs       []string
	MissingIDs      []string
	SchemaVersion   int
}

// Cause returns the stable cause fragment used in tests and block messages.
func (v changeReceiptVerdict) Cause() string {
	switch v.Reason {
	case changeReceiptOK:
		return ""
	case changeReceiptMissing:
		return "missing receipt"
	case changeReceiptUncommitted:
		return "receipt not committed at HEAD"
	case changeReceiptUnreadable:
		return "receipt unreadable — re-verify"
	case changeReceiptUnsupportedSchema:
		return fmt.Sprintf("unsupported receipt schema_version %d", v.SchemaVersion)
	case changeReceiptCriteriaMismatch:
		return "criteria changed (receipt expired)"
	case changeReceiptContentDrift:
		if len(v.DriftedSections) == 0 {
			return "content changed since verification"
		}
		quoted := make([]string, len(v.DriftedSections))
		for i, s := range v.DriftedSections {
			quoted[i] = "`" + s + "`"
		}
		return fmt.Sprintf("content changed since verification (content changed under %s)", strings.Join(quoted, ", "))
	case changeReceiptBoundaryChanged:
		return "evidence boundary changed since verification (receipt expired)"
	case changeReceiptResultsGap:
		return fmt.Sprintf("receipt results missing criteria (%s)", strings.Join(v.MissingIDs, ", "))
	case changeReceiptFailingResults:
		return fmt.Sprintf("receipt records failing criteria (%s)", strings.Join(v.FailedIDs, ", "))
	case changeReceiptDirtyExecution:
		return "verify ran against a worktree that diverged from HEAD (receipt void)"
	case changeReceiptEvidenceUnavailable:
		return "could not read evidence at HEAD (git error)"
	default:
		return "receipt not current"
	}
}

// changeReceiptStatus reports whether a committed receipt attests successful
// verification of the pinned HEAD tree. The verdict is a pure function of
// receipt fields and HEAD content — no refs, no reachability, no worktree for
// the verdict itself. changeReceiptExistsInWorkingTree only refines the missing
// vs uncommitted cause when HEAD has no receipt. Git-seam failures are typed
// blocking verdicts, never errors.
func changeReceiptStatus(rootPath, folderRel string, node changeNode, outputCommand changeGitOutput) changeReceiptVerdict {
	if outputCommand == nil {
		outputCommand = commandOutput
	}
	receiptRel := changeReceiptRelPath(folderRel)
	content, found, err := readCommittedOptional(rootPath, "HEAD", receiptRel, outputCommand)
	if err != nil {
		return changeReceiptVerdict{Reason: changeReceiptEvidenceUnavailable}
	}
	if !found {
		if changeReceiptExistsInWorkingTree(rootPath, folderRel) {
			return changeReceiptVerdict{Reason: changeReceiptUncommitted}
		}
		return changeReceiptVerdict{Reason: changeReceiptMissing}
	}
	var receipt changeVerifyReceipt
	if err := json.Unmarshal([]byte(content), &receipt); err != nil {
		return changeReceiptVerdict{Reason: changeReceiptUnreadable}
	}
	if receipt.SchemaVersion != 2 {
		return changeReceiptVerdict{Reason: changeReceiptUnsupportedSchema, SchemaVersion: receipt.SchemaVersion}
	}
	if !receipt.WorktreeClean {
		return changeReceiptVerdict{Reason: changeReceiptDirtyExecution}
	}
	currentExclusions := ChangeEvidenceExclusions()
	if !slices.Equal(receipt.Exclusions, currentExclusions) || receipt.DigestSpec != ChangeEvidenceDigestSpec {
		return changeReceiptVerdict{Reason: changeReceiptBoundaryChanged}
	}
	criteria := parseChangeExecutableCriteria(node.Content)
	if changeCriteriaDigest(criteria) != receipt.CriteriaDigest {
		return changeReceiptVerdict{Reason: changeReceiptCriteriaMismatch}
	}
	if failed := receiptFailingCriterionIDs(receipt); len(failed) > 0 {
		return changeReceiptVerdict{Reason: changeReceiptFailingResults, FailedIDs: failed}
	}
	if missing := receiptMissingCriterionIDs(receipt, criteria); len(missing) > 0 {
		return changeReceiptVerdict{Reason: changeReceiptResultsGap, MissingIDs: missing}
	}
	head, err := outputCommand(rootPath, "git", "rev-parse", "HEAD")
	if err != nil {
		return changeReceiptVerdict{Reason: changeReceiptEvidenceUnavailable}
	}
	head = strings.TrimSpace(head)
	scope, err := scopeDigest(rootPath, head, currentExclusions, outputCommand)
	if err != nil {
		return changeReceiptVerdict{Reason: changeReceiptEvidenceUnavailable}
	}
	if scope.Digest != receipt.ScopeDigest {
		return changeReceiptVerdict{
			Reason:          changeReceiptContentDrift,
			DriftedSections: driftedScopeSections(receipt.ScopeSections, scope.Sections),
		}
	}
	return changeReceiptVerdict{OK: true, Reason: changeReceiptOK}
}

func receiptMissingCriterionIDs(receipt changeVerifyReceipt, criteria []changeCriterion) []string {
	have := map[string]bool{}
	for _, r := range receipt.Results {
		have[r.ID] = true
	}
	var missing []string
	for _, c := range criteria {
		if !have[c.ID] {
			missing = append(missing, c.ID)
		}
	}
	return missing
}

func driftedScopeSections(recorded, current map[string]string) []string {
	keys := map[string]struct{}{}
	for k := range recorded {
		keys[k] = struct{}{}
	}
	for k := range current {
		keys[k] = struct{}{}
	}
	var drifted []string
	for k := range keys {
		if recorded[k] != current[k] {
			drifted = append(drifted, k)
		}
	}
	slices.Sort(drifted)
	return drifted
}

func changeReceiptExistsInWorkingTree(rootPath, folderRel string) bool {
	folderAbs := filepath.Join(rootPath, filepath.FromSlash(folderRel))
	_, err := os.Stat(filepath.Join(folderAbs, filepath.FromSlash(changeVerifyReceiptFile)))
	return err == nil
}

package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// importLifecycleStatusResult is the insert-time classification of a source status
// for lifecycle-managed entity kinds.
type importLifecycleStatusResult struct {
	Status          string
	OutOfVocabulary bool
	Warning         string
}

// classifyImportLifecycleStatus maps an incoming source status for insert via
// the U1 decideImportStatus matrix (stored=nil).
func classifyImportLifecycleStatus(kind string, entityID string, raw string, noOpinionInsert string) importLifecycleStatusResult {
	decision := decideImportStatus(kind, entityID, nil, raw, noOpinionInsert, true)
	return importLifecycleStatusResult{
		Status:          decision.Status,
		OutOfVocabulary: decision.Warning != "",
		Warning:         decision.Warning,
	}
}

func (m markdownImporter) noteImportStatusWarning(warning string) {
	if warning == "" || m.report == nil {
		return
	}
	m.report.Warnings = append(m.report.Warnings, warning)
}

func (m markdownImporter) recordStatusDecision(decision importStatusDecision) {
	if m.report == nil {
		return
	}
	if decision.Warning != "" {
		m.report.Warnings = append(m.report.Warnings, decision.Warning)
	}
	if decision.Divergence != nil {
		m.report.StatusDivergences = append(m.report.StatusDivergences, *decision.Divergence)
	}
}

// applyImportLifecycleStatus is insert-only classification used by call sites
// that still classify before reading an existing row. Prefer
// resolveImportStatus for upserts that enforce the full matrix.
func (m markdownImporter) applyImportLifecycleStatus(kind string, entityID string, raw string, noOpinionInsert string) string {
	result := classifyImportLifecycleStatus(kind, entityID, raw, noOpinionInsert)
	if result.OutOfVocabulary {
		m.noteImportStatusWarning(result.Warning)
	}
	return result.Status
}

// resolveImportStatus reads any existing row status and applies the normative
// stored×incoming matrix. write is true when the upsert must set status.
func (m markdownImporter) resolveImportStatus(ctx context.Context, table string, entityKind string, entityID string, raw string, insertDefault string, hasVocabulary bool) (status string, write bool, err error) {
	var stored string
	scanErr := m.tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT status FROM %s WHERE project_id = ? AND id = ?`, table), m.projectID, entityID).Scan(&stored)
	var storedPtr *string
	switch {
	case scanErr == nil:
		storedPtr = &stored
	case errors.Is(scanErr, sql.ErrNoRows):
		storedPtr = nil
	default:
		return "", false, fmt.Errorf("read %s %s status: %w", table, entityID, scanErr)
	}
	decision := decideImportStatus(entityKind, entityID, storedPtr, raw, insertDefault, hasVocabulary)
	m.recordStatusDecision(decision)
	return decision.Status, decision.Write, nil
}

package state

import (
	"database/sql"
	"fmt"
	"strings"
)

// ImportReport captures in-transaction markdown import outcomes. Inventory I/O
// and parser warnings stay on MarkdownMigrationPlan.Warnings; this report holds
// status and provenance outcomes only.
type ImportReport struct {
	ReclaimedOrigins  int                      `json:"reclaimed_origins"`
	SkippedEntries    []ImportSkippedEntry     `json:"skipped_entries"`
	StatusDivergences []ImportStatusDivergence `json:"status_divergences"`
	Warnings          []string                 `json:"warnings"`
}

// ImportSkippedEntry records a journal line left untouched because its origin
// failed the reclaim fingerprint.
type ImportSkippedEntry struct {
	JournalEntryID   string `json:"journal_entry_id"`
	CaptureMechanism string `json:"capture_mechanism"`
}

// ImportStatusDivergence records a kept stored status that disagreed with a
// normalized incoming status.
type ImportStatusDivergence struct {
	EntityKind     string `json:"entity_kind"`
	EntityID       string `json:"entity_id"`
	StoredStatus   string `json:"stored_status"`
	IncomingStatus string `json:"incoming_status"`
}

// originImportAction is the disposition for an existing (or absent) origin row.
type originImportAction int

const (
	originImportInsert originImportAction = iota
	originImportRefreshMigration
	originImportReclaim
	originImportSkip
)

type journalOriginScanRow struct {
	CaptureMechanism       string
	EnvelopeVersion        int
	ObservedHarness        sql.NullString
	ObservedHarnessVersion sql.NullString
	HarnessSessionID       sql.NullString
	AgentID                sql.NullString
	SourceEvent            sql.NullString
	Branch                 sql.NullString
	Worktree               sql.NullString
	Head                   sql.NullString
	ChangePath             sql.NullString
	ChangeSHA256           sql.NullString
	Dirty                  sql.NullInt64
	Reconstructable        sql.NullInt64
	DurableResultKind      sql.NullString
	DurableResultID        sql.NullString
	CreatedAt              string
}

type journalEntryScanRow struct {
	HarnessSessionID sql.NullString
	ObservedBranch   sql.NullString
	ObservedWorktree sql.NullString
	CreatedAt        string
}

type incomingStatusClass int

const (
	incomingStatusNoOpinion incomingStatusClass = iota
	incomingStatusNormalized
	incomingStatusOutOfVocabulary
)

type importStatusDecision struct {
	Status     string
	Write      bool
	Divergence *ImportStatusDivergence
	Warning    string
}

// decideOriginImportDisposition implements Decision 1 over (origin, journal).
// A nil origin means insert. A migration origin refreshes. A full 0011-compatible
// unknown fingerprint reclaims. Everything else skips the whole entry.
func decideOriginImportDisposition(origin *journalOriginScanRow, journal *journalEntryScanRow) originImportAction {
	if origin == nil {
		return originImportInsert
	}
	if origin.CaptureMechanism == JournalOriginMechanismMigration {
		return originImportRefreshMigration
	}
	if journal != nil && originMatchesMigrationFingerprint(*origin, *journal) {
		return originImportReclaim
	}
	return originImportSkip
}

func originMatchesMigrationFingerprint(origin journalOriginScanRow, journal journalEntryScanRow) bool {
	if origin.CaptureMechanism != JournalOriginMechanismUnknown {
		return false
	}
	if origin.EnvelopeVersion != 1 {
		return false
	}
	for _, field := range []sql.NullString{
		origin.ObservedHarness,
		origin.ObservedHarnessVersion,
		origin.AgentID,
		origin.SourceEvent,
		origin.Head,
		origin.ChangePath,
		origin.ChangeSHA256,
		origin.DurableResultKind,
		origin.DurableResultID,
	} {
		if field.Valid {
			return false
		}
	}
	if origin.Dirty.Valid || origin.Reconstructable.Valid {
		return false
	}
	if !nullSafeEqualNullString(origin.HarnessSessionID, journal.HarnessSessionID) {
		return false
	}
	if !nullSafeEqualNullString(origin.Branch, journal.ObservedBranch) {
		return false
	}
	if !nullSafeEqualNullString(origin.Worktree, journal.ObservedWorktree) {
		return false
	}
	if origin.CreatedAt != journal.CreatedAt {
		return false
	}
	return true
}

func nullSafeEqualNullString(a, b sql.NullString) bool {
	if !a.Valid && !b.Valid {
		return true
	}
	if !a.Valid || !b.Valid {
		return false
	}
	return a.String == b.String
}

// classifyIncomingStatus maps a raw source status into the Planning Contract's
// three dispositions. Shaping drafts have no lifecycle vocabulary: absent or
// explicit unknown are no-opinion; any other explicit value is carried raw
// without OOV warnings.
func classifyIncomingStatus(entityKind string, raw string, hasVocabulary bool) (incomingStatusClass, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "unknown" {
		return incomingStatusNoOpinion, ""
	}
	if !hasVocabulary {
		return incomingStatusNormalized, trimmed
	}
	if canonical, ok := CanonicalLifecycleStatus(entityKind, trimmed); ok {
		return incomingStatusNormalized, canonical
	}
	return incomingStatusOutOfVocabulary, trimmed
}

// decideImportStatus implements Decision 2 / the normative status matrix.
// stored nil means insert. insertDefault is the per-kind no-opinion insert value.
func decideImportStatus(entityKind string, entityID string, stored *string, rawIncoming string, insertDefault string, hasVocabulary bool) importStatusDecision {
	class, incoming := classifyIncomingStatus(entityKind, rawIncoming, hasVocabulary)

	if stored == nil {
		switch class {
		case incomingStatusNoOpinion:
			return importStatusDecision{Status: insertDefault, Write: true}
		case incomingStatusNormalized:
			return importStatusDecision{Status: incoming, Write: true}
		case incomingStatusOutOfVocabulary:
			return importStatusDecision{
				Status:  incoming,
				Write:   true,
				Warning: importStatusOOVWarning(entityKind, entityID, incoming),
			}
		default:
			return importStatusDecision{Status: insertDefault, Write: true}
		}
	}

	storedStatus := *stored
	switch class {
	case incomingStatusNoOpinion:
		return importStatusDecision{Status: storedStatus, Write: false}
	case incomingStatusNormalized:
		if storedStatus == "unknown" {
			return importStatusDecision{Status: incoming, Write: true}
		}
		decision := importStatusDecision{Status: storedStatus, Write: false}
		if !importStatusesCanonicallyEqual(entityKind, storedStatus, incoming, hasVocabulary) {
			decision.Divergence = &ImportStatusDivergence{
				EntityKind:     entityKind,
				EntityID:       entityID,
				StoredStatus:   storedStatus,
				IncomingStatus: incoming,
			}
		}
		return decision
	case incomingStatusOutOfVocabulary:
		return importStatusDecision{
			Status:  storedStatus,
			Write:   false,
			Warning: importStatusOOVWarning(entityKind, entityID, incoming),
		}
	default:
		return importStatusDecision{Status: storedStatus, Write: false}
	}
}

func importStatusesCanonicallyEqual(entityKind string, stored string, incomingCanonical string, hasVocabulary bool) bool {
	if !hasVocabulary {
		return stored == incomingCanonical
	}
	storedCanonical, ok := CanonicalLifecycleStatus(entityKind, stored)
	if !ok {
		return stored == incomingCanonical
	}
	return storedCanonical == incomingCanonical
}

func importStatusOOVWarning(entityKind string, entityID string, value string) string {
	return fmt.Sprintf("out-of-vocabulary status %q for %s %s", value, entityKind, entityID)
}

func emptyImportReport() ImportReport {
	return ImportReport{
		SkippedEntries:    []ImportSkippedEntry{},
		StatusDivergences: []ImportStatusDivergence{},
		Warnings:          []string{},
	}
}

package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// IntentConversionRow is one legacy journal deferral and its intended or
// applied canonical conversion.
type IntentConversionRow struct {
	OperationKey   string `json:"operation_key"`
	JournalEntryID string `json:"journal_entry_id"`
	SparkID        string `json:"spark_id"`
	IntentID       string `json:"intent_id,omitempty"`
	IntentAlias    string `json:"intent_alias,omitempty"`
	Title          string `json:"title,omitempty"`
	Action         string `json:"action"`
	Reason         string `json:"reason,omitempty"`
}

// IntentConversionResult is the project-specific conversion manifest.
type IntentConversionResult struct {
	ContractVersion    int                   `json:"contract_version"`
	Command            string                `json:"command"`
	DatabaseScope      string                `json:"database_scope,omitempty"`
	DatabasePath       string                `json:"database_path,omitempty"`
	ProjectID          string                `json:"project_id,omitempty"`
	ProjectName        string                `json:"project_name,omitempty"`
	ProjectCurrentPath string                `json:"project_current_path,omitempty"`
	Action             string                `json:"action"`
	Applied            bool                  `json:"applied"`
	BackupVerified     bool                  `json:"backup_verified"`
	BackupPath         string                `json:"backup_path,omitempty"`
	Convertible        int                   `json:"convertible"`
	AlreadyConverted   int                   `json:"already_converted"`
	Unparseable        int                   `json:"unparseable"`
	Rows               []IntentConversionRow `json:"rows"`
}

const (
	IntentConversionActionDryRun = "dry-run"
	IntentConversionActionApply  = "apply"
)

// legacyDeferralPacket extracts the four-field packet from legacy spark text.
// The historical format is "Intent: ...\nWhy: ...\nBoundary: ...\nTrigger: ..."
// followed by a reciprocal Decision reference line.
func legacyDeferralPacket(text string) (body, why, boundary, trigger string, err error) {
	fields := map[string]*string{"Intent": &body, "Why": &why, "Boundary": &boundary, "Trigger": &trigger}
	order := []string{"Intent", "Why", "Boundary", "Trigger"}
	lines := strings.Split(text, "\n")
	current := ""
	for _, line := range lines {
		matched := false
		for _, name := range order {
			if rest, found := strings.CutPrefix(line, name+": "); found {
				*fields[name] = rest
				current = name
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if strings.HasPrefix(line, "Decision: ") || strings.HasPrefix(line, "Spark: ") {
			current = ""
			continue
		}
		if current != "" {
			*fields[current] += "\n" + line
		}
	}
	for _, name := range order {
		if strings.TrimSpace(*fields[name]) == "" {
			return "", "", "", "", fmt.Errorf("legacy packet is missing the %s field", name)
		}
	}
	return body, why, boundary, trigger, nil
}

// ConvertLegacyDeferrals converts historical journal_deferrals into canonical
// deferred Intents. Dry-run performs no writes; apply is backup-first,
// idempotent, and preserves every legacy row.
func ConvertLegacyDeferrals(ctx context.Context, root project.Root, resolver PathResolver, apply bool) (IntentConversionResult, error) {
	if !apply {
		store, err := openProjectStoreReadExisting(ctx, root, resolver)
		if err != nil {
			return IntentConversionResult{}, err
		}
		defer store.Close()
		return store.convertLegacyDeferrals(ctx, root, false, "")
	}

	// Apply verifies a whole-database backup before any mutation; restore
	// scope is the entire database, so the backup must precede project-scoped
	// conversion.
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return IntentConversionResult{}, err
	}
	source, err := classifySchemaUpgradeSource(ctx, databasePath, root)
	if err != nil {
		return IntentConversionResult{}, err
	}
	backupPath, err := createSchemaUpgradeBackup(ctx, root, databasePath, source.Fingerprint, nil)
	if err != nil {
		return IntentConversionResult{}, fmt.Errorf("create conversion backup: %w", err)
	}
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IntentConversionResult{}, err
	}
	defer store.Close()
	result, err := store.convertLegacyDeferrals(ctx, root, true, backupPath)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) convertLegacyDeferrals(ctx context.Context, root project.Root, apply bool, backupPath string) (IntentConversionResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IntentConversionResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IntentConversionResult{}, err
	}
	result := IntentConversionResult{
		ContractVersion:    StateJSONContractVersion,
		Command:            "state migrate deferrals",
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Action:             IntentConversionActionDryRun,
		Rows:               []IntentConversionRow{},
	}
	if apply {
		result.Action = IntentConversionActionApply
		result.BackupPath = backupPath
		result.BackupVerified = backupPath != ""
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: !apply})
	if err != nil {
		return result, fmt.Errorf("begin deferral conversion: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
SELECT d.operation_key, d.journal_entry_id, d.spark_id, s.text, s.created_at
FROM journal_deferrals AS d
JOIN sparks AS s ON s.project_id = d.project_id AND s.id = d.spark_id
WHERE d.project_id = ?
ORDER BY d.created_at, d.operation_key
`, projectID)
	if err != nil {
		return result, fmt.Errorf("read legacy deferrals: %w", err)
	}
	type legacyRow struct {
		OperationKey string
		JournalID    string
		SparkID      string
		SparkText    string
		CreatedAt    string
	}
	legacy := []legacyRow{}
	for rows.Next() {
		var row legacyRow
		if err := rows.Scan(&row.OperationKey, &row.JournalID, &row.SparkID, &row.SparkText, &row.CreatedAt); err != nil {
			rows.Close()
			return result, fmt.Errorf("scan legacy deferral: %w", err)
		}
		legacy = append(legacy, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, err
	}

	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339Nano)
	for _, row := range legacy {
		manifest := IntentConversionRow{
			OperationKey:   row.OperationKey,
			JournalEntryID: row.JournalID,
			SparkID:        row.SparkID,
		}
		var mappedIntent string
		err := tx.QueryRowContext(ctx, `
SELECT intent_id FROM intent_operations WHERE project_id = ? AND operation_key = ?
`, projectID, row.OperationKey).Scan(&mappedIntent)
		switch {
		case err == nil:
			manifest.Action = "already-converted"
			manifest.IntentID = mappedIntent
			result.AlreadyConverted++
			result.Rows = append(result.Rows, manifest)
			continue
		case !errors.Is(err, sql.ErrNoRows):
			return result, fmt.Errorf("consult operation mapping for %s: %w", row.OperationKey, err)
		}

		body, why, boundary, trigger, parseErr := legacyDeferralPacket(row.SparkText)
		if parseErr != nil {
			manifest.Action = "unparseable"
			manifest.Reason = parseErr.Error()
			result.Unparseable++
			result.Rows = append(result.Rows, manifest)
			continue
		}
		title := journalDeferIntentTitle(body)
		intentID := stableMigrationID("intent", projectID, "legacy-conversion", row.OperationKey)
		manifest.IntentID = intentID
		manifest.Title = title
		manifest.Action = "convert"
		result.Convertible++

		if apply {
			alias, aliasErr := s.nextIntentAlias(ctx, tx, projectID, title, now)
			if aliasErr != nil {
				return result, fmt.Errorf("allocate converted intent alias for %s: %w", row.OperationKey, aliasErr)
			}
			manifest.IntentAlias = alias
			storedDigest := intentDigest(intentDeferralPacket(body, why, boundary, trigger))
			deferralID := stableMigrationID("intent-deferral", projectID, row.OperationKey)
			if _, err := tx.ExecContext(ctx, `INSERT INTO intents (id, project_id, created_at) VALUES (?, ?, ?)`, intentID, projectID, timestamp); err != nil {
				return result, fmt.Errorf("insert converted intent %s: %w", row.OperationKey, err)
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_snapshots (id, project_id, intent_id, seq, title, body, content_digest, created_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
`, stableMigrationID("intent-snapshot", projectID, intentID, "1"), projectID, intentID, title, body, intentDigest(title+"\x00"+body), timestamp); err != nil {
				return result, fmt.Errorf("insert converted snapshot %s: %w", row.OperationKey, err)
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_deferrals (id, project_id, intent_id, operation_key, body, why, boundary, revisit_trigger, stored_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, deferralID, projectID, intentID, row.OperationKey, body, why, boundary, trigger, storedDigest, timestamp); err != nil {
				return result, fmt.Errorf("insert converted deferral %s: %w", row.OperationKey, err)
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, reason, deferral_id, created_at)
VALUES (?, ?, ?, 1, 'deferred', 'converted from legacy journal deferral', ?, ?)
`, stableMigrationID("intent-disposition", projectID, intentID, "1"), projectID, intentID, deferralID, timestamp); err != nil {
				return result, fmt.Errorf("insert converted disposition %s: %w", row.OperationKey, err)
			}
			if err := insertAlias(ctx, tx, projectID, "intent", intentID, "intent", alias, timestamp); err != nil {
				return result, fmt.Errorf("insert converted alias %s: %w", row.OperationKey, err)
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_operations (project_id, operation_key, intent_id, stored_digest, journal_entry_id, spark_id, projection_version, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
`, projectID, row.OperationKey, intentID, storedDigest, row.JournalID, row.SparkID, timestamp, timestamp); err != nil {
				return result, fmt.Errorf("insert converted mapping %s: %w", row.OperationKey, err)
			}
			// Historical provenance: decision and spark become source-of edges;
			// legacy rows are preserved untouched.
			for _, sourceEdge := range []struct{ kind, id string }{{"journal_entry", row.JournalID}, {"spark", row.SparkID}} {
				relationshipID := stableMigrationID("relationship", projectID, sourceEdge.kind, sourceEdge.id, "source-of", "intent", intentID)
				if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, ?, ?, 'intent', ?, 'source-of', 'legacy deferral conversion', ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, relationshipID, projectID, sourceEdge.kind, sourceEdge.id, intentID, relationshipOriginCommand, timestamp, timestamp); err != nil {
					return result, fmt.Errorf("insert converted relationship %s: %w", row.OperationKey, err)
				}
			}
			// One migration marker per converted intent.
			if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'intent', ?, 'converted_from_journal_deferral', NULL, NULL, ?, ?, ?)
`, stableMigrationID("event", projectID, "intent", intentID, "converted", row.OperationKey), projectID, intentID, row.OperationKey, timestamp, timestamp); err != nil {
				return result, fmt.Errorf("insert conversion marker %s: %w", row.OperationKey, err)
			}
		}
		result.Rows = append(result.Rows, manifest)
	}

	if apply {
		result.Applied = true
		if err := tx.Commit(); err != nil {
			return result, fmt.Errorf("commit deferral conversion: %w", err)
		}
		return result, nil
	}
	// Dry-run rolls back the read transaction; nothing was written.
	return result, nil
}

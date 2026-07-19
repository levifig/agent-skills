package state

import (
	"context"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// IntakeItem is one unresolved logical item in the local intake projection.
// The CLI reports facts and exact read commands; it never ranks, promotes, or
// chooses a disposition.
type IntakeItem struct {
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	Alias        string `json:"alias,omitempty"`
	Title        string `json:"title"`
	Status       string `json:"status,omitempty"`
	Disposition  string `json:"disposition,omitempty"`
	OperationKey string `json:"operation_key,omitempty"`
	ReadCommand  string `json:"read_command"`
	CreatedAt    string `json:"created_at"`
}

// IntakeListResult is the deterministic project-local intake projection.
type IntakeListResult struct {
	ContractVersion    int          `json:"contract_version"`
	DatabaseScope      string       `json:"database_scope,omitempty"`
	DatabasePath       string       `json:"database_path,omitempty"`
	ProjectID          string       `json:"project_id,omitempty"`
	ProjectName        string       `json:"project_name,omitempty"`
	ProjectCurrentPath string       `json:"project_current_path,omitempty"`
	Items              []IntakeItem `json:"items"`
}

// ListIntake projects unresolved Sparks, Ideas, Brainstorms, Intents, and
// unmigrated legacy deferrals exactly once each. A spark that is a legacy
// deferral projection surfaces as the deferral (pre-conversion) or is
// deduplicated behind its canonical Intent (post-conversion); it never appears
// twice.
func ListIntake(ctx context.Context, root project.Root, resolver PathResolver) (IntakeListResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IntakeListResult{}, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return IntakeListResult{}, err
	}
	identity, err := store.projectIdentity(ctx, projectID)
	if err != nil {
		return IntakeListResult{}, err
	}

	items := []IntakeItem{}

	// Open sparks that are not legacy deferral projections.
	rows, err := store.db.QueryContext(ctx, `
SELECT s.id,
  COALESCE((SELECT alias FROM aliases WHERE project_id = s.project_id AND entity_kind = 'spark' AND entity_id = s.id ORDER BY namespace, alias LIMIT 1), ''),
  s.text, s.status, s.created_at
FROM sparks AS s
WHERE s.project_id = ? AND s.status = 'open'
  AND NOT EXISTS (SELECT 1 FROM journal_deferrals AS d WHERE d.project_id = s.project_id AND d.spark_id = s.id)
ORDER BY s.created_at, s.id
`, projectID)
	if err != nil {
		return IntakeListResult{}, fmt.Errorf("intake sparks: %w", err)
	}
	if err := scanIntakeRows(rows, &items, "spark", func(item *IntakeItem) {
		item.ReadCommand = "loaf spark show " + firstNonEmpty(item.Alias, item.ID)
	}); err != nil {
		return IntakeListResult{}, err
	}

	// Open ideas.
	rows, err = store.db.QueryContext(ctx, `
SELECT i.id,
  COALESCE((SELECT alias FROM aliases WHERE project_id = i.project_id AND entity_kind = 'idea' AND entity_id = i.id ORDER BY namespace, alias LIMIT 1), ''),
  i.title, i.status, i.created_at
FROM ideas AS i
WHERE i.project_id = ? AND i.status = 'open'
ORDER BY i.created_at, i.id
`, projectID)
	if err != nil {
		return IntakeListResult{}, fmt.Errorf("intake ideas: %w", err)
	}
	if err := scanIntakeRows(rows, &items, "idea", func(item *IntakeItem) {
		item.ReadCommand = "loaf idea show " + firstNonEmpty(item.Alias, item.ID)
	}); err != nil {
		return IntakeListResult{}, err
	}

	// Open brainstorms.
	rows, err = store.db.QueryContext(ctx, `
SELECT b.id,
  COALESCE((SELECT alias FROM aliases WHERE project_id = b.project_id AND entity_kind = 'brainstorm' AND entity_id = b.id ORDER BY namespace, alias LIMIT 1), ''),
  b.title, b.status, b.created_at
FROM brainstorms AS b
WHERE b.project_id = ? AND b.status = 'open'
ORDER BY b.created_at, b.id
`, projectID)
	if err != nil {
		return IntakeListResult{}, fmt.Errorf("intake brainstorms: %w", err)
	}
	if err := scanIntakeRows(rows, &items, "brainstorm", func(item *IntakeItem) {
		item.ReadCommand = "loaf brainstorm show " + firstNonEmpty(item.Alias, item.ID)
	}); err != nil {
		return IntakeListResult{}, err
	}

	// Unresolved intents with derived dispositions.
	rows, err = store.db.QueryContext(ctx, `
SELECT i.id,
  COALESCE((SELECT alias FROM aliases WHERE project_id = i.project_id AND entity_kind = 'intent' AND entity_id = i.id ORDER BY namespace, alias LIMIT 1), ''),
  COALESCE((SELECT title FROM intent_snapshots WHERE intent_id = i.id ORDER BY seq DESC LIMIT 1), ''),
  COALESCE((SELECT disposition FROM intent_dispositions WHERE intent_id = i.id ORDER BY seq DESC LIMIT 1), ''),
  i.created_at
FROM intents AS i
WHERE i.project_id = ?
ORDER BY i.created_at, i.id
`, projectID)
	if err != nil {
		return IntakeListResult{}, fmt.Errorf("intake intents: %w", err)
	}
	intentRows := []IntakeItem{}
	for rows.Next() {
		var item IntakeItem
		if err := rows.Scan(&item.ID, &item.Alias, &item.Title, &item.Disposition, &item.CreatedAt); err != nil {
			rows.Close()
			return IntakeListResult{}, fmt.Errorf("scan intake intent: %w", err)
		}
		item.Kind = "intent"
		item.ReadCommand = "loaf intent show " + firstNonEmpty(item.Alias, item.ID)
		intentRows = append(intentRows, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return IntakeListResult{}, err
	}
	for _, item := range intentRows {
		if item.Disposition == "resolved" {
			continue
		}
		items = append(items, item)
	}

	// Legacy deferrals not yet converged into the canonical model.
	rows, err = store.db.QueryContext(ctx, `
SELECT d.operation_key, d.journal_entry_id, s.text, d.created_at
FROM journal_deferrals AS d
JOIN sparks AS s ON s.project_id = d.project_id AND s.id = d.spark_id
WHERE d.project_id = ? AND s.status = 'open'
  AND NOT EXISTS (SELECT 1 FROM intent_operations AS o WHERE o.project_id = d.project_id AND o.operation_key = d.operation_key)
ORDER BY d.created_at, d.operation_key
`, projectID)
	if err != nil {
		return IntakeListResult{}, fmt.Errorf("intake legacy deferrals: %w", err)
	}
	for rows.Next() {
		var item IntakeItem
		var decisionID, sparkText string
		if err := rows.Scan(&item.OperationKey, &decisionID, &sparkText, &item.CreatedAt); err != nil {
			rows.Close()
			return IntakeListResult{}, fmt.Errorf("scan intake legacy deferral: %w", err)
		}
		item.Kind = "legacy_deferral"
		item.ID = decisionID
		item.Title = legacyDeferralTitle(sparkText)
		item.ReadCommand = "loaf journal show " + decisionID
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return IntakeListResult{}, err
	}

	return IntakeListResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Items:              items,
	}, nil
}

type intakeRowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

func scanIntakeRows(rows intakeRowScanner, items *[]IntakeItem, kind string, finish func(*IntakeItem)) error {
	for rows.Next() {
		var item IntakeItem
		if err := rows.Scan(&item.ID, &item.Alias, &item.Title, &item.Status, &item.CreatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan intake %s: %w", kind, err)
		}
		item.Kind = kind
		item.Status = LifecycleStatusForDisplay(kind, item.Status)
		finish(&item)
		*items = append(*items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return rows.Err()
}

// legacyDeferralTitle extracts the Intent line from a legacy spark packet.
func legacyDeferralTitle(sparkText string) string {
	for _, line := range strings.Split(sparkText, "\n") {
		if rest, found := strings.CutPrefix(line, "Intent: "); found {
			return rest
		}
	}
	first, _, _ := strings.Cut(sparkText, "\n")
	return first
}

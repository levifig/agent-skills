package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/state"
)

func nowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// EnqueueOutboundFact records a locally appended fact for outbound push.
func EnqueueOutboundFact(ctx context.Context, store *state.Store, projectID, factID string) error {
	if store == nil {
		return fmt.Errorf("enqueue outbound fact: store is nil")
	}
	projectID = strings.TrimSpace(projectID)
	factID = strings.TrimSpace(factID)
	if projectID == "" || factID == "" {
		return fmt.Errorf("enqueue outbound fact: project id and fact id are required")
	}
	_, err := store.DB().ExecContext(ctx, `
INSERT INTO sync_outbound_queue (project_id, fact_id, enqueued_at)
VALUES (?, ?, ?)
ON CONFLICT(project_id, fact_id) DO NOTHING
`, projectID, factID, formatTime(nowUTC()))
	if err != nil {
		return fmt.Errorf("enqueue outbound fact %q: %w", factID, err)
	}
	return nil
}

func listPendingOutboundFacts(ctx context.Context, store *state.Store, projectID string) ([]string, error) {
	rows, err := store.DB().QueryContext(ctx, `
SELECT fact_id
FROM sync_outbound_queue
WHERE project_id = ? AND pushed_at IS NULL
ORDER BY enqueued_at ASC, fact_id ASC
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list pending outbound facts: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var factID string
		if err := rows.Scan(&factID); err != nil {
			return nil, fmt.Errorf("scan pending outbound fact: %w", err)
		}
		ids = append(ids, factID)
	}
	return ids, rows.Err()
}

func markOutboundFactsPushed(ctx context.Context, store *state.Store, projectID string, factIDs []string) error {
	if len(factIDs) == 0 {
		return nil
	}
	now := formatTime(nowUTC())
	for _, factID := range factIDs {
		if _, err := store.DB().ExecContext(ctx, `
UPDATE sync_outbound_queue
SET pushed_at = ?
WHERE project_id = ? AND fact_id = ?
`, now, projectID, factID); err != nil {
			return fmt.Errorf("mark outbound fact pushed %q: %w", factID, err)
		}
	}
	return nil
}

func readArrivalCursor(ctx context.Context, store *state.Store, projectID string) (int64, error) {
	var cursor int64
	err := store.DB().QueryRowContext(ctx, `
SELECT arrival_cursor
FROM sync_project_cursors
WHERE project_id = ?
`, projectID).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read arrival cursor: %w", err)
	}
	return cursor, nil
}

func writeArrivalCursor(ctx context.Context, store *state.Store, projectID string, cursor int64) error {
	now := formatTime(nowUTC())
	_, err := store.DB().ExecContext(ctx, `
INSERT INTO sync_project_cursors (project_id, arrival_cursor, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET
  arrival_cursor = excluded.arrival_cursor,
  updated_at = excluded.updated_at
`, projectID, cursor, now)
	if err != nil {
		return fmt.Errorf("write arrival cursor: %w", err)
	}
	return nil
}

func countPendingOutbound(ctx context.Context, store *state.Store, projectID string) (int, error) {
	var count int
	err := store.DB().QueryRowContext(ctx, `
SELECT COUNT(*)
FROM sync_outbound_queue
WHERE project_id = ? AND pushed_at IS NULL
`, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending outbound facts: %w", err)
	}
	return count, nil
}

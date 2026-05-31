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

// BrainstormArchiveOptions describes a SQLite-backed brainstorm archive request.
type BrainstormArchiveOptions struct {
	Refs   []string
	Reason string
}

// BrainstormArchiveResult describes a state-backed brainstorm archive mutation.
type BrainstormArchiveResult struct {
	Archived []BrainstormArchiveItem `json:"archived"`
	Skipped  []BrainstormArchiveItem `json:"skipped"`
}

// BrainstormArchiveItem describes one requested brainstorm archive outcome.
type BrainstormArchiveItem struct {
	Brainstorm *TraceEntity `json:"brainstorm,omitempty"`
	Ref        string       `json:"ref,omitempty"`
	Previous   string       `json:"previous_status,omitempty"`
	Status     string       `json:"status,omitempty"`
	Reason     string       `json:"reason,omitempty"`
	EventID    string       `json:"event_id,omitempty"`
	Note       string       `json:"note,omitempty"`
}

// ArchiveBrainstorms archives brainstorms in initialized SQLite state.
func ArchiveBrainstorms(ctx context.Context, root project.Root, resolver PathResolver, options BrainstormArchiveOptions) (BrainstormArchiveResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BrainstormArchiveResult{}, err
	}
	defer store.Close()
	return store.ArchiveBrainstorms(ctx, root, options)
}

// ArchiveBrainstorms archives brainstorms in an open store.
func (s *Store) ArchiveBrainstorms(ctx context.Context, root project.Root, options BrainstormArchiveOptions) (BrainstormArchiveResult, error) {
	if len(options.Refs) == 0 {
		return BrainstormArchiveResult{}, fmt.Errorf("brainstorm archive requires at least one brainstorm")
	}
	projectID := ProjectID(root)
	result := BrainstormArchiveResult{
		Archived: []BrainstormArchiveItem{},
		Skipped:  []BrainstormArchiveItem{},
	}
	for _, ref := range options.Refs {
		item, archived, err := s.archiveBrainstorm(ctx, projectID, ref, options.Reason)
		if err != nil {
			return BrainstormArchiveResult{}, err
		}
		if archived {
			result.Archived = append(result.Archived, item)
		} else {
			result.Skipped = append(result.Skipped, item)
		}
	}
	return result, nil
}

func (s *Store) archiveBrainstorm(ctx context.Context, projectID string, ref string, reason string) (BrainstormArchiveItem, bool, error) {
	brainstorm, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return BrainstormArchiveItem{Ref: ref, Reason: err.Error()}, false, nil
	}
	if brainstorm.Kind != "brainstorm" {
		return BrainstormArchiveItem{Brainstorm: &brainstorm, Ref: ref, Reason: fmt.Sprintf("%q resolves to %s, not brainstorm", ref, brainstorm.Kind)}, false, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BrainstormArchiveItem{}, false, fmt.Errorf("begin brainstorm archive transaction: %w", err)
	}
	defer tx.Rollback()

	var previousStatus string
	err = tx.QueryRowContext(ctx, `SELECT status FROM brainstorms WHERE project_id = ? AND id = ?`, projectID, brainstorm.ID).Scan(&previousStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return BrainstormArchiveItem{Brainstorm: &brainstorm, Ref: ref, Reason: fmt.Sprintf("brainstorm %q not found in SQLite state", ref)}, false, nil
	}
	if err != nil {
		return BrainstormArchiveItem{}, false, fmt.Errorf("read brainstorm status: %w", err)
	}

	if previousStatus == "archived" {
		return BrainstormArchiveItem{Brainstorm: &brainstorm, Ref: ref, Previous: previousStatus, Status: previousStatus, Reason: "already archived"}, false, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE brainstorms SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, "archived", now, projectID, brainstorm.ID); err != nil {
		return BrainstormArchiveItem{}, false, fmt.Errorf("update brainstorm status: %w", err)
	}

	note := firstNonEmpty(strings.TrimSpace(reason), "recorded by brainstorm archive")
	eventID := stableMigrationID("event", projectID, "brainstorm", brainstorm.ID, "status", previousStatus, "archived")
	_, err = tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, "brainstorm", brainstorm.ID, "status_changed", previousStatus, "archived", note, now, now)
	if err != nil {
		return BrainstormArchiveItem{}, false, fmt.Errorf("record brainstorm archive event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return BrainstormArchiveItem{}, false, fmt.Errorf("commit brainstorm archive transaction: %w", err)
	}

	brainstorm.Status = "archived"
	return BrainstormArchiveItem{Brainstorm: &brainstorm, Ref: ref, Previous: previousStatus, Status: "archived", EventID: eventID, Note: note}, true, nil
}

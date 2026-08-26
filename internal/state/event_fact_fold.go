package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type foldedSparkRow struct {
	ProjectID string
	Payload   CoreEventPayload
}

type foldedIdeaRow struct {
	ProjectID string
	Payload   CoreEventPayload
}

type foldedHandoffRow struct {
	ProjectID string
	Payload   CoreEventPayload
}

type foldedReleaseRow struct {
	ProjectID string
	Payload   CoreEventPayload
}

type foldedRefRow struct {
	ProjectID string
	Payload   CoreEventPayload
}

type foldedWorktreeRow struct {
	ProjectID string
	Payload   CoreEventPayload
}

type foldedVerificationRow struct {
	ProjectID string
	Payload   CoreEventPayload
}

func listCoreEventFacts(ctx context.Context, queryer queryContext, projectID string, kinds []string) ([]FactEnvelope, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(kinds))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(`
SELECT id, project_id, kind, payload, env_id, seq, hlc, envelope_v
FROM facts
WHERE kind IN (%s)
`, placeholders)
	args := make([]any, 0, len(kinds)+1)
	for _, kind := range kinds {
		args = append(args, kind)
	}
	if strings.TrimSpace(projectID) != "" {
		query += ` AND project_id = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY project_id, hlc, env_id, id`
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list core event facts: %w", err)
	}
	defer rows.Close()
	var facts []FactEnvelope
	for rows.Next() {
		var envelope FactEnvelope
		if err := rows.Scan(&envelope.ID, &envelope.ProjectID, &envelope.Kind, &envelope.Payload, &envelope.EnvID, &envelope.Seq, &envelope.HLC, &envelope.EnvelopeV); err != nil {
			return nil, fmt.Errorf("scan core event fact: %w", err)
		}
		facts = append(facts, envelope)
	}
	return facts, rows.Err()
}

func coreEventSubjectID(factID string, payload CoreEventPayload) string {
	if id := strings.TrimSpace(payload.SubjectID); id != "" {
		return id
	}
	return factID
}

func applySparkEvent(existing CoreEventPayload, kind string, incoming CoreEventPayload) CoreEventPayload {
	next := existing
	if next.SubjectKind == "" {
		next.SubjectKind = "spark"
	}
	if incoming.SubjectID != "" {
		next.SubjectID = incoming.SubjectID
	}
	if incoming.Alias != "" {
		next.Alias = incoming.Alias
	}
	if incoming.Text != "" {
		next.Text = incoming.Text
	}
	if incoming.Scope != "" || kind == FactKindSparkCaptured {
		next.Scope = incoming.Scope
	}
	if incoming.Status != "" {
		next.Status = incoming.Status
	} else if incoming.ToStatus != "" {
		next.Status = incoming.ToStatus
	} else if kind == FactKindSparkArchived && next.Status == "" {
		next.Status = LifecycleStatusDone
	} else if kind == FactKindSparkCaptured && next.Status == "" {
		next.Status = "open"
	}
	if incoming.RelatedKind != "" {
		next.RelatedKind = incoming.RelatedKind
	}
	if incoming.RelatedID != "" {
		next.RelatedID = incoming.RelatedID
	}
	if incoming.Note != "" {
		next.Note = incoming.Note
	}
	if incoming.CreatedAt != "" && next.CreatedAt == "" {
		next.CreatedAt = incoming.CreatedAt
	}
	if incoming.UpdatedAt != "" {
		next.UpdatedAt = incoming.UpdatedAt
	}
	return next
}

func applyIdeaEvent(existing CoreEventPayload, kind string, incoming CoreEventPayload) CoreEventPayload {
	next := existing
	if next.SubjectKind == "" {
		next.SubjectKind = "idea"
	}
	if incoming.SubjectID != "" {
		next.SubjectID = incoming.SubjectID
	}
	if incoming.Alias != "" {
		next.Alias = incoming.Alias
	}
	if incoming.Title != "" {
		next.Title = incoming.Title
	}
	if incoming.Body != "" {
		next.Body = incoming.Body
	}
	if incoming.Status != "" {
		next.Status = incoming.Status
	} else if incoming.ToStatus != "" {
		next.Status = incoming.ToStatus
	} else if kind == FactKindIdeaArchived && next.Status == "" {
		next.Status = LifecycleStatusArchived
	} else if kind == FactKindIdeaResolved && next.Status == "" {
		next.Status = LifecycleStatusDone
	} else if kind == FactKindIdeaCreated && next.Status == "" {
		next.Status = "open"
	}
	if incoming.RelatedKind != "" {
		next.RelatedKind = incoming.RelatedKind
	}
	if incoming.RelatedID != "" {
		next.RelatedID = incoming.RelatedID
	}
	if incoming.Note != "" {
		next.Note = incoming.Note
	}
	if incoming.CreatedAt != "" && next.CreatedAt == "" {
		next.CreatedAt = incoming.CreatedAt
	}
	if incoming.UpdatedAt != "" {
		next.UpdatedAt = incoming.UpdatedAt
	}
	return next
}

func foldSparkFacts(ctx context.Context, queryer queryContext, projectID string) (map[string]foldedSparkRow, error) {
	facts, err := listCoreEventFacts(ctx, queryer, projectID, sparkFactKinds())
	if err != nil {
		return nil, err
	}
	folded := map[string]foldedSparkRow{}
	for _, fact := range facts {
		payload, err := decodeCoreEventPayload(fact.Payload)
		if err != nil {
			return nil, err
		}
		subjectID := coreEventSubjectID(fact.ID, payload)
		existing := folded[subjectID].Payload
		folded[subjectID] = foldedSparkRow{ProjectID: fact.ProjectID, Payload: applySparkEvent(existing, fact.Kind, payload)}
	}
	return folded, nil
}

func foldIdeaFacts(ctx context.Context, queryer queryContext, projectID string) (map[string]foldedIdeaRow, error) {
	facts, err := listCoreEventFacts(ctx, queryer, projectID, ideaFactKinds())
	if err != nil {
		return nil, err
	}
	folded := map[string]foldedIdeaRow{}
	for _, fact := range facts {
		payload, err := decodeCoreEventPayload(fact.Payload)
		if err != nil {
			return nil, err
		}
		subjectID := coreEventSubjectID(fact.ID, payload)
		existing := folded[subjectID].Payload
		folded[subjectID] = foldedIdeaRow{ProjectID: fact.ProjectID, Payload: applyIdeaEvent(existing, fact.Kind, payload)}
	}
	return folded, nil
}

func foldLatestBySubject(ctx context.Context, queryer queryContext, projectID string, kinds []string) (map[string]foldedHandoffRow, error) {
	facts, err := listCoreEventFacts(ctx, queryer, projectID, kinds)
	if err != nil {
		return nil, err
	}
	folded := map[string]foldedHandoffRow{}
	for _, fact := range facts {
		payload, err := decodeCoreEventPayload(fact.Payload)
		if err != nil {
			return nil, err
		}
		subjectID := coreEventSubjectID(fact.ID, payload)
		if payload.SubjectID == "" {
			payload.SubjectID = subjectID
		}
		folded[subjectID] = foldedHandoffRow{ProjectID: fact.ProjectID, Payload: payload}
	}
	return folded, nil
}

func foldHandoffFacts(ctx context.Context, queryer queryContext, projectID string) (map[string]foldedHandoffRow, error) {
	return foldLatestBySubject(ctx, queryer, projectID, []string{FactKindHandoffRecorded})
}

func foldReleaseFacts(ctx context.Context, queryer queryContext, projectID string) (map[string]foldedReleaseRow, error) {
	generic, err := foldLatestBySubject(ctx, queryer, projectID, []string{FactKindReleaseRecorded})
	if err != nil {
		return nil, err
	}
	out := make(map[string]foldedReleaseRow, len(generic))
	for id, row := range generic {
		out[id] = foldedReleaseRow{ProjectID: row.ProjectID, Payload: row.Payload}
	}
	return out, nil
}

func foldRefFacts(ctx context.Context, queryer queryContext, projectID string) (map[string]foldedRefRow, error) {
	generic, err := foldLatestBySubject(ctx, queryer, projectID, []string{FactKindRefRegistered})
	if err != nil {
		return nil, err
	}
	out := make(map[string]foldedRefRow, len(generic))
	for id, row := range generic {
		out[id] = foldedRefRow{ProjectID: row.ProjectID, Payload: row.Payload}
	}
	return out, nil
}

func foldWorktreeFacts(ctx context.Context, queryer queryContext, projectID string) (map[string]foldedWorktreeRow, error) {
	generic, err := foldLatestBySubject(ctx, queryer, projectID, worktreeFactKinds())
	if err != nil {
		return nil, err
	}
	out := make(map[string]foldedWorktreeRow, len(generic))
	for id, row := range generic {
		out[id] = foldedWorktreeRow{ProjectID: row.ProjectID, Payload: row.Payload}
	}
	return out, nil
}

func foldVerificationFacts(ctx context.Context, queryer queryContext, projectID string) (map[string]foldedVerificationRow, error) {
	generic, err := foldLatestBySubject(ctx, queryer, projectID, []string{FactKindVerificationRecorded})
	if err != nil {
		return nil, err
	}
	out := make(map[string]foldedVerificationRow, len(generic))
	for id, row := range generic {
		out[id] = foldedVerificationRow{ProjectID: row.ProjectID, Payload: row.Payload}
	}
	return out, nil
}

func upsertSparkProjectionTx(ctx context.Context, execer journalWriteExecer, projectID, sparkID string, payload CoreEventPayload) error {
	status := strings.TrimSpace(payload.Status)
	if status == "" {
		status = "open"
	}
	createdAt := firstNonEmpty(payload.CreatedAt, payload.UpdatedAt)
	updatedAt := firstNonEmpty(payload.UpdatedAt, createdAt)
	_, err := execer.ExecContext(ctx, `
INSERT INTO sparks (id, project_id, scope, status, text, source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, NULL, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  scope = excluded.scope,
  status = excluded.status,
  text = excluded.text,
  updated_at = excluded.updated_at
`, sparkID, projectID, emptyToNil(payload.Scope), status, payload.Text, createdAt, updatedAt)
	if err != nil {
		return fmt.Errorf("upsert spark projection %q: %w", sparkID, err)
	}
	return nil
}

func upsertIdeaProjectionTx(ctx context.Context, execer journalWriteExecer, projectID, ideaID string, payload CoreEventPayload) error {
	status := strings.TrimSpace(payload.Status)
	if status == "" {
		status = "open"
	}
	createdAt := firstNonEmpty(payload.CreatedAt, payload.UpdatedAt)
	updatedAt := firstNonEmpty(payload.UpdatedAt, createdAt)
	_, err := execer.ExecContext(ctx, `
INSERT INTO ideas (id, project_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, NULL, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  title = excluded.title,
  status = excluded.status,
  updated_at = excluded.updated_at
`, ideaID, projectID, payload.Title, status, createdAt, updatedAt)
	if err != nil {
		return fmt.Errorf("upsert idea projection %q: %w", ideaID, err)
	}
	return nil
}

func upsertHandoffProjectionTx(ctx context.Context, tx *sql.Tx, projectID, handoffID string, payload CoreEventPayload) error {
	status := strings.TrimSpace(payload.Status)
	if status == "" {
		status = LifecycleStatusDraft
	}
	createdAt := firstNonEmpty(payload.CreatedAt, payload.UpdatedAt)
	updatedAt := firstNonEmpty(payload.UpdatedAt, createdAt)
	correlationColumn, err := handoffCorrelationColumn(ctx, tx)
	if err != nil {
		return err
	}
	insertHandoff := fmt.Sprintf(`
INSERT INTO handoffs (id, project_id, %s, task_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  %s = excluded.%s,
  task_id = excluded.task_id,
  title = excluded.title,
  status = excluded.status,
  updated_at = excluded.updated_at
`, correlationColumn, correlationColumn, correlationColumn)
	if _, err := tx.ExecContext(ctx, insertHandoff, handoffID, projectID, emptyToNil(payload.HarnessSessionID), emptyToNil(payload.TaskID), payload.Title, status, createdAt, updatedAt); err != nil {
		return fmt.Errorf("upsert handoff projection %q: %w", handoffID, err)
	}
	if strings.TrimSpace(payload.Body) == "" {
		return nil
	}
	_, err = upsertArtifactBodyTx(ctx, tx, projectID, "handoff", handoffID, ArtifactBodyKindMarkdown, payload.Body, nil, updatedAt)
	return err
}

func upsertReleaseProjectionTx(ctx context.Context, tx *sql.Tx, projectID, releaseID string, payload CoreEventPayload) error {
	createdAt := firstNonEmpty(payload.CreatedAt, payload.UpdatedAt)
	updatedAt := firstNonEmpty(payload.UpdatedAt, createdAt)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO releases (id, project_id, version, tag, tagged_commit, notes, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  version = excluded.version,
  tag = excluded.tag,
  tagged_commit = excluded.tagged_commit,
  notes = excluded.notes,
  updated_at = excluded.updated_at
`, releaseID, projectID, payload.Version, payload.Tag, payload.TaggedCommit, payload.Notes, createdAt, updatedAt); err != nil {
		return fmt.Errorf("upsert release projection %q: %w", releaseID, err)
	}
	return nil
}

func insertFoldedReleaseMembersTx(ctx context.Context, tx *sql.Tx, projectID, releaseID string, payload CoreEventPayload) error {
	updatedAt := firstNonEmpty(payload.UpdatedAt, payload.CreatedAt)
	for _, member := range payload.Members {
		if err := insertReleaseMemberTx(ctx, tx, projectID, releaseID, member.Kind, member.MemberID, updatedAt); err != nil {
			return err
		}
	}
	return nil
}

func rebuildSparkProjectionFromFactsTx(ctx context.Context, execer journalWriteExecer, projectID string) (int, error) {
	folded, err := foldSparkFacts(ctx, execer, projectID)
	if err != nil {
		return 0, err
	}
	if _, err := execer.ExecContext(ctx, `DELETE FROM sparks WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("clear spark projection: %w", err)
	}
	for sparkID, row := range folded {
		if err := upsertSparkProjectionTx(ctx, execer, projectID, sparkID, row.Payload); err != nil {
			return 0, err
		}
	}
	return len(folded), nil
}

func rebuildIdeaProjectionFromFactsTx(ctx context.Context, execer journalWriteExecer, projectID string) (int, error) {
	folded, err := foldIdeaFacts(ctx, execer, projectID)
	if err != nil {
		return 0, err
	}
	if _, err := execer.ExecContext(ctx, `DELETE FROM ideas WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("clear idea projection: %w", err)
	}
	for ideaID, row := range folded {
		if err := upsertIdeaProjectionTx(ctx, execer, projectID, ideaID, row.Payload); err != nil {
			return 0, err
		}
	}
	return len(folded), nil
}

func rebuildHandoffProjectionFromFactsTx(ctx context.Context, tx *sql.Tx, projectID string) (int, error) {
	folded, err := foldHandoffFacts(ctx, tx, projectID)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM handoffs WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("clear handoff projection: %w", err)
	}
	for handoffID, row := range folded {
		if err := upsertHandoffProjectionTx(ctx, tx, projectID, handoffID, row.Payload); err != nil {
			return 0, err
		}
	}
	return len(folded), nil
}

func rebuildReleaseProjectionFromFactsTx(ctx context.Context, tx *sql.Tx, projectID string) (int, error) {
	folded, err := foldReleaseFacts(ctx, tx, projectID)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM release_members WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("clear release members: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM releases WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("clear release projection: %w", err)
	}
	for releaseID, row := range folded {
		if err := upsertReleaseProjectionTx(ctx, tx, projectID, releaseID, row.Payload); err != nil {
			return 0, err
		}
	}
	for releaseID, row := range folded {
		if err := insertFoldedReleaseMembersTx(ctx, tx, projectID, releaseID, row.Payload); err != nil {
			return 0, err
		}
	}
	return len(folded), nil
}

// RebuildMutableCoreProjectionsForProject rebuilds spark/idea/handoff/release
// projections from grow-only facts for one project.
func RebuildMutableCoreProjectionsForProject(ctx context.Context, store *Store, projectID string) (int, error) {
	if store == nil || store.db == nil {
		return 0, fmt.Errorf("rebuild mutable core projections: store is nil")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin mutable core projection rebuild: %w", err)
	}
	defer tx.Rollback()
	count := 0
	n, err := rebuildSparkProjectionFromFactsTx(ctx, tx, projectID)
	if err != nil {
		return 0, err
	}
	count += n
	n, err = rebuildIdeaProjectionFromFactsTx(ctx, tx, projectID)
	if err != nil {
		return 0, err
	}
	count += n
	n, err = rebuildHandoffProjectionFromFactsTx(ctx, tx, projectID)
	if err != nil {
		return 0, err
	}
	count += n
	n, err = rebuildReleaseProjectionFromFactsTx(ctx, tx, projectID)
	if err != nil {
		return 0, err
	}
	count += n
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit mutable core projection rebuild: %w", err)
	}
	return count, nil
}

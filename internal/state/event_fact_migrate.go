package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

type coreEventSource struct {
	ID         string
	ProjectID  string
	EntityKind string
	EntityID   string
	EventType  string
	FromStatus string
	ToStatus   string
	Note       string
	CreatedAt  string
}

func migrateMutableCoreFactsTx(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return fmt.Errorf("migrate mutable core facts: transaction is nil")
	}
	if !tableExists(ctx, tx, "facts") || !tableExists(ctx, tx, "events") {
		return nil
	}
	if err := replayMutableCoreEventsTx(ctx, tx); err != nil {
		return err
	}
	if err := birthMutableCoreRowsTx(ctx, tx); err != nil {
		return err
	}
	return backfillRootCommitFingerprintsTx(ctx, tx)
}

func replayMutableCoreEventsTx(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
SELECT id, project_id, entity_kind, entity_id, event_type,
       COALESCE(from_status, ''), COALESCE(to_status, ''), COALESCE(note, ''), created_at
FROM events
WHERE entity_kind IN ('spark', 'idea', 'handoff', 'release')
ORDER BY created_at ASC, id ASC
`)
	if err != nil {
		return fmt.Errorf("list mutable-core events for replay: %w", err)
	}
	defer rows.Close()
	var sources []coreEventSource
	for rows.Next() {
		var source coreEventSource
		if err := rows.Scan(&source.ID, &source.ProjectID, &source.EntityKind, &source.EntityID, &source.EventType, &source.FromStatus, &source.ToStatus, &source.Note, &source.CreatedAt); err != nil {
			return fmt.Errorf("scan mutable-core event: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, source := range sources {
		kind := factKindForReplayedEvent(source)
		if kind == "" {
			continue
		}
		var existing int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM facts WHERE id = ?`, source.ID).Scan(&existing); err != nil {
			return fmt.Errorf("inspect replayed event fact %q: %w", source.ID, err)
		}
		if existing > 0 {
			continue
		}
		payload := CoreEventPayload{
			SubjectKind: source.EntityKind,
			SubjectID:   source.EntityID,
			FromStatus:  source.FromStatus,
			ToStatus:    source.ToStatus,
			Status:      source.ToStatus,
			Note:        source.Note,
			CreatedAt:   source.CreatedAt,
			UpdatedAt:   source.CreatedAt,
		}
		if _, err := appendCoreEventFactTx(ctx, tx, source.ProjectID, kind, source.ID, payload, parseCoreEventTime(source.CreatedAt), legacyFactEnvID); err != nil {
			return fmt.Errorf("replay event %q as %s: %w", source.ID, kind, err)
		}
	}
	return nil
}

func factKindForReplayedEvent(source coreEventSource) string {
	switch source.EntityKind {
	case "spark":
		switch strings.TrimSpace(source.ToStatus) {
		case LifecycleStatusDone, LifecycleStatusArchived:
			return FactKindSparkArchived
		default:
			return FactKindSparkCaptured
		}
	case "idea":
		switch strings.TrimSpace(source.ToStatus) {
		case LifecycleStatusDone:
			return FactKindIdeaResolved
		case LifecycleStatusArchived:
			return FactKindIdeaArchived
		default:
			return FactKindIdeaCreated
		}
	case "handoff":
		return FactKindHandoffRecorded
	case "release":
		return FactKindReleaseRecorded
	default:
		return ""
	}
}

func birthMutableCoreRowsTx(ctx context.Context, tx *sql.Tx) error {
	if err := birthSparksTx(ctx, tx); err != nil {
		return err
	}
	if err := birthIdeasTx(ctx, tx); err != nil {
		return err
	}
	if err := birthHandoffsTx(ctx, tx); err != nil {
		return err
	}
	if err := birthReleasesTx(ctx, tx); err != nil {
		return err
	}
	if err := birthBackendRefsTx(ctx, tx); err != nil {
		return err
	}
	if err := birthWorkContractRefsTx(ctx, tx); err != nil {
		return err
	}
	if err := birthWorktreesTx(ctx, tx); err != nil {
		return err
	}
	return birthVerificationReceiptsTx(ctx, tx)
}

func logFactReplayDiscrepancyTx(ctx context.Context, tx *sql.Tx, projectID, entityKind, entityID, reason string) error {
	if !tableExists(ctx, tx, "fact_replay_discrepancies") {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := stableMigrationID("fact-replay", projectID, entityKind, entityID, reason, now)
	_, err := tx.ExecContext(ctx, `
INSERT INTO fact_replay_discrepancies (id, project_id, entity_kind, entity_id, reason, logged_at)
VALUES (?, ?, ?, ?, ?, ?)
`, id, projectID, entityKind, entityID, reason, now)
	if err != nil {
		return fmt.Errorf("log fact replay discrepancy for %s %s: %w", entityKind, entityID, err)
	}
	return nil
}

func sparkProjectionMatches(scope, status, text, createdAt, updatedAt string, payload CoreEventPayload) bool {
	wantStatus := firstNonEmpty(payload.Status, "open")
	return payload.Scope == scope &&
		wantStatus == status &&
		payload.Text == text &&
		payload.CreatedAt == createdAt &&
		payload.UpdatedAt == updatedAt
}

func ideaProjectionMatches(title, status, createdAt, updatedAt string, payload CoreEventPayload) bool {
	wantStatus := firstNonEmpty(payload.Status, "open")
	return payload.Title == title &&
		wantStatus == status &&
		payload.CreatedAt == createdAt &&
		payload.UpdatedAt == updatedAt
}

func handoffProjectionMatches(sessionID, taskID, title, status, createdAt, updatedAt string, payload CoreEventPayload) bool {
	wantStatus := firstNonEmpty(payload.Status, LifecycleStatusDraft)
	return payload.HarnessSessionID == sessionID &&
		payload.TaskID == taskID &&
		payload.Title == title &&
		wantStatus == status &&
		payload.CreatedAt == createdAt &&
		payload.UpdatedAt == updatedAt
}

func birthSparksTx(ctx context.Context, tx *sql.Tx) error {
	folded, err := foldSparkFacts(ctx, tx, "")
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, project_id, COALESCE(scope, ''), status, text, created_at, updated_at
FROM sparks
ORDER BY project_id, created_at, id
`)
	if err != nil {
		return fmt.Errorf("list sparks for birth facts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, scope, status, text, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &scope, &status, &text, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan spark for birth fact: %w", err)
		}
		foldedRow, ok := folded[id]
		if ok && foldedRow.ProjectID == projectID && sparkProjectionMatches(scope, status, text, createdAt, updatedAt, foldedRow.Payload) {
			continue
		}
		reason := "missing spark fact"
		if ok {
			reason = "spark fold diverged from projection"
		}
		if err := logFactReplayDiscrepancyTx(ctx, tx, projectID, "spark", id, reason); err != nil {
			return err
		}
		payload := CoreEventPayload{
			SubjectKind: "spark",
			SubjectID:   id,
			Status:      status,
			Text:        text,
			Scope:       scope,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		kind := FactKindSparkCaptured
		if status == LifecycleStatusDone || status == LifecycleStatusArchived {
			kind = FactKindSparkArchived
		}
		if _, err := appendCoreEventFactTx(ctx, tx, projectID, kind, "", payload, time.Now().UTC(), legacyFactEnvID); err != nil {
			return fmt.Errorf("birth spark fact %q: %w", id, err)
		}
	}
	return rows.Err()
}

func birthIdeasTx(ctx context.Context, tx *sql.Tx) error {
	folded, err := foldIdeaFacts(ctx, tx, "")
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, project_id, title, status, created_at, updated_at
FROM ideas
ORDER BY project_id, created_at, id
`)
	if err != nil {
		return fmt.Errorf("list ideas for birth facts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, title, status, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &title, &status, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan idea for birth fact: %w", err)
		}
		foldedRow, ok := folded[id]
		if ok && foldedRow.ProjectID == projectID && ideaProjectionMatches(title, status, createdAt, updatedAt, foldedRow.Payload) {
			continue
		}
		reason := "missing idea fact"
		if ok {
			reason = "idea fold diverged from projection"
		}
		if err := logFactReplayDiscrepancyTx(ctx, tx, projectID, "idea", id, reason); err != nil {
			return err
		}
		payload := CoreEventPayload{
			SubjectKind: "idea",
			SubjectID:   id,
			Title:       title,
			Status:      status,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		kind := FactKindIdeaCreated
		switch status {
		case LifecycleStatusDone:
			kind = FactKindIdeaResolved
		case LifecycleStatusArchived:
			kind = FactKindIdeaArchived
		}
		if _, err := appendCoreEventFactTx(ctx, tx, projectID, kind, "", payload, time.Now().UTC(), legacyFactEnvID); err != nil {
			return fmt.Errorf("birth idea fact %q: %w", id, err)
		}
	}
	return rows.Err()
}

func birthHandoffsTx(ctx context.Context, tx *sql.Tx) error {
	folded, err := foldHandoffFacts(ctx, tx, "")
	if err != nil {
		return err
	}
	correlationColumn, err := handoffCorrelationColumn(ctx, tx)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`
SELECT id, project_id, COALESCE(%s, ''), COALESCE(task_id, ''), title, status, created_at, updated_at
FROM handoffs
ORDER BY project_id, created_at, id
`, correlationColumn))
	if err != nil {
		return fmt.Errorf("list handoffs for birth facts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, sessionID, taskID, title, status, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &sessionID, &taskID, &title, &status, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan handoff for birth fact: %w", err)
		}
		body := loadArtifactBodyContent(ctx, tx, projectID, "handoff", id)
		foldedRow, ok := folded[id]
		if ok && foldedRow.ProjectID == projectID && handoffProjectionMatches(sessionID, taskID, title, status, createdAt, updatedAt, foldedRow.Payload) && foldedRow.Payload.Body == body {
			continue
		}
		reason := "missing handoff fact"
		if ok {
			reason = "handoff fold diverged from projection"
		}
		if err := logFactReplayDiscrepancyTx(ctx, tx, projectID, "handoff", id, reason); err != nil {
			return err
		}
		payload := CoreEventPayload{
			SubjectKind:      "handoff",
			SubjectID:        id,
			Title:            title,
			Status:           status,
			Body:             body,
			HarnessSessionID: sessionID,
			TaskID:           taskID,
			CreatedAt:        createdAt,
			UpdatedAt:        updatedAt,
		}
		if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindHandoffRecorded, "", payload, time.Now().UTC(), legacyFactEnvID); err != nil {
			return fmt.Errorf("birth handoff fact %q: %w", id, err)
		}
	}
	return rows.Err()
}

func loadArtifactBodyContent(ctx context.Context, queryer queryContext, projectID, kind, entityID string) string {
	var content string
	err := queryer.QueryRowContext(ctx, `
SELECT content FROM artifact_bodies
WHERE project_id = ? AND entity_kind = ? AND entity_id = ?
ORDER BY updated_at DESC, id DESC
LIMIT 1
`, projectID, kind, entityID).Scan(&content)
	if err != nil {
		return ""
	}
	return content
}

func birthReleasesTx(ctx context.Context, tx *sql.Tx) error {
	folded, err := foldReleaseFacts(ctx, tx, "")
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, project_id, version, tag, tagged_commit, COALESCE(notes, ''), created_at, updated_at
FROM releases
ORDER BY project_id, created_at, id
`)
	if err != nil {
		return fmt.Errorf("list releases for birth facts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, version, tag, commit, notes, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &version, &tag, &commit, &notes, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan release for birth fact: %w", err)
		}
		members, err := loadReleaseMemberFactsTx(ctx, tx, projectID, id)
		if err != nil {
			return err
		}
		foldedRow, ok := folded[id]
		if ok && foldedRow.ProjectID == projectID && foldedRow.Payload.Version == version && foldedRow.Payload.Tag == tag && foldedRow.Payload.TaggedCommit == commit && foldedRow.Payload.Notes == notes {
			continue
		}
		reason := "missing release fact"
		if ok {
			reason = "release fold diverged from projection"
		}
		if err := logFactReplayDiscrepancyTx(ctx, tx, projectID, "release", id, reason); err != nil {
			return err
		}
		payload := CoreEventPayload{
			SubjectKind:  "release",
			SubjectID:    id,
			Version:      version,
			Tag:          tag,
			TaggedCommit: commit,
			Notes:        notes,
			Members:      members,
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}
		if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindReleaseRecorded, "", payload, time.Now().UTC(), legacyFactEnvID); err != nil {
			return fmt.Errorf("birth release fact %q: %w", id, err)
		}
	}
	return rows.Err()
}

func loadReleaseMemberFactsTx(ctx context.Context, queryer queryContext, projectID, releaseID string) ([]ReleaseMemberFact, error) {
	rows, err := queryer.QueryContext(ctx, `
SELECT member_kind, member_id FROM release_members
WHERE project_id = ? AND release_id = ?
ORDER BY recorded_at, id
`, projectID, releaseID)
	if err != nil {
		return nil, fmt.Errorf("list release members for %q: %w", releaseID, err)
	}
	defer rows.Close()
	var members []ReleaseMemberFact
	for rows.Next() {
		var member ReleaseMemberFact
		if err := rows.Scan(&member.Kind, &member.MemberID); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func birthBackendRefsTx(ctx context.Context, tx *sql.Tx) error {
	folded, err := foldRefFacts(ctx, tx, "")
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, project_id, backend, entity_kind, entity_id, external_kind, external_id,
       COALESCE(external_url, ''), sync_status, created_at, updated_at
FROM backend_mappings
ORDER BY project_id, created_at, id
`)
	if err != nil {
		return fmt.Errorf("list backend mappings for birth facts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, backend, entityKind, entityID, externalKind, externalID, externalURL, syncStatus, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &backend, &entityKind, &entityID, &externalKind, &externalID, &externalURL, &syncStatus, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan backend mapping for birth fact: %w", err)
		}
		if foldedRow, ok := folded[id]; ok && foldedRow.Payload.ExternalID == externalID && foldedRow.Payload.EntityID == entityID {
			continue
		}
		payload := CoreEventPayload{
			SubjectKind:  "ref",
			SubjectID:    id,
			Backend:      backend,
			EntityKind:   entityKind,
			EntityID:     entityID,
			ExternalKind: externalKind,
			ExternalID:   externalID,
			ExternalURL:  externalURL,
			SyncStatus:   syncStatus,
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}
		if err := logFactReplayDiscrepancyTx(ctx, tx, projectID, "ref", id, "missing backend mapping fact"); err != nil {
			return err
		}
		if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindRefRegistered, "", payload, time.Now().UTC(), legacyFactEnvID); err != nil {
			return fmt.Errorf("birth backend ref fact %q: %w", id, err)
		}
	}
	return rows.Err()
}

func birthWorkContractRefsTx(ctx context.Context, tx *sql.Tx) error {
	if !tableExists(ctx, tx, "work_contract_mappings") {
		return nil
	}
	folded, err := foldRefFacts(ctx, tx, "")
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, project_id, provider, provider_ref, mapping_kind, mapping_value, created_at, updated_at
FROM work_contract_mappings
ORDER BY project_id, created_at, id
`)
	if err != nil {
		return fmt.Errorf("list work contract mappings for birth facts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, provider, providerRef, mappingKind, mappingValue, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &provider, &providerRef, &mappingKind, &mappingValue, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan work contract mapping for birth fact: %w", err)
		}
		if foldedRow, ok := folded[id]; ok && foldedRow.Payload.MappingValue == mappingValue {
			continue
		}
		payload := CoreEventPayload{
			SubjectKind:  "ref",
			SubjectID:    id,
			Provider:     provider,
			ProviderRef:  providerRef,
			MappingKind:  mappingKind,
			MappingValue: mappingValue,
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}
		if err := logFactReplayDiscrepancyTx(ctx, tx, projectID, "ref", id, "missing work-contract mapping fact"); err != nil {
			return err
		}
		if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindRefRegistered, "", payload, time.Now().UTC(), legacyFactEnvID); err != nil {
			return fmt.Errorf("birth work-contract ref fact %q: %w", id, err)
		}
	}
	return rows.Err()
}

func birthWorktreesTx(ctx context.Context, tx *sql.Tx) error {
	folded, err := foldWorktreeFacts(ctx, tx, "")
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, project_id, COALESCE(started_branch, ''), COALESCE(started_worktree, ''), created_at, updated_at
FROM issues
WHERE started_branch IS NOT NULL AND trim(started_branch) != ''
ORDER BY project_id, id
`)
	if err != nil {
		return fmt.Errorf("list started issues for birth facts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, branch, worktree, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &branch, &worktree, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan started issue for birth fact: %w", err)
		}
		if foldedRow, ok := folded[id]; ok && foldedRow.Payload.Branch == branch && foldedRow.Payload.Worktree == worktree {
			continue
		}
		payload := CoreEventPayload{
			SubjectKind: "issue",
			SubjectID:   id,
			Branch:      branch,
			Worktree:    worktree,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		if err := logFactReplayDiscrepancyTx(ctx, tx, projectID, "worktree", id, "missing issue worktree fact"); err != nil {
			return err
		}
		if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindWorktreeBound, "", payload, time.Now().UTC(), legacyFactEnvID); err != nil {
			return fmt.Errorf("birth issue worktree fact %q: %w", id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !tableExists(ctx, tx, "work_contract_workspace") {
		return nil
	}
	wsRows, err := tx.QueryContext(ctx, `
SELECT contract_id, project_id, COALESCE(started_branch, ''), COALESCE(started_worktree, ''), created_at, updated_at
FROM work_contract_workspace
WHERE started_branch IS NOT NULL AND trim(started_branch) != ''
ORDER BY project_id, contract_id
`)
	if err != nil {
		return fmt.Errorf("list work contract workspaces for birth facts: %w", err)
	}
	defer wsRows.Close()
	for wsRows.Next() {
		var id, projectID, branch, worktree, createdAt, updatedAt string
		if err := wsRows.Scan(&id, &projectID, &branch, &worktree, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan work contract workspace for birth fact: %w", err)
		}
		if foldedRow, ok := folded[id]; ok && foldedRow.Payload.Branch == branch && foldedRow.Payload.Worktree == worktree {
			continue
		}
		payload := CoreEventPayload{
			SubjectKind: "work_contract",
			SubjectID:   id,
			Branch:      branch,
			Worktree:    worktree,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		if err := logFactReplayDiscrepancyTx(ctx, tx, projectID, "worktree", id, "missing work-contract worktree fact"); err != nil {
			return err
		}
		if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindWorktreeBound, "", payload, time.Now().UTC(), legacyFactEnvID); err != nil {
			return fmt.Errorf("birth work-contract worktree fact %q: %w", id, err)
		}
	}
	return wsRows.Err()
}

func birthVerificationReceiptsTx(ctx context.Context, tx *sql.Tx) error {
	if !tableExists(ctx, tx, "work_contract_receipts") {
		return nil
	}
	folded, err := foldVerificationFacts(ctx, tx, "")
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, project_id, provider, provider_ref, receipt_kind, receipt_value, created_at, updated_at
FROM work_contract_receipts
ORDER BY project_id, created_at, id
`)
	if err != nil {
		return fmt.Errorf("list verification receipts for birth facts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, provider, providerRef, receiptKind, receiptValue, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &provider, &providerRef, &receiptKind, &receiptValue, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan verification receipt for birth fact: %w", err)
		}
		if foldedRow, ok := folded[id]; ok && foldedRow.Payload.ReceiptValue == receiptValue {
			continue
		}
		payload := CoreEventPayload{
			SubjectKind:  "verification",
			SubjectID:    id,
			Provider:     provider,
			ProviderRef:  providerRef,
			ReceiptKind:  receiptKind,
			ReceiptValue: receiptValue,
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}
		if err := logFactReplayDiscrepancyTx(ctx, tx, projectID, "verification", id, "missing verification receipt fact"); err != nil {
			return err
		}
		if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindVerificationRecorded, "", payload, time.Now().UTC(), legacyFactEnvID); err != nil {
			return fmt.Errorf("birth verification fact %q: %w", id, err)
		}
	}
	return rows.Err()
}

func backfillRootCommitFingerprintsTx(ctx context.Context, tx *sql.Tx) error {
	if !tableExists(ctx, tx, "project_attachment_evidence") {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
SELECT p.id, pp.path
FROM projects AS p
JOIN project_paths AS pp ON pp.project_id = p.id AND pp.is_current = 1
ORDER BY p.id
`)
	if err != nil {
		return fmt.Errorf("list project paths for root-commit backfill: %w", err)
	}
	defer rows.Close()
	type projectPath struct {
		id   string
		path string
	}
	var projects []projectPath
	for rows.Next() {
		var item projectPath
		if err := rows.Scan(&item.id, &item.path); err != nil {
			return fmt.Errorf("scan project path for root-commit backfill: %w", err)
		}
		projects = append(projects, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, item := range projects {
		if strings.TrimSpace(item.path) == "" {
			continue
		}
		if _, err := os.Stat(item.path); err != nil {
			continue
		}
		root, err := project.ResolveRoot(item.path)
		if err != nil {
			continue
		}
		fingerprint, err := project.RootCommitFingerprint(root)
		if err != nil || strings.TrimSpace(fingerprint) == "" {
			continue
		}
		var existing int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM project_attachment_evidence
WHERE project_id = ? AND evidence_kind = ? AND evidence_value = ?
`, item.id, EvidenceKindRootCommit, fingerprint).Scan(&existing); err != nil {
			return fmt.Errorf("inspect root-commit evidence: %w", err)
		}
		if existing > 0 {
			continue
		}
		evidenceID := stableMigrationID("project-evidence", item.id, EvidenceKindRootCommit, fingerprint)
		stamp := now.Format(time.RFC3339)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO project_attachment_evidence (id, project_id, evidence_kind, evidence_value, first_seen_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, evidence_kind, evidence_value) DO UPDATE SET
  last_seen_at = excluded.last_seen_at
`, evidenceID, item.id, EvidenceKindRootCommit, fingerprint, stamp, stamp); err != nil {
			return fmt.Errorf("insert root-commit evidence: %w", err)
		}
		encoded, err := encodeProjectEvidencePayload(ProjectEvidencePayload{Kind: EvidenceKindRootCommit, Value: fingerprint})
		if err != nil {
			return err
		}
		if _, err := appendFactTx(ctx, tx, AppendFactInput{
			ProjectID: item.id,
			Kind:      FactKindProjectEvidence,
			Payload:   encoded,
			EnvID:     legacyFactEnvID,
			Now:       now,
		}); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return fmt.Errorf("append root-commit evidence fact: %w", err)
		}
	}
	return nil
}

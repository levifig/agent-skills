package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// MutableCoreFactParity describes fold-vs-projection parity for migrated kinds.
type MutableCoreFactParity struct {
	Sparks        KindFactParity `json:"sparks"`
	Ideas         KindFactParity `json:"ideas"`
	Handoffs      KindFactParity `json:"handoffs"`
	Releases      KindFactParity `json:"releases"`
	Refs          KindFactParity `json:"refs"`
	Worktrees     KindFactParity `json:"worktrees"`
	Verifications KindFactParity `json:"verifications"`
	Ready         bool           `json:"ready"`
}

// KindFactParity is one migrated kind's fold-vs-projection counts.
type KindFactParity struct {
	FactSubjects   int  `json:"fact_subjects"`
	ProjectionRows int  `json:"projection_rows"`
	Missing        int  `json:"missing"`
	Extra          int  `json:"extra"`
	Changed        int  `json:"changed"`
	Ready          bool `json:"ready"`
}

const MutableCoreFactParityDivergenceCode = "mutable-core-fact-parity-diverged"

func (p KindFactParity) finish() KindFactParity {
	p.Ready = p.Missing == 0 && p.Extra == 0 && p.Changed == 0
	return p
}

// InspectMutableCoreFactParity compares folded mutable-core facts to projections.
func InspectMutableCoreFactParity(ctx context.Context, store *Store) (MutableCoreFactParity, error) {
	if store == nil || store.db == nil {
		return MutableCoreFactParity{}, fmt.Errorf("inspect mutable core fact parity: store is nil")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return MutableCoreFactParity{}, fmt.Errorf("begin mutable core fact parity snapshot: %w", err)
	}
	defer tx.Rollback()
	return inspectMutableCoreFactParity(ctx, tx)
}

func inspectMutableCoreFactParity(ctx context.Context, queryer queryContext) (MutableCoreFactParity, error) {
	if queryer == nil {
		return MutableCoreFactParity{}, fmt.Errorf("inspect mutable core fact parity: queryer is nil")
	}
	if !tableExists(ctx, queryer, "facts") {
		return MutableCoreFactParity{Ready: true}, nil
	}
	var err error
	var parity MutableCoreFactParity
	parity.Sparks, err = inspectSparkFactParity(ctx, queryer)
	if err != nil {
		return MutableCoreFactParity{}, err
	}
	parity.Ideas, err = inspectIdeaFactParity(ctx, queryer)
	if err != nil {
		return MutableCoreFactParity{}, err
	}
	parity.Handoffs, err = inspectHandoffFactParity(ctx, queryer)
	if err != nil {
		return MutableCoreFactParity{}, err
	}
	parity.Releases, err = inspectReleaseFactParity(ctx, queryer)
	if err != nil {
		return MutableCoreFactParity{}, err
	}
	parity.Refs, err = inspectRefFactParity(ctx, queryer)
	if err != nil {
		return MutableCoreFactParity{}, err
	}
	parity.Worktrees, err = inspectWorktreeFactParity(ctx, queryer)
	if err != nil {
		return MutableCoreFactParity{}, err
	}
	parity.Verifications, err = inspectVerificationFactParity(ctx, queryer)
	if err != nil {
		return MutableCoreFactParity{}, err
	}
	parity.Ready = parity.Sparks.Ready && parity.Ideas.Ready && parity.Handoffs.Ready &&
		parity.Releases.Ready && parity.Refs.Ready && parity.Worktrees.Ready && parity.Verifications.Ready
	return parity, nil
}

func inspectSparkFactParity(ctx context.Context, queryer queryContext) (KindFactParity, error) {
	folded, err := foldSparkFacts(ctx, queryer, "")
	if err != nil {
		return KindFactParity{}, err
	}
	rows, err := queryer.QueryContext(ctx, `
SELECT id, project_id, COALESCE(scope, ''), status, text, created_at, updated_at
FROM sparks
`)
	if err != nil {
		return KindFactParity{}, fmt.Errorf("list spark projections for parity: %w", err)
	}
	defer rows.Close()
	parity := KindFactParity{FactSubjects: len(folded)}
	seen := map[string]struct{}{}
	for rows.Next() {
		var id, projectID, scope, status, text, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &scope, &status, &text, &createdAt, &updatedAt); err != nil {
			return KindFactParity{}, err
		}
		parity.ProjectionRows++
		seen[id] = struct{}{}
		fact, ok := folded[id]
		if !ok {
			parity.Extra++
			continue
		}
		if fact.ProjectID != projectID || !sparkProjectionMatches(scope, status, text, createdAt, updatedAt, fact.Payload) {
			parity.Changed++
		}
	}
	if err := rows.Err(); err != nil {
		return KindFactParity{}, err
	}
	for id := range folded {
		if _, ok := seen[id]; !ok {
			parity.Missing++
		}
	}
	return parity.finish(), nil
}

func inspectIdeaFactParity(ctx context.Context, queryer queryContext) (KindFactParity, error) {
	folded, err := foldIdeaFacts(ctx, queryer, "")
	if err != nil {
		return KindFactParity{}, err
	}
	rows, err := queryer.QueryContext(ctx, `SELECT id, project_id, title, status, created_at, updated_at FROM ideas`)
	if err != nil {
		return KindFactParity{}, fmt.Errorf("list idea projections for parity: %w", err)
	}
	defer rows.Close()
	parity := KindFactParity{FactSubjects: len(folded)}
	seen := map[string]struct{}{}
	for rows.Next() {
		var id, projectID, title, status, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &title, &status, &createdAt, &updatedAt); err != nil {
			return KindFactParity{}, err
		}
		parity.ProjectionRows++
		seen[id] = struct{}{}
		fact, ok := folded[id]
		if !ok {
			parity.Extra++
			continue
		}
		if fact.ProjectID != projectID || !ideaProjectionMatches(title, status, createdAt, updatedAt, fact.Payload) {
			parity.Changed++
		}
	}
	if err := rows.Err(); err != nil {
		return KindFactParity{}, err
	}
	for id := range folded {
		if _, ok := seen[id]; !ok {
			parity.Missing++
		}
	}
	return parity.finish(), nil
}

func inspectHandoffFactParity(ctx context.Context, queryer queryContext) (KindFactParity, error) {
	folded, err := foldHandoffFacts(ctx, queryer, "")
	if err != nil {
		return KindFactParity{}, err
	}
	column := "harness_session_id"
	if tx, ok := queryer.(*sql.Tx); ok {
		if live, err := handoffCorrelationColumn(ctx, tx); err == nil && live != "" {
			column = live
		}
	}
	rows, err := queryer.QueryContext(ctx, fmt.Sprintf(`
SELECT id, project_id, COALESCE(%s, ''), COALESCE(task_id, ''), title, status, created_at, updated_at
FROM handoffs
`, column))
	if err != nil {
		return KindFactParity{}, fmt.Errorf("list handoff projections for parity: %w", err)
	}
	defer rows.Close()
	parity := KindFactParity{FactSubjects: len(folded)}
	seen := map[string]struct{}{}
	for rows.Next() {
		var id, projectID, sessionID, taskID, title, status, createdAt, updatedAt string
		if err := rows.Scan(&id, &projectID, &sessionID, &taskID, &title, &status, &createdAt, &updatedAt); err != nil {
			return KindFactParity{}, err
		}
		parity.ProjectionRows++
		seen[id] = struct{}{}
		fact, ok := folded[id]
		if !ok {
			parity.Extra++
			continue
		}
		if fact.ProjectID != projectID || !handoffProjectionMatches(sessionID, taskID, title, status, createdAt, updatedAt, fact.Payload) {
			parity.Changed++
		}
	}
	if err := rows.Err(); err != nil {
		return KindFactParity{}, err
	}
	for id := range folded {
		if _, ok := seen[id]; !ok {
			parity.Missing++
		}
	}
	return parity.finish(), nil
}

func inspectReleaseFactParity(ctx context.Context, queryer queryContext) (KindFactParity, error) {
	folded, err := foldReleaseFacts(ctx, queryer, "")
	if err != nil {
		return KindFactParity{}, err
	}
	rows, err := queryer.QueryContext(ctx, `
SELECT id, project_id, version, tag, tagged_commit, COALESCE(notes, '')
FROM releases
`)
	if err != nil {
		return KindFactParity{}, fmt.Errorf("list release projections for parity: %w", err)
	}
	defer rows.Close()
	parity := KindFactParity{FactSubjects: len(folded)}
	seen := map[string]struct{}{}
	for rows.Next() {
		var id, projectID, version, tag, commit, notes string
		if err := rows.Scan(&id, &projectID, &version, &tag, &commit, &notes); err != nil {
			return KindFactParity{}, err
		}
		parity.ProjectionRows++
		seen[id] = struct{}{}
		fact, ok := folded[id]
		if !ok {
			parity.Extra++
			continue
		}
		if fact.ProjectID != projectID || fact.Payload.Version != version || fact.Payload.Tag != tag || fact.Payload.TaggedCommit != commit || fact.Payload.Notes != notes {
			parity.Changed++
		}
	}
	if err := rows.Err(); err != nil {
		return KindFactParity{}, err
	}
	for id := range folded {
		if _, ok := seen[id]; !ok {
			parity.Missing++
		}
	}
	return parity.finish(), nil
}

func inspectRefFactParity(ctx context.Context, queryer queryContext) (KindFactParity, error) {
	folded, err := foldRefFacts(ctx, queryer, "")
	if err != nil {
		return KindFactParity{}, err
	}
	parity := KindFactParity{FactSubjects: len(folded)}
	seen := map[string]struct{}{}
	rows, err := queryer.QueryContext(ctx, `SELECT id, project_id, entity_id, external_id FROM backend_mappings`)
	if err != nil {
		return KindFactParity{}, fmt.Errorf("list backend mappings for parity: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, entityID, externalID string
		if err := rows.Scan(&id, &projectID, &entityID, &externalID); err != nil {
			return KindFactParity{}, err
		}
		parity.ProjectionRows++
		seen[id] = struct{}{}
		fact, ok := folded[id]
		if !ok {
			parity.Extra++
			continue
		}
		if fact.ProjectID != projectID || fact.Payload.EntityID != entityID || fact.Payload.ExternalID != externalID {
			parity.Changed++
		}
	}
	if err := rows.Err(); err != nil {
		return KindFactParity{}, err
	}
	if tableExists(ctx, queryer, "work_contract_mappings") {
		mappingRows, err := queryer.QueryContext(ctx, `SELECT id, project_id, mapping_value FROM work_contract_mappings`)
		if err != nil {
			return KindFactParity{}, fmt.Errorf("list work contract mappings for parity: %w", err)
		}
		defer mappingRows.Close()
		for mappingRows.Next() {
			var id, projectID, value string
			if err := mappingRows.Scan(&id, &projectID, &value); err != nil {
				return KindFactParity{}, err
			}
			parity.ProjectionRows++
			seen[id] = struct{}{}
			fact, ok := folded[id]
			if !ok {
				parity.Extra++
				continue
			}
			if fact.ProjectID != projectID || fact.Payload.MappingValue != value {
				parity.Changed++
			}
		}
		if err := mappingRows.Err(); err != nil {
			return KindFactParity{}, err
		}
	}
	for id := range folded {
		if _, ok := seen[id]; !ok {
			parity.Missing++
		}
	}
	return parity.finish(), nil
}

func inspectWorktreeFactParity(ctx context.Context, queryer queryContext) (KindFactParity, error) {
	folded, err := foldWorktreeFacts(ctx, queryer, "")
	if err != nil {
		return KindFactParity{}, err
	}
	parity := KindFactParity{FactSubjects: len(folded)}
	seen := map[string]struct{}{}
	rows, err := queryer.QueryContext(ctx, `
SELECT id, project_id, COALESCE(started_branch, ''), COALESCE(started_worktree, '')
FROM issues
WHERE started_branch IS NOT NULL AND trim(started_branch) != ''
`)
	if err != nil {
		return KindFactParity{}, fmt.Errorf("list started issues for parity: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, branch, worktree string
		if err := rows.Scan(&id, &projectID, &branch, &worktree); err != nil {
			return KindFactParity{}, err
		}
		parity.ProjectionRows++
		seen[id] = struct{}{}
		fact, ok := folded[id]
		if !ok {
			parity.Extra++
			continue
		}
		if fact.ProjectID != projectID || fact.Payload.Branch != branch || fact.Payload.Worktree != worktree {
			parity.Changed++
		}
	}
	if err := rows.Err(); err != nil {
		return KindFactParity{}, err
	}
	if tableExists(ctx, queryer, "work_contract_workspace") {
		wsRows, err := queryer.QueryContext(ctx, `
SELECT contract_id, project_id, COALESCE(started_branch, ''), COALESCE(started_worktree, '')
FROM work_contract_workspace
WHERE started_branch IS NOT NULL AND trim(started_branch) != ''
`)
		if err != nil {
			return KindFactParity{}, fmt.Errorf("list work contract workspaces for parity: %w", err)
		}
		defer wsRows.Close()
		for wsRows.Next() {
			var id, projectID, branch, worktree string
			if err := wsRows.Scan(&id, &projectID, &branch, &worktree); err != nil {
				return KindFactParity{}, err
			}
			parity.ProjectionRows++
			seen[id] = struct{}{}
			fact, ok := folded[id]
			if !ok {
				parity.Extra++
				continue
			}
			if fact.ProjectID != projectID || fact.Payload.Branch != branch || fact.Payload.Worktree != worktree {
				parity.Changed++
			}
		}
		if err := wsRows.Err(); err != nil {
			return KindFactParity{}, err
		}
	}
	for id, row := range folded {
		if strings.TrimSpace(row.Payload.Branch) == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			parity.Missing++
		}
	}
	return parity.finish(), nil
}

func inspectVerificationFactParity(ctx context.Context, queryer queryContext) (KindFactParity, error) {
	if !tableExists(ctx, queryer, "work_contract_receipts") {
		return KindFactParity{Ready: true}, nil
	}
	folded, err := foldVerificationFacts(ctx, queryer, "")
	if err != nil {
		return KindFactParity{}, err
	}
	rows, err := queryer.QueryContext(ctx, `SELECT id, project_id, receipt_value FROM work_contract_receipts`)
	if err != nil {
		return KindFactParity{}, fmt.Errorf("list verification receipts for parity: %w", err)
	}
	defer rows.Close()
	parity := KindFactParity{FactSubjects: len(folded)}
	seen := map[string]struct{}{}
	for rows.Next() {
		var id, projectID, value string
		if err := rows.Scan(&id, &projectID, &value); err != nil {
			return KindFactParity{}, err
		}
		parity.ProjectionRows++
		seen[id] = struct{}{}
		fact, ok := folded[id]
		if !ok {
			parity.Extra++
			continue
		}
		if fact.ProjectID != projectID || fact.Payload.ReceiptValue != value {
			parity.Changed++
		}
	}
	if err := rows.Err(); err != nil {
		return KindFactParity{}, err
	}
	for id := range folded {
		if _, ok := seen[id]; !ok {
			parity.Missing++
		}
	}
	return parity.finish(), nil
}

package sqlite

import (
	"context"
	"database/sql"

	"github.com/levifig/loaf/vnext/continuity"
)

// Snapshot returns the complete deterministic projection for one project.
func (store *Store) Snapshot(ctx context.Context, projectID continuity.ProjectID, request continuity.SnapshotRequest) (continuity.Snapshot, error) {
	if err := request.Validate(); err != nil {
		return continuity.Snapshot{}, err
	}
	if err := projectID.Validate(); err != nil {
		return continuity.Snapshot{}, refieldProblemV1(err, "project_id")
	}
	return store.snapshotV1(ctx, projectID, request.AtMillis)
}

// DeriveContext returns the deterministic fixed-layer context for one project.
func (store *Store) DeriveContext(ctx context.Context, projectID continuity.ProjectID, request continuity.ContextRequest) (continuity.ContextDigest, error) {
	if err := request.Validate(); err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := projectID.Validate(); err != nil {
		return continuity.ContextDigest{}, refieldProblemV1(err, "project_id")
	}
	return store.deriveContextV1(ctx, projectID, request)
}

func (store *Store) snapshotV1(ctx context.Context, projectID continuity.ProjectID, atMillis int64) (continuity.Snapshot, error) {
	if store == nil {
		return continuity.Snapshot{}, storeClosedProblemV1()
	}
	if ctx == nil {
		return continuity.Snapshot{}, &continuity.Problem{Code: continuity.ProblemInvalid, Field: "context", Detail: "must not be nil"}
	}
	if err := ctx.Err(); err != nil {
		return continuity.Snapshot{}, err
	}
	facts, err := store.loadSnapshotFactsV1(ctx, projectID)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if len(facts) == 0 {
		return continuity.Snapshot{}, &continuity.Problem{Code: continuity.ProblemProjectNotRegistered, Field: "project_id", Detail: "has no continuity identity"}
	}
	return foldProjectSnapshotV1(ctx, projectID, atMillis, facts)
}

func (store *Store) deriveContextV1(ctx context.Context, projectID continuity.ProjectID, request continuity.ContextRequest) (continuity.ContextDigest, error) {
	if store == nil {
		return continuity.ContextDigest{}, storeClosedProblemV1()
	}
	if ctx == nil {
		return continuity.ContextDigest{}, &continuity.Problem{Code: continuity.ProblemInvalid, Field: "context", Detail: "must not be nil"}
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}
	facts, err := store.loadSnapshotFactsV1(ctx, projectID)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if len(facts) == 0 {
		return continuity.ContextDigest{}, &continuity.Problem{Code: continuity.ProblemProjectNotRegistered, Field: "project_id", Detail: "has no continuity identity"}
	}
	snapshot, err := foldProjectSnapshotV1(ctx, projectID, request.AtMillis, facts)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	relations, err := resolveContextFocusRelationsV1(ctx, facts, request.Focus)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	return deriveContextDigestV1(ctx, snapshot, request, relations)
}

func (store *Store) loadSnapshotFactsV1(ctx context.Context, projectID continuity.ProjectID) ([]storedFactV1, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return nil, storeClosedProblemV1()
	}

	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, storeUnavailableProblemV1()
	}
	defer tx.Rollback()

	facts, err := loadProjectFactsV1(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, storeUnavailableProblemV1()
	}
	return facts, nil
}

func loadProjectFactsV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) ([]storedFactV1, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
  fact_id,
  project_id,
  subject_kind,
  subject_id,
  fact_kind,
  payload_version,
  content_json,
  environment_id,
  environment_sequence,
  hlc_wall_millis,
  hlc_logical,
  envelope_version
FROM continuity_facts
WHERE project_id = ?
ORDER BY
  hlc_wall_millis ASC,
  hlc_logical ASC,
  environment_id COLLATE BINARY ASC,
  fact_id COLLATE BINARY ASC`, string(projectID))
	if err != nil {
		return nil, transactionOperationProblemV1(ctx)
	}

	facts := make([]storedFactV1, 0)
	for rows.Next() {
		fact, err := scanStoredFactRowsV1(ctx, rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, transactionOperationProblemV1(ctx)
	}
	if err := rows.Close(); err != nil {
		return nil, transactionOperationProblemV1(ctx)
	}
	return facts, nil
}

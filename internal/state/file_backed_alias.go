package state

import (
	"context"
	"fmt"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// RegisterFileBackedEntityAlias records a file-backed entity alias for trace and linking.
func RegisterFileBackedEntityAlias(ctx context.Context, root project.Root, resolver PathResolver, kind, alias, title string) error {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return err
	}
	entityID := stableMigrationID(kind, projectID, alias)
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin file-backed alias transaction: %w", err)
	}
	defer tx.Rollback()
	if err := insertAlias(ctx, tx, projectID, kind, entityID, kind, alias, now); err != nil {
		return err
	}
	return tx.Commit()
}

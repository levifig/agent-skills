package state

import (
	"context"
	"fmt"
	"os"

	"github.com/levifig/loaf/internal/project"
)

// projectStoreIntent describes how a project-scoped command may open the
// global state database. Registration is intentionally not an ordinary store
// intent: only Initialize and the explicit project identity APIs may create or
// update a project-path mapping.
type projectStoreIntent uint8

const (
	projectStoreReadExisting projectStoreIntent = iota + 1
	projectStoreMutateExisting
)

// openProjectStore opens an already-existing project store for one of the two
// ordinary intents. Both intents require a durable database file and resolve
// the current project path before returning. A lookup failure closes the store
// so unknown checkouts cannot leave an open connection behind.
func openProjectStore(ctx context.Context, root project.Root, resolver PathResolver, intent projectStoreIntent) (*Store, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	} else if err != nil {
		return nil, fmt.Errorf("inspect state database: %w", err)
	}

	var store *Store
	switch intent {
	case projectStoreReadExisting:
		store, err = OpenStoreReadOnly(databasePath)
		if err == nil {
			err = store.RequireCurrentSchema(ctx)
		}
	case projectStoreMutateExisting:
		store, err = OpenStore(databasePath)
		if err == nil {
			err = store.RequireCurrentSchema(ctx)
		}
	default:
		return nil, fmt.Errorf("unsupported project store intent %d", intent)
	}
	if err != nil {
		if store != nil {
			if closeErr := store.Close(); closeErr != nil {
				return nil, fmt.Errorf("%w; close state database: %v", err, closeErr)
			}
		}
		return nil, err
	}

	if _, err := store.LookupProjectIdentityForRoot(ctx, root); err != nil {
		if closeErr := store.Close(); closeErr != nil {
			return nil, fmt.Errorf("%w; close state database: %v", err, closeErr)
		}
		return nil, err
	}
	return store, nil
}

func openProjectStoreReadExisting(ctx context.Context, root project.Root, resolver PathResolver) (*Store, error) {
	return openProjectStore(ctx, root, resolver, projectStoreReadExisting)
}

// openProjectStoreReadExistingForJournalSearch validates every canonical
// invariant while deliberately leaving one derived invariant for the journal
// search boundary to report with its structured divergence error. This is not
// a general read escape hatch: callers must immediately use
// requireJournalSearchReady before returning derived results.
func openProjectStoreReadExistingForJournalSearch(ctx context.Context, root project.Root, resolver PathResolver) (*Store, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	} else if err != nil {
		return nil, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		return nil, err
	}
	if err := requireCurrentSchemaForDerivedRepair(ctx, store); err == nil {
		_, err = store.LookupProjectIdentityForRoot(ctx, root)
	}
	if err != nil {
		if closeErr := store.Close(); closeErr != nil {
			return nil, fmt.Errorf("%w; close state database: %v", err, closeErr)
		}
		return nil, err
	}
	return store, nil
}

func openProjectStoreMutateExisting(ctx context.Context, root project.Root, resolver PathResolver) (*Store, error) {
	return openProjectStore(ctx, root, resolver, projectStoreMutateExisting)
}

// openInitializedStore is retained for non-journal callers while they migrate
// to explicit store intents. It now mutates only existing schema state and
// never registers or refreshes a project identity.
func openInitializedStore(root project.Root, resolver PathResolver) (*Store, error) {
	return openProjectStoreMutateExisting(context.Background(), root, resolver)
}

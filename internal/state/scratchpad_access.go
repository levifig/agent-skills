package state

import (
	"context"

	"github.com/levifig/loaf/internal/project"
)

// OpenProjectStoreForWrite opens an initialized project store for mutation.
func OpenProjectStoreForWrite(ctx context.Context, root project.Root, resolver PathResolver) (*Store, error) {
	return openProjectStoreMutateExisting(ctx, root, resolver)
}

// ProjectIDForRoot resolves the durable project id for a root path.
func (s *Store) ProjectIDForRoot(ctx context.Context, root project.Root) (string, error) {
	return s.projectID(ctx, root)
}

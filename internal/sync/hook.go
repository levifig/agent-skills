package sync

import (
	"context"

	"github.com/levifig/loaf/internal/state"
)

func init() {
	state.SetSyncEnqueueHook(enqueueOutboundFact)
}

func enqueueOutboundFact(ctx context.Context, store *state.Store, projectID, factID string) error {
	return EnqueueOutboundFact(ctx, store, projectID, factID)
}

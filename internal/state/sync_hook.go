package state

import "context"

// SyncEnqueueHook enqueues a locally appended fact for outbound sync.
type SyncEnqueueHook func(ctx context.Context, store *Store, projectID, factID string) error

var syncEnqueueHook SyncEnqueueHook

// SetSyncEnqueueHook registers the client sync outbound queue hook.
func SetSyncEnqueueHook(hook SyncEnqueueHook) {
	syncEnqueueHook = hook
}

func invokeSyncEnqueueHook(ctx context.Context, store *Store, projectID, factID string) error {
	if syncEnqueueHook == nil {
		return nil
	}
	return syncEnqueueHook(ctx, store, projectID, factID)
}

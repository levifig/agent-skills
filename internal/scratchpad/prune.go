package scratchpad

import (
	"context"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/syncserver"
)

// PruneServerChannel deletes relay scratchpad blobs for one channel.
// The caller must authenticate admin access to the server store first.
func PruneServerChannel(ctx context.Context, server *syncserver.Store, projectID, channel string) (int64, error) {
	if server == nil {
		return 0, fmt.Errorf("prune scratchpad channel: server store is nil")
	}
	channel, err := normalizeChannel(channel)
	if err != nil {
		return 0, err
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return 0, fmt.Errorf("prune scratchpad channel: project id is required")
	}
	return server.PruneScratchpadChannel(ctx, projectID, channel)
}


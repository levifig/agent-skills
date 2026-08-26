package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

type enrollmentPayload struct {
	EnvID string `json:"env_id"`
}

func listExpectedEnvironments(ctx context.Context, store *state.Store, projectID string) (map[string]struct{}, error) {
	rows, err := store.DB().QueryContext(ctx, `
SELECT payload
FROM facts
WHERE project_id = ? AND kind = ?
ORDER BY hlc ASC, env_id ASC, id ASC
`, projectID, state.FactKindEnrollment)
	if err != nil {
		return nil, fmt.Errorf("list enrollment facts: %w", err)
	}
	defer rows.Close()
	expected := map[string]struct{}{}
	for rows.Next() {
		var payloadRaw string
		if err := rows.Scan(&payloadRaw); err != nil {
			return nil, fmt.Errorf("scan enrollment fact: %w", err)
		}
		var payload enrollmentPayload
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			return nil, fmt.Errorf("decode enrollment payload: %w", err)
		}
		envID := strings.TrimSpace(payload.EnvID)
		if envID == "" {
			continue
		}
		expected[envID] = struct{}{}
	}
	return expected, rows.Err()
}

package sync

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

// EnvSeqGap describes an interior omission in one environment's monotonic seq stream.
type EnvSeqGap struct {
	EnvID    string  `json:"env_id"`
	Missing  []int64 `json:"missing"`
	Observed int     `json:"observed"`
	Expected bool    `json:"expected"`
}

// DetectEnvSeqGaps scans local facts for interior seq gaps per environment.
func DetectEnvSeqGaps(ctx context.Context, store *state.Store, projectID string) ([]EnvSeqGap, error) {
	if store == nil {
		return nil, fmt.Errorf("detect env seq gaps: store is nil")
	}
	expected, err := listExpectedEnvironments(ctx, store, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := store.DB().QueryContext(ctx, `
SELECT env_id, seq
FROM facts
WHERE project_id = ? AND kind != ?
ORDER BY env_id ASC, seq ASC
`, projectID, state.FactKindEnrollment)
	if err != nil {
		return nil, fmt.Errorf("list fact env seq: %w", err)
	}
	defer rows.Close()

	byEnv := map[string][]int64{}
	for rows.Next() {
		var envID string
		var seq int64
		if err := rows.Scan(&envID, &seq); err != nil {
			return nil, fmt.Errorf("scan fact env seq: %w", err)
		}
		byEnv[envID] = append(byEnv[envID], seq)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	gaps := make([]EnvSeqGap, 0)
	for envID, seqs := range byEnv {
		if len(seqs) == 0 {
			continue
		}
		missing := interiorMissing(seqs)
		if len(missing) == 0 {
			continue
		}
		_, isExpected := expected[envID]
		gaps = append(gaps, EnvSeqGap{
			EnvID:    envID,
			Missing:  missing,
			Observed: len(seqs),
			Expected: isExpected,
		})
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].EnvID < gaps[j].EnvID })
	return gaps, nil
}

func interiorMissing(seqs []int64) []int64 {
	if len(seqs) <= 1 {
		return nil
	}
	seen := map[int64]struct{}{}
	min, max := seqs[0], seqs[0]
	for _, seq := range seqs {
		seen[seq] = struct{}{}
		if seq < min {
			min = seq
		}
		if seq > max {
			max = seq
		}
	}
	missing := make([]int64, 0)
	for seq := min; seq <= max; seq++ {
		if _, ok := seen[seq]; !ok {
			missing = append(missing, seq)
		}
	}
	return missing
}

func formatGapWarnings(gaps []EnvSeqGap) []string {
	warnings := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		parts := make([]string, 0, len(gap.Missing))
		for _, seq := range gap.Missing {
			parts = append(parts, fmt.Sprintf("%d", seq))
		}
		label := "observed"
		if gap.Expected {
			label = "expected"
		}
		warnings = append(warnings, fmt.Sprintf("env-seq gap (%s env %s): missing seq %s",
			label, gap.EnvID, strings.Join(parts, ", ")))
	}
	return warnings
}

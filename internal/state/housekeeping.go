package state

import (
	"context"
	"fmt"

	"github.com/levifig/loaf/internal/project"
)

// HousekeepingSummary is the SQLite-backed housekeeping read model.
type HousekeepingSummary struct {
	ContractVersion    int                            `json:"contract_version,omitempty"`
	DatabaseScope      string                         `json:"database_scope,omitempty"`
	DatabasePath       string                         `json:"database_path"`
	ProjectID          string                         `json:"project_id,omitempty"`
	ProjectName        string                         `json:"project_name,omitempty"`
	ProjectCurrentPath string                         `json:"project_current_path,omitempty"`
	Version            int                            `json:"version"`
	Sections           map[string]HousekeepingSection `json:"sections"`
	Signals            []string                       `json:"signals"`
}

// HousekeepingSection summarizes one operational state area.
type HousekeepingSection struct {
	Total            int            `json:"total"`
	ByStatus         map[string]int `json:"by_status,omitempty"`
	CleanupCandidate int            `json:"cleanup_candidate"`
}

// Housekeeping returns lifecycle and cleanup signals from initialized SQLite state.
func Housekeeping(ctx context.Context, root project.Root, resolver PathResolver) (HousekeepingSummary, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return HousekeepingSummary{}, err
	}
	defer store.Close()
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return HousekeepingSummary{}, err
	}
	return store.Housekeeping(ctx, root, databasePath)
}

// Housekeeping returns lifecycle and cleanup signals from an open store.
func (s *Store) Housekeeping(ctx context.Context, root project.Root, databasePath string) (HousekeepingSummary, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return HousekeepingSummary{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return HousekeepingSummary{}, err
	}
	sections := map[string]HousekeepingSection{}
	specs, err := s.housekeepingStatusSection(ctx, "specs", projectID, "complete", "archived")
	if err != nil {
		return HousekeepingSummary{}, err
	}
	sections["specs"] = specs
	tasks, err := s.housekeepingStatusSection(ctx, "tasks", projectID, "done", "archived")
	if err != nil {
		return HousekeepingSummary{}, err
	}
	sections["tasks"] = tasks
	ideas, err := s.housekeepingStatusSection(ctx, "ideas", projectID, "resolved", "archived")
	if err != nil {
		return HousekeepingSummary{}, err
	}
	sections["ideas"] = ideas
	sparks, err := s.housekeepingStatusSection(ctx, "sparks", projectID, "resolved", "archived")
	if err != nil {
		return HousekeepingSummary{}, err
	}
	sections["sparks"] = sparks
	brainstorms, err := s.housekeepingStatusSection(ctx, "brainstorms", projectID, "resolved", "archived")
	if err != nil {
		return HousekeepingSummary{}, err
	}
	sections["brainstorms"] = brainstorms
	reports, err := s.housekeepingStatusSection(ctx, "reports", projectID, "final", "archived")
	if err != nil {
		return HousekeepingSummary{}, err
	}
	sections["reports"] = reports
	shapingDrafts, err := s.housekeepingStatusSection(ctx, "shaping_drafts", projectID, "absorbed", "archived")
	if err != nil {
		return HousekeepingSummary{}, err
	}
	sections["shaping_drafts"] = shapingDrafts

	return HousekeepingSummary{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Version:            1,
		Sections:           sections,
		Signals:            housekeepingSignals(sections),
	}, nil
}

func (s *Store) housekeepingStatusSection(ctx context.Context, table string, projectID string, cleanupStatuses ...string) (HousekeepingSection, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT status, COUNT(*) FROM %s WHERE project_id = ? GROUP BY status`, table), projectID)
	if err != nil {
		return HousekeepingSection{}, fmt.Errorf("query housekeeping %s: %w", table, err)
	}
	section := HousekeepingSection{ByStatus: map[string]int{}}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return HousekeepingSection{}, fmt.Errorf("scan housekeeping %s: %w", table, err)
		}
		section.ByStatus[status] = count
		section.Total += count
		for _, cleanupStatus := range cleanupStatuses {
			if status == cleanupStatus {
				section.CleanupCandidate += count
			}
		}
	}
	if err := rows.Close(); err != nil {
		return HousekeepingSection{}, fmt.Errorf("close housekeeping %s: %w", table, err)
	}
	if err := rows.Err(); err != nil {
		return HousekeepingSection{}, fmt.Errorf("iterate housekeeping %s: %w", table, err)
	}
	return section, nil
}

func housekeepingSignals(sections map[string]HousekeepingSection) []string {
	var signals []string
	for _, name := range []string{"specs", "tasks", "ideas", "sparks", "brainstorms", "reports", "shaping_drafts"} {
		section := sections[name]
		if section.CleanupCandidate > 0 {
			signals = append(signals, fmt.Sprintf("%s:%d", name, section.CleanupCandidate))
		}
	}
	return signals
}

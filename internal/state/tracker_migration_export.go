package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/levifig/loaf/internal/project"
)

const (
	// TrackerMigrationContractVersion versions the provider-neutral packet,
	// independently from Loaf's internal state and backup contracts.
	TrackerMigrationContractVersion = 1
	ExportKindTrackerMigration      = "tracker-migration"
)

// TrackerMigrationProject identifies the source project without exposing its
// database or filesystem location.
type TrackerMigrationProject struct {
	ID        string `json:"id"`
	LabelHint string `json:"label_hint"`
}

// TrackerMigrationCriterion is the provider-facing part of one definition of
// done item. Local verification commands stay local because they may contain
// machine-specific paths or implementation details.
type TrackerMigrationCriterion struct {
	Position int    `json:"position"`
	Text     string `json:"text"`
}

// TrackerMigrationIssue is one open, non-archived issue to create or adopt in
// an external tracker. Its source ID and alias are retained for collision
// detection and receipts.
type TrackerMigrationIssue struct {
	ID               string                      `json:"id"`
	Alias            string                      `json:"alias,omitempty"`
	ParentID         string                      `json:"parent_id,omitempty"`
	Kind             string                      `json:"kind"`
	Status           string                      `json:"status"`
	Title            string                      `json:"title"`
	Body             string                      `json:"body"`
	DefinitionOfDone []TrackerMigrationCriterion `json:"definition_of_done"`
}

// TrackerMigrationRelationship is an issue-to-issue edge whose endpoints are
// both present in the packet.
type TrackerMigrationRelationship struct {
	FromIssueID string `json:"from_issue_id"`
	ToIssueID   string `json:"to_issue_id"`
	Type        string `json:"type"`
	Reason      string `json:"reason,omitempty"`
}

// TrackerMigrationPacket is deliberately narrower than IssueExportSnapshot.
// It cannot serialize local paths, database metadata, worktree state, sessions,
// or private continuity because those fields do not exist in this type.
type TrackerMigrationPacket struct {
	ContractVersion int                            `json:"contract_version"`
	ExportKind      string                         `json:"export_kind"`
	Project         TrackerMigrationProject        `json:"project"`
	Issues          []TrackerMigrationIssue        `json:"issues"`
	Relationships   []TrackerMigrationRelationship `json:"relationships"`
}

// ExportTrackerMigration returns a read-only provider-neutral migration packet
// for the current project.
func ExportTrackerMigration(ctx context.Context, root project.Root, resolver PathResolver) (TrackerMigrationPacket, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return TrackerMigrationPacket{}, err
	}
	defer store.Close()
	return store.ExportTrackerMigration(ctx, root)
}

type trackerMigrationExportHooks struct {
	afterIssuesRead func() error
}

// ExportTrackerMigration reads every packet field from one SQLite read
// transaction, then projects that snapshot onto the external-tracker boundary.
func (s *Store) ExportTrackerMigration(ctx context.Context, root project.Root) (TrackerMigrationPacket, error) {
	return s.exportTrackerMigration(ctx, root, trackerMigrationExportHooks{})
}

func (s *Store) exportTrackerMigration(ctx context.Context, root project.Root, hooks trackerMigrationExportHooks) (TrackerMigrationPacket, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return TrackerMigrationPacket{}, fmt.Errorf("begin tracker migration export: %w", err)
	}
	defer tx.Rollback()

	sourceProject, err := trackerMigrationProjectTx(ctx, tx, root)
	if err != nil {
		return TrackerMigrationPacket{}, fmt.Errorf("read tracker migration project identity: %w", err)
	}
	issueIDs, err := listIssueIDsTx(ctx, tx, sourceProject.ID, IssueListOptions{Archived: true}, "")
	if err != nil {
		return TrackerMigrationPacket{}, err
	}
	sourceIssues := make([]Issue, 0, len(issueIDs))
	for _, issueID := range issueIDs {
		issue, err := loadIssueTx(ctx, tx, sourceProject.ID, issueID)
		if err != nil {
			return TrackerMigrationPacket{}, err
		}
		sourceIssues = append(sourceIssues, issue)
	}
	if hooks.afterIssuesRead != nil {
		if err := hooks.afterIssuesRead(); err != nil {
			return TrackerMigrationPacket{}, fmt.Errorf("tracker migration export test hook: %w", err)
		}
	}
	relationships, err := exportIssueRelationshipsTx(ctx, tx, sourceProject.ID)
	if err != nil {
		return TrackerMigrationPacket{}, err
	}

	packet := TrackerMigrationPacket{
		ContractVersion: TrackerMigrationContractVersion,
		ExportKind:      ExportKindTrackerMigration,
		Project:         sourceProject,
		Issues:          []TrackerMigrationIssue{},
		Relationships:   []TrackerMigrationRelationship{},
	}
	openIssueIDs := make(map[string]struct{}, len(sourceIssues))
	for _, issue := range sourceIssues {
		if issue.ArchivedAt != "" || !trackerMigrationOpenStatus(issue.Status) {
			continue
		}
		openIssueIDs[issue.ID] = struct{}{}
	}
	for _, issue := range sourceIssues {
		if _, ok := openIssueIDs[issue.ID]; !ok {
			continue
		}
		criteria := make([]TrackerMigrationCriterion, 0, len(issue.Criteria))
		for _, criterion := range issue.Criteria {
			criteria = append(criteria, TrackerMigrationCriterion{
				Position: criterion.Position,
				Text:     criterion.Text,
			})
		}
		parentID := issue.ParentID
		if _, ok := openIssueIDs[parentID]; !ok {
			parentID = ""
		}
		packet.Issues = append(packet.Issues, TrackerMigrationIssue{
			ID:               issue.ID,
			Alias:            issue.Alias,
			ParentID:         parentID,
			Kind:             issue.Kind,
			Status:           issue.Status,
			Title:            issue.Title,
			Body:             issue.Body,
			DefinitionOfDone: criteria,
		})
	}
	for _, relationship := range relationships {
		if relationship.FromEntityKind != issueEntityKind || relationship.ToEntityKind != issueEntityKind {
			continue
		}
		if _, ok := openIssueIDs[relationship.FromEntityID]; !ok {
			continue
		}
		if _, ok := openIssueIDs[relationship.ToEntityID]; !ok {
			continue
		}
		packet.Relationships = append(packet.Relationships, TrackerMigrationRelationship{
			FromIssueID: relationship.FromEntityID,
			ToIssueID:   relationship.ToEntityID,
			Type:        relationship.RelationshipType,
			Reason:      relationship.Reason,
		})
	}
	if err := tx.Commit(); err != nil {
		return TrackerMigrationPacket{}, fmt.Errorf("finish tracker migration export snapshot: %w", err)
	}
	return packet, nil
}

func trackerMigrationProjectTx(ctx context.Context, tx *sql.Tx, root project.Root) (TrackerMigrationProject, error) {
	var source TrackerMigrationProject
	err := tx.QueryRowContext(ctx, `
SELECT p.id, COALESCE(NULLIF(p.friendly_name, ''), p.id)
FROM project_paths requested
JOIN projects p ON p.id = requested.project_id
WHERE requested.path = ?
`, root.Path()).Scan(&source.ID, &source.LabelHint)
	if err != nil {
		return TrackerMigrationProject{}, err
	}
	return source, nil
}

func trackerMigrationOpenStatus(status string) bool {
	switch status {
	case IssueStatusTriage, IssueStatusBacklog, IssueStatusTodo, IssueStatusActive:
		return true
	default:
		return false
	}
}

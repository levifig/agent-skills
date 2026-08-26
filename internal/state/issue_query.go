package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// IssueListOptions filters a project issue listing.
type IssueListOptions struct {
	Status   string
	Kind     string
	Archived bool
	Started  bool
}

// IssueSummary is a compact issue row used in trees, children, and frontier.
type IssueSummary struct {
	ID     string `json:"id"`
	Alias  string `json:"alias,omitempty"`
	Parent string `json:"parent_id,omitempty"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// RenderIssueMarkdown is the shaping body loaf push writes to Linear.
func RenderIssueMarkdown(result IssueResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", result.Issue.Title)
	if strings.TrimSpace(result.Issue.Body) != "" {
		fmt.Fprintln(&b)
		b.WriteString(result.Issue.Body)
		if !strings.HasSuffix(result.Issue.Body, "\n") {
			fmt.Fprintln(&b)
		}
	}
	if len(result.Issue.Criteria) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "## Definition of Done")
		fmt.Fprintln(&b)
		checked := result.Issue.Status == IssueStatusDone
		for _, criterion := range result.Issue.Criteria {
			mark := " "
			if checked {
				mark = "x"
			}
			fmt.Fprintf(&b, "- [%s] %s\n", mark, criterion.Text)
		}
	}
	if len(result.Children) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "## Children")
		fmt.Fprintln(&b)
		for _, child := range result.Children {
			fmt.Fprintf(&b, "- %s: %s\n", firstNonEmpty(child.Alias, child.ID), child.Title)
		}
	}
	return b.String()
}

// IssueResult is one issue plus project identity for CLI mutation/show JSON.
type IssueResult struct {
	ContractVersion    int            `json:"contract_version,omitempty"`
	DatabaseScope      string         `json:"database_scope,omitempty"`
	DatabasePath       string         `json:"database_path,omitempty"`
	ProjectID          string         `json:"project_id,omitempty"`
	ProjectName        string         `json:"project_name,omitempty"`
	ProjectCurrentPath string         `json:"project_current_path,omitempty"`
	Issue              Issue          `json:"issue"`
	Parent             *IssueSummary  `json:"parent,omitempty"`
	Children           []IssueSummary `json:"children,omitempty"`
	Bucket             string         `json:"bucket,omitempty"`
}

// IssueListResult is a project-scoped issue listing.
type IssueListResult struct {
	ContractVersion    int     `json:"contract_version,omitempty"`
	DatabaseScope      string  `json:"database_scope,omitempty"`
	DatabasePath       string  `json:"database_path,omitempty"`
	ProjectID          string  `json:"project_id,omitempty"`
	ProjectName        string  `json:"project_name,omitempty"`
	ProjectCurrentPath string  `json:"project_current_path,omitempty"`
	Issues             []Issue `json:"issues"`
}

// IssueTreeNode is one node in a recursive issue tree.
type IssueTreeNode struct {
	ID       string          `json:"id"`
	Alias    string          `json:"alias,omitempty"`
	Kind     string          `json:"kind"`
	Title    string          `json:"title"`
	Status   string          `json:"status"`
	Children []IssueTreeNode `json:"children,omitempty"`
}

// IssueTreeResult is a recursive tree of issues.
type IssueTreeResult struct {
	ContractVersion    int             `json:"contract_version,omitempty"`
	DatabaseScope      string          `json:"database_scope,omitempty"`
	DatabasePath       string          `json:"database_path,omitempty"`
	ProjectID          string          `json:"project_id,omitempty"`
	ProjectName        string          `json:"project_name,omitempty"`
	ProjectCurrentPath string          `json:"project_current_path,omitempty"`
	Roots              []IssueTreeNode `json:"roots"`
}

// IssueFrontierResult is the derived pick-up-next view.
type IssueFrontierResult struct {
	ContractVersion    int                   `json:"contract_version,omitempty"`
	DatabaseScope      string                `json:"database_scope,omitempty"`
	DatabasePath       string                `json:"database_path,omitempty"`
	ProjectID          string                `json:"project_id,omitempty"`
	ProjectName        string                `json:"project_name,omitempty"`
	ProjectCurrentPath string                `json:"project_current_path,omitempty"`
	Issues             []IssueSummary        `json:"issues"`
	Refs               []WorkContractSummary `json:"refs,omitempty"`
}

// IssueCriterionExport is one criterion row in an issue export.
type IssueCriterionExport struct {
	ID       string `json:"id"`
	IssueID  string `json:"issue_id"`
	Position int    `json:"position"`
	Text     string `json:"text"`
	Command  string `json:"command,omitempty"`
	Expect   string `json:"expect,omitempty"`
	Tier     string `json:"tier"`
}

// IssueRelationshipExport is one issue-touching relationship in an export.
type IssueRelationshipExport struct {
	ID               string `json:"id"`
	FromEntityKind   string `json:"from_entity_kind"`
	FromEntityID     string `json:"from_entity_id"`
	ToEntityKind     string `json:"to_entity_kind"`
	ToEntityID       string `json:"to_entity_id"`
	RelationshipType string `json:"relationship_type"`
	Reason           string `json:"reason,omitempty"`
}

// IssueExportIdentity is the stored issue_identity row for a project export.
// It is omitted when the project has no identity row; exports never materialize a default.
type IssueExportIdentity struct {
	Authority  string `json:"authority"`
	Prefix     string `json:"prefix"`
	NextNumber int    `json:"next_number"`
}

// IssueExportSnapshot is a project-scoped JSON backup of issues.
type IssueExportSnapshot struct {
	ContractVersion    int                       `json:"contract_version"`
	ExportKind         string                    `json:"export_kind"`
	Format             string                    `json:"format"`
	DatabaseScope      string                    `json:"database_scope"`
	ProjectID          string                    `json:"project_id"`
	ProjectName        string                    `json:"project_name"`
	ProjectCurrentPath string                    `json:"project_current_path"`
	DatabasePath       string                    `json:"database_path"`
	Identity           *IssueExportIdentity      `json:"identity,omitempty"`
	Issues             []Issue                   `json:"issues"`
	Criteria           []IssueCriterionExport    `json:"criteria"`
	Claims             []IssueCriterionClaim     `json:"claims"`
	Relationships      []IssueRelationshipExport `json:"relationships"`
}

// ListIssues returns project issues matching the filters. Archived issues are
// hidden unless Archived is set.
func ListIssues(ctx context.Context, root project.Root, resolver PathResolver, options IssueListOptions) (IssueListResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IssueListResult{}, err
	}
	defer store.Close()
	return store.ListIssues(ctx, root, options)
}

// ListIssues returns project issues from an open store.
func (s *Store) ListIssues(ctx context.Context, root project.Root, options IssueListOptions) (IssueListResult, error) {
	if err := validateIssueListOptions(options); err != nil {
		return IssueListResult{}, err
	}
	identity, err := s.issueContext(ctx, root)
	if err != nil {
		return IssueListResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IssueListResult{}, fmt.Errorf("begin issue list: %w", err)
	}
	defer tx.Rollback()

	ids, err := listIssueIDsTx(ctx, tx, identity.ID, options, "")
	if err != nil {
		return IssueListResult{}, err
	}
	issues := make([]Issue, 0, len(ids))
	for _, id := range ids {
		issue, err := loadIssueTx(ctx, tx, identity.ID, id)
		if err != nil {
			return IssueListResult{}, err
		}
		issues = append(issues, issue)
	}
	return IssueListResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Issues:             issues,
	}, nil
}

// ShowIssue returns one issue with parent, children, and advisory bucket.
func ShowIssue(ctx context.Context, root project.Root, resolver PathResolver, ref string) (IssueResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IssueResult{}, err
	}
	defer store.Close()
	return store.ShowIssue(ctx, root, ref)
}

// ShowIssue returns one issue from an open store.
func (s *Store) ShowIssue(ctx context.Context, root project.Root, ref string) (IssueResult, error) {
	identity, err := s.issueContext(ctx, root)
	if err != nil {
		return IssueResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IssueResult{}, fmt.Errorf("begin issue show: %w", err)
	}
	defer tx.Rollback()

	issueID, _, err := resolveIssueRefTx(ctx, tx, identity.ID, ref)
	if err != nil {
		return IssueResult{}, err
	}
	return loadIssueResultTx(ctx, tx, identity, issueID)
}

// IssueTree returns a recursive tree from ref, or the whole project when ref is empty.
func IssueTree(ctx context.Context, root project.Root, resolver PathResolver, ref string, archived bool) (IssueTreeResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IssueTreeResult{}, err
	}
	defer store.Close()
	return store.IssueTree(ctx, root, ref, archived)
}

// IssueTree returns a recursive tree from an open store.
func (s *Store) IssueTree(ctx context.Context, root project.Root, ref string, archived bool) (IssueTreeResult, error) {
	identity, err := s.issueContext(ctx, root)
	if err != nil {
		return IssueTreeResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IssueTreeResult{}, fmt.Errorf("begin issue tree: %w", err)
	}
	defer tx.Rollback()

	summaries, err := listIssueSummariesTx(ctx, tx, identity.ID, IssueListOptions{Archived: archived})
	if err != nil {
		return IssueTreeResult{}, err
	}
	rootIDs := make([]string, 0)
	if strings.TrimSpace(ref) != "" {
		issueID, _, err := resolveIssueRefTx(ctx, tx, identity.ID, ref)
		if err != nil {
			return IssueTreeResult{}, err
		}
		rootIDs = append(rootIDs, issueID)
	} else {
		for _, summary := range summaries {
			if summary.Parent == "" {
				rootIDs = append(rootIDs, summary.ID)
			}
		}
	}
	roots, err := buildIssueTree(summaries, rootIDs)
	if err != nil {
		return IssueTreeResult{}, err
	}
	return IssueTreeResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Roots:              roots,
	}, nil
}

// ListIssueFrontier returns non-archived triage/backlog/todo issues that are
// not blocked. The view is derived at read time and never stored.
func ListIssueFrontier(ctx context.Context, root project.Root, resolver PathResolver) (IssueFrontierResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IssueFrontierResult{}, err
	}
	defer store.Close()
	return store.ListIssueFrontier(ctx, root)
}

// ListIssueFrontier returns the derived pick-up-next view from an open store.
func (s *Store) ListIssueFrontier(ctx context.Context, root project.Root) (IssueFrontierResult, error) {
	identity, err := s.issueContext(ctx, root)
	if err != nil {
		return IssueFrontierResult{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT i.id, i.parent_id, i.kind, i.title, i.status,
  (SELECT a.alias FROM aliases a WHERE a.project_id = i.project_id AND a.entity_kind = ? AND a.entity_id = i.id ORDER BY a.namespace, a.alias LIMIT 1)
FROM issues AS i
WHERE i.project_id = ?
  AND i.archived_at IS NULL
  AND i.status IN (?, ?, ?)
  AND NOT EXISTS (
    SELECT 1 FROM relationships r
    JOIN issues blocker ON blocker.project_id = r.project_id AND blocker.id = r.from_entity_id
    WHERE r.project_id = i.project_id
      AND r.from_entity_kind = ?
      AND r.to_entity_kind = ?
      AND r.to_entity_id = i.id
      AND r.relationship_type = ?
      AND blocker.status NOT IN (?, ?, ?)
  )
  AND NOT EXISTS (
    SELECT 1 FROM relationships r
    JOIN issues blocker ON blocker.project_id = r.project_id AND blocker.id = r.to_entity_id
    WHERE r.project_id = i.project_id
      AND r.from_entity_kind = ?
      AND r.to_entity_kind = ?
      AND r.from_entity_id = i.id
      AND r.relationship_type = ?
      AND blocker.status NOT IN (?, ?, ?)
  )
ORDER BY i.created_at, i.id
`, issueEntityKind, identity.ID,
		IssueStatusTriage, IssueStatusBacklog, IssueStatusTodo,
		issueEntityKind, issueEntityKind, IssueRelationshipBlocks,
		IssueStatusDone, IssueStatusCancelled, IssueStatusDuplicate,
		issueEntityKind, issueEntityKind, IssueRelationshipBlockedBy,
		IssueStatusDone, IssueStatusCancelled, IssueStatusDuplicate)
	if err != nil {
		return IssueFrontierResult{}, fmt.Errorf("query issue frontier: %w", err)
	}
	defer rows.Close()

	issues := []IssueSummary{}
	for rows.Next() {
		var summary IssueSummary
		var parent, alias sql.NullString
		if err := rows.Scan(&summary.ID, &parent, &summary.Kind, &summary.Title, &summary.Status, &alias); err != nil {
			return IssueFrontierResult{}, fmt.Errorf("scan issue frontier: %w", err)
		}
		summary.Parent = parent.String
		summary.Alias = alias.String
		issues = append(issues, summary)
	}
	if err := rows.Err(); err != nil {
		return IssueFrontierResult{}, fmt.Errorf("iterate issue frontier: %w", err)
	}
	refs, err := s.listWorkContractFrontier(ctx, identity.ID)
	if err != nil {
		return IssueFrontierResult{}, err
	}
	return IssueFrontierResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Issues:             issues,
		Refs:               refs,
	}, nil
}

// ExportIssues returns a project-scoped JSON backup of issues, criteria,
// criterion claims, issue-touching relationships, and the stored issue
// identity when one exists.
func ExportIssues(ctx context.Context, root project.Root, resolver PathResolver) (IssueExportSnapshot, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IssueExportSnapshot{}, err
	}
	defer store.Close()
	return store.ExportIssues(ctx, root)
}

// ExportIssues returns a project-scoped issue backup from an open store.
func (s *Store) ExportIssues(ctx context.Context, root project.Root) (IssueExportSnapshot, error) {
	listed, err := s.ListIssues(ctx, root, IssueListOptions{Archived: true})
	if err != nil {
		return IssueExportSnapshot{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IssueExportSnapshot{}, fmt.Errorf("begin issue export: %w", err)
	}
	defer tx.Rollback()

	criteria, err := exportIssueCriteriaTx(ctx, tx, listed.ProjectID)
	if err != nil {
		return IssueExportSnapshot{}, err
	}
	claims, err := exportIssueCriterionClaimsTx(ctx, tx, listed.ProjectID)
	if err != nil {
		return IssueExportSnapshot{}, err
	}
	relationships, err := exportIssueRelationshipsTx(ctx, tx, listed.ProjectID)
	if err != nil {
		return IssueExportSnapshot{}, err
	}
	identity, err := lookupStoredIssueIdentityTx(ctx, tx, listed.ProjectID)
	if err != nil {
		return IssueExportSnapshot{}, err
	}
	return IssueExportSnapshot{
		ContractVersion:    StateJSONContractVersion,
		ExportKind:         ExportKindIssue,
		Format:             ExportFormatJSON,
		DatabaseScope:      listed.DatabaseScope,
		ProjectID:          listed.ProjectID,
		ProjectName:        listed.ProjectName,
		ProjectCurrentPath: listed.ProjectCurrentPath,
		DatabasePath:       listed.DatabasePath,
		Identity:           identity,
		Issues:             listed.Issues,
		Criteria:           criteria,
		Claims:             claims,
		Relationships:      relationships,
	}, nil
}

func (s *Store) issueContext(ctx context.Context, root project.Root) (ProjectIdentity, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ProjectIdentity{}, err
	}
	return s.projectIdentity(ctx, projectID)
}

func validateIssueListOptions(options IssueListOptions) error {
	if status := strings.TrimSpace(options.Status); status != "" && !validIssueStatus(status) {
		return &IssueValidationError{Field: "status", Err: fmt.Errorf("must be one of triage, backlog, todo, active, done, cancelled, duplicate")}
	}
	if kind := strings.TrimSpace(options.Kind); kind != "" && !validIssueKind(kind) {
		return &IssueValidationError{Field: "kind", Err: fmt.Errorf("must be delivery or decision")}
	}
	return nil
}

func listIssueIDsTx(ctx context.Context, tx *sql.Tx, projectID string, options IssueListOptions, parentID string) ([]string, error) {
	query := `
SELECT id FROM issues
WHERE project_id = ?`
	args := []any{projectID}
	if parentID != "" {
		query += `
  AND parent_id = ?`
		args = append(args, parentID)
	}
	if status := strings.TrimSpace(options.Status); status != "" {
		query += `
  AND status = ?`
		args = append(args, status)
	}
	if kind := strings.TrimSpace(options.Kind); kind != "" {
		query += `
  AND kind = ?`
		args = append(args, kind)
	}
	if options.Started {
		query += `
  AND started_worktree IS NOT NULL AND trim(started_worktree) != ''`
	} else if !options.Archived {
		query += `
  AND archived_at IS NULL`
	}
	query += `
ORDER BY created_at, id`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list issue ids: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan issue id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue ids: %w", err)
	}
	return ids, nil
}

func listIssueSummariesTx(ctx context.Context, tx *sql.Tx, projectID string, options IssueListOptions) ([]IssueSummary, error) {
	query := `
SELECT i.id, i.parent_id, i.kind, i.title, i.status,
  (SELECT a.alias FROM aliases a WHERE a.project_id = i.project_id AND a.entity_kind = ? AND a.entity_id = i.id ORDER BY a.namespace, a.alias LIMIT 1)
FROM issues AS i
WHERE i.project_id = ?`
	args := []any{issueEntityKind, projectID}
	if !options.Archived {
		query += `
  AND i.archived_at IS NULL`
	}
	query += `
ORDER BY i.created_at, i.id`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list issue summaries: %w", err)
	}
	defer rows.Close()
	summaries := []IssueSummary{}
	for rows.Next() {
		var summary IssueSummary
		var parent, alias sql.NullString
		if err := rows.Scan(&summary.ID, &parent, &summary.Kind, &summary.Title, &summary.Status, &alias); err != nil {
			return nil, fmt.Errorf("scan issue summary: %w", err)
		}
		summary.Parent = parent.String
		summary.Alias = alias.String
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue summaries: %w", err)
	}
	return summaries, nil
}

func loadIssueResultTx(ctx context.Context, tx *sql.Tx, identity ProjectIdentity, issueID string) (IssueResult, error) {
	issue, err := loadIssueTx(ctx, tx, identity.ID, issueID)
	if err != nil {
		return IssueResult{}, err
	}
	var parent *IssueSummary
	if issue.ParentID != "" {
		summary, err := loadIssueSummaryTx(ctx, tx, identity.ID, issue.ParentID)
		if err != nil {
			return IssueResult{}, err
		}
		parent = &summary
	}
	children, err := listIssueChildrenTx(ctx, tx, identity.ID, issue.ID)
	if err != nil {
		return IssueResult{}, err
	}
	bucket, err := loadIssueBucketTx(ctx, tx, identity.ID, issue.ID)
	if err != nil {
		return IssueResult{}, err
	}
	return IssueResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Issue:              issue,
		Parent:             parent,
		Children:           children,
		Bucket:             bucket,
	}, nil
}

func loadIssueSummaryTx(ctx context.Context, tx *sql.Tx, projectID, issueID string) (IssueSummary, error) {
	var summary IssueSummary
	var parent, alias sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT i.id, i.parent_id, i.kind, i.title, i.status,
  (SELECT a.alias FROM aliases a WHERE a.project_id = i.project_id AND a.entity_kind = ? AND a.entity_id = i.id ORDER BY a.namespace, a.alias LIMIT 1)
FROM issues AS i
WHERE i.project_id = ? AND i.id = ?
`, issueEntityKind, projectID, issueID).Scan(&summary.ID, &parent, &summary.Kind, &summary.Title, &summary.Status, &alias)
	if err != nil {
		return IssueSummary{}, fmt.Errorf("load issue summary %s: %w", issueID, err)
	}
	summary.Parent = parent.String
	summary.Alias = alias.String
	return summary, nil
}

func listIssueChildrenTx(ctx context.Context, tx *sql.Tx, projectID, parentID string) ([]IssueSummary, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT i.id, i.parent_id, i.kind, i.title, i.status,
  (SELECT a.alias FROM aliases a WHERE a.project_id = i.project_id AND a.entity_kind = ? AND a.entity_id = i.id ORDER BY a.namespace, a.alias LIMIT 1)
FROM issues AS i
WHERE i.project_id = ? AND i.parent_id = ?
ORDER BY i.created_at, i.id
`, issueEntityKind, projectID, parentID)
	if err != nil {
		return nil, fmt.Errorf("list issue children: %w", err)
	}
	defer rows.Close()
	children := []IssueSummary{}
	for rows.Next() {
		var summary IssueSummary
		var parent, alias sql.NullString
		if err := rows.Scan(&summary.ID, &parent, &summary.Kind, &summary.Title, &summary.Status, &alias); err != nil {
			return nil, fmt.Errorf("scan issue child: %w", err)
		}
		summary.Parent = parent.String
		summary.Alias = alias.String
		children = append(children, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue children: %w", err)
	}
	return children, nil
}

func buildIssueTree(summaries []IssueSummary, rootIDs []string) ([]IssueTreeNode, error) {
	byParent := map[string][]IssueSummary{}
	byID := map[string]IssueSummary{}
	for _, summary := range summaries {
		byID[summary.ID] = summary
		byParent[summary.Parent] = append(byParent[summary.Parent], summary)
	}
	visited := map[string]bool{}
	var walk func(id string) (IssueTreeNode, error)
	walk = func(id string) (IssueTreeNode, error) {
		if visited[id] {
			return IssueTreeNode{}, fmt.Errorf("parent cycle detected in stored issue data at %s", id)
		}
		visited[id] = true
		summary := byID[id]
		node := IssueTreeNode{
			ID:     summary.ID,
			Alias:  summary.Alias,
			Kind:   summary.Kind,
			Title:  summary.Title,
			Status: summary.Status,
		}
		for _, child := range byParent[id] {
			childNode, err := walk(child.ID)
			if err != nil {
				return IssueTreeNode{}, err
			}
			node.Children = append(node.Children, childNode)
		}
		return node, nil
	}
	roots := make([]IssueTreeNode, 0, len(rootIDs))
	for _, id := range rootIDs {
		if _, ok := byID[id]; !ok {
			continue
		}
		node, err := walk(id)
		if err != nil {
			return nil, err
		}
		roots = append(roots, node)
	}
	return roots, nil
}

// lookupStoredIssueIdentityTx reads the project's issue_identity row without
// inserting a default. A missing row is represented as nil.
func lookupStoredIssueIdentityTx(ctx context.Context, tx *sql.Tx, projectID string) (*IssueExportIdentity, error) {
	var identity IssueExportIdentity
	err := tx.QueryRowContext(ctx, `
SELECT authority, prefix, next_number
FROM issue_identity WHERE project_id = ?
`, projectID).Scan(&identity.Authority, &identity.Prefix, &identity.NextNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load stored issue identity: %w", err)
	}
	return &identity, nil
}

func exportIssueCriterionClaimsTx(ctx context.Context, tx *sql.Tx, projectID string) ([]IssueCriterionClaim, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, child_criterion_id, parent_criterion_id
FROM issue_criterion_claims
WHERE project_id = ?
ORDER BY id
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("export issue criterion claims: %w", err)
	}
	defer rows.Close()
	claims := []IssueCriterionClaim{}
	for rows.Next() {
		var claim IssueCriterionClaim
		if err := rows.Scan(&claim.ID, &claim.ChildCriterionID, &claim.ParentCriterionID); err != nil {
			return nil, fmt.Errorf("scan exported issue criterion claim: %w", err)
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exported issue criterion claims: %w", err)
	}
	return claims, nil
}

func exportIssueCriteriaTx(ctx context.Context, tx *sql.Tx, projectID string) ([]IssueCriterionExport, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, issue_id, position, text, COALESCE(command, ''), COALESCE(expect, ''), tier
FROM issue_criteria
WHERE project_id = ?
ORDER BY issue_id, position, id
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("export issue criteria: %w", err)
	}
	defer rows.Close()
	criteria := []IssueCriterionExport{}
	for rows.Next() {
		var criterion IssueCriterionExport
		if err := rows.Scan(&criterion.ID, &criterion.IssueID, &criterion.Position, &criterion.Text, &criterion.Command, &criterion.Expect, &criterion.Tier); err != nil {
			return nil, fmt.Errorf("scan exported issue criterion: %w", err)
		}
		criteria = append(criteria, criterion)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exported issue criteria: %w", err)
	}
	return criteria, nil
}

func exportIssueRelationshipsTx(ctx context.Context, tx *sql.Tx, projectID string) ([]IssueRelationshipExport, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, COALESCE(reason, '')
FROM relationships
WHERE project_id = ?
  AND (from_entity_kind = ? OR to_entity_kind = ?)
ORDER BY id
`, projectID, issueEntityKind, issueEntityKind)
	if err != nil {
		return nil, fmt.Errorf("export issue relationships: %w", err)
	}
	defer rows.Close()
	relationships := []IssueRelationshipExport{}
	for rows.Next() {
		var relationship IssueRelationshipExport
		if err := rows.Scan(&relationship.ID, &relationship.FromEntityKind, &relationship.FromEntityID, &relationship.ToEntityKind, &relationship.ToEntityID, &relationship.RelationshipType, &relationship.Reason); err != nil {
			return nil, fmt.Errorf("scan exported issue relationship: %w", err)
		}
		relationships = append(relationships, relationship)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exported issue relationships: %w", err)
	}
	return relationships, nil
}

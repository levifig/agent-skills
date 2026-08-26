package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// Linear adapter config lives in backend_mappings when the table can hold it
// honestly, with env-var fallbacks for the network token and one-off overrides:
//
//	backend=linear entity_kind=project entity_id=<project_id>
//	  external_kind=team           external_id=<TEAM_KEY>
//	  external_kind=status:<type>  external_id=<Linear state name>
//
// Env fallbacks (no new table):
//
//	LINEAR_API_URL          injectable GraphQL endpoint (tests: httptest)
//	.agents/loaf.json issue.prefix when issue.authority is linear
//	LINEAR_TEAM_KEY         team key when no mapping row or loaf.json prefix exists
//	LINEAR_STATUS_<TYPE>    optional name override (TRIAGE, BACKLOG, TODO, ACTIVE, DONE, CANCELLED, DUPLICATE)
//
// The Linear bearer env var is required for network calls and is never stored.

// LinearAdapterConfig is the per-project adapter settings.
type LinearAdapterConfig struct {
	TeamKey         string
	StatusOverrides map[string]string
}

// LinearMintError is returned when identity cannot be delegated to Linear.
// The CLI must not fall back to a local alias.
type LinearMintError struct {
	Err error
}

func (e *LinearMintError) Error() string {
	if e == nil || e.Err == nil {
		return linearMintOfflineMessage("")
	}
	return linearMintOfflineMessage(e.Err.Error())
}

func (e *LinearMintError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// LinearOrphanError is returned when Linear created an issue but the local
// bind failed. The minted key is preserved so the operator can adopt it.
type LinearOrphanError struct {
	Identifier string
	URL        string
	Err        error
}

func (e *LinearOrphanError) Error() string {
	if e == nil {
		return "Linear issue was created but not bound locally"
	}
	key := strings.TrimSpace(e.Identifier)
	if key == "" {
		key = "unknown"
	}
	msg := fmt.Sprintf("Linear issue %s was created but not bound locally; run loaf issue pull %s to adopt it", key, key)
	if e.Err == nil {
		return msg
	}
	return e.Err.Error() + "\n" + msg
}

func (e *LinearOrphanError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// LinearReleaseUnsupportedSkip is the silent degradation when the workspace
// does not expose Linear Releases.
const LinearReleaseUnsupportedSkip = "workspace does not expose Linear Releases"

func linearMintOfflineMessage(detail string) string {
	base := "cannot mint a Linear issue identifier while the tracker is offline; capture the work via `loaf spark` or `loaf idea` instead; loaf issue new will not mint a local alias"
	if strings.TrimSpace(detail) == "" {
		return base
	}
	return detail + "\n" + base
}

// LinearPushResult is the outcome of writing Loaf-owned shaping to Linear.
type LinearPushResult struct {
	Issue            Issue       `json:"issue"`
	Linear           LinearIssue `json:"linear"`
	DescriptionWrote bool        `json:"description_wrote"`
	StatusWrote      bool        `json:"status_wrote"`
	StatusSkipped    string      `json:"status_skipped,omitempty"`
}

// LinearReconcileConflict is one field where local and tracker disagree.
type LinearReconcileConflict struct {
	Field      string `json:"field"`
	Local      string `json:"local,omitempty"`
	Tracker    string `json:"tracker,omitempty"`
	LocalAt    string `json:"local_at,omitempty"`
	TrackerAt  string `json:"tracker_at,omitempty"`
	Mover      string `json:"mover,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	ReportOnly bool   `json:"report_only,omitempty"`
	Unresolved bool   `json:"unresolved,omitempty"`
}

// LinearReconcileResult is the comparison of one (or many) mapped issues.
type LinearReconcileResult struct {
	Issue     Issue                     `json:"issue"`
	Linear    LinearIssue               `json:"linear"`
	Conflicts []LinearReconcileConflict `json:"conflicts,omitempty"`
	InSync    bool                      `json:"in_sync"`
}

// LinearPullResult is the adopted issue plus any --tree descendants.
type LinearPullResult struct {
	Issue Issue   `json:"issue"`
	Tree  []Issue `json:"tree,omitempty"`
}

// LinearReleasePushResult is the Linear Release created or updated on cut.
type LinearReleasePushResult struct {
	Supported bool          `json:"supported"`
	Release   LinearRelease `json:"release,omitempty"`
	Skipped   string        `json:"skipped,omitempty"`
	Unmapped  []string      `json:"unmapped,omitempty"`
}

var linearTypeByStatus = map[string]string{
	IssueStatusTriage:    "triage",
	IssueStatusBacklog:   "backlog",
	IssueStatusTodo:      "unstarted",
	IssueStatusActive:    "started",
	IssueStatusDone:      "completed",
	IssueStatusCancelled: "canceled",
	IssueStatusDuplicate: "canceled",
}

var linearStatusByType = map[string]string{
	"triage":    IssueStatusTriage,
	"backlog":   IssueStatusBacklog,
	"unstarted": IssueStatusTodo,
	"started":   IssueStatusActive,
	"completed": IssueStatusDone,
	"canceled":  IssueStatusCancelled,
	"cancelled": IssueStatusCancelled,
}

func MapLinearStateType(typeName string) string {
	if status, ok := linearStatusByType[strings.ToLower(strings.TrimSpace(typeName))]; ok {
		return status
	}
	return IssueStatusTriage
}

func linearStatusOverrideEnv(status string) string {
	return fmt.Sprintf(linearStatusOverrideEnvFmt, strings.ToUpper(status))
}

// WriteLinearTeamConfig stores the Linear team key in backend_mappings.
func WriteLinearTeamConfig(ctx context.Context, root project.Root, resolver PathResolver, teamKey string) error {
	teamKey = strings.TrimSpace(teamKey)
	if teamKey == "" {
		return fmt.Errorf("linear team key must be nonempty")
	}
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return err
	}
	return store.upsertBackendMapping(ctx, root, backendMapping{
		EntityKind:   "project",
		EntityID:     projectID,
		ExternalKind: linearExternalKindTeam,
		ExternalID:   teamKey,
		SyncStatus:   linearSyncLinked,
	})
}

// LoadLinearAdapterConfig reads team key and status-name overrides.
func LoadLinearAdapterConfig(ctx context.Context, root project.Root, resolver PathResolver) (LinearAdapterConfig, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return LinearAdapterConfig{}, err
	}
	defer store.Close()
	return store.LoadLinearAdapterConfig(ctx, root)
}

// LoadLinearAdapterConfig reads adapter config from an open store.
func (s *Store) LoadLinearAdapterConfig(ctx context.Context, root project.Root) (LinearAdapterConfig, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return LinearAdapterConfig{}, err
	}
	cfg := LinearAdapterConfig{StatusOverrides: map[string]string{}}
	rows, err := s.db.QueryContext(ctx, `
SELECT external_kind, external_id
FROM backend_mappings
WHERE project_id = ? AND backend = ? AND entity_kind = 'project' AND entity_id = ?
`, projectID, linearBackend, projectID)
	if err != nil {
		return LinearAdapterConfig{}, fmt.Errorf("load linear adapter config: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind, id string
		if err := rows.Scan(&kind, &id); err != nil {
			return LinearAdapterConfig{}, fmt.Errorf("scan linear adapter config: %w", err)
		}
		kind = strings.TrimSpace(kind)
		id = strings.TrimSpace(id)
		switch {
		case kind == linearExternalKindTeam && id != "":
			cfg.TeamKey = id
		case strings.HasPrefix(kind, linearExternalKindStatus) && id != "":
			status := strings.TrimPrefix(kind, linearExternalKindStatus)
			if validIssueStatus(status) {
				cfg.StatusOverrides[status] = id
			}
		}
	}
	if err := rows.Err(); err != nil {
		return LinearAdapterConfig{}, fmt.Errorf("iterate linear adapter config: %w", err)
	}
	if cfg.TeamKey == "" {
		if issueCfg, err := LoadIssueProjectConfig(root.Path()); err == nil && issueCfg.Authority == IssueAuthorityLinear && issueCfg.Prefix != "" {
			cfg.TeamKey = issueCfg.Prefix
		}
	}
	if envTeam := strings.TrimSpace(os.Getenv(LinearEnvTeamKey)); envTeam != "" && cfg.TeamKey == "" {
		cfg.TeamKey = envTeam
	}
	for _, status := range issueStatuses {
		if envName := strings.TrimSpace(os.Getenv(linearStatusOverrideEnv(status))); envName != "" {
			if _, exists := cfg.StatusOverrides[status]; !exists {
				cfg.StatusOverrides[status] = envName
			}
		}
	}
	return cfg, nil
}

func resolveLinearStateID(team LinearTeam, status string, overrides map[string]string) (string, error) {
	if name := strings.TrimSpace(overrides[status]); name != "" {
		for _, state := range team.States {
			if strings.EqualFold(state.Name, name) {
				return state.ID, nil
			}
		}
		return "", fmt.Errorf("linear workflow has no state named %q for status %s", name, status)
	}
	wantType := linearTypeByStatus[status]
	if wantType == "" {
		return "", fmt.Errorf("no linear state type for status %s", status)
	}
	for _, state := range team.States {
		if strings.EqualFold(state.Type, wantType) {
			return state.ID, nil
		}
	}
	return "", fmt.Errorf("linear team has no %s-type state for status %s", wantType, status)
}

type backendMapping struct {
	EntityKind   string
	EntityID     string
	ExternalKind string
	ExternalID   string
	ExternalURL  string
	SyncStatus   string
	UpdatedAt    string
}

func (s *Store) upsertBackendMapping(ctx context.Context, root project.Root, mapping backendMapping) error {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin backend mapping: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := upsertBackendMappingTx(ctx, tx, projectID, mapping, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit backend mapping: %w", err)
	}
	return nil
}

func isLinearConfigMapping(mapping backendMapping) bool {
	if mapping.EntityKind != "project" {
		return false
	}
	return mapping.ExternalKind == linearExternalKindTeam || strings.HasPrefix(mapping.ExternalKind, linearExternalKindStatus)
}

func upsertBackendMappingTx(ctx context.Context, tx *sql.Tx, projectID string, mapping backendMapping, now string) error {
	mapping.EntityKind = strings.TrimSpace(mapping.EntityKind)
	mapping.EntityID = strings.TrimSpace(mapping.EntityID)
	mapping.ExternalKind = strings.TrimSpace(mapping.ExternalKind)
	mapping.ExternalID = strings.TrimSpace(mapping.ExternalID)
	mapping.SyncStatus = strings.TrimSpace(mapping.SyncStatus)
	if mapping.SyncStatus == "" {
		mapping.SyncStatus = linearSyncLinked
	}
	if isLinearConfigMapping(mapping) {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM backend_mappings
WHERE project_id = ? AND backend = ? AND entity_kind = ? AND entity_id = ? AND external_kind = ?
`, projectID, linearBackend, mapping.EntityKind, mapping.EntityID, mapping.ExternalKind); err != nil {
			return fmt.Errorf("replace linear config mapping: %w", err)
		}
	} else {
		var existingID string
		err := tx.QueryRowContext(ctx, `
SELECT id FROM backend_mappings
WHERE project_id = ? AND backend = ? AND external_kind = ? AND external_id = ?
`, projectID, linearBackend, mapping.ExternalKind, mapping.ExternalID).Scan(&existingID)
		switch {
		case err == nil:
			if _, err := tx.ExecContext(ctx, `
UPDATE backend_mappings
SET entity_kind = ?, entity_id = ?, external_url = ?, sync_status = ?, updated_at = ?
WHERE id = ?
`, mapping.EntityKind, mapping.EntityID, emptyToNil(mapping.ExternalURL), mapping.SyncStatus, now, existingID); err != nil {
				return fmt.Errorf("update backend mapping: %w", err)
			}
			return appendBackendMappingFactTx(ctx, tx, projectID, existingID, mapping, now)
		case err != sql.ErrNoRows:
			return fmt.Errorf("lookup backend mapping: %w", err)
		}
	}
	id, err := newOpaqueStateID("bmap")
	if err != nil {
		return fmt.Errorf("mint backend mapping id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, projectID, linearBackend, mapping.EntityKind, mapping.EntityID, mapping.ExternalKind, mapping.ExternalID, emptyToNil(mapping.ExternalURL), mapping.SyncStatus, now, now); err != nil {
		return fmt.Errorf("insert backend mapping: %w", err)
	}
	return appendBackendMappingFactTx(ctx, tx, projectID, id, mapping, now)
}

func appendBackendMappingFactTx(ctx context.Context, tx *sql.Tx, projectID, mappingID string, mapping backendMapping, now string) error {
	_, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindRefRegistered, "", CoreEventPayload{
		SubjectKind:  "ref",
		SubjectID:    mappingID,
		Backend:      linearBackend,
		EntityKind:   mapping.EntityKind,
		EntityID:     mapping.EntityID,
		ExternalKind: mapping.ExternalKind,
		ExternalID:   mapping.ExternalID,
		ExternalURL:  mapping.ExternalURL,
		SyncStatus:   mapping.SyncStatus,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, parseCoreEventTime(now), "")
	return err
}

func lookupLinearIssueMappingTx(ctx context.Context, tx *sql.Tx, projectID, entityID, externalID string) (backendMapping, error) {
	row := tx.QueryRowContext(ctx, `
SELECT entity_kind, entity_id, external_kind, external_id, COALESCE(external_url, ''), sync_status, updated_at
FROM backend_mappings
WHERE project_id = ? AND backend = ? AND entity_kind = ? AND external_kind = ?
  AND (entity_id = ? OR external_id = ?)
ORDER BY CASE WHEN external_id = ? THEN 0 ELSE 1 END, updated_at DESC, entity_id
LIMIT 1
`, projectID, linearBackend, issueEntityKind, linearExternalKindIssue, entityID, externalID, externalID)
	var mapping backendMapping
	if err := row.Scan(&mapping.EntityKind, &mapping.EntityID, &mapping.ExternalKind, &mapping.ExternalID, &mapping.ExternalURL, &mapping.SyncStatus, &mapping.UpdatedAt); err != nil {
		return backendMapping{}, err
	}
	return mapping, nil
}

func listLinearIssueMappingsTx(ctx context.Context, tx *sql.Tx, projectID string) ([]backendMapping, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT entity_kind, entity_id, external_kind, external_id, COALESCE(external_url, ''), sync_status, updated_at
FROM backend_mappings
WHERE project_id = ? AND backend = ? AND entity_kind = ? AND external_kind = ?
ORDER BY external_id
`, projectID, linearBackend, issueEntityKind, linearExternalKindIssue)
	if err != nil {
		return nil, fmt.Errorf("list linear issue mappings: %w", err)
	}
	defer rows.Close()
	var mappings []backendMapping
	for rows.Next() {
		var mapping backendMapping
		if err := rows.Scan(&mapping.EntityKind, &mapping.EntityID, &mapping.ExternalKind, &mapping.ExternalID, &mapping.ExternalURL, &mapping.SyncStatus, &mapping.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan linear issue mapping: %w", err)
		}
		mappings = append(mappings, mapping)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linear issue mappings: %w", err)
	}
	return mappings, nil
}

func latestIssueStatusChangedAtTx(ctx context.Context, tx *sql.Tx, projectID, issueID string) (time.Time, error) {
	var raw string
	err := tx.QueryRowContext(ctx, `
SELECT created_at FROM events
WHERE project_id = ? AND entity_kind = ? AND entity_id = ? AND event_type = 'status_changed'
ORDER BY created_at DESC, id DESC
LIMIT 1
`, projectID, issueEntityKind, issueID).Scan(&raw)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("latest issue status event: %w", err)
	}
	return parseLinearTime(raw)
}

func parseComparableTime(value string) time.Time {
	parsed, err := parseLinearTime(value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func applyIssueStatus(ctx context.Context, store *Store, root project.Root, ref, status string) (Issue, error) {
	status = strings.TrimSpace(status)
	switch status {
	case "", IssueStatusTriage:
		return store.GetIssue(ctx, root, ref)
	case IssueStatusCancelled, IssueStatusDuplicate:
		return store.RemoveIssue(ctx, root, IssueRemoveOptions{Ref: ref, Status: IssueStatusCancelled})
	default:
		return store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: ref, Status: status, SetStatus: true})
	}
}

// MintLinearIssue creates the Linear issue for a linear-authority project and
// returns the minted identifier. It does not write local state.
func MintLinearIssue(ctx context.Context, root project.Root, resolver PathResolver, client *LinearClient, options IssueCreateOptions) (LinearIssue, error) {
	if client == nil {
		return LinearIssue{}, &LinearMintError{Err: fmt.Errorf("linear client is not configured")}
	}
	cfg, err := LoadLinearAdapterConfig(ctx, root, resolver)
	if err != nil {
		return LinearIssue{}, &LinearMintError{Err: err}
	}
	if strings.TrimSpace(cfg.TeamKey) == "" {
		return LinearIssue{}, &LinearMintError{Err: fmt.Errorf("linear team key is not configured; set issue.prefix in .agents/loaf.json, %s, or a backend_mappings row (backend=linear, external_kind=team)", LinearEnvTeamKey)}
	}
	team, err := client.TeamByKey(ctx, cfg.TeamKey)
	if err != nil {
		return LinearIssue{}, &LinearMintError{Err: err}
	}
	input := LinearCreateIssueInput{TeamID: team.ID, Title: options.Title, Description: options.Body}
	if strings.TrimSpace(options.Parent) != "" {
		store, err := openProjectStoreReadExisting(ctx, root, resolver)
		if err != nil {
			return LinearIssue{}, &LinearMintError{Err: err}
		}
		parent, err := store.GetIssue(ctx, root, options.Parent)
		store.Close()
		if err != nil {
			return LinearIssue{}, &LinearMintError{Err: err}
		}
		parentKey := parent.Alias
		if parentKey == "" {
			parentKey = parent.ID
		}
		remote, err := client.Issue(ctx, parentKey)
		if err != nil {
			return LinearIssue{}, &LinearMintError{Err: fmt.Errorf("resolve linear parent %s: %w", parentKey, err)}
		}
		input.ParentID = remote.ID
	}
	created, err := client.CreateIssue(ctx, input)
	if err != nil {
		return LinearIssue{}, &LinearMintError{Err: err}
	}
	return created, nil
}

// BindLinearIssue records the Linear key as the issue alias and mapping row.
// next_number is not touched.
func BindLinearIssue(ctx context.Context, root project.Root, resolver PathResolver, issueID, identifier, url string) error {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.BindLinearIssue(ctx, root, issueID, identifier, url)
}

// BindLinearIssue records the Linear key on an open store.
func (s *Store) BindLinearIssue(ctx context.Context, root project.Root, issueID, identifier, url string) error {
	identifier = strings.TrimSpace(identifier)
	issueID = strings.TrimSpace(issueID)
	if identifier == "" || issueID == "" {
		return fmt.Errorf("bind linear issue requires a local id and linear key")
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin bind linear issue: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var existingAlias string
	err = tx.QueryRowContext(ctx, `
SELECT alias FROM aliases WHERE project_id = ? AND entity_kind = ? AND entity_id = ? AND namespace = ?
`, projectID, issueEntityKind, issueID, issueNamespace).Scan(&existingAlias)
	switch {
	case err == nil:
		if existingAlias != identifier {
			return fmt.Errorf("issue %s already has alias %s", issueID, existingAlias)
		}
	case err == sql.ErrNoRows:
		if err := insertAlias(ctx, tx, projectID, issueEntityKind, issueID, issueNamespace, identifier, now); err != nil {
			return err
		}
	default:
		return fmt.Errorf("lookup issue alias: %w", err)
	}
	if err := upsertBackendMappingTx(ctx, tx, projectID, backendMapping{
		EntityKind:   issueEntityKind,
		EntityID:     issueID,
		ExternalKind: linearExternalKindIssue,
		ExternalID:   identifier,
		ExternalURL:  url,
		SyncStatus:   linearSyncLinked,
	}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit bind linear issue: %w", err)
	}
	return nil
}

// PullLinearIssue adopts a Linear issue, and with tree its descendants.
func PullLinearIssue(ctx context.Context, root project.Root, resolver PathResolver, client *LinearClient, identifier string, tree bool) (LinearPullResult, error) {
	if client == nil {
		return LinearPullResult{}, fmt.Errorf("linear client is not configured")
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return LinearPullResult{}, fmt.Errorf("issue pull requires a Linear key")
	}
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return LinearPullResult{}, err
	}
	defer store.Close()
	adopted, err := store.adoptLinearIssue(ctx, root, client, identifier, "")
	if err != nil {
		return LinearPullResult{}, err
	}
	result := LinearPullResult{Issue: adopted, Tree: []Issue{adopted}}
	if !tree {
		return result, nil
	}
	if err := store.pullLinearChildren(ctx, root, client, identifier, &result.Tree); err != nil {
		return LinearPullResult{}, err
	}
	return result, nil
}

func (s *Store) pullLinearChildren(ctx context.Context, root project.Root, client *LinearClient, parentKey string, collected *[]Issue) error {
	remote, err := client.Issue(ctx, parentKey)
	if err != nil {
		return err
	}
	for _, childKey := range remote.ChildKeys {
		child, err := s.adoptLinearIssue(ctx, root, client, childKey, parentKey)
		if err != nil {
			return err
		}
		*collected = append(*collected, child)
		if err := s.pullLinearChildren(ctx, root, client, childKey, collected); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) adoptLinearIssue(ctx context.Context, root project.Root, client *LinearClient, identifier, parentKey string) (Issue, error) {
	remote, err := client.Issue(ctx, identifier)
	if err != nil {
		return Issue{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Issue{}, fmt.Errorf("begin adopt lookup: %w", err)
	}
	existing, lookupErr := lookupLinearIssueMappingTx(ctx, tx, projectID, "", remote.Identifier)
	_ = tx.Rollback()
	if lookupErr == nil {
		issue, err := s.GetIssue(ctx, root, existing.EntityID)
		if err != nil {
			return Issue{}, err
		}
		return s.refreshAdoptedLinearIssue(ctx, root, issue, remote, parentKey)
	}
	if lookupErr != sql.ErrNoRows {
		return Issue{}, lookupErr
	}

	parent, err := s.resolveLinearParentID(ctx, projectID, parentKey, remote.ParentKey)
	if err != nil {
		return Issue{}, err
	}

	created, err := s.CreateIssue(ctx, root, IssueCreateOptions{
		Title:  remote.Title,
		Body:   remote.Description,
		Parent: parent,
		Alias:  remote.Identifier,
	})
	if err != nil {
		existingLocal, getErr := s.GetIssue(ctx, root, remote.Identifier)
		if getErr != nil {
			return Issue{}, err
		}
		if bindErr := s.BindLinearIssue(ctx, root, existingLocal.ID, remote.Identifier, remote.URL); bindErr != nil {
			return Issue{}, bindErr
		}
		return s.refreshAdoptedLinearIssue(ctx, root, existingLocal, remote, parentKey)
	}
	if err := s.BindLinearIssue(ctx, root, created.ID, remote.Identifier, remote.URL); err != nil {
		return Issue{}, err
	}
	status := MapLinearStateType(remote.State.Type)
	if status != IssueStatusTriage {
		created, err = applyIssueStatus(ctx, s, root, created.ID, status)
		if err != nil {
			return Issue{}, err
		}
	}
	return created, nil
}

func (s *Store) resolveLinearParentID(ctx context.Context, projectID, parentKey, remoteParentKey string) (string, error) {
	parent := parentKey
	if parent == "" {
		parent = remoteParentKey
	}
	if parent == "" {
		return "", nil
	}
	parentTx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", fmt.Errorf("begin parent lookup: %w", err)
	}
	parentMapping, parentErr := lookupLinearIssueMappingTx(ctx, parentTx, projectID, "", parent)
	_ = parentTx.Rollback()
	if parentErr == nil {
		return parentMapping.EntityID, nil
	}
	if parentErr != sql.ErrNoRows {
		return "", parentErr
	}
	// Parent is not local yet; adopt without the edge. --tree walks
	// parents first so this is only the single-issue pull case.
	return "", nil
}

func (s *Store) refreshAdoptedLinearIssue(ctx context.Context, root project.Root, issue Issue, remote LinearIssue, parentKey string) (Issue, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, err
	}
	parentID, err := s.resolveLinearParentID(ctx, projectID, parentKey, remote.ParentKey)
	if err != nil {
		return Issue{}, err
	}
	if parentID != issue.ParentID {
		updated, err := s.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: issue.ID, Parent: parentID, SetParent: true})
		if err != nil {
			return Issue{}, err
		}
		issue = updated
	}
	if issue.Alias != remote.Identifier {
		if err := s.replaceLinearIssueAlias(ctx, root, issue.ID, remote.Identifier, remote.URL); err != nil {
			return Issue{}, err
		}
		refreshed, err := s.GetIssue(ctx, root, issue.ID)
		if err != nil {
			return Issue{}, err
		}
		issue = refreshed
	}
	status := MapLinearStateType(remote.State.Type)
	if issue.Status != status {
		updated, err := applyIssueStatus(ctx, s, root, issue.ID, status)
		if err != nil {
			return Issue{}, err
		}
		issue = updated
	}
	return issue, nil
}

func (s *Store) replaceLinearIssueAlias(ctx context.Context, root project.Root, issueID, identifier, url string) error {
	identifier = strings.TrimSpace(identifier)
	issueID = strings.TrimSpace(issueID)
	if identifier == "" || issueID == "" {
		return fmt.Errorf("replace linear alias requires a local id and linear key")
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin replace linear alias: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var existingAlias string
	err = tx.QueryRowContext(ctx, `
SELECT alias FROM aliases WHERE project_id = ? AND entity_kind = ? AND entity_id = ? AND namespace = ?
`, projectID, issueEntityKind, issueID, issueNamespace).Scan(&existingAlias)
	switch {
	case err == nil:
		if existingAlias != identifier {
			if _, err := tx.ExecContext(ctx, `
UPDATE aliases SET alias = ?, updated_at = ?
WHERE project_id = ? AND entity_kind = ? AND entity_id = ? AND namespace = ?
`, identifier, now, projectID, issueEntityKind, issueID, issueNamespace); err != nil {
				return fmt.Errorf("update linear issue alias: %w", err)
			}
		}
	case err == sql.ErrNoRows:
		if err := insertAlias(ctx, tx, projectID, issueEntityKind, issueID, issueNamespace, identifier, now); err != nil {
			return err
		}
	default:
		return fmt.Errorf("lookup issue alias: %w", err)
	}
	if err := upsertBackendMappingTx(ctx, tx, projectID, backendMapping{
		EntityKind:   issueEntityKind,
		EntityID:     issueID,
		ExternalKind: linearExternalKindIssue,
		ExternalID:   identifier,
		ExternalURL:  url,
		SyncStatus:   linearSyncLinked,
	}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace linear alias: %w", err)
	}
	return nil
}

// PushLinearIssue writes the render body and, when local status is newer, status.
func PushLinearIssue(ctx context.Context, root project.Root, resolver PathResolver, client *LinearClient, ref, description string) (LinearPushResult, error) {
	if client == nil {
		return LinearPushResult{}, fmt.Errorf("linear client is not configured")
	}
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return LinearPushResult{}, err
	}
	defer store.Close()
	return store.PushLinearIssue(ctx, root, client, ref, description)
}

// PushLinearIssue writes description and maybe status from an open store.
func (s *Store) PushLinearIssue(ctx context.Context, root project.Root, client *LinearClient, ref, description string) (LinearPushResult, error) {
	issue, err := s.GetIssue(ctx, root, ref)
	if err != nil {
		return LinearPushResult{}, err
	}
	cfg, err := s.LoadLinearAdapterConfig(ctx, root)
	if err != nil {
		return LinearPushResult{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return LinearPushResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return LinearPushResult{}, fmt.Errorf("begin push lookup: %w", err)
	}
	mapping, err := lookupLinearIssueMappingTx(ctx, tx, projectID, issue.ID, issue.Alias)
	localChangedAt, statusErr := latestIssueStatusChangedAtTx(ctx, tx, projectID, issue.ID)
	_ = tx.Rollback()
	if err != nil {
		return LinearPushResult{}, fmt.Errorf("issue %s has no linear mapping; pull it first", issueDisplayHint(issue))
	}
	if statusErr != nil {
		return LinearPushResult{}, statusErr
	}
	remote, err := client.Issue(ctx, mapping.ExternalID)
	if err != nil {
		return LinearPushResult{}, err
	}
	input := LinearUpdateIssueInput{Description: &description}
	result := LinearPushResult{Issue: issue, Linear: remote, DescriptionWrote: true}
	remoteStatus := MapLinearStateType(remote.State.Type)
	if issue.Status != remoteStatus {
		if !localChangedAt.IsZero() && localChangedAt.After(remote.UpdatedAt) {
			team, err := client.TeamByKey(ctx, cfg.TeamKey)
			if err != nil {
				return LinearPushResult{}, err
			}
			stateID, err := resolveLinearStateID(team, issue.Status, cfg.StatusOverrides)
			if err != nil {
				return LinearPushResult{}, err
			}
			input.StateID = stateID
			result.StatusWrote = true
		} else {
			result.StatusSkipped = "tracker status is newer or equal; not overwritten"
		}
	}
	updated, err := client.UpdateIssue(ctx, remote.ID, input)
	if err != nil {
		return LinearPushResult{}, err
	}
	result.Linear = updated
	if err := s.upsertBackendMapping(ctx, root, backendMapping{
		EntityKind:   issueEntityKind,
		EntityID:     issue.ID,
		ExternalKind: linearExternalKindIssue,
		ExternalID:   updated.Identifier,
		ExternalURL:  updated.URL,
		SyncStatus:   linearSyncLinked,
	}); err != nil {
		return LinearPushResult{}, err
	}
	return result, nil
}

func issueDisplayHint(issue Issue) string {
	if issue.Alias != "" {
		return issue.Alias
	}
	return issue.ID
}

// ReconcileLinearIssue compares local and tracker and surfaces conflicts.
func ReconcileLinearIssue(ctx context.Context, root project.Root, resolver PathResolver, client *LinearClient, ref string, takeLocal, takeTracker bool) (LinearReconcileResult, error) {
	if takeLocal && takeTracker {
		return LinearReconcileResult{}, fmt.Errorf("issue reconcile accepts at most one of --take-local and --take-tracker")
	}
	if client == nil {
		return LinearReconcileResult{}, fmt.Errorf("linear client is not configured")
	}
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return LinearReconcileResult{}, err
	}
	defer store.Close()
	return store.ReconcileLinearIssue(ctx, root, client, ref, takeLocal, takeTracker)
}

// ReconcileLinearIssues reconciles every mapped Linear issue.
func ReconcileLinearIssues(ctx context.Context, root project.Root, resolver PathResolver, client *LinearClient, takeLocal, takeTracker bool) ([]LinearReconcileResult, error) {
	if takeLocal && takeTracker {
		return nil, fmt.Errorf("issue reconcile accepts at most one of --take-local and --take-tracker")
	}
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return nil, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin reconcile list: %w", err)
	}
	mappings, err := listLinearIssueMappingsTx(ctx, tx, projectID)
	_ = tx.Rollback()
	if err != nil {
		return nil, err
	}
	results := make([]LinearReconcileResult, 0, len(mappings))
	for _, mapping := range mappings {
		result, err := store.ReconcileLinearIssue(ctx, root, client, mapping.EntityID, takeLocal, takeTracker)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// ReconcileLinearIssue compares one mapped issue on an open store.
func (s *Store) ReconcileLinearIssue(ctx context.Context, root project.Root, client *LinearClient, ref string, takeLocal, takeTracker bool) (LinearReconcileResult, error) {
	issue, err := s.GetIssue(ctx, root, ref)
	if err != nil {
		return LinearReconcileResult{}, err
	}
	cfg, err := s.LoadLinearAdapterConfig(ctx, root)
	if err != nil {
		return LinearReconcileResult{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return LinearReconcileResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return LinearReconcileResult{}, fmt.Errorf("begin reconcile lookup: %w", err)
	}
	mapping, err := lookupLinearIssueMappingTx(ctx, tx, projectID, issue.ID, issue.Alias)
	localChangedAt, statusErr := latestIssueStatusChangedAtTx(ctx, tx, projectID, issue.ID)
	_ = tx.Rollback()
	if err != nil {
		return LinearReconcileResult{}, fmt.Errorf("issue %s has no linear mapping; pull it first", issueDisplayHint(issue))
	}
	if statusErr != nil {
		return LinearReconcileResult{}, statusErr
	}
	remote, err := client.Issue(ctx, mapping.ExternalID)
	if err != nil {
		return LinearReconcileResult{}, err
	}
	result := LinearReconcileResult{Issue: issue, Linear: remote}
	lastSync := parseComparableTime(mapping.UpdatedAt)
	shown, err := s.ShowIssue(ctx, root, issue.ID)
	if err != nil {
		return LinearReconcileResult{}, err
	}
	rendered := RenderIssueMarkdown(shown)

	if issue.Title != remote.Title {
		updated, err := s.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: issue.ID, Title: remote.Title, SetTitle: true})
		if err != nil {
			return LinearReconcileResult{}, err
		}
		result.Issue = updated
		result.Conflicts = append(result.Conflicts, LinearReconcileConflict{
			Field:      "title",
			Local:      issue.Title,
			Tracker:    remote.Title,
			Resolution: "tracker wins; local title updated",
		})
		issue = updated
	}

	if strings.TrimSpace(rendered) != strings.TrimSpace(remote.Description) {
		result.Conflicts = append(result.Conflicts, LinearReconcileConflict{
			Field:      "description",
			Local:      rendered,
			Tracker:    remote.Description,
			ReportOnly: true,
			Resolution: "report only; loaf owns shaping body, tracker description is not applied",
		})
	}

	remoteStatus := MapLinearStateType(remote.State.Type)
	if issue.Status != remoteStatus {
		localMoved := !localChangedAt.IsZero() && localChangedAt.After(lastSync)
		trackerMoved := !remote.UpdatedAt.IsZero() && remote.UpdatedAt.After(lastSync)
		mover := "unknown"
		switch {
		case localMoved && trackerMoved:
			mover = "both"
		case localMoved:
			mover = "local"
		case trackerMoved:
			mover = "tracker"
		default:
			mover = "both"
		}
		conflict := LinearReconcileConflict{
			Field:     "status",
			Local:     issue.Status,
			Tracker:   remoteStatus,
			LocalAt:   formatOptionalTime(localChangedAt),
			TrackerAt: formatOptionalTime(remote.UpdatedAt),
			Mover:     mover,
		}
		switch {
		case takeLocal:
			team, err := client.TeamByKey(ctx, cfg.TeamKey)
			if err != nil {
				return LinearReconcileResult{}, err
			}
			stateID, err := resolveLinearStateID(team, issue.Status, cfg.StatusOverrides)
			if err != nil {
				return LinearReconcileResult{}, err
			}
			updated, err := client.UpdateIssue(ctx, remote.ID, LinearUpdateIssueInput{StateID: stateID})
			if err != nil {
				return LinearReconcileResult{}, err
			}
			result.Linear = updated
			conflict.Resolution = "took local; tracker status updated"
		case takeTracker:
			updated, err := applyIssueStatus(ctx, s, root, issue.ID, remoteStatus)
			if err != nil {
				return LinearReconcileResult{}, err
			}
			result.Issue = updated
			conflict.Resolution = "took tracker; local status updated through events"
		default:
			conflict.Unresolved = true
			conflict.Resolution = "unresolved; pass --take-local or --take-tracker (never silent last-writer-wins)"
		}
		result.Conflicts = append(result.Conflicts, conflict)
	}

	if err := s.upsertBackendMapping(ctx, root, backendMapping{
		EntityKind:   issueEntityKind,
		EntityID:     result.Issue.ID,
		ExternalKind: linearExternalKindIssue,
		ExternalID:   remote.Identifier,
		ExternalURL:  remote.URL,
		SyncStatus:   linearSyncLinked,
	}); err != nil {
		return LinearReconcileResult{}, err
	}
	result.InSync = true
	for _, conflict := range result.Conflicts {
		if conflict.Unresolved {
			result.InSync = false
			break
		}
	}
	return result, nil
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

// PushLinearRelease creates or updates the Linear Release for a recorded cut.
func PushLinearRelease(ctx context.Context, root project.Root, resolver PathResolver, client *LinearClient, release Release) (LinearReleasePushResult, error) {
	if client == nil {
		return LinearReleasePushResult{}, fmt.Errorf("linear client is not configured")
	}
	supported, err := client.ReleasesSupported(ctx)
	if err != nil {
		return LinearReleasePushResult{}, err
	}
	if !supported {
		return LinearReleasePushResult{Skipped: LinearReleaseUnsupportedSkip}, nil
	}
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return LinearReleasePushResult{}, err
	}
	defer store.Close()
	return store.PushLinearRelease(ctx, root, client, release)
}

// PushLinearRelease writes the Linear Release from an open store.
func (s *Store) PushLinearRelease(ctx context.Context, root project.Root, client *LinearClient, release Release) (LinearReleasePushResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return LinearReleasePushResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return LinearReleasePushResult{}, fmt.Errorf("begin linear release lookup: %w", err)
	}
	var issueIDs []string
	var issueKeys []string
	var unmapped []string
	for _, member := range release.Members {
		if member.Kind != ReleaseMemberKindIssue {
			continue
		}
		mapping, err := lookupLinearIssueMappingTx(ctx, tx, projectID, member.MemberID, "")
		if err != nil {
			unmapped = append(unmapped, linearReleaseMemberKeyTx(ctx, tx, projectID, member.MemberID))
			continue
		}
		remote, err := client.Issue(ctx, mapping.ExternalID)
		if err != nil {
			_ = tx.Rollback()
			return LinearReleasePushResult{Unmapped: unmapped}, err
		}
		issueIDs = append(issueIDs, remote.ID)
		issueKeys = append(issueKeys, remote.Identifier)
	}
	existingExternal := ""
	_ = tx.QueryRowContext(ctx, `
SELECT external_id FROM backend_mappings
WHERE project_id = ? AND backend = ? AND entity_kind = 'release' AND entity_id = ? AND external_kind = ?
`, projectID, linearBackend, release.ID, linearExternalKindRelease).Scan(&existingExternal)
	_ = tx.Rollback()

	name := release.Version
	if name == "" {
		name = release.Tag
	}
	var remote LinearRelease
	if existingExternal != "" {
		remote, err = client.UpdateRelease(ctx, existingExternal, name, issueIDs)
	} else {
		remote, err = client.CreateRelease(ctx, name, issueIDs)
	}
	if err != nil {
		return LinearReleasePushResult{Unmapped: unmapped}, err
	}
	if err := s.upsertBackendMapping(ctx, root, backendMapping{
		EntityKind:   "release",
		EntityID:     release.ID,
		ExternalKind: linearExternalKindRelease,
		ExternalID:   remote.ID,
		SyncStatus:   linearSyncLinked,
	}); err != nil {
		return LinearReleasePushResult{Unmapped: unmapped}, err
	}
	if len(remote.IssueKeys) == 0 {
		remote.IssueKeys = issueKeys
		remote.IssueIDs = issueIDs
	}
	return LinearReleasePushResult{Supported: true, Release: remote, Unmapped: unmapped}, nil
}

func linearReleaseMemberKeyTx(ctx context.Context, tx *sql.Tx, projectID, issueID string) string {
	var alias string
	err := tx.QueryRowContext(ctx, `
SELECT alias FROM aliases WHERE project_id = ? AND entity_kind = ? AND entity_id = ? AND namespace = ?
`, projectID, issueEntityKind, issueID, issueNamespace).Scan(&alias)
	if err == nil && strings.TrimSpace(alias) != "" {
		return alias
	}
	return issueID
}

// PublishLinearReadiness applies ready-for-agent or ready-for-human on Linear.
func PublishLinearReadiness(ctx context.Context, root project.Root, resolver PathResolver, client *LinearClient, issueRef, label, reason string) error {
	if client == nil {
		return fmt.Errorf("linear client is not configured")
	}
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return err
	}
	defer store.Close()
	issue, err := store.GetIssue(ctx, root, issueRef)
	if err != nil {
		return err
	}
	cfg, err := store.LoadLinearAdapterConfig(ctx, root)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.TeamKey) == "" {
		return fmt.Errorf("linear team key is not configured; set %s or a backend_mappings team row", LinearEnvTeamKey)
	}
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin readiness lookup: %w", err)
	}
	mapping, err := lookupLinearIssueMappingTx(ctx, tx, projectID, issue.ID, issue.Alias)
	_ = tx.Rollback()
	key := issue.Alias
	if err == nil {
		key = mapping.ExternalID
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("issue %s has no linear key; pull or mint it first", issue.ID)
	}
	remote, err := client.Issue(ctx, key)
	if err != nil {
		return err
	}
	team, err := client.TeamByKey(ctx, cfg.TeamKey)
	if err != nil {
		return err
	}
	labelID, err := client.EnsureLabel(ctx, team.ID, label)
	if err != nil {
		return err
	}
	if _, err := client.UpdateIssue(ctx, remote.ID, LinearUpdateIssueInput{AddedLabelIDs: []string{labelID}}); err != nil {
		return err
	}
	if strings.TrimSpace(reason) != "" {
		if err := client.CreateComment(ctx, remote.ID, reason); err != nil {
			return err
		}
	}
	return nil
}

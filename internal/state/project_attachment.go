package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	FactKindProjectMinted   = "project.minted"
	FactKindProjectEvidence = "project.evidence"

	EvidenceKindRemote     = "remote"
	EvidenceKindRootCommit = "root_commit"
	EvidenceKindPath       = "path"
	EvidenceKindConf       = "conf"

	DuplicateUniverseSuspectCode = "duplicate-universe-suspect"

	ConfResolutionUnattended = "unattended"
	ConfResolutionInteractive = "interactive"
)

func init() {
	RegisterFactKind(FactKindProjectMinted, "ledger")
	RegisterFactKind(FactKindProjectEvidence, "ledger")
}

// ProjectEvidencePayload is the fact body for attachment evidence.
type ProjectEvidencePayload struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Raw   string `json:"raw,omitempty"`
}

// ConfResolutionRequiredError reports an unknown conf label that needs first-contact confirm.
type ConfResolutionRequiredError struct {
	ConfID            string `json:"conf_id"`
	ConfProjectID     string `json:"conf_project_id"`
	SuggestedProject  string `json:"suggested_project_id,omitempty"`
	KnownConfProjects []string `json:"known_conf_projects,omitempty"`
}

func (e *ConfResolutionRequiredError) Error() string {
	if e == nil {
		return "project conf requires first-contact confirmation"
	}
	return fmt.Sprintf("conf label %q is unknown on this substrate; first contact must confirm project %q before mapping", e.ConfID, e.ConfProjectID)
}

// ConfOutOfScopeError reports a conf label mapped to a different project.
type ConfOutOfScopeError struct {
	ConfID         string `json:"conf_id"`
	ConfProjectID  string `json:"conf_project_id"`
	MappedProjectID string `json:"mapped_project_id"`
}

func (e *ConfOutOfScopeError) Error() string {
	if e == nil {
		return "project conf is out of scope"
	}
	return fmt.Sprintf("conf label %q maps to project %q but conf declares %q", e.ConfID, e.MappedProjectID, e.ConfProjectID)
}

// DuplicateUniverseReport lists substrate projects that share attachment evidence.
type DuplicateUniverseReport struct {
	RemoteGroups     []DuplicateEvidenceGroup `json:"remote_groups,omitempty"`
	RootCommitGroups []DuplicateEvidenceGroup `json:"root_commit_groups,omitempty"`
	Ready            bool                     `json:"ready"`
}

type DuplicateEvidenceGroup struct {
	Kind       string   `json:"kind"`
	Value      string   `json:"value"`
	ProjectIDs []string `json:"project_ids"`
}

// RegisterConfLabel maps a conf rendezvous label to a durable project id.
func (s *Store) RegisterConfLabel(ctx context.Context, confID, projectID string) error {
	confID = strings.TrimSpace(confID)
	projectID = strings.TrimSpace(projectID)
	if confID == "" || projectID == "" {
		return fmt.Errorf("register conf label: conf_id and project_id are required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO project_conf_labels (conf_id, project_id, first_seen_at, last_seen_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(conf_id) DO UPDATE SET
  project_id = excluded.project_id,
  last_seen_at = excluded.last_seen_at
`, confID, projectID, now, now); err != nil {
		return fmt.Errorf("register conf label: %w", err)
	}
	payload, err := encodeProjectEvidencePayload(ProjectEvidencePayload{Kind: EvidenceKindConf, Value: confID})
	if err != nil {
		return err
	}
	_, err = AppendFact(ctx, s, AppendFactInput{
		ProjectID: projectID,
		Kind:      FactKindProjectEvidence,
		Payload:   payload,
	})
	return err
}

// RecordAttachmentEvidence accumulates one attachment evidence row and ledger fact.
func (s *Store) RecordAttachmentEvidence(ctx context.Context, projectID, kind, value, raw string) error {
	projectID = strings.TrimSpace(projectID)
	kind = strings.TrimSpace(kind)
	value = strings.TrimSpace(value)
	if projectID == "" || kind == "" || value == "" {
		return fmt.Errorf("record attachment evidence: project_id, kind, and value are required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id := stableMigrationID("project-evidence", projectID, kind, value)
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO project_attachment_evidence (id, project_id, evidence_kind, evidence_value, first_seen_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, evidence_kind, evidence_value) DO UPDATE SET
  last_seen_at = excluded.last_seen_at
`, id, projectID, kind, value, now, now); err != nil {
		return fmt.Errorf("record attachment evidence: %w", err)
	}
	payload, err := encodeProjectEvidencePayload(ProjectEvidencePayload{Kind: kind, Value: value, Raw: raw})
	if err != nil {
		return err
	}
	_, err = AppendFact(ctx, s, AppendFactInput{
		ProjectID: projectID,
		Kind:      FactKindProjectEvidence,
		Payload:   payload,
	})
	return err
}

// RecordCheckoutAttachmentEvidence captures git remotes, root fingerprint, and path for one checkout.
func (s *Store) RecordCheckoutAttachmentEvidence(ctx context.Context, root project.Root, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("record checkout attachment evidence: project_id is required")
	}
	if err := s.RecordAttachmentEvidence(ctx, projectID, EvidenceKindPath, root.Path(), ""); err != nil {
		return err
	}
	remotes, err := project.ListNormalizedRemotes(root)
	if err != nil {
		return err
	}
	for _, remote := range remotes {
		if err := s.RecordAttachmentEvidence(ctx, projectID, EvidenceKindRemote, remote, ""); err != nil {
			return err
		}
	}
	fingerprint, err := project.RootCommitFingerprint(root)
	if err != nil {
		return err
	}
	if fingerprint != "" {
		if err := s.RecordAttachmentEvidence(ctx, projectID, EvidenceKindRootCommit, fingerprint, ""); err != nil {
			return err
		}
	}
	return nil
}

// ResolveProjectByConf resolves substrate identity from `.agents/loaf.conf`.
func (s *Store) ResolveProjectByConf(ctx context.Context, conf project.ProjectConf, mode string) (ProjectIdentity, error) {
	confID := strings.TrimSpace(conf.ConfID)
	projectID := strings.TrimSpace(conf.ProjectID)
	if projectID == "" {
		return ProjectIdentity{}, fmt.Errorf("resolve project by conf: project_id is required")
	}
	if confID == "" {
		if mode == ConfResolutionUnattended {
			return ProjectIdentity{}, fmt.Errorf("resolve project by conf: conf_id is required for unattended resolution")
		}
		return s.projectIdentity(ctx, projectID)
	}
	mappedID, err := s.confLabelProjectID(ctx, confID)
	if err != nil {
		return ProjectIdentity{}, err
	}
	switch {
	case mappedID == "":
		if mode == ConfResolutionUnattended {
			return ProjectIdentity{}, &ConfResolutionRequiredError{ConfID: confID, ConfProjectID: projectID}
		}
		return ProjectIdentity{}, &ConfResolutionRequiredError{
			ConfID:           confID,
			ConfProjectID:    projectID,
			SuggestedProject: projectID,
		}
	case mappedID != projectID:
		return ProjectIdentity{}, &ConfOutOfScopeError{ConfID: confID, ConfProjectID: projectID, MappedProjectID: mappedID}
	default:
		return s.projectIdentity(ctx, projectID)
	}
}

func (s *Store) confLabelProjectID(ctx context.Context, confID string) (string, error) {
	var projectID string
	err := s.db.QueryRowContext(ctx, `SELECT project_id FROM project_conf_labels WHERE conf_id = ?`, confID).Scan(&projectID)
	switch {
	case err == nil:
		return projectID, nil
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	default:
		return "", fmt.Errorf("read conf label: %w", err)
	}
}

// InspectDuplicateUniverseSuspects flags projects sharing normalized remotes or root fingerprints.
func InspectDuplicateUniverseSuspects(ctx context.Context, store *Store) (DuplicateUniverseReport, error) {
	if store == nil || store.db == nil {
		return DuplicateUniverseReport{}, fmt.Errorf("inspect duplicate universe: store is nil")
	}
	report := DuplicateUniverseReport{Ready: true}
	for _, kind := range []string{EvidenceKindRemote, EvidenceKindRootCommit} {
		groups, err := duplicateEvidenceGroups(ctx, store, kind)
		if err != nil {
			return DuplicateUniverseReport{}, err
		}
		for _, group := range groups {
			if len(group.ProjectIDs) < 2 {
				continue
			}
			report.Ready = false
			switch kind {
			case EvidenceKindRemote:
				report.RemoteGroups = append(report.RemoteGroups, group)
			case EvidenceKindRootCommit:
				report.RootCommitGroups = append(report.RootCommitGroups, group)
			}
		}
	}
	return report, nil
}

func duplicateEvidenceGroups(ctx context.Context, store *Store, kind string) ([]DuplicateEvidenceGroup, error) {
	rows, err := store.db.QueryContext(ctx, `
SELECT evidence_value, project_id
FROM project_attachment_evidence
WHERE evidence_kind = ?
ORDER BY evidence_value, project_id
`, kind)
	if err != nil {
		return nil, fmt.Errorf("list attachment evidence for %q: %w", kind, err)
	}
	defer rows.Close()
	byValue := map[string][]string{}
	for rows.Next() {
		var value, projectID string
		if err := rows.Scan(&value, &projectID); err != nil {
			return nil, fmt.Errorf("scan attachment evidence: %w", err)
		}
		byValue[value] = append(byValue[value], projectID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachment evidence: %w", err)
	}
	groups := make([]DuplicateEvidenceGroup, 0, len(byValue))
	for value, projectIDs := range byValue {
		groups = append(groups, DuplicateEvidenceGroup{Kind: kind, Value: value, ProjectIDs: projectIDs})
	}
	return groups, nil
}

func duplicateUniverseDiagnostics(report DuplicateUniverseReport) []Diagnostic {
	if report.Ready {
		return nil
	}
	details := map[string]any{}
	if len(report.RemoteGroups) > 0 {
		details["remote_groups"] = report.RemoteGroups
	}
	if len(report.RootCommitGroups) > 0 {
		details["root_commit_groups"] = report.RootCommitGroups
	}
	return []Diagnostic{{
		Severity: "warn",
		Code:     DuplicateUniverseSuspectCode,
		Category: RepairCategoryProjectIdentity,
		Policy:   DiagnosticPolicyWarningDrift,
		Message:  "multiple projects share attachment evidence; inspect for duplicate universes before attach",
		Details:  details,
	}}
}

func encodeProjectEvidencePayload(payload ProjectEvidencePayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode project evidence payload: %w", err)
	}
	return string(raw), nil
}

// BootstrapProjectConf registers conf labels and records attachment evidence.
// It writes `.agents/loaf.conf` only for genesis projects that have no conf file
// and no prior attachment evidence — never silently backfilling existing projects.
func (s *Store) BootstrapProjectConf(ctx context.Context, root project.Root) error {
	identity, err := s.LookupProjectIdentityForRoot(ctx, root)
	if err != nil {
		return err
	}
	if conf, readErr := project.ReadProjectConf(root); readErr == nil {
		if conf.ConfID != "" {
			return s.RegisterConfLabel(ctx, conf.ConfID, identity.ID)
		}
		return nil
	}
	hasEvidence, err := s.projectHasAttachmentEvidence(ctx, identity.ID)
	if err != nil {
		return err
	}
	if hasEvidence {
		return nil
	}
	conf := project.ProjectConf{ProjectID: identity.ID}
	if err := project.WriteProjectConf(root, conf); err != nil {
		return err
	}
	written, err := project.ReadProjectConf(root)
	if err != nil {
		return err
	}
	if err := s.RegisterConfLabel(ctx, written.ConfID, identity.ID); err != nil {
		return err
	}
	if err := s.RecordCheckoutAttachmentEvidence(ctx, root, identity.ID); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{
		"project_id": identity.ID,
		"conf_id":    written.ConfID,
		"path":       root.Path(),
	})
	if err != nil {
		return fmt.Errorf("encode project minted payload: %w", err)
	}
	_, err = AppendFact(ctx, s, AppendFactInput{
		ProjectID: identity.ID,
		Kind:      FactKindProjectMinted,
		Payload:   string(payload),
	})
	return err
}

func (s *Store) projectHasAttachmentEvidence(ctx context.Context, projectID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM project_attachment_evidence
WHERE project_id = ?
`, projectID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count attachment evidence: %w", err)
	}
	return count > 0, nil
}

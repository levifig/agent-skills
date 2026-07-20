package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// FindingCreateOptions describes a finding creation request.
type FindingCreateOptions struct {
	Report     string
	Run        string
	Title      string
	Status     string
	Severity   string
	Confidence string
	Dimension  string
	Path       string
	LineStart  int
	LineEnd    int
	Symbol     string
	Metadata   string
	Body       string
	SetBody    bool
}

// FindingListOptions filters finding lists.
type FindingListOptions struct {
	Report     string
	Run        string
	Status     string
	Severity   string
	Confidence string
	Dimension  string
}

// FindingVerdictOptions describes a verdict recorded against a finding.
type FindingVerdictOptions struct {
	Finding           string
	Run               string
	Outcome           string
	Rationale         string
	ReproductionNotes string
	Metadata          string
}

// FindingCreateResult describes a created finding.
type FindingCreateResult struct {
	ContractVersion    int           `json:"contract_version,omitempty"`
	DatabaseScope      string        `json:"database_scope,omitempty"`
	DatabasePath       string        `json:"database_path,omitempty"`
	ProjectID          string        `json:"project_id,omitempty"`
	ProjectName        string        `json:"project_name,omitempty"`
	ProjectCurrentPath string        `json:"project_current_path,omitempty"`
	Finding            FindingDetail `json:"finding"`
	EventID            string        `json:"event_id,omitempty"`
}

// FindingShow is the state-backed single-finding read model.
type FindingShow struct {
	ContractVersion    int           `json:"contract_version,omitempty"`
	DatabaseScope      string        `json:"database_scope,omitempty"`
	DatabasePath       string        `json:"database_path,omitempty"`
	ProjectID          string        `json:"project_id,omitempty"`
	ProjectName        string        `json:"project_name,omitempty"`
	ProjectCurrentPath string        `json:"project_current_path,omitempty"`
	Query              string        `json:"query"`
	Finding            FindingDetail `json:"finding"`
}

// FindingList is the state-backed finding-list read model.
type FindingList struct {
	ContractVersion    int                    `json:"contract_version,omitempty"`
	DatabaseScope      string                 `json:"database_scope,omitempty"`
	DatabasePath       string                 `json:"database_path,omitempty"`
	ProjectID          string                 `json:"project_id,omitempty"`
	ProjectName        string                 `json:"project_name,omitempty"`
	ProjectCurrentPath string                 `json:"project_current_path,omitempty"`
	Filters            FindingListOptions     `json:"filters,omitempty"`
	Findings           map[string]FindingItem `json:"findings"`
}

// FindingVerdictResult describes a recorded verdict and updated finding.
type FindingVerdictResult struct {
	ContractVersion    int           `json:"contract_version,omitempty"`
	DatabaseScope      string        `json:"database_scope,omitempty"`
	DatabasePath       string        `json:"database_path,omitempty"`
	ProjectID          string        `json:"project_id,omitempty"`
	ProjectName        string        `json:"project_name,omitempty"`
	ProjectCurrentPath string        `json:"project_current_path,omitempty"`
	Finding            FindingDetail `json:"finding"`
	Verdict            VerdictDetail `json:"verdict"`
	EventID            string        `json:"event_id,omitempty"`
}

// FindingItem is a compact finding list row.
type FindingItem struct {
	ID         string `json:"id"`
	Alias      string `json:"alias,omitempty"`
	Report     string `json:"report"`
	Run        string `json:"run,omitempty"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Severity   string `json:"severity"`
	Confidence string `json:"confidence"`
	Dimension  string `json:"dimension,omitempty"`
	Path       string `json:"path,omitempty"`
	LineStart  int    `json:"line_start,omitempty"`
	LineEnd    int    `json:"line_end,omitempty"`
	Symbol     string `json:"symbol,omitempty"`
}

// FindingDetail contains finding metadata, body, verdicts, and relationships.
type FindingDetail struct {
	ID            string              `json:"id"`
	Alias         string              `json:"alias,omitempty"`
	Report        TraceEntity         `json:"report"`
	Run           *TraceEntity        `json:"run,omitempty"`
	Title         string              `json:"title"`
	Status        string              `json:"status"`
	Severity      string              `json:"severity"`
	Confidence    string              `json:"confidence"`
	Dimension     string              `json:"dimension,omitempty"`
	Path          string              `json:"path,omitempty"`
	LineStart     int                 `json:"line_start,omitempty"`
	LineEnd       int                 `json:"line_end,omitempty"`
	Symbol        string              `json:"symbol,omitempty"`
	Metadata      string              `json:"metadata,omitempty"`
	Body          string              `json:"body,omitempty"`
	Verdicts      []VerdictDetail     `json:"verdicts,omitempty"`
	Relationships []TraceRelationship `json:"relationships,omitempty"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
}

// VerdictDetail contains verdict row metadata.
type VerdictDetail struct {
	ID                string       `json:"id"`
	Finding           TraceEntity  `json:"finding"`
	Run               *TraceEntity `json:"run,omitempty"`
	Outcome           string       `json:"outcome"`
	Rationale         string       `json:"rationale"`
	ReproductionNotes string       `json:"reproduction_notes,omitempty"`
	Metadata          string       `json:"metadata,omitempty"`
	CreatedAt         string       `json:"created_at"`
	UpdatedAt         string       `json:"updated_at"`
}

// CreateFinding creates a finding row in initialized SQLite state.
func CreateFinding(ctx context.Context, root project.Root, resolver PathResolver, options FindingCreateOptions) (FindingCreateResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return FindingCreateResult{}, err
	}
	defer store.Close()
	return store.CreateFinding(ctx, root, options)
}

// ShowFinding returns one finding from initialized SQLite state.
func ShowFinding(ctx context.Context, root project.Root, resolver PathResolver, ref string) (FindingShow, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return FindingShow{}, err
	}
	defer store.Close()
	return store.ShowFinding(ctx, root, ref)
}

// ListFindings lists findings from initialized SQLite state.
func ListFindings(ctx context.Context, root project.Root, resolver PathResolver, options FindingListOptions) (FindingList, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return FindingList{}, err
	}
	defer store.Close()
	return store.ListFindings(ctx, root, options)
}

// RecordFindingVerdict records a verdict and updates the parent finding's current status.
func RecordFindingVerdict(ctx context.Context, root project.Root, resolver PathResolver, options FindingVerdictOptions) (FindingVerdictResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return FindingVerdictResult{}, err
	}
	defer store.Close()
	return store.RecordFindingVerdict(ctx, root, options)
}

// CreateFinding creates a finding row in an open store.
func (s *Store) CreateFinding(ctx context.Context, root project.Root, options FindingCreateOptions) (FindingCreateResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return FindingCreateResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return FindingCreateResult{}, err
	}
	report, run, normalized, err := s.normalizeFindingCreateOptions(ctx, projectID, options)
	if err != nil {
		return FindingCreateResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FindingCreateResult{}, fmt.Errorf("begin finding create transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339)
	alias, err := s.nextFindingAlias(ctx, tx, projectID, normalized.Title, now)
	if err != nil {
		return FindingCreateResult{}, err
	}
	findingID := stableMigrationID("finding", projectID, alias)
	var runID any
	if run != nil {
		runID = run.ID
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO findings (id, project_id, report_id, run_id, title, status, severity, confidence, dimension, path, line_start, line_end, symbol, metadata, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, findingID, projectID, report.ID, runID, normalized.Title, normalized.Status, normalized.Severity, normalized.Confidence, emptyToNil(normalized.Dimension), emptyToNil(filepath.ToSlash(normalized.Path)), intToNil(normalized.LineStart), intToNil(normalized.LineEnd), emptyToNil(normalized.Symbol), emptyToNil(normalized.Metadata), timestamp, timestamp); err != nil {
		return FindingCreateResult{}, fmt.Errorf("insert finding %s: %w", alias, err)
	}
	if err := insertAlias(ctx, tx, projectID, "finding", findingID, "finding", alias, timestamp); err != nil {
		return FindingCreateResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, 'report', ?, 'finding', ?, 'contains', 'recorded by finding create', 'command', ?, ?)
ON CONFLICT(id) DO NOTHING
`, stableMigrationID("relationship", projectID, "report", report.ID, "contains", "finding", findingID), projectID, report.ID, findingID, timestamp, timestamp); err != nil {
		return FindingCreateResult{}, fmt.Errorf("record report finding relationship: %w", err)
	}
	if run != nil {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, 'run', ?, 'finding', ?, 'produces', 'recorded by finding create', 'command', ?, ?)
ON CONFLICT(id) DO NOTHING
`, stableMigrationID("relationship", projectID, "run", run.ID, "produces", "finding", findingID), projectID, run.ID, findingID, timestamp, timestamp); err != nil {
			return FindingCreateResult{}, fmt.Errorf("record run finding relationship: %w", err)
		}
	}
	eventID := stableMigrationID("event", projectID, "finding", findingID, "created", normalized.Status)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'finding', ?, 'status_changed', NULL, ?, 'recorded by finding create', ?, ?)
`, eventID, projectID, findingID, normalized.Status, timestamp, timestamp); err != nil {
		return FindingCreateResult{}, fmt.Errorf("record finding create event: %w", err)
	}
	if normalized.SetBody {
		if _, err := upsertArtifactBodyTx(ctx, tx, projectID, "finding", findingID, ArtifactBodyKindMarkdown, normalized.Body, nil, timestamp); err != nil {
			return FindingCreateResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return FindingCreateResult{}, fmt.Errorf("commit finding create transaction: %w", err)
	}

	detail, err := s.findingDetail(ctx, root, projectID, TraceEntity{Kind: "finding", ID: findingID, Alias: alias})
	if err != nil {
		return FindingCreateResult{}, err
	}
	return FindingCreateResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Finding:            detail,
		EventID:            eventID,
	}, nil
}

// ShowFinding returns one finding from an open store.
func (s *Store) ShowFinding(ctx context.Context, root project.Root, ref string) (FindingShow, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return FindingShow{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return FindingShow{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return FindingShow{}, err
	}
	if entity.Kind != "finding" {
		return FindingShow{}, fmt.Errorf("finding show target %q resolved to %s, not finding", ref, entity.Kind)
	}
	detail, err := s.findingDetail(ctx, root, projectID, entity)
	if err != nil {
		return FindingShow{}, err
	}
	return FindingShow{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Finding:            detail,
	}, nil
}

// ListFindings lists findings from an open store.
func (s *Store) ListFindings(ctx context.Context, root project.Root, options FindingListOptions) (FindingList, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return FindingList{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return FindingList{}, err
	}
	normalized, err := s.normalizeFindingListOptions(ctx, projectID, options)
	if err != nil {
		return FindingList{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  findings.id,
  COALESCE(finding_alias.alias, ''),
  COALESCE(report_alias.alias, findings.report_id),
  COALESCE(run_alias.alias, findings.run_id, ''),
  findings.title,
  findings.status,
  findings.severity,
  findings.confidence,
  COALESCE(findings.dimension, ''),
  COALESCE(findings.path, ''),
  COALESCE(findings.line_start, 0),
  COALESCE(findings.line_end, 0),
  COALESCE(findings.symbol, '')
FROM findings
LEFT JOIN aliases finding_alias
  ON finding_alias.project_id = findings.project_id
 AND finding_alias.entity_kind = 'finding'
 AND finding_alias.entity_id = findings.id
 AND finding_alias.namespace = 'finding'
LEFT JOIN aliases report_alias
  ON report_alias.project_id = findings.project_id
 AND report_alias.entity_kind = 'report'
 AND report_alias.entity_id = findings.report_id
 AND report_alias.namespace = 'report'
LEFT JOIN aliases run_alias
  ON run_alias.project_id = findings.project_id
 AND run_alias.entity_kind = 'run'
 AND run_alias.entity_id = findings.run_id
 AND run_alias.namespace = 'run'
WHERE findings.project_id = ?
  AND (? = '' OR findings.report_id = ?)
  AND (? = '' OR findings.run_id = ?)
  AND (? = '' OR findings.status = ?)
  AND (? = '' OR findings.severity = ?)
  AND (? = '' OR findings.confidence = ?)
  AND (? = '' OR findings.dimension = ?)
ORDER BY CASE findings.severity
    WHEN 'critical' THEN 1
    WHEN 'high' THEN 2
    WHEN 'medium' THEN 3
    WHEN 'low' THEN 4
    WHEN 'info' THEN 5
    ELSE 99
  END,
  finding_alias.alias,
  findings.id
`, projectID, normalized.Report, normalized.Report, normalized.Run, normalized.Run, normalized.Status, normalized.Status, normalized.Severity, normalized.Severity, normalized.Confidence, normalized.Confidence, normalized.Dimension, normalized.Dimension)
	if err != nil {
		return FindingList{}, fmt.Errorf("query findings: %w", err)
	}
	result := FindingList{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Filters:            normalized,
		Findings:           map[string]FindingItem{},
	}
	for rows.Next() {
		var item FindingItem
		if err := rows.Scan(&item.ID, &item.Alias, &item.Report, &item.Run, &item.Title, &item.Status, &item.Severity, &item.Confidence, &item.Dimension, &item.Path, &item.LineStart, &item.LineEnd, &item.Symbol); err != nil {
			rows.Close()
			return FindingList{}, fmt.Errorf("scan finding: %w", err)
		}
		key := firstNonEmpty(item.Alias, item.ID)
		result.Findings[key] = item
	}
	if err := rows.Close(); err != nil {
		return FindingList{}, fmt.Errorf("close findings: %w", err)
	}
	if err := rows.Err(); err != nil {
		return FindingList{}, fmt.Errorf("iterate findings: %w", err)
	}
	return result, nil
}

// RecordFindingVerdict records a verdict and updates the parent finding status in an open store.
func (s *Store) RecordFindingVerdict(ctx context.Context, root project.Root, options FindingVerdictOptions) (FindingVerdictResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return FindingVerdictResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return FindingVerdictResult{}, err
	}
	finding, run, normalized, err := s.normalizeVerdictOptions(ctx, projectID, options)
	if err != nil {
		return FindingVerdictResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FindingVerdictResult{}, fmt.Errorf("begin verdict transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	verdictID := stableMigrationID("verdict", projectID, finding.ID, normalized.Outcome, now, normalized.Rationale)
	var runID any
	if run != nil {
		runID = run.ID
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO verdicts (id, project_id, finding_id, run_id, outcome, rationale, reproduction_notes, metadata, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, verdictID, projectID, finding.ID, runID, normalized.Outcome, normalized.Rationale, emptyToNil(normalized.ReproductionNotes), emptyToNil(normalized.Metadata), now, now); err != nil {
		return FindingVerdictResult{}, fmt.Errorf("insert verdict: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE findings SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, normalized.Outcome, now, projectID, finding.ID); err != nil {
		return FindingVerdictResult{}, fmt.Errorf("update finding status: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, 'finding', ?, 'verdict', ?, 'adjudicated_by', 'recorded by finding verdict', 'command', ?, ?)
ON CONFLICT(id) DO NOTHING
`, stableMigrationID("relationship", projectID, "finding", finding.ID, "adjudicated_by", "verdict", verdictID), projectID, finding.ID, verdictID, now, now); err != nil {
		return FindingVerdictResult{}, fmt.Errorf("record finding verdict relationship: %w", err)
	}
	if run != nil {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, 'run', ?, 'verdict', ?, 'records', 'recorded by finding verdict', 'command', ?, ?)
ON CONFLICT(id) DO NOTHING
`, stableMigrationID("relationship", projectID, "run", run.ID, "records", "verdict", verdictID), projectID, run.ID, verdictID, now, now); err != nil {
			return FindingVerdictResult{}, fmt.Errorf("record run verdict relationship: %w", err)
		}
	}
	eventID := stableMigrationID("event", projectID, "finding", finding.ID, "status", finding.Status, normalized.Outcome)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'finding', ?, 'status_changed', ?, ?, 'recorded by finding verdict', ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, finding.ID, finding.Status, normalized.Outcome, now, now); err != nil {
		return FindingVerdictResult{}, fmt.Errorf("record finding verdict event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return FindingVerdictResult{}, fmt.Errorf("commit verdict transaction: %w", err)
	}

	updated, err := s.findingDetail(ctx, root, projectID, finding)
	if err != nil {
		return FindingVerdictResult{}, err
	}
	verdict, err := s.verdictDetail(ctx, projectID, verdictID)
	if err != nil {
		return FindingVerdictResult{}, err
	}
	return FindingVerdictResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Finding:            updated,
		Verdict:            verdict,
		EventID:            eventID,
	}, nil
}

func (s *Store) normalizeFindingCreateOptions(ctx context.Context, projectID string, options FindingCreateOptions) (TraceEntity, *TraceEntity, FindingCreateOptions, error) {
	reportRef := strings.TrimSpace(options.Report)
	if reportRef == "" {
		return TraceEntity{}, nil, FindingCreateOptions{}, fmt.Errorf("finding create requires --report")
	}
	report, err := s.resolveTraceEntity(ctx, projectID, reportRef)
	if err != nil {
		return TraceEntity{}, nil, FindingCreateOptions{}, err
	}
	if report.Kind != "report" {
		return TraceEntity{}, nil, FindingCreateOptions{}, fmt.Errorf("--report %q resolves to %s, not report", options.Report, report.Kind)
	}
	var run *TraceEntity
	if strings.TrimSpace(options.Run) != "" {
		resolved, err := s.resolveTraceEntity(ctx, projectID, options.Run)
		if err != nil {
			return TraceEntity{}, nil, FindingCreateOptions{}, err
		}
		if resolved.Kind != "run" {
			return TraceEntity{}, nil, FindingCreateOptions{}, fmt.Errorf("--run %q resolves to %s, not run", options.Run, resolved.Kind)
		}
		run = &resolved
	}
	normalized := options
	normalized.Title = strings.TrimSpace(options.Title)
	if normalized.Title == "" {
		return TraceEntity{}, nil, FindingCreateOptions{}, fmt.Errorf("finding create requires --title")
	}
	normalized.Status = firstNonEmpty(strings.TrimSpace(options.Status), "open")
	if !ValidFindingStatus(normalized.Status) {
		return TraceEntity{}, nil, FindingCreateOptions{}, fmt.Errorf("invalid finding status %q", normalized.Status)
	}
	normalized.Severity = firstNonEmpty(strings.TrimSpace(options.Severity), "medium")
	if !ValidFindingSeverity(normalized.Severity) {
		return TraceEntity{}, nil, FindingCreateOptions{}, fmt.Errorf("invalid finding severity %q", normalized.Severity)
	}
	normalized.Confidence = firstNonEmpty(strings.TrimSpace(options.Confidence), "medium")
	if !ValidFindingConfidence(normalized.Confidence) {
		return TraceEntity{}, nil, FindingCreateOptions{}, fmt.Errorf("invalid finding confidence %q", normalized.Confidence)
	}
	normalized.Dimension = strings.TrimSpace(options.Dimension)
	normalized.Path = strings.TrimSpace(options.Path)
	normalized.Symbol = strings.TrimSpace(options.Symbol)
	normalized.Metadata = strings.TrimSpace(options.Metadata)
	if normalized.Metadata != "" && !json.Valid([]byte(normalized.Metadata)) {
		return TraceEntity{}, nil, FindingCreateOptions{}, fmt.Errorf("finding metadata must be valid JSON")
	}
	if normalized.LineEnd > 0 && normalized.LineStart > 0 && normalized.LineEnd < normalized.LineStart {
		return TraceEntity{}, nil, FindingCreateOptions{}, fmt.Errorf("finding line_end cannot be before line_start")
	}
	normalized.Body = strings.TrimSpace(options.Body)
	normalized.SetBody = options.SetBody && normalized.Body != ""
	return report, run, normalized, nil
}

func (s *Store) normalizeFindingListOptions(ctx context.Context, projectID string, options FindingListOptions) (FindingListOptions, error) {
	normalized := options
	if strings.TrimSpace(options.Report) != "" {
		report, err := s.resolveTraceEntity(ctx, projectID, options.Report)
		if err != nil {
			return FindingListOptions{}, err
		}
		if report.Kind != "report" {
			return FindingListOptions{}, fmt.Errorf("--report %q resolves to %s, not report", options.Report, report.Kind)
		}
		normalized.Report = report.ID
	}
	if strings.TrimSpace(options.Run) != "" {
		run, err := s.resolveTraceEntity(ctx, projectID, options.Run)
		if err != nil {
			return FindingListOptions{}, err
		}
		if run.Kind != "run" {
			return FindingListOptions{}, fmt.Errorf("--run %q resolves to %s, not run", options.Run, run.Kind)
		}
		normalized.Run = run.ID
	}
	normalized.Status = strings.TrimSpace(options.Status)
	if normalized.Status != "" && !ValidFindingStatus(normalized.Status) {
		return FindingListOptions{}, fmt.Errorf("invalid finding status %q", normalized.Status)
	}
	normalized.Severity = strings.TrimSpace(options.Severity)
	if normalized.Severity != "" && !ValidFindingSeverity(normalized.Severity) {
		return FindingListOptions{}, fmt.Errorf("invalid finding severity %q", normalized.Severity)
	}
	normalized.Confidence = strings.TrimSpace(options.Confidence)
	if normalized.Confidence != "" && !ValidFindingConfidence(normalized.Confidence) {
		return FindingListOptions{}, fmt.Errorf("invalid finding confidence %q", normalized.Confidence)
	}
	normalized.Dimension = strings.TrimSpace(options.Dimension)
	return normalized, nil
}

func (s *Store) normalizeVerdictOptions(ctx context.Context, projectID string, options FindingVerdictOptions) (TraceEntity, *TraceEntity, FindingVerdictOptions, error) {
	if strings.TrimSpace(options.Finding) == "" {
		return TraceEntity{}, nil, FindingVerdictOptions{}, fmt.Errorf("finding verdict requires a finding")
	}
	finding, err := s.resolveTraceEntity(ctx, projectID, options.Finding)
	if err != nil {
		return TraceEntity{}, nil, FindingVerdictOptions{}, err
	}
	if finding.Kind != "finding" {
		return TraceEntity{}, nil, FindingVerdictOptions{}, fmt.Errorf("finding verdict target %q resolved to %s, not finding", options.Finding, finding.Kind)
	}
	var run *TraceEntity
	if strings.TrimSpace(options.Run) != "" {
		resolved, err := s.resolveTraceEntity(ctx, projectID, options.Run)
		if err != nil {
			return TraceEntity{}, nil, FindingVerdictOptions{}, err
		}
		if resolved.Kind != "run" {
			return TraceEntity{}, nil, FindingVerdictOptions{}, fmt.Errorf("--run %q resolves to %s, not run", options.Run, resolved.Kind)
		}
		run = &resolved
	}
	normalized := options
	normalized.Outcome = strings.TrimSpace(options.Outcome)
	if !ValidVerdictOutcome(normalized.Outcome) {
		return TraceEntity{}, nil, FindingVerdictOptions{}, fmt.Errorf("invalid verdict outcome %q", normalized.Outcome)
	}
	normalized.Rationale = strings.TrimSpace(options.Rationale)
	if normalized.Rationale == "" {
		return TraceEntity{}, nil, FindingVerdictOptions{}, fmt.Errorf("finding verdict requires --rationale")
	}
	normalized.ReproductionNotes = strings.TrimSpace(options.ReproductionNotes)
	normalized.Metadata = strings.TrimSpace(options.Metadata)
	if normalized.Metadata != "" && !json.Valid([]byte(normalized.Metadata)) {
		return TraceEntity{}, nil, FindingVerdictOptions{}, fmt.Errorf("verdict metadata must be valid JSON")
	}
	return finding, run, normalized, nil
}

func (s *Store) findingDetail(ctx context.Context, root project.Root, projectID string, entity TraceEntity) (FindingDetail, error) {
	var reportID, title, status, severity, confidence, createdAt, updatedAt string
	var runID, dimension, path, symbol, metadata sql.NullString
	var lineStart, lineEnd sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT report_id, run_id, title, status, severity, confidence, dimension, path, line_start, line_end, symbol, metadata, created_at, updated_at
FROM findings
WHERE project_id = ? AND id = ?
`, projectID, entity.ID).Scan(&reportID, &runID, &title, &status, &severity, &confidence, &dimension, &path, &lineStart, &lineEnd, &symbol, &metadata, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FindingDetail{}, fmt.Errorf("finding %q not found in SQLite state", firstNonEmpty(entity.Alias, entity.ID))
	}
	if err != nil {
		return FindingDetail{}, fmt.Errorf("read finding %s: %w", entity.ID, err)
	}
	alias := firstNonEmpty(entity.Alias)
	if alias == "" {
		if found, err := s.entityAlias(ctx, projectID, "finding", entity.ID); err == nil {
			alias = found
		}
	}
	report, err := s.entityDetails(ctx, projectID, "report", reportID)
	if err != nil {
		return FindingDetail{}, err
	}
	if reportAlias, err := s.entityAlias(ctx, projectID, "report", reportID); err == nil {
		report.Alias = reportAlias
	}
	var run *TraceEntity
	if runID.Valid && runID.String != "" {
		runEntity, err := s.entityDetails(ctx, projectID, "run", runID.String)
		if err != nil {
			return FindingDetail{}, err
		}
		if runAlias, err := s.entityAlias(ctx, projectID, "run", runID.String); err == nil {
			runEntity.Alias = runAlias
		}
		run = &runEntity
	}
	body, err := s.artifactBodyOrSourceBody(ctx, root.Path(), projectID, "finding", entity.ID, sql.NullString{})
	if err != nil {
		return FindingDetail{}, err
	}
	verdicts, err := s.findingVerdicts(ctx, projectID, entity.ID)
	if err != nil {
		return FindingDetail{}, err
	}
	relationships, err := s.traceRelationships(ctx, projectID, TraceEntity{Kind: "finding", ID: entity.ID, Alias: alias, Title: title, Status: status})
	if err != nil {
		return FindingDetail{}, err
	}
	return FindingDetail{
		ID:            entity.ID,
		Alias:         alias,
		Report:        report,
		Run:           run,
		Title:         title,
		Status:        status,
		Severity:      severity,
		Confidence:    confidence,
		Dimension:     dimension.String,
		Path:          filepath.ToSlash(path.String),
		LineStart:     int(lineStart.Int64),
		LineEnd:       int(lineEnd.Int64),
		Symbol:        symbol.String,
		Metadata:      metadata.String,
		Body:          body,
		Verdicts:      verdicts,
		Relationships: relationships,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}

func (s *Store) findingVerdicts(ctx context.Context, projectID string, findingID string) ([]VerdictDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id
FROM verdicts
WHERE project_id = ? AND finding_id = ?
ORDER BY created_at, id
`, projectID, findingID)
	if err != nil {
		return nil, fmt.Errorf("query finding verdicts: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan finding verdict id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close finding verdicts: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate finding verdicts: %w", err)
	}
	verdicts := make([]VerdictDetail, 0, len(ids))
	for _, id := range ids {
		verdict, err := s.verdictDetail(ctx, projectID, id)
		if err != nil {
			return nil, err
		}
		verdicts = append(verdicts, verdict)
	}
	return verdicts, nil
}

func (s *Store) verdictDetail(ctx context.Context, projectID string, verdictID string) (VerdictDetail, error) {
	var findingID, outcome, rationale, createdAt, updatedAt string
	var runID, reproductionNotes, metadata sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT finding_id, run_id, outcome, rationale, reproduction_notes, metadata, created_at, updated_at
FROM verdicts
WHERE project_id = ? AND id = ?
`, projectID, verdictID).Scan(&findingID, &runID, &outcome, &rationale, &reproductionNotes, &metadata, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return VerdictDetail{}, fmt.Errorf("verdict %q not found in SQLite state", verdictID)
	}
	if err != nil {
		return VerdictDetail{}, fmt.Errorf("read verdict %s: %w", verdictID, err)
	}
	finding, err := s.entityDetails(ctx, projectID, "finding", findingID)
	if err != nil {
		return VerdictDetail{}, err
	}
	if alias, err := s.entityAlias(ctx, projectID, "finding", findingID); err == nil {
		finding.Alias = alias
	}
	var run *TraceEntity
	if runID.Valid && runID.String != "" {
		runEntity, err := s.entityDetails(ctx, projectID, "run", runID.String)
		if err != nil {
			return VerdictDetail{}, err
		}
		if alias, err := s.entityAlias(ctx, projectID, "run", runID.String); err == nil {
			runEntity.Alias = alias
		}
		run = &runEntity
	}
	return VerdictDetail{
		ID:                verdictID,
		Finding:           finding,
		Run:               run,
		Outcome:           outcome,
		Rationale:         rationale,
		ReproductionNotes: reproductionNotes.String,
		Metadata:          metadata.String,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}, nil
}

func (s *Store) nextFindingAlias(ctx context.Context, tx *sql.Tx, projectID string, title string, now time.Time) (string, error) {
	slug := normalizeSparkSlug(title)
	if slug == "" {
		slug = "finding"
	}
	prefix := "FINDING-" + now.UTC().Format("20060102") + "-" + slug
	for next := 1; ; next++ {
		alias := prefix
		if next > 1 {
			alias = fmt.Sprintf("%s-%d", prefix, next)
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = 'finding' AND alias = ?`, projectID, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("check finding alias %s: %w", alias, err)
		}
	}
}

func intToNil(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

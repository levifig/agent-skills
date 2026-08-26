package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	JournalEntryTypeQuestion   = "question"
	JournalEntryTypeResolution = "resolution"

	DecisionReHomeOriginKind = "decision-rehome"
)

// DecisionRecordOptions describes a ledger-stored decision question.
type DecisionRecordOptions struct {
	Title  string
	Body   string
	Parent string
	Scope  string
}

// DecisionRecord is the ledger projection for a decision question.
type DecisionRecord struct {
	ContractVersion    int    `json:"contract_version,omitempty"`
	DatabaseScope      string `json:"database_scope,omitempty"`
	DatabasePath       string `json:"database_path,omitempty"`
	ProjectID          string `json:"project_id,omitempty"`
	ProjectName        string `json:"project_name,omitempty"`
	ProjectCurrentPath string `json:"project_current_path,omitempty"`
	QuestionID         string `json:"question_id"`
	ResolutionID       string `json:"resolution_id,omitempty"`
	Scope              string `json:"scope"`
	Question           string `json:"question"`
	Resolution         string `json:"resolution,omitempty"`
	Kind               string `json:"kind"`
}

// DecisionReHomeOptions controls migration of one decision-kind issue row.
type DecisionReHomeOptions struct {
	Ref        string
	Resolution string
	DryRun     bool
}

// DecisionReHomeResult reports ledger facts created from a retired issue row.
type DecisionReHomeResult struct {
	ContractVersion    int    `json:"contract_version,omitempty"`
	DatabaseScope      string `json:"database_scope,omitempty"`
	DatabasePath       string `json:"database_path,omitempty"`
	ProjectID          string `json:"project_id,omitempty"`
	ProjectName        string `json:"project_name,omitempty"`
	ProjectCurrentPath string `json:"project_current_path,omitempty"`
	IssueID            string `json:"issue_id"`
	IssueAlias         string `json:"issue_alias,omitempty"`
	QuestionID         string `json:"question_id"`
	ResolutionID       string `json:"resolution_id,omitempty"`
	Scope              string `json:"scope"`
	Question           string `json:"question"`
	Resolution         string `json:"resolution,omitempty"`
	Retired            bool   `json:"retired"`
	Action             string `json:"action"`
}

// RecordDecisionQuestion writes a decision question to the ledger without minting an issue row.
func RecordDecisionQuestion(ctx context.Context, root project.Root, resolver PathResolver, options DecisionRecordOptions) (DecisionRecord, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return DecisionRecord{}, err
	}
	defer store.Close()
	return store.RecordDecisionQuestion(ctx, root, options)
}

func (s *Store) RecordDecisionQuestion(ctx context.Context, root project.Root, options DecisionRecordOptions) (DecisionRecord, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return DecisionRecord{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return DecisionRecord{}, err
	}
	question, err := normalizeDecisionQuestion(options.Title, options.Body)
	if err != nil {
		return DecisionRecord{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return DecisionRecord{}, fmt.Errorf("begin decision question: %w", err)
	}
	defer tx.Rollback()

	scope, err := decisionScopeTx(ctx, tx, projectID, options.Scope, options.Parent)
	if err != nil {
		return DecisionRecord{}, err
	}
	nowTime := time.Now().UTC()
	questionID, err := appendDecisionJournalFactTx(ctx, tx, projectID, scope, question, JournalEntryTypeQuestion, nowTime, "", "")
	if err != nil {
		return DecisionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return DecisionRecord{}, fmt.Errorf("commit decision question: %w", err)
	}
	return DecisionRecord{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		QuestionID:         questionID,
		Scope:              scope,
		Question:           question,
		Kind:               IssueKindDecision,
	}, nil
}

// ReHomeDecisionIssue migrates one decision-kind issue row to ledger question/resolution facts.
func ReHomeDecisionIssue(ctx context.Context, root project.Root, resolver PathResolver, options DecisionReHomeOptions) (DecisionReHomeResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return DecisionReHomeResult{}, err
	}
	defer store.Close()
	return store.ReHomeDecisionIssue(ctx, root, options)
}

func (s *Store) ReHomeDecisionIssue(ctx context.Context, root project.Root, options DecisionReHomeOptions) (DecisionReHomeResult, error) {
	ref := strings.TrimSpace(options.Ref)
	if ref == "" {
		return DecisionReHomeResult{}, fmt.Errorf("decision re-home requires an issue ref")
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return DecisionReHomeResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return DecisionReHomeResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return DecisionReHomeResult{}, fmt.Errorf("begin decision re-home: %w", err)
	}
	defer tx.Rollback()

	issueID, issueAlias, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return DecisionReHomeResult{}, err
	}
	issue, err := loadIssueTx(ctx, tx, projectID, issueID)
	if err != nil {
		return DecisionReHomeResult{}, err
	}
	if issue.Kind != IssueKindDecision {
		return DecisionReHomeResult{}, fmt.Errorf("issue %s is kind %q; decision re-home applies only to decision-kind rows", firstNonEmpty(issueAlias, ref), issue.Kind)
	}
	if issue.ArchivedAt != "" {
		return DecisionReHomeResult{}, fmt.Errorf("issue %s is already archived", firstNonEmpty(issueAlias, ref))
	}
	question, err := normalizeDecisionQuestion(issue.Title, issue.Body)
	if err != nil {
		return DecisionReHomeResult{}, err
	}
	parentRef := ""
	if issue.ParentID != "" {
		parentRef = issue.ParentID
	}
	scope, err := decisionScopeTx(ctx, tx, projectID, "", parentRef)
	if err != nil {
		return DecisionReHomeResult{}, err
	}
	resolution := strings.TrimSpace(options.Resolution)
	if resolution == "" && issue.Status == IssueStatusDone {
		resolution = decisionResolutionFromBody(issue.Title, issue.Body)
	}

	result := DecisionReHomeResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		IssueID:            issue.ID,
		IssueAlias:         issueAlias,
		Scope:              scope,
		Question:           question,
		Resolution:         resolution,
		Action:             "applied",
	}
	if options.DryRun {
		result.Action = "dry-run"
		return result, nil
	}

	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339Nano)
	originIssueID := issue.ID
	questionID, err := appendDecisionJournalFactTx(ctx, tx, projectID, scope, question, JournalEntryTypeQuestion, nowTime, DecisionReHomeOriginKind, originIssueID)
	if err != nil {
		return DecisionReHomeResult{}, err
	}
	result.QuestionID = questionID

	if resolution != "" {
		resolutionID, err := appendDecisionJournalFactTx(ctx, tx, projectID, scope, resolution, JournalEntryTypeResolution, nowTime, DecisionReHomeOriginKind, originIssueID)
		if err != nil {
			return DecisionReHomeResult{}, err
		}
		result.ResolutionID = resolutionID
	}

	archivedAt := now
	if _, err := tx.ExecContext(ctx, `
UPDATE issues SET status = ?, archived_at = ?, updated_at = ? WHERE project_id = ? AND id = ?
`, IssueStatusCancelled, archivedAt, now, projectID, issue.ID); err != nil {
		return DecisionReHomeResult{}, fmt.Errorf("retire re-homed decision issue: %w", err)
	}
	if issue.Status != IssueStatusCancelled {
		if _, err := insertIssueStatusEventTx(ctx, tx, projectID, issue.ID, issue.Status, IssueStatusCancelled, "re-homed to ledger question/resolution facts", now); err != nil {
			return DecisionReHomeResult{}, err
		}
	}
	result.Retired = true

	if err := tx.Commit(); err != nil {
		return DecisionReHomeResult{}, fmt.Errorf("commit decision re-home: %w", err)
	}
	return result, nil
}

func normalizeDecisionQuestion(title, body string) (string, error) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if !decisionHasQuestion(title, body) {
		return "", &IssueValidationError{Field: "question", Err: fmt.Errorf("decision needs a sharp question (a '?' in the title or body)")}
	}
	switch {
	case title == "":
		return body, nil
	case body == "" || body == title:
		return title, nil
	default:
		return title + "\n\n" + body, nil
	}
}

func decisionHasQuestion(title, body string) bool {
	return strings.Contains(title, "?") || strings.Contains(body, "?")
}

func decisionResolutionFromBody(title, body string) string {
	body = strings.TrimSpace(body)
	title = strings.TrimSpace(title)
	switch {
	case body == "":
		return ""
	case body == title:
		return ""
	case strings.Contains(body, "?"):
		return ""
	default:
		return body
	}
}

func decisionScopeTx(ctx context.Context, tx *sql.Tx, projectID, explicitScope, parentRef string) (string, error) {
	if scope := strings.TrimSpace(explicitScope); scope != "" {
		return scope, nil
	}
	parentRef = strings.TrimSpace(parentRef)
	if parentRef == "" {
		return "project", nil
	}
	if IsAuthorityRef(parentRef) {
		ref, err := ParseAuthorityRef(parentRef)
		if err != nil {
			return "", err
		}
		return ref.String(), nil
	}
	parentID, parentAlias, err := resolveIssueRefTx(ctx, tx, projectID, parentRef)
	if err != nil {
		return "", err
	}
	if authorityRef, ok, err := lookupAuthorityRefForIssueTx(ctx, tx, projectID, parentID); err != nil {
		return "", err
	} else if ok {
		return authorityRef.String(), nil
	}
	parent, err := loadIssueTx(ctx, tx, projectID, parentID)
	if err != nil {
		return "", err
	}
	if branch := strings.TrimSpace(parent.StartedBranch); branch != "" {
		return AuthorityRef{Provider: AuthorityProviderBranch, Key: branch}.String(), nil
	}
	if parentAlias != "" {
		return parentAlias, nil
	}
	return "project", nil
}

func lookupAuthorityRefForIssueTx(ctx context.Context, tx *sql.Tx, projectID, issueID string) (AuthorityRef, bool, error) {
	var provider, key string
	err := tx.QueryRowContext(ctx, `
SELECT provider, provider_ref
FROM work_contract_mappings
WHERE project_id = ? AND mapping_kind = ? AND mapping_value = ?
LIMIT 1
`, projectID, RenderOutMappingIssueID, issueID).Scan(&provider, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthorityRef{}, false, nil
	}
	if err != nil {
		return AuthorityRef{}, false, fmt.Errorf("lookup authority ref for issue %q: %w", issueID, err)
	}
	return AuthorityRef{Provider: provider, Key: key}, true, nil
}

func appendDecisionJournalFactTx(ctx context.Context, tx *sql.Tx, projectID, scope, message, entryType string, nowTime time.Time, originKind, originID string) (string, error) {
	now := nowTime.Format(time.RFC3339Nano)
	id, err := mintFactID(nowTime)
	if err != nil {
		return "", err
	}
	payload := JournalFactPayload{
		EntryType: entryType,
		Scope:     scope,
		Message:   message,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := appendJournalFactTx(ctx, tx, projectID, id, payload, nowTime); err != nil {
		return "", err
	}
	if err := insertJournalProjectionTx(ctx, tx, projectID, id, payload); err != nil {
		return "", err
	}
	if err := insertJournalSearchTx(ctx, tx, projectID, id, "", entryType, scope, message); err != nil {
		return "", err
	}
	if originKind != "" && originID != "" {
		if err := insertJournalOriginTx(ctx, tx, id, JournalOriginInput{
			EnvelopeVersion:   JournalOriginEnvelopeVersion,
			CaptureMechanism:  JournalOriginMechanismMigration,
			DurableResultKind: originKind,
			DurableResultID:   originID,
		}); err != nil {
			return "", err
		}
	}
	return id, nil
}

func createDecisionLedgerIssueTx(ctx context.Context, tx *sql.Tx, projectID string, options IssueCreateOptions, now string) (Issue, error) {
	question, err := normalizeDecisionQuestion(options.Title, options.Body)
	if err != nil {
		return Issue{}, err
	}
	scope, err := decisionScopeTx(ctx, tx, projectID, "", options.Parent)
	if err != nil {
		return Issue{}, err
	}
	nowTime, err := time.Parse(time.RFC3339Nano, now)
	if err != nil {
		return Issue{}, fmt.Errorf("parse decision timestamp: %w", err)
	}
	questionID, err := appendDecisionJournalFactTx(ctx, tx, projectID, scope, question, JournalEntryTypeQuestion, nowTime, "", "")
	if err != nil {
		return Issue{}, err
	}
	return Issue{
		ID:        questionID,
		Kind:      IssueKindDecision,
		Title:     strings.TrimSpace(options.Title),
		Body:      options.Body,
		Status:    IssueStatusTriage,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// CreateIssueLegacyDecisionRow inserts a decision-kind issue row for tests and
// legacy migration scenarios. New decision questions should use RecordDecisionQuestion.
func CreateIssueLegacyDecisionRow(ctx context.Context, root project.Root, resolver PathResolver, options IssueCreateOptions) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.CreateIssueLegacyDecisionRow(ctx, root, options)
}

func (s *Store) CreateIssueLegacyDecisionRow(ctx context.Context, root project.Root, options IssueCreateOptions) (Issue, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	issue, err := createLegacyDecisionIssueTx(ctx, tx, projectID, options, now)
	if err != nil {
		return Issue{}, err
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "commit", Err: err}
	}
	return issue, nil
}

func createLegacyDecisionIssueTx(ctx context.Context, tx *sql.Tx, projectID string, options IssueCreateOptions, now string) (Issue, error) {
	title, err := normalizeIssueTitle(options.Title)
	if err != nil {
		return Issue{}, err
	}
	kind, err := normalizeIssueKind(options.Kind)
	if err != nil {
		return Issue{}, err
	}
	if kind != IssueKindDecision {
		return Issue{}, &IssueValidationError{Field: "kind", Err: fmt.Errorf("legacy decision row helper requires decision kind")}
	}
	issueID, err := newOpaqueStateID("issue")
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "id", Err: err}
	}
	parentID := ""
	if strings.TrimSpace(options.Parent) != "" {
		parentID, _, err = resolveIssueRefTx(ctx, tx, projectID, options.Parent)
		if err != nil {
			return Issue{}, err
		}
	}
	alias := strings.TrimSpace(options.Alias)
	if alias == "" {
		alias, err = mintLocalIssueAliasTx(ctx, tx, projectID, now)
		if err != nil {
			return Issue{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO issues (id, project_id, parent_id, kind, title, body, fog, status, archived_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
`, issueID, projectID, emptyToNil(parentID), kind, title, options.Body, emptyToNil(strings.TrimSpace(options.Fog)), IssueStatusTriage, now, now); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "issue", Err: err}
	}
	if alias != "" {
		if err := insertAlias(ctx, tx, projectID, issueEntityKind, issueID, issueNamespace, alias, now); err != nil {
			return Issue{}, &IssueTransactionError{Stage: "alias", Err: err}
		}
	}
	if _, err := insertIssueStatusEventTx(ctx, tx, projectID, issueID, "", IssueStatusTriage, "recorded by legacy decision create", now); err != nil {
		return Issue{}, err
	}
	return loadIssueTx(ctx, tx, projectID, issueID)
}


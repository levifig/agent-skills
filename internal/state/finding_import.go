package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// FindingImportJSONOptions describes a row-shaped finding/verdict import.
type FindingImportJSONOptions struct {
	Report       string
	ReportKind   string
	Source       string
	Run          string
	FindingFiles []string
	VerdictFiles []string
}

// FindingImportJSONResult describes imported and skipped finding/verdict rows.
type FindingImportJSONResult struct {
	ContractVersion    int                     `json:"contract_version,omitempty"`
	DatabaseScope      string                  `json:"database_scope,omitempty"`
	DatabasePath       string                  `json:"database_path,omitempty"`
	ProjectID          string                  `json:"project_id,omitempty"`
	ProjectName        string                  `json:"project_name,omitempty"`
	ProjectCurrentPath string                  `json:"project_current_path,omitempty"`
	Report             TraceEntity             `json:"report"`
	Files              []FindingImportJSONFile `json:"files"`
	FindingsImported   int                     `json:"findings_imported"`
	FindingsSkipped    int                     `json:"findings_skipped"`
	VerdictsImported   int                     `json:"verdicts_imported"`
	VerdictsSkipped    int                     `json:"verdicts_skipped"`
}

// FindingImportJSONFile describes one imported JSON file.
type FindingImportJSONFile struct {
	Path             string `json:"path"`
	Kind             string `json:"kind"`
	Rows             int    `json:"rows"`
	FindingsImported int    `json:"findings_imported,omitempty"`
	FindingsSkipped  int    `json:"findings_skipped,omitempty"`
	VerdictsImported int    `json:"verdicts_imported,omitempty"`
	VerdictsSkipped  int    `json:"verdicts_skipped,omitempty"`
}

// ImportFindingJSON imports row-shaped finding and verdict JSON into initialized SQLite state.
func ImportFindingJSON(ctx context.Context, root project.Root, resolver PathResolver, options FindingImportJSONOptions) (FindingImportJSONResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return FindingImportJSONResult{}, err
	}
	defer store.Close()
	return store.ImportFindingJSON(ctx, root, options)
}

// ImportFindingJSON imports row-shaped finding and verdict JSON into an open store.
func (s *Store) ImportFindingJSON(ctx context.Context, root project.Root, options FindingImportJSONOptions) (FindingImportJSONResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return FindingImportJSONResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return FindingImportJSONResult{}, err
	}
	report, err := s.ensureFindingImportReport(ctx, root, projectID, options)
	if err != nil {
		return FindingImportJSONResult{}, err
	}
	if len(options.FindingFiles) == 0 && len(options.VerdictFiles) == 0 {
		return FindingImportJSONResult{}, fmt.Errorf("finding import-json requires --findings or --verdicts")
	}

	result := FindingImportJSONResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Report:             report,
	}
	for _, path := range options.FindingFiles {
		rows, err := readFindingImportRows(path, "findings")
		if err != nil {
			return FindingImportJSONResult{}, err
		}
		file := FindingImportJSONFile{Path: filepath.ToSlash(path), Kind: "findings", Rows: len(rows)}
		for _, row := range rows {
			imported, finding, err := s.importFindingRow(ctx, root, projectID, report, options.Run, path, row)
			if err != nil {
				return FindingImportJSONResult{}, err
			}
			if imported {
				file.FindingsImported++
				result.FindingsImported++
			} else {
				file.FindingsSkipped++
				result.FindingsSkipped++
			}
			for _, verdictRow := range nestedImportRows(row, "verdicts", "verdict") {
				importedVerdict, err := s.importVerdictRow(ctx, root, projectID, finding, options.Run, path, verdictRow)
				if err != nil {
					return FindingImportJSONResult{}, err
				}
				if importedVerdict {
					file.VerdictsImported++
					result.VerdictsImported++
				} else {
					file.VerdictsSkipped++
					result.VerdictsSkipped++
				}
			}
		}
		result.Files = append(result.Files, file)
	}
	for _, path := range options.VerdictFiles {
		rows, err := readFindingImportRows(path, "verdicts")
		if err != nil {
			return FindingImportJSONResult{}, err
		}
		file := FindingImportJSONFile{Path: filepath.ToSlash(path), Kind: "verdicts", Rows: len(rows)}
		for _, row := range rows {
			finding, err := s.resolveVerdictImportFinding(ctx, projectID, row)
			if err != nil {
				return FindingImportJSONResult{}, err
			}
			imported, err := s.importVerdictRow(ctx, root, projectID, finding, options.Run, path, row)
			if err != nil {
				return FindingImportJSONResult{}, err
			}
			if imported {
				file.VerdictsImported++
				result.VerdictsImported++
			} else {
				file.VerdictsSkipped++
				result.VerdictsSkipped++
			}
		}
		result.Files = append(result.Files, file)
	}
	return result, nil
}

func (s *Store) ensureFindingImportReport(ctx context.Context, root project.Root, projectID string, options FindingImportJSONOptions) (TraceEntity, error) {
	ref := strings.TrimSpace(options.Report)
	if ref == "" {
		return TraceEntity{}, fmt.Errorf("finding import-json requires --report")
	}
	report, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err == nil {
		if report.Kind != "report" {
			return TraceEntity{}, fmt.Errorf("--report %q resolves to %s, not report", options.Report, report.Kind)
		}
		return report, nil
	}
	if !strings.Contains(err.Error(), "not found") {
		return TraceEntity{}, err
	}
	slug := strings.TrimSpace(ref)
	if strings.HasPrefix(strings.ToLower(slug), "report-") {
		slug = slug[len("report-"):]
	}
	source := firstNonEmpty(strings.TrimSpace(options.Source), "finding-import-json")
	created, err := s.CreateReport(ctx, root, ReportCreateOptions{
		Slug:   slug,
		Kind:   firstNonEmpty(strings.TrimSpace(options.ReportKind), "audit"),
		Source: source,
	})
	if err != nil {
		return TraceEntity{}, err
	}
	return created.Report, nil
}

func (s *Store) importFindingRow(ctx context.Context, root project.Root, projectID string, report TraceEntity, runRef string, sourcePath string, row map[string]any) (bool, TraceEntity, error) {
	key := findingImportRowKey(sourcePath, row)
	sourceAlias := importAlias("finding-import", sourcePath, key)
	if existing, ok, err := s.resolveImportAlias(ctx, projectID, "finding-import", sourceAlias); err != nil || ok {
		return false, existing, err
	}

	options := FindingCreateOptions{
		Report:     firstNonEmpty(report.Alias, report.ID),
		Run:        firstNonEmpty(stringField(row, "run", "run_ref", "run_alias"), runRef),
		Title:      firstNonEmpty(stringField(row, "title", "summary", "name", "check", "rule"), key),
		Status:     strings.ToLower(firstNonEmpty(stringField(row, "status"), "open")),
		Severity:   strings.ToLower(firstNonEmpty(stringField(row, "severity", "level"), "medium")),
		Confidence: strings.ToLower(firstNonEmpty(stringField(row, "confidence"), "medium")),
		Dimension:  firstNonEmpty(stringField(row, "dimension", "category", "group"), dimensionFromFindingPath(sourcePath)),
		Path:       stringField(row, "path", "file", "file_path", "filename"),
		LineStart:  intField(row, "line_start", "lineStart", "start_line", "line"),
		LineEnd:    intField(row, "line_end", "lineEnd", "end_line"),
		Symbol:     stringField(row, "symbol", "function", "object"),
		Metadata:   importMetadata(row, sourcePath, key),
		Body:       stringField(row, "body", "details", "detail", "description", "message"),
	}
	options.SetBody = strings.TrimSpace(options.Body) != ""
	created, err := s.CreateFinding(ctx, root, options)
	if err != nil {
		return false, TraceEntity{}, err
	}
	if err := s.upsertImportAlias(ctx, projectID, "finding", created.Finding.ID, "finding-import", sourceAlias); err != nil {
		return false, TraceEntity{}, err
	}
	if genericAlias := genericImportAlias("finding-import", key); genericAlias != sourceAlias {
		if err := s.upsertImportAlias(ctx, projectID, "finding", created.Finding.ID, "finding-import-key", genericAlias); err != nil {
			return false, TraceEntity{}, err
		}
	}
	return true, TraceEntity{Kind: "finding", ID: created.Finding.ID, Alias: created.Finding.Alias, Title: created.Finding.Title, Status: created.Finding.Status}, nil
}

func (s *Store) importVerdictRow(ctx context.Context, root project.Root, projectID string, finding TraceEntity, runRef string, sourcePath string, row map[string]any) (bool, error) {
	key := verdictImportRowKey(sourcePath, finding, row)
	sourceAlias := importAlias("verdict-import", sourcePath, key)
	if _, ok, err := s.resolveImportAlias(ctx, projectID, "verdict-import", sourceAlias); err != nil || ok {
		return false, err
	}
	options := FindingVerdictOptions{
		Finding:           firstNonEmpty(finding.Alias, finding.ID),
		Run:               firstNonEmpty(stringField(row, "run", "run_ref", "run_alias"), runRef),
		Outcome:           strings.ToLower(firstNonEmpty(stringField(row, "outcome", "status", "verdict"), "confirmed")),
		Rationale:         firstNonEmpty(stringField(row, "rationale", "reason", "message", "summary"), "Imported verdict"),
		ReproductionNotes: stringField(row, "reproduction_notes", "reproductionNotes", "notes"),
		Metadata:          importMetadata(row, sourcePath, key),
	}
	created, err := s.RecordFindingVerdict(ctx, root, options)
	if err != nil {
		return false, err
	}
	if err := s.upsertImportAlias(ctx, projectID, "verdict", created.Verdict.ID, "verdict-import", sourceAlias); err != nil {
		return false, err
	}
	if genericAlias := genericImportAlias("verdict-import", key); genericAlias != sourceAlias {
		if err := s.upsertImportAlias(ctx, projectID, "verdict", created.Verdict.ID, "verdict-import-key", genericAlias); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Store) resolveVerdictImportFinding(ctx context.Context, projectID string, row map[string]any) (TraceEntity, error) {
	for _, ref := range []string{
		stringField(row, "finding", "finding_alias", "finding_ref", "target"),
		genericImportAlias("finding-import", stringField(row, "finding_id", "finding_key", "findingId", "findingKey")),
	} {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		entity, err := s.resolveTraceEntity(ctx, projectID, ref)
		if err != nil {
			continue
		}
		if entity.Kind == "finding" {
			return entity, nil
		}
	}
	return TraceEntity{}, fmt.Errorf("verdict import row requires finding, finding_alias, or finding_id")
}

func (s *Store) resolveImportAlias(ctx context.Context, projectID string, namespace string, alias string) (TraceEntity, bool, error) {
	var kind, id string
	err := s.db.QueryRowContext(ctx, `
SELECT entity_kind, entity_id
FROM aliases
WHERE project_id = ? AND namespace = ? AND alias = ?
`, projectID, namespace, alias).Scan(&kind, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return TraceEntity{}, false, nil
	}
	if err != nil {
		return TraceEntity{}, false, fmt.Errorf("resolve import alias %s:%s: %w", namespace, alias, err)
	}
	entity, err := s.entityDetails(ctx, projectID, kind, id)
	if err != nil {
		return TraceEntity{}, false, err
	}
	if canonical, err := s.entityAlias(ctx, projectID, kind, id); err == nil {
		entity.Alias = canonical
	}
	return entity, true, nil
}

func (s *Store) upsertImportAlias(ctx context.Context, projectID string, entityKind string, entityID string, namespace string, alias string) error {
	now := nowUTC()
	id := stableMigrationID("alias", projectID, namespace, alias)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, namespace, alias) DO NOTHING
`, id, projectID, entityKind, entityID, namespace, alias, now, now)
	if err != nil {
		return fmt.Errorf("upsert import alias %s:%s: %w", namespace, alias, err)
	}
	return nil
}

func readFindingImportRows(path string, primaryKey string) ([]map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var decoded any
	if err := json.Unmarshal(content, &decoded); err != nil {
		return nil, fmt.Errorf("parse %s as JSON: %w", path, err)
	}
	rows := extractImportRows(decoded, primaryKey, "items", "rows", "results")
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s does not contain row-shaped JSON", path)
	}
	return rows, nil
}

func extractImportRows(value any, keys ...string) []map[string]any {
	switch typed := value.(type) {
	case []any:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
		return rows
	case map[string]any:
		for _, key := range keys {
			if child, ok := typed[key]; ok {
				if rows := extractImportRows(child, keys...); len(rows) > 0 {
					return rows
				}
			}
		}
		if looksLikeImportRow(typed) {
			return []map[string]any{typed}
		}
		var rows []map[string]any
		mapKeys := make([]string, 0, len(typed))
		for key := range typed {
			mapKeys = append(mapKeys, key)
		}
		sort.Strings(mapKeys)
		for _, key := range mapKeys {
			switch child := typed[key].(type) {
			case map[string]any:
				row := cloneImportRow(child)
				if stringField(row, "key", "id", "finding_id", "finding_key") == "" {
					row["_parent_key"] = key
				}
				if stringField(row, "dimension") == "" {
					row["_parent_dimension"] = key
				}
				rows = append(rows, row)
			case []any:
				for _, item := range child {
					if row, ok := item.(map[string]any); ok {
						cloned := cloneImportRow(row)
						if stringField(cloned, "dimension") == "" {
							cloned["_parent_dimension"] = key
						}
						rows = append(rows, cloned)
					}
				}
			}
		}
		return rows
	default:
		return nil
	}
}

func nestedImportRows(row map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			return extractImportRows(value, "items", "rows", "results")
		}
	}
	return nil
}

func looksLikeImportRow(row map[string]any) bool {
	for _, key := range []string{"title", "summary", "message", "severity", "confidence", "outcome", "verdict", "rationale", "path", "file"} {
		if _, ok := row[key]; ok {
			return true
		}
	}
	return false
}

func cloneImportRow(row map[string]any) map[string]any {
	cloned := make(map[string]any, len(row)+2)
	for key, value := range row {
		cloned[key] = value
	}
	return cloned
}

func findingImportRowKey(sourcePath string, row map[string]any) string {
	if key := stringField(row, "import_key", "key", "id", "finding_id", "finding_key", "alias", "slug", "_parent_key"); key != "" {
		return key
	}
	parts := []string{
		stringField(row, "title", "summary", "message", "name", "check", "rule"),
		stringField(row, "dimension", "category", "group", "_parent_dimension"),
		stringField(row, "path", "file", "file_path", "filename"),
	}
	if line := intField(row, "line_start", "lineStart", "start_line", "line"); line > 0 {
		parts = append(parts, strconv.Itoa(line))
	}
	var nonEmpty []string
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	if len(nonEmpty) == 0 {
		return filepath.Base(sourcePath)
	}
	return strings.Join(nonEmpty, "|")
}

func verdictImportRowKey(sourcePath string, finding TraceEntity, row map[string]any) string {
	if key := stringField(row, "import_key", "key", "id", "verdict_id", "verdict_key", "alias", "slug", "_parent_key"); key != "" {
		return key
	}
	parts := []string{
		firstNonEmpty(finding.Alias, finding.ID),
		stringField(row, "outcome", "status", "verdict"),
		stringField(row, "rationale", "reason", "message", "summary"),
	}
	return firstNonEmpty(strings.Join(parts, "|"), filepath.Base(sourcePath))
}

func importAlias(prefix string, sourcePath string, key string) string {
	return prefix + "-" + normalizeSparkSlug(filepath.Base(sourcePath)) + "-" + shortImportSlug(key)
}

func genericImportAlias(prefix string, key string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	return prefix + "-" + shortImportSlug(key)
}

func shortImportSlug(value string) string {
	slug := normalizeSparkSlug(value)
	if slug == "" {
		slug = importHash(value)
	}
	if len(slug) > 80 {
		slug = slug[:64] + "-" + importHash(value)
	}
	return slug
}

func importHash(value string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(value))
	return strconv.FormatUint(uint64(hash.Sum32()), 36)
}

func dimensionFromFindingPath(path string) string {
	base := filepath.Base(path)
	if strings.HasPrefix(base, "find.") && strings.HasSuffix(base, ".json") {
		return strings.TrimSuffix(strings.TrimPrefix(base, "find."), ".json")
	}
	return ""
}

func importMetadata(row map[string]any, sourcePath string, key string) string {
	metadata := map[string]any{
		"import_source": filepath.ToSlash(sourcePath),
		"import_key":    key,
	}
	if raw, ok := row["metadata"]; ok {
		switch typed := raw.(type) {
		case map[string]any:
			for key, value := range typed {
				metadata[key] = value
			}
		case string:
			var parsed map[string]any
			if json.Unmarshal([]byte(typed), &parsed) == nil {
				for key, value := range parsed {
					metadata[key] = value
				}
			} else if strings.TrimSpace(typed) != "" {
				metadata["source_metadata"] = typed
			}
		}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func stringField(row map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := row[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		case json.Number:
			return typed.String()
		}
	}
	return ""
}

func intField(row map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := row[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil {
				return parsed
			}
		case json.Number:
			parsed, err := typed.Int64()
			if err == nil {
				return int(parsed)
			}
		}
	}
	return 0
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

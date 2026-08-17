package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	IssuePrefixConfigMissingCode  = "issue-prefix-config-missing"
	IssuePrefixConfigDriftCode    = "issue-prefix-config-drift"
	IssueAuthorityConfigDriftCode = "issue-authority-config-drift"
	IssueLinearAuthorityGapCode   = "issue-linear-authority-gap"
	IssueLinearPrefixMissingCode  = "issue-linear-prefix-missing"
	IssueConfigUnreadableCode     = "issue-config-unreadable"

	loafConfigSchemaVersion = "1.0.0"
)

// IssueProjectConfig is the project-owned issue identity in .agents/loaf.json.
// Prefix is the local alias prefix, or the Linear team key when authority is linear.
type IssueProjectConfig struct {
	Authority string `json:"authority,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Present   bool   `json:"present,omitempty"`
}

// IssuePrefixPersistCommand returns the exact persist/apply command for a prefix.
func IssuePrefixPersistCommand(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "loaf issue identity --prefix <PREFIX>"
	}
	return "loaf issue identity --prefix " + prefix
}

// IssueAuthorityCommand returns the exact authority setter.
func IssueAuthorityCommand(authority string) string {
	authority = strings.TrimSpace(authority)
	if authority == "" {
		return "loaf issue identity --authority <local|linear|github>"
	}
	return "loaf issue identity --authority " + authority
}

// LoadIssueProjectConfig reads issue.authority and issue.prefix from loaf.json.
// A missing file or missing issue section is not an error.
func LoadIssueProjectConfig(projectRoot string) (IssueProjectConfig, error) {
	body, err := readIssueProjectConfigFile(projectRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return IssueProjectConfig{}, nil
		}
		return IssueProjectConfig{}, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return IssueProjectConfig{}, fmt.Errorf("parse .agents/loaf.json: %w", err)
	}
	return parseIssueProjectConfig(parsed)
}

func readIssueProjectConfigFile(projectRoot string) ([]byte, error) {
	return os.ReadFile(filepath.Join(projectRoot, ".agents", "loaf.json"))
}

func parseIssueProjectConfig(config map[string]any) (IssueProjectConfig, error) {
	raw, ok := config["issue"]
	if !ok {
		return IssueProjectConfig{}, nil
	}
	section, ok := raw.(map[string]any)
	if !ok {
		return IssueProjectConfig{}, fmt.Errorf("issue must be an object")
	}
	cfg := IssueProjectConfig{Present: true}
	if value, exists := section["authority"]; exists {
		text, ok := value.(string)
		if !ok {
			return cfg, fmt.Errorf("issue.authority must be a string")
		}
		if strings.TrimSpace(text) != "" {
			authority, err := normalizeIssueAuthorityValue(text)
			if err != nil {
				return cfg, err
			}
			cfg.Authority = authority
		}
	}
	if value, exists := section["prefix"]; exists {
		text, ok := value.(string)
		if !ok {
			return cfg, fmt.Errorf("issue.prefix must be a string")
		}
		if strings.TrimSpace(text) != "" {
			prefix, err := normalizeStoredIssuePrefix(text)
			if err != nil {
				return cfg, err
			}
			cfg.Prefix = prefix
		}
	}
	return cfg, nil
}

// IssueProjectConfigFindings validates the issue object on a parsed loaf.json map.
// Missing prefix/authority are warnings; malformed values are errors.
func IssueProjectConfigFindings(config map[string]any) (warnings []string, errors []string) {
	raw, exists := config["issue"]
	if !exists {
		return []string{"issue.prefix is not configured; persist with `loaf issue identity --prefix <PREFIX>`"}, nil
	}
	section, ok := raw.(map[string]any)
	if !ok {
		return nil, []string{"issue must be an object"}
	}
	if value, ok := section["authority"]; ok {
		text, isString := value.(string)
		if !isString {
			errors = append(errors, "issue.authority must be a string")
		} else if strings.TrimSpace(text) != "" {
			if _, err := normalizeIssueAuthorityValue(text); err != nil {
				errors = append(errors, "issue.authority must be local, linear, or github")
			}
		} else {
			warnings = append(warnings, "issue.authority is not configured; local is assumed")
		}
	} else {
		warnings = append(warnings, "issue.authority is not configured; local is assumed")
	}
	if value, ok := section["prefix"]; !ok {
		warnings = append(warnings, "issue.prefix is not configured; persist with `loaf issue identity --prefix <PREFIX>`")
	} else {
		text, isString := value.(string)
		if !isString {
			errors = append(errors, "issue.prefix must be a string")
		} else if strings.TrimSpace(text) == "" {
			warnings = append(warnings, "issue.prefix is not configured; persist with `loaf issue identity --prefix <PREFIX>`")
		} else if _, err := normalizeStoredIssuePrefix(text); err != nil {
			errors = append(errors, "issue.prefix must be a letter-first alphanumeric token")
		}
	}
	return warnings, errors
}

// WriteIssueProjectConfig merges issue.authority and issue.prefix into loaf.json.
// A missing file is created with schema version and initialized timestamp.
func WriteIssueProjectConfig(projectRoot, authority, prefix string) error {
	if strings.TrimSpace(authority) == "" {
		authority = DefaultIssueAuthority
	}
	normalizedAuthority, err := normalizeIssueAuthorityValue(authority)
	if err != nil {
		return err
	}
	normalizedPrefix, err := normalizeStoredIssuePrefix(prefix)
	if err != nil {
		return err
	}
	path := filepath.Join(projectRoot, ".agents", "loaf.json")
	config := map[string]any{}
	body, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		config["version"] = loafConfigSchemaVersion
		config["initialized"] = time.Now().UTC().Format(time.RFC3339)
	case err != nil:
		return fmt.Errorf("read .agents/loaf.json: %w", err)
	default:
		if err := json.Unmarshal(body, &config); err != nil {
			return fmt.Errorf("parse .agents/loaf.json: %w", err)
		}
		if config == nil {
			config = map[string]any{}
		}
	}
	issue, _ := config["issue"].(map[string]any)
	if issue == nil {
		issue = map[string]any{}
	}
	issue["authority"] = normalizedAuthority
	issue["prefix"] = normalizedPrefix
	config["issue"] = issue
	if _, ok := config["version"]; !ok {
		config["version"] = loafConfigSchemaVersion
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create .agents directory: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write .agents/loaf.json: %w", err)
	}
	return nil
}

// ApplyIssueProjectConfig materializes loaf.json issue identity when the project
// has no identity row yet. Existing rows are left for doctor to nag.
func (s *Store) ApplyIssueProjectConfig(ctx context.Context, root project.Root) error {
	cfg, err := LoadIssueProjectConfig(root.Path())
	if err != nil || (cfg.Prefix == "" && cfg.Authority == "") {
		return nil
	}
	_, ok, err := s.LookupIssueIdentity(ctx, root)
	if err != nil || ok {
		return err
	}
	if _, err := s.SetIssueIdentity(ctx, root, IssueIdentityOptions{Authority: cfg.Authority, Prefix: cfg.Prefix}); err != nil {
		return err
	}
	if cfg.Authority == IssueAuthorityLinear && cfg.Prefix != "" {
		projectID, err := s.projectID(ctx, root)
		if err != nil {
			return err
		}
		return s.upsertBackendMapping(ctx, root, backendMapping{
			EntityKind:   "project",
			EntityID:     projectID,
			ExternalKind: linearExternalKindTeam,
			ExternalID:   cfg.Prefix,
			SyncStatus:   linearSyncLinked,
		})
	}
	return nil
}

func configuredIssueIdentity(projectRoot string) (prefix, authority string) {
	cfg, err := LoadIssueProjectConfig(projectRoot)
	if err != nil {
		return "", ""
	}
	return cfg.Prefix, cfg.Authority
}

func resolveIssueIdentityDefaultsTx(ctx context.Context, tx *sql.Tx, projectID string) (prefix, authority string, err error) {
	var name, path string
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(friendly_name, ''), COALESCE(current_path, '')
FROM projects WHERE id = ?
`, projectID).Scan(&name, &path); err != nil {
		return "", "", fmt.Errorf("read project slug for issue prefix: %w", err)
	}
	prefix, authority = configuredIssueIdentity(path)
	if prefix == "" {
		prefix = DeriveIssuePrefix(name, path)
		if prefix == "" {
			prefix = DefaultIssuePrefix
		}
	}
	if authority == "" {
		authority = DefaultIssueAuthority
	}
	return prefix, authority, nil
}

func persistIssueProjectConfig(projectRoot, authority, prefix string) error {
	if strings.TrimSpace(projectRoot) == "" {
		return nil
	}
	return WriteIssueProjectConfig(projectRoot, authority, prefix)
}

func persistAlignedIssueConfigs(ctx context.Context, store *Store, items []IssuePrefixAlignItem) error {
	for _, item := range items {
		proj, err := store.projectIdentity(ctx, item.ProjectID)
		if err != nil {
			return err
		}
		identity, err := store.LookupIssueIdentityForProject(ctx, item.ProjectID)
		authority := DefaultIssueAuthority
		if err == nil && identity.Authority != "" {
			authority = identity.Authority
		}
		if err := persistIssueProjectConfig(proj.CurrentPath, authority, item.ToPrefix); err != nil {
			return fmt.Errorf("rewrite applied; persist .agents/loaf.json failed: %w", err)
		}
	}
	return nil
}

func (s *Store) linearTeamKeyForProject(ctx context.Context, projectID string) (string, error) {
	var key string
	err := s.db.QueryRowContext(ctx, `
SELECT external_id FROM backend_mappings
WHERE project_id = ? AND backend = ? AND entity_kind = 'project' AND entity_id = ? AND external_kind = ?
`, projectID, linearBackend, projectID, linearExternalKindTeam).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read linear team key: %w", err)
	}
	return strings.TrimSpace(key), nil
}

// ReportIssuePrefixConfig compares loaf.json issue identity with the materialized row.
func ReportIssuePrefixConfig(ctx context.Context, root project.Root, resolver PathResolver) (IssuePrefixConfigReport, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IssuePrefixConfigReport{}, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return IssuePrefixConfigReport{}, err
	}
	return store.issuePrefixConfigReport(ctx, projectID)
}

// IssuePrefixConfigReport is the doctor/inspect view of config vs SQLite identity.
type IssuePrefixConfigReport struct {
	Identity            IssueIdentity
	Config              IssueProjectConfig
	HasIdentity         bool
	PrefixDrift         bool
	AuthorityDrift      bool
	MissingConfig       bool
	LinearGap           bool
	LinearPrefixMissing bool
	Unreadable          string
}

func (s *Store) issuePrefixConfigReport(ctx context.Context, projectID string) (IssuePrefixConfigReport, error) {
	report := IssuePrefixConfigReport{}
	identity, err := s.LookupIssueIdentityForProject(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return report, nil
	}
	if err != nil {
		return report, err
	}
	report.Identity = identity
	report.HasIdentity = true
	proj, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return report, err
	}
	cfg, err := LoadIssueProjectConfig(proj.CurrentPath)
	if err != nil {
		report.Unreadable = err.Error()
		return report, nil
	}
	report.Config = cfg
	if cfg.Prefix != "" && cfg.Prefix != identity.Prefix {
		report.PrefixDrift = true
	}
	if cfg.Authority != "" && cfg.Authority != identity.Authority {
		report.AuthorityDrift = true
	}
	if cfg.Prefix == "" {
		report.MissingConfig = true
	}
	linearOn, err := linearIntegrationEnabled(proj.CurrentPath)
	if err != nil {
		linearOn = false
	}
	teamKey, err := s.linearTeamKeyForProject(ctx, projectID)
	if err != nil {
		return report, err
	}
	if identity.Authority != IssueAuthorityLinear && cfg.Authority != IssueAuthorityLinear && (linearOn || teamKey != "") {
		report.LinearGap = true
	}
	if identity.Authority == IssueAuthorityLinear && identity.Prefix == "" && teamKey == "" && cfg.Prefix == "" {
		report.LinearPrefixMissing = true
	}
	return report, nil
}

func (r IssuePrefixConfigReport) diagnostics() []Diagnostic {
	if r.Unreadable != "" {
		return []Diagnostic{{
			Severity: "warn",
			Code:     IssueConfigUnreadableCode,
			Category: RepairCategoryIssueIdentity,
			Policy:   DiagnosticPolicyWarningDrift,
			Message:  r.Unreadable,
		}}
	}
	if !r.HasIdentity {
		return nil
	}
	var diagnostics []Diagnostic
	if r.PrefixDrift {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "warn",
			Code:     IssuePrefixConfigDriftCode,
			Category: RepairCategoryIssueIdentity,
			Policy:   DiagnosticPolicyWarningDrift,
			Message:  fmt.Sprintf("issue prefix in .agents/loaf.json is %s but SQLite has %s; apply with `%s`", r.Config.Prefix, r.Identity.Prefix, IssuePrefixPersistCommand(r.Config.Prefix)),
			Details: map[string]any{
				"config_prefix":   r.Config.Prefix,
				"stored_prefix":   r.Identity.Prefix,
				"preview_command": IssuePrefixPersistCommand(r.Config.Prefix) + " --dry-run",
				"apply_command":   IssuePrefixPersistCommand(r.Config.Prefix),
			},
		})
	}
	if r.AuthorityDrift {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "warn",
			Code:     IssueAuthorityConfigDriftCode,
			Category: RepairCategoryIssueIdentity,
			Policy:   DiagnosticPolicyWarningDrift,
			Message:  fmt.Sprintf("issue.authority in .agents/loaf.json is %s but SQLite has %s; apply with `%s`", r.Config.Authority, r.Identity.Authority, IssueAuthorityCommand(r.Config.Authority)),
			Details: map[string]any{
				"config_authority": r.Config.Authority,
				"stored_authority": r.Identity.Authority,
				"apply_command":    IssueAuthorityCommand(r.Config.Authority),
			},
		})
	}
	if r.MissingConfig && !r.PrefixDrift {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "warn",
			Code:     IssuePrefixConfigMissingCode,
			Category: RepairCategoryIssueIdentity,
			Policy:   DiagnosticPolicyWarningDrift,
			Message:  fmt.Sprintf("issue prefix is not in .agents/loaf.json; persist with `%s`", IssuePrefixPersistCommand(r.Identity.Prefix)),
			Details: map[string]any{
				"stored_prefix":   r.Identity.Prefix,
				"preview_command": IssuePrefixPersistCommand(r.Identity.Prefix) + " --dry-run",
				"apply_command":   IssuePrefixPersistCommand(r.Identity.Prefix),
			},
		})
	}
	if r.LinearGap {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "warn",
			Code:     IssueLinearAuthorityGapCode,
			Category: RepairCategoryIssueIdentity,
			Policy:   DiagnosticPolicyWarningDrift,
			Message:  fmt.Sprintf("Linear is configured but issue authority is %s; set `%s`", r.Identity.Authority, IssueAuthorityCommand(IssueAuthorityLinear)),
			Details: map[string]any{
				"stored_authority": r.Identity.Authority,
				"apply_command":    IssueAuthorityCommand(IssueAuthorityLinear),
			},
		})
	}
	if r.LinearPrefixMissing {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "warn",
			Code:     IssueLinearPrefixMissingCode,
			Category: RepairCategoryIssueIdentity,
			Policy:   DiagnosticPolicyWarningDrift,
			Message:  "issue.authority is linear but no team key/prefix is set; persist with `loaf issue identity --prefix <TEAM_KEY>`",
			Details: map[string]any{
				"apply_command": IssuePrefixPersistCommand(""),
			},
		})
	}
	return diagnostics
}

func (r IssuePrefixConfigReport) DoctorMessage() (string, string) {
	switch {
	case r.Unreadable != "":
		return "Could not read issue identity from .agents/loaf.json", r.Unreadable
	case r.PrefixDrift:
		return fmt.Sprintf("Issue prefix in .agents/loaf.json is %s but SQLite has %s; apply with `%s`", r.Config.Prefix, r.Identity.Prefix, IssuePrefixPersistCommand(r.Config.Prefix)), "Config is the project-owned source of truth"
	case r.AuthorityDrift:
		return fmt.Sprintf("issue.authority in .agents/loaf.json is %s but SQLite has %s; apply with `%s`", r.Config.Authority, r.Identity.Authority, IssueAuthorityCommand(r.Config.Authority)), ""
	case r.LinearGap:
		return fmt.Sprintf("Linear is configured but issue authority is %s; set `%s`", r.Identity.Authority, IssueAuthorityCommand(IssueAuthorityLinear)), ""
	case r.LinearPrefixMissing:
		return "issue.authority is linear but no team key/prefix is set; persist with `loaf issue identity --prefix <TEAM_KEY>`", ""
	case r.MissingConfig:
		return fmt.Sprintf("Issue prefix is not in .agents/loaf.json; persist with `%s`", IssuePrefixPersistCommand(r.Identity.Prefix)), "New projects record this at init; existing projects persist with identity --prefix"
	default:
		return "", ""
	}
}

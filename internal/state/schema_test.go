package state

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var requiredInitialTables = []string{
	"projects",
	"aliases",
	"specs",
	"tasks",
	"ideas",
	"sparks",
	"brainstorms",
	"shaping_drafts",
	"sessions",
	"reports",
	"journal_entries",
	"events",
	"relationships",
	"tags",
	"entity_tags",
	"bundles",
	"bundle_members",
	"sources",
	"backend_mappings",
	"hook_events",
	"exports",
	"session_state_snapshots",
	"schema_migrations",
}

func TestSchemaMigrationsAreOrderedAndChecksummed(t *testing.T) {
	migrations := SchemaMigrations()
	if len(migrations) != 2 {
		t.Fatalf("len(SchemaMigrations()) = %d, want 2", len(migrations))
	}

	for i, migration := range migrations {
		if migration.Version != i+1 {
			t.Fatalf("migration[%d].Version = %d, want %d", i, migration.Version, i+1)
		}
	}
	if migrations[0].Name != "initial_operational_state" {
		t.Fatalf("migration[0].Name = %q, want initial_operational_state", migrations[0].Name)
	}
	if migrations[1].Name != "session_state_snapshots" {
		t.Fatalf("migration[1].Name = %q, want session_state_snapshots", migrations[1].Name)
	}
	for _, migration := range migrations {
		if strings.TrimSpace(migration.SQL) == "" {
			t.Fatalf("migration %d SQL is empty", migration.Version)
		}
		if !strings.HasSuffix(migration.SQL, "\n") {
			t.Fatalf("migration %d SQL should end with a newline for stable checksums", migration.Version)
		}
		if len(migration.Checksum()) != 64 {
			t.Fatalf("migration %d Checksum() length = %d, want 64", migration.Version, len(migration.Checksum()))
		}
		if migration.Checksum() != SchemaMigrations()[migration.Version-1].Checksum() {
			t.Fatalf("migration %d Checksum() is not deterministic", migration.Version)
		}
	}
}

func TestInitialSchemaContainsRequiredTableSet(t *testing.T) {
	got := tableNames(currentSchemaSQL())
	want := append([]string(nil), requiredInitialTables...)
	sort.Strings(want)

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("table names:\n got: %v\nwant: %v", got, want)
	}
}

func TestOperationalTablesHaveStableIDsAndTimestamps(t *testing.T) {
	sql := currentSchemaSQL()
	for _, table := range requiredInitialTables {
		if table == "schema_migrations" {
			continue
		}
		body := tableBody(t, sql, table)
		for _, required := range []string{
			"id TEXT PRIMARY KEY NOT NULL",
			"created_at TEXT NOT NULL",
			"updated_at TEXT NOT NULL",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s is missing %q in:\n%s", table, required, body)
			}
		}
	}
}

func TestInitialSchemaPreservesLineageAndExports(t *testing.T) {
	sql := currentSchemaSQL()

	for table, columns := range map[string][]string{
		"relationships":           {"relationship_type", "from_entity_kind", "to_entity_kind", "reason"},
		"sources":                 {"source_kind", "path", "hash", "imported_at"},
		"exports":                 {"export_kind", "format", "state_version", "generated_at"},
		"session_state_snapshots": {"content", "observed_branch", "observed_worktree"},
	} {
		body := tableBody(t, sql, table)
		for _, column := range columns {
			if !strings.Contains(body, column) {
				t.Fatalf("%s is missing lineage/export column %q", table, column)
			}
		}
	}
}

func TestProjectScopedTablesUseForeignKeys(t *testing.T) {
	sql := currentSchemaSQL()
	for _, table := range requiredInitialTables {
		if table == "projects" || table == "schema_migrations" {
			continue
		}
		body := tableBody(t, sql, table)
		if !strings.Contains(body, "FOREIGN KEY (project_id) REFERENCES projects(id)") {
			t.Fatalf("%s does not constrain project_id to projects(id)", table)
		}
	}
}

func TestInitialSchemaDoesNotDefineSecretStorageColumns(t *testing.T) {
	columnsByTable := schemaColumnNames(t, currentSchemaSQL())
	for _, forbidden := range []string{
		"token",
		"password",
		"key",
		"api_key",
		"secret",
		"credential",
		"refresh",
	} {
		for table, columns := range columnsByTable {
			for _, column := range columns {
				if strings.Contains(column, forbidden) {
					t.Fatalf("%s column %q contains forbidden secret-storage term %q", table, column, forbidden)
				}
			}
		}
	}
}

func TestBackendMappingsStoreOnlyExternalIdentityMetadata(t *testing.T) {
	columns := schemaColumnNames(t, currentSchemaSQL())["backend_mappings"]
	want := []string{
		"id",
		"project_id",
		"backend",
		"entity_kind",
		"entity_id",
		"external_kind",
		"external_id",
		"external_url",
		"sync_status",
		"created_at",
		"updated_at",
	}
	if strings.Join(columns, "\n") != strings.Join(want, "\n") {
		t.Fatalf("backend_mappings columns:\n got: %v\nwant: %v", columns, want)
	}
}

func TestSchemaDocumentationMirrorsExecutableMigration(t *testing.T) {
	sqlDoc := readRepoFile(t, "docs", "schema", "0001_initial.sql")
	if sqlDoc != SchemaMigrations()[0].SQL {
		t.Fatal("docs/schema/0001_initial.sql must match embedded migration 0001 exactly")
	}
	sqlDoc = readRepoFile(t, "docs", "schema", "0002_session_state_snapshots.sql")
	if sqlDoc != SchemaMigrations()[1].SQL {
		t.Fatal("docs/schema/0002_session_state_snapshots.sql must match embedded migration 0002 exactly")
	}

	dbmlDoc := readRepoFile(t, "docs", "schema", "operational-state.dbml")
	mermaidDoc := readRepoFile(t, "docs", "schema", "operational-state.mmd")
	sqlColumnsByTable := schemaColumnNames(t, currentSchemaSQL())
	dbmlColumnsByTable := dbmlColumnNames(t, dbmlDoc)
	mermaidColumnsByTable := mermaidColumnNames(t, mermaidDoc)
	for _, table := range requiredInitialTables {
		if !regexp.MustCompile(`(?m)^Table\s+` + regexp.QuoteMeta(table) + `\s+\{`).MatchString(dbmlDoc) {
			t.Fatalf("operational-state.dbml missing Table %s block", table)
		}
		if !regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(table) + `\s+\{`).MatchString(mermaidDoc) {
			t.Fatalf("operational-state.mmd missing %s entity", table)
		}
		want := strings.Join(sqlColumnsByTable[table], "\n")
		if got := strings.Join(dbmlColumnsByTable[table], "\n"); got != want {
			t.Fatalf("operational-state.dbml %s columns:\n got: %v\nwant: %v", table, dbmlColumnsByTable[table], sqlColumnsByTable[table])
		}
		if got := strings.Join(mermaidColumnsByTable[table], "\n"); got != want {
			t.Fatalf("operational-state.mmd %s columns:\n got: %v\nwant: %v", table, mermaidColumnsByTable[table], sqlColumnsByTable[table])
		}
	}

	wantMermaidRelationships := []string{
		"bundles ||--o{ bundle_members : contains",
		"projects ||--o{ aliases : scopes",
		"projects ||--o{ backend_mappings : scopes",
		"projects ||--o{ brainstorms : scopes",
		"projects ||--o{ bundle_members : scopes",
		"projects ||--o{ bundles : scopes",
		"projects ||--o{ entity_tags : scopes",
		"projects ||--o{ events : scopes",
		"projects ||--o{ exports : scopes",
		"projects ||--o{ hook_events : scopes",
		"projects ||--o{ ideas : scopes",
		"projects ||--o{ journal_entries : scopes",
		"projects ||--o{ relationships : scopes",
		"projects ||--o{ reports : scopes",
		"projects ||--o{ session_state_snapshots : scopes",
		"projects ||--o{ sessions : scopes",
		"projects ||--o{ shaping_drafts : scopes",
		"projects ||--o{ sources : scopes",
		"projects ||--o{ sparks : scopes",
		"projects ||--o{ specs : scopes",
		"projects ||--o{ tags : scopes",
		"projects ||--o{ tasks : scopes",
		"sessions ||--o{ journal_entries : records",
		"sessions ||--o{ session_state_snapshots : summarizes",
		"sources ||--o{ brainstorms : bodies",
		"sources ||--o{ ideas : bodies",
		"sources ||--o{ reports : bodies",
		"sources ||--o{ sessions : bodies",
		"sources ||--o{ shaping_drafts : bodies",
		"sources ||--o{ sparks : origins",
		"sources ||--o{ specs : bodies",
		"sources ||--o{ tasks : bodies",
		"specs ||--o{ journal_entries : contextualizes",
		"specs ||--o{ tasks : contains",
		"tags ||--o{ entity_tags : labels",
		"tasks ||--o{ journal_entries : contextualizes",
	}
	gotMermaidRelationships := mermaidRelationships(t, mermaidDoc)
	if got := strings.Join(gotMermaidRelationships, "\n"); got != strings.Join(wantMermaidRelationships, "\n") {
		t.Fatalf("operational-state.mmd relationships:\n got: %v\nwant: %v", gotMermaidRelationships, wantMermaidRelationships)
	}
}

func tableNames(sql string) []string {
	re := regexp.MustCompile(`(?im)^CREATE TABLE IF NOT EXISTS ([a-z_]+) \(`)
	matches := re.FindAllStringSubmatch(sql, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}
	sort.Strings(names)
	return names
}

func currentSchemaSQL() string {
	var parts []string
	for _, migration := range SchemaMigrations() {
		parts = append(parts, migration.SQL)
	}
	return strings.Join(parts, "\n")
}

func tableBody(t *testing.T, sql string, table string) string {
	t.Helper()
	re := regexp.MustCompile(`(?is)CREATE TABLE IF NOT EXISTS ` + regexp.QuoteMeta(table) + ` \((.*?)\);`)
	match := re.FindStringSubmatch(sql)
	if len(match) != 2 {
		t.Fatalf("table %s not found", table)
	}
	return match[1]
}

func schemaColumnNames(t *testing.T, sql string) map[string][]string {
	t.Helper()
	columnsByTable := make(map[string][]string)
	for _, table := range requiredInitialTables {
		body := tableBody(t, sql, table)
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(line, ","))
			if line == "" {
				continue
			}
			upper := strings.ToUpper(line)
			if strings.HasPrefix(upper, "UNIQUE ") ||
				strings.HasPrefix(upper, "FOREIGN KEY ") ||
				strings.HasPrefix(upper, "PRIMARY KEY ") ||
				strings.HasPrefix(upper, "CHECK ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			columnsByTable[table] = append(columnsByTable[table], strings.ToLower(fields[0]))
		}
	}
	return columnsByTable
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(body)
}

func dbmlColumnNames(t *testing.T, doc string) map[string][]string {
	t.Helper()
	columnsByTable := make(map[string][]string)
	re := regexp.MustCompile(`(?ms)^Table\s+([a-z_]+)\s+\{\n(.*?)^\}`)
	for _, match := range re.FindAllStringSubmatch(doc, -1) {
		table := match[1]
		for _, line := range strings.Split(match[2], "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line == "indexes {" || line == "}" || strings.HasPrefix(line, "(") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			columnsByTable[table] = append(columnsByTable[table], strings.ToLower(fields[0]))
		}
	}
	return columnsByTable
}

func mermaidColumnNames(t *testing.T, doc string) map[string][]string {
	t.Helper()
	columnsByTable := make(map[string][]string)
	re := regexp.MustCompile(`(?ms)^\s{2}([a-z_]+)\s+\{\n(.*?)^\s{2}\}`)
	for _, match := range re.FindAllStringSubmatch(doc, -1) {
		table := match[1]
		for _, line := range strings.Split(match[2], "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			columnsByTable[table] = append(columnsByTable[table], strings.ToLower(fields[1]))
		}
	}
	return columnsByTable
}

func mermaidRelationships(t *testing.T, doc string) []string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^\s{2}([a-z_]+)\s+\|\|--o\{\s+([a-z_]+)\s+:\s+([a-z_]+)\s*$`)
	var relationships []string
	for _, match := range re.FindAllStringSubmatch(doc, -1) {
		relationships = append(relationships, match[1]+" ||--o{ "+match[2]+" : "+match[3])
	}
	sort.Strings(relationships)
	return relationships
}

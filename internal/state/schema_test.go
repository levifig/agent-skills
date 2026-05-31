package state

import (
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
	"schema_migrations",
}

func TestSchemaMigrationsAreOrderedAndChecksummed(t *testing.T) {
	migrations := SchemaMigrations()
	if len(migrations) != 1 {
		t.Fatalf("len(SchemaMigrations()) = %d, want 1", len(migrations))
	}

	migration := migrations[0]
	if migration.Version != 1 {
		t.Fatalf("Version = %d, want 1", migration.Version)
	}
	if migration.Name != "initial_operational_state" {
		t.Fatalf("Name = %q, want initial_operational_state", migration.Name)
	}
	if strings.TrimSpace(migration.SQL) == "" {
		t.Fatal("SQL is empty")
	}
	if !strings.HasSuffix(migration.SQL, "\n") {
		t.Fatal("SQL should end with a newline for stable checksums")
	}
	if len(migration.Checksum()) != 64 {
		t.Fatalf("Checksum() length = %d, want 64", len(migration.Checksum()))
	}
	if migration.Checksum() != SchemaMigrations()[0].Checksum() {
		t.Fatal("Checksum() is not deterministic")
	}
}

func TestInitialSchemaContainsRequiredTableSet(t *testing.T) {
	got := tableNames(SchemaMigrations()[0].SQL)
	want := append([]string(nil), requiredInitialTables...)
	sort.Strings(want)

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("table names:\n got: %v\nwant: %v", got, want)
	}
}

func TestOperationalTablesHaveStableIDsAndTimestamps(t *testing.T) {
	sql := SchemaMigrations()[0].SQL
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
	sql := SchemaMigrations()[0].SQL

	for table, columns := range map[string][]string{
		"relationships": {"relationship_type", "from_entity_kind", "to_entity_kind", "reason"},
		"sources":       {"source_kind", "path", "hash", "imported_at"},
		"exports":       {"export_kind", "format", "state_version", "generated_at"},
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
	sql := SchemaMigrations()[0].SQL
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
	columnsByTable := schemaColumnNames(t, SchemaMigrations()[0].SQL)
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
	columns := schemaColumnNames(t, SchemaMigrations()[0].SQL)["backend_mappings"]
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

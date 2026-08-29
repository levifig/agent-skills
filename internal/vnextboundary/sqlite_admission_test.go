package vnextboundary

import "testing"

func TestDependencyBoundaryAllowsDatabaseSQLInContinuitySQLitePackage(t *testing.T) {
	t.Parallel()

	root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
		"vnext/continuity/sqlite/store.go": `package sqlite

import "database/sql"

func open() *sql.DB { return nil }
`,
	})
	assertNoBoundaryViolations(t, inspectFixtureBoundary(t, root))
}

func TestDependencyBoundaryAllowsBlankNcrucesDriverInExactDriverFile(t *testing.T) {
	t.Parallel()

	root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
		"vnext/continuity/sqlite/driver.go": `package sqlite

import (
	"database/sql"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func open() *sql.DB { return nil }
`,
	})
	assertNoBoundaryViolations(t, inspectFixtureBoundary(t, root))
}

func TestDependencyBoundaryRejectsDatabaseSQLOutsideContinuitySQLitePackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "kernel", path: "vnext/internal/kernel/kernel.go"},
		{name: "continuity root", path: "vnext/continuity/store.go"},
		{name: "nested sqlite package", path: "vnext/continuity/sqlite/nested/store.go"},
		{name: "sqlite prefix spoof", path: "vnext/continuity/sqlitex/store.go"},
		{name: "sqlite suffix spoof", path: "vnext/continuity/sqlite_store.go"},
		{name: "parent sqlite file", path: "vnext/continuity/sqlite.go"},
		{name: "reordered path", path: "vnext/sqlite/continuity/store.go"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				test.path: "package sample\n\nimport _ \"database/sql\"\n",
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), forbiddenImportRule)
		})
	}
}

func TestDependencyBoundaryRejectsNcrucesDriverOutsideExactFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "same package other file", path: "vnext/continuity/sqlite/store.go"},
		{name: "nested driver file", path: "vnext/continuity/sqlite/nested/driver.go"},
		{name: "prefix spoof", path: "vnext/continuity/sqlitex/driver.go"},
		{name: "suffix spoof", path: "vnext/continuity/sqlite/driver_test.go.bak.go"},
		{name: "parent driver file", path: "vnext/continuity/driver.go"},
		{name: "kernel", path: "vnext/internal/kernel/driver.go"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				test.path: "package sample\n\nimport _ \"github.com/ncruces/go-sqlite3/driver\"\n",
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), thirdPartyImportRule)
		})
	}
}

func TestDependencyBoundaryRejectsNonblankNcrucesDriverImport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "default name", body: "package sqlite\n\nimport \"github.com/ncruces/go-sqlite3/driver\"\n"},
		{name: "named import", body: "package sqlite\n\nimport sqlite3 \"github.com/ncruces/go-sqlite3/driver\"\n"},
		{name: "dot import", body: "package sqlite\n\nimport . \"github.com/ncruces/go-sqlite3/driver\"\n"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				"vnext/continuity/sqlite/driver.go": test.body,
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), thirdPartyImportRule)
		})
	}
}

func TestDependencyBoundaryRejectsOtherNcrucesImports(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		importPath string
	}{
		{name: "sqlite module root", importPath: "github.com/ncruces/go-sqlite3"},
		{name: "driver prefix", importPath: "github.com/ncruces/go-sqlite3/driver/x"},
		{name: "driver suffix spoof", importPath: "github.com/ncruces/go-sqlite3/driverfoo"},
		{name: "ext", importPath: "github.com/ncruces/go-sqlite3/ext"},
		{name: "vfs", importPath: "github.com/ncruces/go-sqlite3/vfs"},
		{name: "wasm", importPath: "github.com/ncruces/go-sqlite3-wasm/v2"},
		{name: "julianday", importPath: "github.com/ncruces/julianday"},
		{name: "module root", importPath: "github.com/ncruces"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				"vnext/continuity/sqlite/driver.go": "package sqlite\n\nimport _ \"" + test.importPath + "\"\n",
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), thirdPartyImportRule)
		})
	}
}

func TestDependencyBoundaryRejectsSpoofedDriverImportPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		importPath string
	}{
		{name: "domain suffix", importPath: "example.com/github.com/ncruces/go-sqlite3/driver"},
		{name: "trailing extra segment", importPath: "github.com/ncruces/go-sqlite3/driver/driver"},
		{name: "lookalike org", importPath: "github.com/ncruces-go/sqlite3/driver"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				"vnext/continuity/sqlite/driver.go": "package sqlite\n\nimport _ \"" + test.importPath + "\"\n",
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), thirdPartyImportRule)
		})
	}
}

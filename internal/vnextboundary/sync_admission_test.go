package vnextboundary

import "testing"

func TestDependencyBoundaryAllowsDatabaseSQLInRelaySQLitePackage(t *testing.T) {
	t.Parallel()

	root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
		"vnext/sync/relay/sqlite/store.go": `package sqlite

import "database/sql"

func open() *sql.DB { return nil }
`,
	})
	assertNoBoundaryViolations(t, inspectFixtureBoundary(t, root))
}

func TestDependencyBoundaryAllowsBlankRelaySQLiteDriverInExactFile(t *testing.T) {
	t.Parallel()

	root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
		"vnext/sync/relay/sqlite/driver.go": `package sqlite

import _ "github.com/ncruces/go-sqlite3/driver"
`,
	})
	assertNoBoundaryViolations(t, inspectFixtureBoundary(t, root))
}

func TestDependencyBoundaryRejectsDatabaseSQLOutsideExactSQLitePackages(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"vnext/sync/client/store.go",
		"vnext/sync/relay/store.go",
		"vnext/sync/relay/sqlitex/store.go",
		"vnext/sync/relay/sqlite/nested/store.go",
		"vnext/sync/sqlite/store.go",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				path: "package sample\n\nimport \"database/sql\"\n",
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), forbiddenImportRule)
		})
	}
}

func TestDependencyBoundaryRejectsRelayDriverOutsideExactFile(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"vnext/sync/relay/sqlite/store.go",
		"vnext/sync/relay/sqlite/nested/driver.go",
		"vnext/sync/relay/sqlitex/driver.go",
		"vnext/sync/relay/driver.go",
		"vnext/sync/sqlite/driver.go",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				path: "package sample\n\nimport _ \"github.com/ncruces/go-sqlite3/driver\"\n",
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), thirdPartyImportRule)
		})
	}
}

func TestDependencyBoundaryAllowsXChaChaInExactAdapterFile(t *testing.T) {
	t.Parallel()

	root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
		"vnext/sync/crypto/xchacha.go": `package crypto

import "golang.org/x/crypto/chacha20poly1305"

const keySize = chacha20poly1305.KeySize
`,
	})
	assertNoBoundaryViolations(t, inspectFixtureBoundary(t, root))
}

func TestDependencyBoundaryRejectsXChaChaOutsideExactAdapterFile(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"vnext/sync/crypto/crypto.go",
		"vnext/sync/crypto/nested/xchacha.go",
		"vnext/sync/cryptox/xchacha.go",
		"vnext/sync/xchacha.go",
		"vnext/continuity/sqlite/xchacha.go",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				path: "package sample\n\nimport \"golang.org/x/crypto/chacha20poly1305\"\n",
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), thirdPartyImportRule)
		})
	}
}

func TestDependencyBoundaryRejectsAliasedOrOtherXCryptoImports(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		importName string
		importPath string
	}{
		{name: "named", importName: "aead", importPath: "golang.org/x/crypto/chacha20poly1305"},
		{name: "dot", importName: ".", importPath: "golang.org/x/crypto/chacha20poly1305"},
		{name: "blank", importName: "_", importPath: "golang.org/x/crypto/chacha20poly1305"},
		{name: "module root", importPath: "golang.org/x/crypto"},
		{name: "argon2", importPath: "golang.org/x/crypto/argon2"},
		{name: "prefix spoof", importPath: "golang.org/x/crypto/chacha20poly1305/extra"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			binding := ""
			if test.importName != "" {
				binding = test.importName + " "
			}
			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				"vnext/sync/crypto/xchacha.go": "package crypto\n\nimport " + binding + "\"" + test.importPath + "\"\n",
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), thirdPartyImportRule)
		})
	}
}

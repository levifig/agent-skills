package vnextcontinuitysqlitecontract

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
)

const sqliteSourceRoot = "../../vnext/continuity/sqlite"

func TestContinuitySQLiteContractOracleIsTestOnlyAndImportClosed(t *testing.T) {
	t.Parallel()

	allowedNonStandard := map[string]struct{}{
		"github.com/levifig/loaf/vnext/continuity":        {},
		"github.com/levifig/loaf/vnext/continuity/sqlite": {},
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read contract package: %v", err)
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			t.Errorf("%s is a symlink", entry.Name())
			continue
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			t.Errorf("%s is production code; the oracle must remain test-only", entry.Name())
		}
		file := parseGoFile(t, entry.Name())
		for _, specification := range file.Imports {
			importPath, err := strconv.Unquote(specification.Path.Value)
			if err != nil {
				t.Errorf("unquote import in %s: %v", entry.Name(), err)
				continue
			}
			if _, allowed := allowedNonStandard[importPath]; allowed {
				continue
			}
			if strings.Contains(strings.SplitN(importPath, "/", 2)[0], ".") {
				t.Errorf("%s imports unadmitted non-standard package %q", entry.Name(), importPath)
			}
		}
	}
}

func TestContinuitySQLiteContractHasExactSourceAndAPI(t *testing.T) {
	t.Parallel()

	wantFiles := []string{"driver.go", "filesystem_attributes_windows.go", "filesystem_unix.go", "filesystem_windows.go", "schema.go", "store.go"}
	wantExports := []string{"Open", "Store", "Store.Close"}
	files, exports := inspectSQLiteSource(t)
	if !reflect.DeepEqual(files, wantFiles) {
		t.Fatalf("SQLite production source inventory = %v, want %v", files, wantFiles)
	}
	if !reflect.DeepEqual(exports, wantExports) {
		t.Fatalf("SQLite exported API = %v, want %v", exports, wantExports)
	}
}

func TestContinuitySQLiteContractPinsDriverBoundary(t *testing.T) {
	t.Parallel()

	wantImports := map[string][]string{
		"driver.go":                        {"_:github.com/ncruces/go-sqlite3/driver"},
		"filesystem_attributes_windows.go": {"fmt", "os", "syscall"},
		"filesystem_unix.go":               {"errors", "fmt", "os", "path/filepath", "strings"},
		"filesystem_windows.go":            {"errors", "fmt", "os", "path/filepath", "strings"},
		"schema.go":                        {"crypto/sha256", "database/sql", "encoding/hex", "fmt", "strings"},
		"store.go": {
			"context",
			"database/sql",
			"errors",
			"fmt",
			"github.com/levifig/loaf/vnext/continuity",
			"net/url",
			"os",
			"path/filepath",
			"strings",
			"time",
		},
	}

	for fileName, want := range wantImports {
		file := parseGoFile(t, filepath.Join(sqliteSourceRoot, fileName))
		var got []string
		for _, specification := range file.Imports {
			importPath, err := strconv.Unquote(specification.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", fileName, err)
			}
			if specification.Name != nil {
				importPath = specification.Name.Name + ":" + importPath
			}
			got = append(got, importPath)
		}
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s imports = %v, want %v", fileName, got, want)
		}
	}
}

func TestContinuitySQLiteContractRejectsRawAndExternalAuthoritySurfaces(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(sqliteSourceRoot)
	if err != nil {
		t.Fatalf("read SQLite source root: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(sqliteSourceRoot, entry.Name()))
		if err != nil {
			t.Errorf("read %s: %v", entry.Name(), err)
			continue
		}
		lower := strings.ToLower(string(contents))
		for _, forbidden := range []string{
			"json.rawmessage", " provider", " credential", " secret", " api_key", " oauth",
			" tracker", " linear", " jira", " assignment", " hierarchy", " dependency",
			"definition_of_done", "acceptance_criteria", "retry_queue", "retry queue", " endpoint",
		} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s contains forbidden external authority surface %q", entry.Name(), strings.TrimSpace(forbidden))
			}
		}
		upper := strings.Join(strings.Fields(strings.ToUpper(string(contents))), " ")
		for _, forbidden := range []string{"UPDATE CONTINUITY_FACTS", "DELETE FROM CONTINUITY_FACTS", "DROP TABLE CONTINUITY_FACTS"} {
			if strings.Contains(upper, forbidden) {
				t.Errorf("%s contains forbidden canonical-fact mutation %q", entry.Name(), forbidden)
			}
		}
	}
}

func TestContinuitySQLiteContractPinsExactSchema(t *testing.T) {
	t.Parallel()

	db := openContractDatabase(t)
	defer db.Close()

	var applicationID, userVersion int
	if err := db.QueryRow(`PRAGMA application_id`).Scan(&applicationID); err != nil {
		t.Fatalf("read application id: %v", err)
	}
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatalf("read user version: %v", err)
	}
	if applicationID != 1280267825 || userVersion != 1 {
		t.Fatalf("schema pragmas = application_id %d, user_version %d", applicationID, userVersion)
	}

	var line, checksum string
	var version int
	if err := db.QueryRow(`SELECT schema_line, schema_version, schema_checksum FROM continuity_schema WHERE singleton = 1`).Scan(&line, &version, &checksum); err != nil {
		t.Fatalf("read schema identity: %v", err)
	}
	if line != "vnext" || version != 1 || checksum != "6fcc409d4d49d1f7702e57ea96c493623ef37eec4e1aae6ec888f542532f0004" {
		t.Fatalf("schema identity = %q, %d, %q", line, version, checksum)
	}

	wantObjects := []objectDigest{
		{kind: "index", name: "ix_continuity_facts_project_order", table: "continuity_facts", digest: "576f27dbbd45eb321a02382151c8caa7c3253d13bae84e780bbb8beb3e622c5b"},
		{kind: "index", name: "ix_continuity_facts_subject_order", table: "continuity_facts", digest: "4e442bf181f90b62b48372a200f3eb7ad1ac2863efc36f25d23db11e062d0ab9"},
		{kind: "index", name: "ux_continuity_project_identity", table: "continuity_facts", digest: "53c7715e0a375cff5f23cf76cc860b1df863e89050e451ffcfce720e67d28720"},
		{kind: "table", name: "continuity_facts", table: "continuity_facts", digest: "cc2165e3ec85a50478f7ee550bf4a25db6b3282b56de7b1332a007961c75555f"},
		{kind: "table", name: "continuity_schema", table: "continuity_schema", digest: "d5a4ef2584db92e7ca999ec9b012c14a08c83bb181d8260a49d79d820d1a59dd"},
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", digest: "2efe75c94d4bd6bdce39f25536d6856e61b681c127ddb60b11ae32420b9163da"},
	}
	if got := readObjectDigests(t, db); !reflect.DeepEqual(got, wantObjects) {
		t.Fatalf("schema objects = %#v, want %#v", got, wantObjects)
	}

	wantColumns := map[string][]columnSpec{
		"continuity_schema": {
			{name: "singleton", dataType: "INTEGER", notNull: 1, primaryKey: 1},
			{name: "schema_line", dataType: "TEXT", notNull: 1},
			{name: "schema_version", dataType: "INTEGER", notNull: 1},
			{name: "schema_checksum", dataType: "TEXT", notNull: 1},
		},
		"continuity_facts": {
			{name: "fact_id", dataType: "TEXT", notNull: 1, primaryKey: 1},
			{name: "project_id", dataType: "TEXT", notNull: 1},
			{name: "subject_kind", dataType: "TEXT", notNull: 1},
			{name: "subject_id", dataType: "TEXT", notNull: 1},
			{name: "fact_kind", dataType: "TEXT", notNull: 1},
			{name: "payload_version", dataType: "INTEGER", notNull: 1},
			{name: "content_json", dataType: "TEXT", notNull: 1},
			{name: "environment_id", dataType: "TEXT", notNull: 1},
			{name: "environment_sequence", dataType: "INTEGER", notNull: 1},
			{name: "hlc_wall_millis", dataType: "INTEGER", notNull: 1},
			{name: "hlc_logical", dataType: "INTEGER", notNull: 1},
			{name: "envelope_version", dataType: "INTEGER", notNull: 1},
		},
	}
	for tableName, want := range wantColumns {
		if got := readColumns(t, db, tableName); !reflect.DeepEqual(got, want) {
			t.Errorf("%s columns = %#v, want %#v", tableName, got, want)
		}
	}

	wantIndexes := []indexSpec{
		{name: "ix_continuity_facts_project_order", unique: 0, origin: "c", partial: 0, keyColumns: []string{"project_id", "hlc_wall_millis", "hlc_logical", "environment_id", "fact_id"}},
		{name: "ix_continuity_facts_subject_order", unique: 0, origin: "c", partial: 0, keyColumns: []string{"project_id", "subject_kind", "subject_id", "hlc_wall_millis", "hlc_logical", "environment_id", "fact_id"}},
		{name: "sqlite_autoindex_continuity_facts_1", unique: 1, origin: "pk", partial: 0, keyColumns: []string{"fact_id"}},
		{name: "sqlite_autoindex_continuity_facts_2", unique: 1, origin: "u", partial: 0, keyColumns: []string{"project_id", "environment_id", "environment_sequence"}},
		{name: "ux_continuity_project_identity", unique: 1, origin: "c", partial: 1, keyColumns: []string{"project_id"}},
	}
	if got := readIndexes(t, db); !reflect.DeepEqual(got, wantIndexes) {
		t.Fatalf("fact indexes = %#v, want %#v", got, wantIndexes)
	}

	for _, tableName := range []string{"continuity_schema", "continuity_facts"} {
		rows, err := db.Query(`PRAGMA foreign_key_list(` + tableName + `)`)
		if err != nil {
			t.Fatalf("read %s foreign keys: %v", tableName, err)
		}
		if rows.Next() {
			rows.Close()
			t.Fatalf("%s unexpectedly declares a foreign key", tableName)
		}
		rows.Close()
	}
}

func TestContinuitySQLiteContractPinsOpenSignature(t *testing.T) {
	t.Parallel()

	var open func(string, continuity.EnvironmentID) (*continuitysqlite.Store, error) = continuitysqlite.Open
	var closeable interface{ Close() error } = (*continuitysqlite.Store)(nil)
	if open == nil || closeable == nil {
		t.Fatal("continuity SQLite API is unexpectedly nil")
	}
}

func TestContinuitySQLiteContractPinsStoreRepresentation(t *testing.T) {
	t.Parallel()

	type fieldSpec struct {
		name      string
		typeName  string
		exported  bool
		anonymous bool
	}
	storeType := reflect.TypeOf(continuitysqlite.Store{})
	gotFields := make([]fieldSpec, 0, storeType.NumField())
	for index := 0; index < storeType.NumField(); index++ {
		field := storeType.Field(index)
		gotFields = append(gotFields, fieldSpec{
			name:      field.Name,
			typeName:  field.Type.String(),
			exported:  field.IsExported(),
			anonymous: field.Anonymous,
		})
	}
	wantFields := []fieldSpec{
		{name: "db", typeName: "*sql.DB"},
		{name: "environmentID", typeName: "continuity.EnvironmentID"},
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("Store fields = %#v, want %#v", gotFields, wantFields)
	}

	pointerType := reflect.TypeOf((*continuitysqlite.Store)(nil))
	if pointerType.NumMethod() != 1 {
		t.Fatalf("*Store method count = %d, want 1", pointerType.NumMethod())
	}
	method := pointerType.Method(0)
	if method.Name != "Close" || method.Type.String() != "func(*sqlite.Store) error" {
		t.Fatalf("*Store method = %s %s, want Close with error result", method.Name, method.Type)
	}
}

func TestContinuitySQLiteContractPinsOpenSafetyImplementation(t *testing.T) {
	t.Parallel()

	wantSourceDigests := map[string]string{
		"driver.go":                        "bd8fc591dd2d5c9c2c8f5f9ea6320b913a08ad8b9e1f9243c4e1a93538970a44",
		"filesystem_attributes_windows.go": "1e3b5f12e4debbc5432f2153818862cdc39ec617409e526b750e6db093dcb0a0",
		"filesystem_unix.go":               "4a06e1818660145b50a3a28a7c0b4e994df0eb5fc46bed56d1f424259000b0e2",
		"filesystem_windows.go":            "0770148405c2b7f215a92e3706443bd6bf6438790c926f6c5a18461a09628818",
		"schema.go":                        "5eb35bcf96fed17242202e75b24af1a82555b42c3079a87d4d491d67051d57ab",
		"store.go":                         "24e0f153ff6f36d1f827f5034e5bdc2c27ca760ce67e755987a06b7255eb1ec1",
	}
	for fileName, wantDigest := range wantSourceDigests {
		gotDigest := digestSourceFile(t, filepath.Join(sqliteSourceRoot, fileName))
		if gotDigest != wantDigest {
			t.Errorf("%s source digest = %q, want %q", fileName, gotDigest, wantDigest)
		}
	}

	want := map[string]map[string]string{
		"driver.go": {},
		"filesystem_attributes_windows.go": {
			"windowsReparsePoint": "81f5d3403c7fb9f2fbb37255d5061ae761a50c56fadc53b5aa52c048cc7c0440",
		},
		"filesystem_unix.go": {
			"databaseURLPath":                    "a4fa2923449759d339aa49022c0068bf2cc615e18d558762954c9be63da9365a",
			"securePrivateFilePlatform":          "358bedcdc24452ef3951e35358f97c1656f2490fc78cb734c4e7eb0ecb080133",
			"validateExistingUnixPathComponents": "e6c2bc130ca8d6742805bac6a9d7b518f01eecac2f8060ba3c78a2439d2f13a6",
			"validatePrivateDirectoryPlatform":   "73ddadd3b323b7ff9872b82e41b643d2acf5b1229a86714b7dc406b61d0737a6",
			"validatePrivateFilePlatform":        "bce83a861542a75dc778d2b7f252d19820868a18c7e92acc20052ac41abde4fc",
			"validateStateRootLocationPlatform":  "818cc5d8212da884024e801df3c8273bc5d71ff3a3c856d0c05d7c82377af3ef",
			"validateStateRootPlatform":          "d465119f3cd961c818f763b8540fb4544733f08dc5542953ba8fbeb215dacfc7",
		},
		"filesystem_windows.go": {
			"databaseURLPath":                       "6539227ce439034da79cd08ef3df63b5e89821bd0b8200b75d528fc7dda7e35e",
			"isWindowsUNC":                          "9fc97df4a9d797e9d464c94a93891271ec59c83beafa825db11c28e9b58b8737",
			"pathWithinWindowsRoot":                 "8d2439757c50d9ba3e2f43708cdc0c6ef09469c0113be2f0211231b129076237",
			"securePrivateFilePlatform":             "cfb416d3d80180bb04a15e94dc97241c9eb50016aa988af4c488dee89e0f17eb",
			"validateExistingWindowsPathComponents": "362c51d2e892771ea2de3b2a69a5299777bea8d58f843377af4f117e9aa9dfa9",
			"validateObservedWindowsPath":           "655fc09ffbd4d1633c0c74a7214b0e483ca2156b6d72dcc13a06d09fe3884ea8",
			"validatePrivateDirectoryPlatform":      "b62ff8dbdae49190bf22a1779d687fd53d161792a98d8a9f50e5af732ac89d63",
			"validatePrivateFilePlatform":           "9fd50adfb65959ed2c792e0c3c84dc4b97f16fb677494776f04f02331a6ab1d1",
			"validateStateRootLocationPlatform":     "4b10e1964a7edd4c2b5b0b9bd2169f4f3bf7f893d4bd43716a897a4d4bea644b",
			"validateStateRootPlatform":             "db32888248aa99a7a5ee8f72060f1fc1ae8eb6e07e6c9ac7b19aa6f8eebc6751",
			"validateWindowsPathComponents":         "7613a329580b81601f4f6d08cc74abcab3d132504ed9e7ccebb075a7c38a2a0a",
			"windowsUserDataRoot":                   "97b6595bfcf4d216279ef990542b9b02509a1763f09ac0cfdd1898a69b5bc3d9",
		},
		"schema.go": {
			"checksumSchema":          "e697037955f01095488bb1258a6f4ed1107c40fb10407ad2beb7e473e389efdb",
			"expectedSchemaObjects":   "966a8404232d9612b7c59245b2a709786930097715d60b547ecc962f403ed204",
			"initializeSchemaIfEmpty": "373d4af0aac75988cdb891d1494a6d0560fa0959a9f5635d95a6b0d183dbca09",
			"normalizeSQL":            "ba395a0d4dddb73b4b9669ad5a6cd12d28494ba464560910074abbddd0d96e98",
			"validateSchema":          "0ee48b24d251815fa8ba379118493edf06fd843ea4a3ea6ba41d9febf9fb6079",
		},
		"store.go": {
			"Close":                     "5e7af3f47dfc7188a8078f2fa6f02f7d0cd1109be72d119a38c2fae00f6f6558",
			"Open":                      "77457caca0cdd1027505d492281681247ca6e4001e839b2572809dab35d1aeb3",
			"databaseDSN":               "31b0b9b4bd1269c1e5f8c521c17fcac5ed99cfe1609e48e1937744a729788e19",
			"inspectRegularPrivateFile": "177aaa7ec2087d38ca322829f4a54d9c333a51aea70be68823d1d9d34772e64a",
			"openDatabase":              "63865fcf446d578fad398587c9c01c6e7330f6117673125cd4afd351b57c3dfc",
			"prepareDatabaseFile":       "22eb5fd9fe6e793d6f846255ffd83be7e43748b2d41fcac76e4bc6f4322e74fb",
			"preparePrivateDirectory":   "5024c5c56c7bf3b92ba98eaf138d8c78542146bdc992051583772566fb629fa0",
			"retryableOpenError":        "b359c6f459c7490b1f8aed8df8e3f137456003c3a1d5b0a842cc639f53e437ca",
			"schemaIsEmpty":             "d96549881a4a329b5815e73d4b144cbe6b8c3cb49a56b5e98c7736d7532ac4fc",
			"secureSQLiteSidecars":      "07822026b1400cdb4eca86b9da2a5feac0cb18bca51e0f9d8c0a2b276671d410",
			"validOpaqueID":             "c0072295d2bd1701a690020b9a9b3791eba2c22429671460ffac401896d1bdb1",
			"validateEnvironmentID":     "6f4006fba6c237bb97b739529c6a82f739cf3a75758fa97625b36da20389fb67",
			"verifySQLiteFiles":         "32e01c16e83df7de6a0034055df7ff29d169823a2d268fc412b824609ad8891d",
		},
	}

	for fileName, wantFunctions := range want {
		got := digestFunctions(t, filepath.Join(sqliteSourceRoot, fileName))
		if len(got) != len(wantFunctions) {
			t.Errorf("%s function inventory = %#v, want %#v", fileName, got, wantFunctions)
		}
		for functionName, wantDigest := range wantFunctions {
			if got[functionName] != wantDigest {
				t.Errorf("%s %s digest = %q, want %q", fileName, functionName, got[functionName], wantDigest)
			}
		}
	}
}

func TestContinuitySQLiteContractPinsOpenBehavior(t *testing.T) {
	t.Parallel()

	t.Run("special URI path and persistent WAL", func(t *testing.T) {
		stateRoot := filepath.Join(contractTempDir(t), "state #% ü")
		store, err := continuitysqlite.Open(stateRoot, "oracle-environment")
		if err != nil {
			t.Fatalf("open continuity store: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("close continuity store: %v", err)
		}

		db := openContractDatabaseAt(t, stateRoot, "ro")
		defer db.Close()
		var journalMode string
		if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
			t.Fatalf("read journal mode: %v", err)
		}
		if journalMode != "wal" {
			t.Fatalf("journal mode = %q, want wal", journalMode)
		}

		if runtime.GOOS != "windows" {
			assertContractMode(t, filepath.Join(stateRoot, "vnext"), 0o700)
			assertContractMode(t, filepath.Join(stateRoot, "vnext", "continuity.sqlite"), 0o600)
		}
	})

	t.Run("invalid paths", func(t *testing.T) {
		root := filepath.Join(contractTempDir(t), "state")
		assertContractOpenRefused(t, "relative")
		assertContractOpenRefused(t, root+string(filepath.Separator)+".")
	})

	t.Run("symlinked state surfaces", func(t *testing.T) {
		t.Run("state root", func(t *testing.T) {
			link := filepath.Join(contractTempDir(t), "state-link")
			if err := os.Symlink(contractTempDir(t), link); err != nil {
				t.Skipf("create symlink: %v", err)
			}
			assertContractOpenRefused(t, link)
		})

		t.Run("state root ancestor", func(t *testing.T) {
			parent := contractTempDir(t)
			target := contractTempDir(t)
			link := filepath.Join(parent, "linked-parent")
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("create symlink: %v", err)
			}
			assertContractOpenRefused(t, filepath.Join(link, "new-state"))
			if _, err := os.Lstat(filepath.Join(target, "new-state")); !os.IsNotExist(err) {
				t.Fatalf("Open created state through a symlink ancestor: %v", err)
			}
		})

		t.Run("private directory", func(t *testing.T) {
			root := contractTempDir(t)
			if err := os.Symlink(contractTempDir(t), filepath.Join(root, "vnext")); err != nil {
				t.Skipf("create symlink: %v", err)
			}
			assertContractOpenRefused(t, root)
		})

		t.Run("database", func(t *testing.T) {
			root := contractTempDir(t)
			privateDirectory := filepath.Join(root, "vnext")
			if err := os.Mkdir(privateDirectory, 0o700); err != nil {
				t.Fatalf("create private directory: %v", err)
			}
			target := filepath.Join(contractTempDir(t), "target.sqlite")
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
			if err != nil {
				t.Fatalf("create target: %v", err)
			}
			file.Close()
			if err := os.Symlink(target, filepath.Join(privateDirectory, "continuity.sqlite")); err != nil {
				t.Skipf("create symlink: %v", err)
			}
			assertContractOpenRefused(t, root)
		})

		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			suffix := suffix
			t.Run("sidecar "+suffix, func(t *testing.T) {
				root := contractTempDir(t)
				privateDirectory := filepath.Join(root, "vnext")
				if err := os.Mkdir(privateDirectory, 0o700); err != nil {
					t.Fatalf("create private directory: %v", err)
				}
				databasePath := filepath.Join(privateDirectory, "continuity.sqlite")
				file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
				if err != nil {
					t.Fatalf("create database: %v", err)
				}
				file.Close()
				if err := os.Symlink(filepath.Join(contractTempDir(t), "target"), databasePath+suffix); err != nil {
					t.Skipf("create symlink: %v", err)
				}
				assertContractOpenRefused(t, root)
			})
		}
	})

	t.Run("insecure Unix modes", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX mode contract")
		}

		t.Run("state root", func(t *testing.T) {
			root := filepath.Join(contractTempDir(t), "state")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatalf("create state root: %v", err)
			}
			if err := os.Chmod(root, 0o770); err != nil {
				t.Fatalf("weaken state root: %v", err)
			}
			assertContractOpenRefused(t, root)
		})

		t.Run("private directory", func(t *testing.T) {
			root := contractTempDir(t)
			if err := os.Mkdir(filepath.Join(root, "vnext"), 0o755); err != nil {
				t.Fatalf("create private directory: %v", err)
			}
			assertContractOpenRefused(t, root)
		})

		t.Run("database", func(t *testing.T) {
			root := contractTempDir(t)
			privateDirectory := filepath.Join(root, "vnext")
			if err := os.Mkdir(privateDirectory, 0o700); err != nil {
				t.Fatalf("create private directory: %v", err)
			}
			file, err := os.OpenFile(filepath.Join(privateDirectory, "continuity.sqlite"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
			if err != nil {
				t.Fatalf("create database: %v", err)
			}
			file.Close()
			assertContractOpenRefused(t, root)
		})
	})

	t.Run("concurrent bootstrap", func(t *testing.T) {
		const openers = 8
		stateRoot := filepath.Join(contractTempDir(t), "state")
		start := make(chan struct{})
		errorsByOpener := make([]error, openers)
		var wait sync.WaitGroup
		wait.Add(openers)
		for opener := 0; opener < openers; opener++ {
			opener := opener
			go func() {
				defer wait.Done()
				<-start
				store, err := continuitysqlite.Open(stateRoot, continuity.EnvironmentID(fmt.Sprintf("oracle-%d", opener)))
				if err == nil {
					err = store.Close()
				}
				errorsByOpener[opener] = err
			}()
		}
		close(start)
		wait.Wait()
		for opener, err := range errorsByOpener {
			if err != nil {
				t.Errorf("concurrent Open() caller %d: %v", opener, err)
			}
		}
	})
}

func TestContinuitySQLiteContractMatchesEveryCatalogFactToExactlyOneSubjectKind(t *testing.T) {
	t.Parallel()

	persistedKinds := []continuity.RecordKind{
		continuity.RecordProjectIdentity,
		continuity.RecordJournalEntry,
		continuity.RecordWrap,
		continuity.RecordSpark,
		continuity.RecordIdea,
		continuity.RecordDecision,
		continuity.RecordExploration,
		continuity.RecordCheckpoint,
		continuity.RecordFinding,
		continuity.RecordHandoff,
		continuity.RecordScratchpad,
		continuity.RecordExternalReference,
		continuity.RecordVerificationEvidence,
	}
	definitions := continuity.FactCatalog()
	if len(definitions) != 32 {
		t.Fatalf("fact catalog has %d definitions, want 32", len(definitions))
	}

	for definitionIndex, definition := range definitions {
		definitionIndex, definition := definitionIndex, definition
		t.Run(string(definition.Kind), func(t *testing.T) {
			db := openWritableContractDatabase(t)
			defer db.Close()

			projectID := fmt.Sprintf("correct-project-%d", definitionIndex)
			sequence := 1
			if definition.Kind != continuity.FactProjectRegistered {
				if err := insertContractFact(db, "correct-registration", projectID, continuity.RecordProjectIdentity, continuity.SubjectID(projectID), continuity.FactProjectRegistered, sequence); err != nil {
					t.Fatalf("insert project identity: %v", err)
				}
				sequence++
			}
			subjectID := continuity.SubjectID("correct-subject")
			if definition.Record == continuity.RecordProjectIdentity {
				subjectID = continuity.SubjectID(projectID)
			}
			if err := insertContractFact(db, "correct-fact", projectID, definition.Record, subjectID, definition.Kind, sequence); err != nil {
				t.Fatalf("schema rejected catalog pair (%s, %s): %v", definition.Record, definition.Kind, err)
			}

			for wrongIndex, wrongKind := range persistedKinds {
				if wrongKind == definition.Record {
					continue
				}
				wrongProjectID := fmt.Sprintf("wrong-project-%d-%d", definitionIndex, wrongIndex)
				wrongSequence := 1
				if definition.Kind != continuity.FactProjectRegistered {
					if err := insertContractFact(db, fmt.Sprintf("wrong-registration-%d", wrongIndex), wrongProjectID, continuity.RecordProjectIdentity, continuity.SubjectID(wrongProjectID), continuity.FactProjectRegistered, wrongSequence); err != nil {
						t.Fatalf("insert wrong-case project identity: %v", err)
					}
					wrongSequence++
				}
				wrongSubjectID := continuity.SubjectID(fmt.Sprintf("wrong-subject-%d", wrongIndex))
				if wrongKind == continuity.RecordProjectIdentity {
					wrongSubjectID = continuity.SubjectID(wrongProjectID)
				}
				tx, err := db.Begin()
				if err != nil {
					t.Fatalf("begin wrong-kind probe: %v", err)
				}
				err = insertContractFact(tx, fmt.Sprintf("wrong-fact-%d", wrongIndex), wrongProjectID, wrongKind, wrongSubjectID, definition.Kind, wrongSequence)
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					t.Fatalf("rollback wrong-kind probe: %v", rollbackErr)
				}
				if err == nil {
					t.Errorf("schema accepted %s under wrong subject kind %s; want only %s", definition.Kind, wrongKind, definition.Record)
				}
			}
		})
	}
}

type objectDigest struct {
	kind   string
	name   string
	table  string
	digest string
}

type columnSpec struct {
	name       string
	dataType   string
	notNull    int
	primaryKey int
	hidden     int
}

type indexSpec struct {
	name       string
	unique     int
	origin     string
	partial    int
	keyColumns []string
}

func openContractDatabase(t *testing.T) *sql.DB {
	t.Helper()

	stateRoot := filepath.Join(contractTempDir(t), "state")
	store, err := continuitysqlite.Open(stateRoot, continuity.EnvironmentID("oracle-environment"))
	if err != nil {
		t.Fatalf("open continuity store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close continuity store: %v", err)
	}

	return openContractDatabaseAt(t, stateRoot, "ro")
}

func openWritableContractDatabase(t *testing.T) *sql.DB {
	t.Helper()

	stateRoot := filepath.Join(contractTempDir(t), "state")
	store, err := continuitysqlite.Open(stateRoot, continuity.EnvironmentID("oracle-environment"))
	if err != nil {
		t.Fatalf("open continuity store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close continuity store: %v", err)
	}
	return openContractDatabaseAt(t, stateRoot, "rw")
}

func openContractDatabaseAt(t *testing.T, stateRoot, mode string) *sql.DB {
	t.Helper()

	values := url.Values{}
	values.Set("mode", mode)
	databasePath := filepath.Join(stateRoot, "vnext", "continuity.sqlite")
	urlPath := filepath.ToSlash(databasePath)
	if runtime.GOOS == "windows" && filepath.VolumeName(databasePath) != "" && !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     urlPath,
		RawQuery: values.Encode(),
	}).String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open continuity database for inspection: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("ping continuity database for inspection: %v", err)
	}
	return db
}

func contractTempDir(t *testing.T) string {
	t.Helper()

	if runtime.GOOS != "windows" {
		return t.TempDir()
	}
	root, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("resolve LocalAppData contract-test root: %v", err)
	}
	directory, err := os.MkdirTemp(root, "loaf-continuity-contract-")
	if err != nil {
		t.Fatalf("create LocalAppData contract-test directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove LocalAppData contract-test directory: %v", err)
		}
	})
	return directory
}

type contractExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func insertContractFact(executor contractExecer, factID, projectID string, subjectKind continuity.RecordKind, subjectID continuity.SubjectID, factKind continuity.FactKind, sequence int) error {
	_, err := executor.Exec(`
INSERT INTO continuity_facts(
  fact_id,
  project_id,
  subject_kind,
  subject_id,
  fact_kind,
  payload_version,
  content_json,
  environment_id,
  environment_sequence,
  hlc_wall_millis,
  hlc_logical,
  envelope_version
) VALUES(?, ?, ?, ?, ?, 1, '{}', 'oracle-environment', ?, 1, 0, 1)`,
		factID,
		projectID,
		string(subjectKind),
		string(subjectID),
		string(factKind),
		sequence,
	)
	return err
}

func assertContractOpenRefused(t *testing.T, stateRoot string) {
	t.Helper()

	store, err := continuitysqlite.Open(stateRoot, "oracle-environment")
	if err == nil {
		store.Close()
		t.Fatal("Open() error = nil, want refusal")
	}
}

func assertContractMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("inspect %s: %v", path, err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("mode for %s = %o, want %o", path, info.Mode().Perm(), want)
	}
}

func digestFunctions(t *testing.T, path string) map[string]string {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	digests := make(map[string]string)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		var formatted bytes.Buffer
		if err := format.Node(&formatted, fileSet, function); err != nil {
			t.Fatalf("format %s.%s: %v", path, function.Name.Name, err)
		}
		if _, duplicate := digests[function.Name.Name]; duplicate {
			t.Fatalf("%s has multiple functions or methods named %s", path, function.Name.Name)
		}
		digests[function.Name.Name] = fmt.Sprintf("%x", sha256.Sum256(formatted.Bytes()))
	}
	return digests
}

func digestSourceFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(contents))
}

func readObjectDigests(t *testing.T, db *sql.DB) []objectDigest {
	t.Helper()

	rows, err := db.Query(`
SELECT type, name, tbl_name, sql
FROM sqlite_schema
WHERE name NOT LIKE 'sqlite_%'
ORDER BY type, name`)
	if err != nil {
		t.Fatalf("read schema objects: %v", err)
	}
	defer rows.Close()

	var objects []objectDigest
	for rows.Next() {
		var object objectDigest
		var definition string
		if err := rows.Scan(&object.kind, &object.name, &object.table, &definition); err != nil {
			t.Fatalf("scan schema object: %v", err)
		}
		normalized := strings.Join(strings.Fields(definition), " ")
		object.digest = fmt.Sprintf("%x", sha256.Sum256([]byte(normalized)))
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema objects: %v", err)
	}
	return objects
}

func readColumns(t *testing.T, db *sql.DB, tableName string) []columnSpec {
	t.Helper()

	rows, err := db.Query(`PRAGMA table_xinfo(` + tableName + `)`)
	if err != nil {
		t.Fatalf("read %s columns: %v", tableName, err)
	}
	defer rows.Close()

	var columns []columnSpec
	for rows.Next() {
		var column columnSpec
		var cid int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &column.name, &column.dataType, &column.notNull, &defaultValue, &column.primaryKey, &column.hidden); err != nil {
			t.Fatalf("scan %s column: %v", tableName, err)
		}
		if defaultValue.Valid {
			t.Fatalf("%s.%s has unexpected default %q", tableName, column.name, defaultValue.String)
		}
		columns = append(columns, column)
	}
	return columns
}

func readIndexes(t *testing.T, db *sql.DB) []indexSpec {
	t.Helper()

	rows, err := db.Query(`PRAGMA index_list(continuity_facts)`)
	if err != nil {
		t.Fatalf("read fact indexes: %v", err)
	}
	defer rows.Close()

	var indexes []indexSpec
	for rows.Next() {
		var index indexSpec
		var sequence int
		if err := rows.Scan(&sequence, &index.name, &index.unique, &index.origin, &index.partial); err != nil {
			t.Fatalf("scan fact index: %v", err)
		}
		index.keyColumns = readIndexKeyColumns(t, db, index.name)
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate fact indexes: %v", err)
	}
	sort.Slice(indexes, func(left, right int) bool { return indexes[left].name < indexes[right].name })
	return indexes
}

func readIndexKeyColumns(t *testing.T, db *sql.DB, indexName string) []string {
	t.Helper()

	rows, err := db.Query(`PRAGMA index_xinfo(` + indexName + `)`)
	if err != nil {
		t.Fatalf("read %s columns: %v", indexName, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var sequence, cid, descending, key int
		var name sql.NullString
		var collation string
		if err := rows.Scan(&sequence, &cid, &name, &descending, &collation, &key); err != nil {
			t.Fatalf("scan %s column: %v", indexName, err)
		}
		if key == 1 {
			if !name.Valid {
				t.Fatalf("%s has unnamed key column", indexName)
			}
			columns = append(columns, name.String)
		}
	}
	return columns
}

func inspectSQLiteSource(t *testing.T) ([]string, []string) {
	t.Helper()

	entries, err := os.ReadDir(sqliteSourceRoot)
	if err != nil {
		t.Fatalf("read SQLite source root: %v", err)
	}
	var files []string
	var exports []string
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			t.Errorf("%s is a symlink", entry.Name())
			continue
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		files = append(files, entry.Name())
		file := parseGoFile(t, filepath.Join(sqliteSourceRoot, entry.Name()))
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						if !ast.IsExported(specification.Name.Name) {
							continue
						}
						exports = append(exports, specification.Name.Name)
						if structure, ok := specification.Type.(*ast.StructType); ok {
							for _, field := range structure.Fields.List {
								if len(field.Names) == 0 {
									t.Errorf("%s embeds an anonymous field", specification.Name.Name)
									continue
								}
								for _, name := range field.Names {
									if ast.IsExported(name.Name) {
										t.Errorf("%s.%s is an exported field", specification.Name.Name, name.Name)
									}
								}
							}
						}
					case *ast.ValueSpec:
						for _, name := range specification.Names {
							if ast.IsExported(name.Name) {
								exports = append(exports, name.Name)
							}
						}
					}
				}
			case *ast.FuncDecl:
				if !ast.IsExported(declaration.Name.Name) {
					continue
				}
				name := declaration.Name.Name
				if declaration.Recv != nil && len(declaration.Recv.List) == 1 {
					name = receiverName(declaration.Recv.List[0].Type) + "." + name
				}
				exports = append(exports, name)
			}
		}
	}
	sort.Strings(files)
	sort.Strings(exports)
	return files, exports
}

func receiverName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return receiverName(expression.X)
	default:
		return "unknown"
	}
}

func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

package state

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestRelationshipOriginRegistryPinsClosedVocabulary(t *testing.T) {
	// The vocabulary is closed at mechanism level; any drift from the
	// documented values is a contract break, not a refactor.
	wantAllowed := []string{"imported", "manual", "command"}
	if got := allowedRelationshipOrigins(); !reflect.DeepEqual(got, wantAllowed) {
		t.Fatalf("allowedRelationshipOrigins() = %#v, want %#v", got, wantAllowed)
	}

	wantLegacy := map[string]string{
		"intent-create":      "command",
		"legacy-conversion":  "command",
		"exploration-create": "command",
		"system":             "command",
	}
	if got := legacyRelationshipOriginReclassifications(); !reflect.DeepEqual(got, wantLegacy) {
		t.Fatalf("legacyRelationshipOriginReclassifications() = %#v, want %#v", got, wantLegacy)
	}

	fragment, args := relationshipOriginNotAllowedFragment("origin")
	if fragment != "origin NOT IN (?, ?, ?)" {
		t.Fatalf("relationshipOriginNotAllowedFragment() = %q, want parameterized NOT IN over three origins", fragment)
	}
	if !reflect.DeepEqual(args, []any{"imported", "manual", "command"}) {
		t.Fatalf("relationshipOriginNotAllowedFragment() args = %#v, want allowed origins", args)
	}
}

// Source-scan patterns for statements that write the relationships.origin
// column. The scan below keeps Decision 3 executable: the next ceremony cannot
// ship a novel provenance value without failing the parity test.
var (
	relationshipInsertPattern    = regexp.MustCompile(`(?is)insert\s+into\s+relationships\s*\(`)
	relationshipUpdateSetPattern = regexp.MustCompile(`(?is)update\s+relationships\s+set\b`)
	sqlValuesOpenPattern         = regexp.MustCompile(`(?is)^\s*values\s*\(`)
	sqlWherePattern              = regexp.MustCompile(`(?i)\bwhere\b`)
	sqlOriginAssignPattern       = regexp.MustCompile(`(?i)\borigin\s*=\s*`)
)

// relationshipOriginWrite describes one origin write found inside a SQL string
// literal: either an inline SQL literal value, or the ordinal of its `?` among
// every placeholder in the statement for mapping onto the enclosing call's
// bind arguments.
type relationshipOriginWrite struct {
	sqlLiteral  string
	isLiteral   bool
	placeholder int
}

// findRelationshipOriginWrites locates every write to relationships.origin in
// one SQL string: INSERT column/VALUES pairs and UPDATE ... SET assignments.
// Unrecognizable origin expressions are errors so the invariant fails closed
// instead of silently skipping a statement the scan cannot classify.
func findRelationshipOriginWrites(query string) ([]relationshipOriginWrite, error) {
	writes := []relationshipOriginWrite{}
	for _, match := range relationshipInsertPattern.FindAllStringIndex(query, -1) {
		columnsEnd := strings.Index(query[match[1]:], ")")
		if columnsEnd < 0 {
			return nil, fmt.Errorf("unterminated relationships column list in %q", query)
		}
		columns := strings.Split(query[match[1]:match[1]+columnsEnd], ",")
		originIndex := -1
		for i, column := range columns {
			if strings.EqualFold(strings.TrimSpace(column), "origin") {
				originIndex = i
			}
		}
		if originIndex == -1 {
			continue
		}
		afterColumns := match[1] + columnsEnd + 1
		valuesOpen := sqlValuesOpenPattern.FindStringIndex(query[afterColumns:])
		if valuesOpen == nil {
			return nil, fmt.Errorf("origin-writing INSERT without a recognizable VALUES tuple in %q", query)
		}
		items, offsets, err := splitSQLTupleItems(query, afterColumns+valuesOpen[1])
		if err != nil {
			return nil, err
		}
		if originIndex >= len(items) {
			return nil, fmt.Errorf("VALUES tuple shorter than column list in %q", query)
		}
		write, err := classifySQLOriginValue(query, items[originIndex], offsets[originIndex])
		if err != nil {
			return nil, err
		}
		writes = append(writes, write)
	}
	for _, match := range relationshipUpdateSetPattern.FindAllStringIndex(query, -1) {
		clauseEnd := len(query)
		if where := sqlWherePattern.FindStringIndex(query[match[1]:]); where != nil {
			clauseEnd = match[1] + where[0]
		}
		assign := sqlOriginAssignPattern.FindStringIndex(query[match[1]:clauseEnd])
		if assign == nil {
			continue
		}
		valueStart := match[1] + assign[1]
		value := query[valueStart:clauseEnd]
		if comma := indexTopLevelComma(value); comma >= 0 {
			value = value[:comma]
		}
		write, err := classifySQLOriginValue(query, value, valueStart)
		if err != nil {
			return nil, err
		}
		writes = append(writes, write)
	}
	return writes, nil
}

// splitSQLTupleItems splits a parenthesized SQL tuple starting just after its
// opening parenthesis, honoring single-quoted literals and nested parentheses.
// It returns each item trimmed alongside the absolute offset of its first
// non-space character.
func splitSQLTupleItems(query string, start int) ([]string, []int, error) {
	depth := 1
	inQuote := false
	items := []string{}
	offsets := []int{}
	itemStart := start
	record := func(end int) {
		raw := query[itemStart:end]
		leading := len(raw) - len(strings.TrimLeft(raw, " \t\r\n"))
		items = append(items, strings.TrimSpace(raw))
		offsets = append(offsets, itemStart+leading)
	}
	for i := start; i < len(query); i++ {
		switch query[i] {
		case '\'':
			inQuote = !inQuote
		case '(':
			if !inQuote {
				depth++
			}
		case ')':
			if !inQuote {
				depth--
				if depth == 0 {
					record(i)
					return items, offsets, nil
				}
			}
		case ',':
			if !inQuote && depth == 1 {
				record(i)
				itemStart = i + 1
			}
		}
	}
	return nil, nil, fmt.Errorf("unterminated VALUES tuple in %q", query)
}

// indexTopLevelComma returns the offset of the first comma outside quotes and
// parentheses, or -1.
func indexTopLevelComma(fragment string) int {
	depth := 0
	inQuote := false
	for i := 0; i < len(fragment); i++ {
		switch fragment[i] {
		case '\'':
			inQuote = !inQuote
		case '(':
			if !inQuote {
				depth++
			}
		case ')':
			if !inQuote {
				depth--
			}
		case ',':
			if !inQuote && depth == 0 {
				return i
			}
		}
	}
	return -1
}

// classifySQLOriginValue resolves one origin value expression to either its
// inline SQL literal or the ordinal of its placeholder within the statement.
func classifySQLOriginValue(query string, item string, offset int) (relationshipOriginWrite, error) {
	trimmed := strings.TrimSpace(item)
	switch {
	case trimmed == "?":
		return relationshipOriginWrite{placeholder: strings.Count(query[:offset], "?")}, nil
	case len(trimmed) >= 2 && strings.HasPrefix(trimmed, "'") && strings.HasSuffix(trimmed, "'"):
		return relationshipOriginWrite{isLiteral: true, sqlLiteral: trimmed[1 : len(trimmed)-1]}, nil
	default:
		return relationshipOriginWrite{}, fmt.Errorf("unrecognized origin value %q in %q; keep origin values scannable (literal or ?)", trimmed, query)
	}
}

// relationshipOriginRuntimeBinds classifies the origin binds that are runtime
// values by construction and therefore cannot be resolved statically. Each key
// is file:function:identifier — stable across edits, unlike a line number — and
// each entry is a deliberate decision that the named value is validated against
// the registry before it reaches SQL:
//
//   - repair's backfill origin is the operator's --origin selection, rejected by
//     RepairRelationshipOrigins unless it is 'imported' or 'manual'.
//   - repair's reclassification target is read from
//     legacyRelationshipOriginReclassifications, whose values are registry
//     constants pinned by TestRelationshipOriginRegistryPinsClosedVocabulary.
//
// Any other unresolvable bind fails the scan. Adding a key here is the explicit
// way to say "this one is checked elsewhere"; silence is not.
var relationshipOriginRuntimeBinds = map[string]bool{
	"repair.go:backfillMissingRelationshipOrigins:origin": true,
	"repair.go:reclassifyLegacyRelationshipOrigin:target": true,
}

// enclosingFunctionName names the function declaration containing pos, or ""
// for package-level expressions.
func enclosingFunctionName(file *ast.File, pos token.Pos) string {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || pos < funcDecl.Pos() || pos > funcDecl.End() {
			continue
		}
		return funcDecl.Name.Name
	}
	return ""
}

// TestRelationshipOriginSourceLiteralsAreRegistryListed is the registry parity
// scan: every non-test source file in this package is parsed and every origin
// literal written to relationships.origin — inline in SQL, bound as a Go
// string literal, or bound through a package-level string constant — must be
// registry-listed, which also keeps the retired legacy origins from
// reappearing in origin position.
//
// The scan fails closed. A bind it cannot classify — a local variable, a field,
// a call result — is an error, not a skip, because `origin := "new-ceremony"`
// would otherwise evade the invariant entirely. The only escape is an explicit
// relationshipOriginRuntimeBinds entry naming a caller-validated runtime value.
func TestRelationshipOriginSourceLiteralsAreRegistryListed(t *testing.T) {
	allowed := map[string]bool{}
	for _, origin := range allowedRelationshipOrigins() {
		allowed[origin] = true
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = file
	}

	// Package-level string constants and variables resolve by name so a writer
	// cannot launder a novel origin through one level of const indirection.
	stringConstants := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || (genDecl.Tok != token.CONST && genDecl.Tok != token.VAR) {
				continue
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok || len(valueSpec.Names) != len(valueSpec.Values) {
					continue
				}
				for i, ident := range valueSpec.Names {
					literal, ok := valueSpec.Values[i].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					if value, err := strconv.Unquote(literal.Value); err == nil {
						stringConstants[ident.Name] = value
					}
				}
			}
		}
	}

	scanned := 0
	for fileName, file := range files {
		processedQueries := map[token.Pos]bool{}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			for argIndex, arg := range call.Args {
				queryLiteral, ok := arg.(*ast.BasicLit)
				if !ok || queryLiteral.Kind != token.STRING {
					continue
				}
				query, err := strconv.Unquote(queryLiteral.Value)
				if err != nil {
					continue
				}
				writes, err := findRelationshipOriginWrites(query)
				if err != nil {
					t.Errorf("%s: %v", fset.Position(queryLiteral.Pos()), err)
					continue
				}
				if len(writes) > 0 {
					processedQueries[queryLiteral.Pos()] = true
				}
				for _, write := range writes {
					scanned++
					if write.isLiteral {
						if !allowed[write.sqlLiteral] {
							t.Errorf("%s: SQL origin literal %q is not registry-listed", fset.Position(queryLiteral.Pos()), write.sqlLiteral)
						}
						continue
					}
					boundIndex := argIndex + 1 + write.placeholder
					if boundIndex >= len(call.Args) {
						t.Errorf("%s: cannot statically resolve the origin bind argument (placeholder %d); keep bind arguments inline for this scan", fset.Position(queryLiteral.Pos()), write.placeholder)
						continue
					}
					switch bound := call.Args[boundIndex].(type) {
					case *ast.BasicLit:
						if bound.Kind != token.STRING {
							t.Errorf("%s: origin bound to non-string literal %s", fset.Position(bound.Pos()), bound.Value)
							continue
						}
						value, err := strconv.Unquote(bound.Value)
						if err != nil || !allowed[value] {
							t.Errorf("%s: bound origin literal %s is not registry-listed", fset.Position(bound.Pos()), bound.Value)
						}
					case *ast.Ident:
						if value, ok := stringConstants[bound.Name]; ok {
							if !allowed[value] {
								t.Errorf("%s: origin constant %s = %q is not registry-listed", fset.Position(bound.Pos()), bound.Name, value)
							}
							continue
						}
						key := fileName + ":" + enclosingFunctionName(file, bound.Pos()) + ":" + bound.Name
						if !relationshipOriginRuntimeBinds[key] {
							t.Errorf("%s: origin bound to unresolvable identifier %s (scan key %q); bind a registry constant, or add the key to relationshipOriginRuntimeBinds with a comment if this is a caller-validated runtime value", fset.Position(bound.Pos()), bound.Name, key)
						}
					default:
						t.Errorf("%s: origin bound to unclassifiable expression %T; bind a registry constant or a caller-validated runtime value the scan can key on", fset.Position(call.Args[boundIndex].Pos()), call.Args[boundIndex])
					}
				}
			}
			return true
		})
		// A second pass over raw string literals: origin-writing SQL that is
		// not an inline call argument evades placeholder mapping, so it must
		// not exist.
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			if processedQueries[literal.Pos()] {
				return true
			}
			writes, err := findRelationshipOriginWrites(value)
			if err == nil && len(writes) > 0 {
				t.Errorf("%s: origin-writing SQL must be an inline argument of its exec call so this scan can resolve its bind arguments", fset.Position(literal.Pos()))
			}
			return true
		})
	}
	// The package currently holds 21 origin-writing statements; a collapsing
	// scan would silently vacate the invariant, so pin a generous floor.
	if scanned < 15 {
		t.Fatalf("origin-write scan found %d statement(s), want at least 15 — the scanner or the writers regressed", scanned)
	}
}

func assertZeroRelationshipOriginDiagnostics(t *testing.T, store *Store, wantRelationships int) {
	t.Helper()
	ctx := context.Background()
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM relationships WHERE origin = ?`, relationshipOriginCommand).Scan(&count); err != nil {
		t.Fatalf("count command-origin relationships error = %v", err)
	}
	if count != wantRelationships {
		t.Fatalf("command-origin relationships = %d, want %d", count, wantRelationships)
	}
	diagnostics, err := inspectRelationshipOriginInvariants(ctx, store)
	if err != nil {
		t.Fatalf("inspectRelationshipOriginInvariants() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after writer", diagnostics)
	}
}

func TestDoctorCleanAfterIntentCreateWithSource(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()
	spark, err := store.CaptureSpark(ctx, root, SparkCaptureOptions{Text: "origin hygiene source"})
	if err != nil {
		t.Fatalf("CaptureSpark() error = %v", err)
	}
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title:   "Origin hygiene intent",
		Body:    "Body.",
		Sources: []string{spark.Spark.Alias},
	}); err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	assertZeroRelationshipOriginDiagnostics(t, store, 1)
}

func TestDoctorCleanAfterExplorationCreateWithSource(t *testing.T) {
	root, _, store := explorationFixture(t)
	ctx := context.Background()
	intent, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "Exploration source intent", Body: "Body."})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	if _, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{
		Title:   "Origin hygiene exploration",
		Sources: []string{intent.Intent.Alias},
	}); err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}
	assertZeroRelationshipOriginDiagnostics(t, store, 1)
}

func TestDoctorCleanAfterLegacyConversion(t *testing.T) {
	root, resolver, store, databasePath := conversionFixture(t)
	projectID := projectIDForTest(t, store, root)
	seedLegacyDeferral(t, store, projectID, "legacy-origin", "Intent: legacy body\nWhy: why\nBoundary: boundary\nTrigger: trigger")
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := ConvertLegacyDeferrals(context.Background(), root, resolver, true); err != nil {
		t.Fatalf("ConvertLegacyDeferrals(apply) error = %v", err)
	}

	reopened, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer reopened.Close()
	// Conversion writes one source-of edge per decision and spark source.
	assertZeroRelationshipOriginDiagnostics(t, reopened, 2)
}

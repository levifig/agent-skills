package vnextcontinuitycontract

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestContinuityProjectionContractPinsExactSourceAndValidation(t *testing.T) {
	t.Parallel()

	path := filepath.Join(continuityRoot, "projections.go")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read projection source: %v", err)
	}
	if got, want := fmt.Sprintf("%x", sha256.Sum256(contents)), "6aa14fa88bc39dc8a80b95cbcc9446c8c4dd75c49265869789a994641b45e75a"; got != want {
		t.Errorf("projection exact source digest = %q, want %q", got, want)
	}

	wantFunctions := map[string]string{
		"ContextRequest.Validate":  "3a7d9b55a18557d899d2c93b62a3395f658b8e9ad925c04e4e3e37f927921afa",
		"SnapshotRequest.Validate": "270173e03c979de061628e91e844e89d4886e82a1b0b72bfeeac64eb183a6315",
	}
	if got := digestProjectionFunctions(t, path); !reflect.DeepEqual(got, wantFunctions) {
		t.Errorf("projection exact function digests = %#v, want %#v", got, wantFunctions)
	}
}

func TestContinuityProjectionContractPinsClosedStateVocabularies(t *testing.T) {
	t.Parallel()

	type constantSpec struct {
		typeName string
		value    string
	}
	want := map[string]constantSpec{
		"DecisionOpen":       {typeName: "DecisionState", value: "open"},
		"DecisionResolved":   {typeName: "DecisionState", value: "resolved"},
		"DecisionSuperseded": {typeName: "DecisionState", value: "superseded"},
		"FindingCurrent":     {typeName: "FindingState", value: "current"},
		"FindingRetracted":   {typeName: "FindingState", value: "retracted"},
		"IdeaActive":         {typeName: "IdeaDisposition", value: "active"},
		"IdeaArchived":       {typeName: "IdeaDisposition", value: "archived"},
		"IdeaPromoted":       {typeName: "IdeaDisposition", value: "promoted"},
		"IdeaResolved":       {typeName: "IdeaDisposition", value: "resolved"},
		"ScratchpadClosed":   {typeName: "ScratchpadState", value: "closed"},
		"ScratchpadOpen":     {typeName: "ScratchpadState", value: "open"},
	}

	path := filepath.Join(continuityRoot, "projections.go")
	file := parseProjectionFile(t, path)
	got := make(map[string]constantSpec)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, specification := range general.Specs {
			value, ok := specification.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				t.Fatalf("projection const declaration is not one exact typed literal")
			}
			if !ast.IsExported(value.Names[0].Name) {
				continue
			}
			typeName, ok := value.Type.(*ast.Ident)
			if !ok {
				t.Fatalf("%s has a non-identifier type", value.Names[0].Name)
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Fatalf("%s is not a string literal", value.Names[0].Name)
			}
			decoded, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("unquote %s: %v", value.Names[0].Name, err)
			}
			got[value.Names[0].Name] = constantSpec{typeName: typeName.Name, value: decoded}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projection state constants = %#v, want %#v", got, want)
	}
}

func digestProjectionFunctions(t *testing.T, path string) map[string]string {
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
		name := function.Name.Name
		if function.Recv != nil && len(function.Recv.List) == 1 {
			name = projectionReceiverName(t, function.Recv.List[0].Type) + "." + name
		}
		var formatted bytes.Buffer
		if err := format.Node(&formatted, fileSet, function); err != nil {
			t.Fatalf("format %s.%s: %v", path, name, err)
		}
		if _, duplicate := digests[name]; duplicate {
			t.Fatalf("%s has multiple functions or methods named %s", path, name)
		}
		digests[name] = fmt.Sprintf("%x", sha256.Sum256(formatted.Bytes()))
	}
	return digests
}

func projectionReceiverName(t *testing.T, expression ast.Expr) string {
	t.Helper()

	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return "*" + projectionReceiverName(t, expression.X)
	default:
		t.Fatalf("unsupported projection receiver %T", expression)
		return ""
	}
}

func parseProjectionFile(t *testing.T, path string) *ast.File {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

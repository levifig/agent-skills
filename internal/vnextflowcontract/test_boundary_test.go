package vnextflowcontract

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFlowContractValidatorIsTestOnlyAndStandardLibrary(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read validator package: %v", err)
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
			t.Errorf("%s is production code; the validator package must remain test-only", entry.Name())
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parse %s: %v", entry.Name(), err)
			continue
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Errorf("unquote import %s in %s: %v", spec.Path.Value, entry.Name(), err)
				continue
			}
			firstSegment := strings.SplitN(importPath, "/", 2)[0]
			if strings.Contains(firstSegment, ".") {
				t.Errorf("%s imports non-standard package %q", entry.Name(), importPath)
			}
		}
	}
}

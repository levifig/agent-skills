// Package vnextboundary enforces the vNext bootstrap source boundary from the
// stable root module, outside the source tree it checks.
package vnextboundary

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	legacyImportRule        = "legacy-module-import"
	thirdPartyImportRule    = "third-party-import"
	nestedModuleRule        = "nested-module"
	nativeBuildInputRule    = "forbidden-native-build-input"
	forbiddenImportRule     = "forbidden-bootstrap-import"
	forbiddenDirectiveRule  = "forbidden-compiler-directive"
	forbiddenCapabilityRule = "forbidden-bootstrap-capability"
	sourceParseRule         = "unparseable-source"
	sourceSymlinkRule       = "source-symlink"
)

type moduleMetadata struct {
	Root            string
	Path            string
	VNextRoot       string
	VNextImportPath string
}

type boundaryViolation struct {
	Path   string
	Rule   string
	Detail string
}

func (violation boundaryViolation) String() string {
	if violation.Path == "" {
		return fmt.Sprintf("%s: %s", violation.Rule, violation.Detail)
	}
	return fmt.Sprintf("%s: %s: %s", violation.Path, violation.Rule, violation.Detail)
}

func discoverModuleMetadata(start string) (moduleMetadata, error) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return moduleMetadata{}, fmt.Errorf("resolve start path: %w", err)
	}
	if info, statErr := os.Stat(directory); statErr != nil {
		return moduleMetadata{}, fmt.Errorf("inspect start path: %w", statErr)
	} else if !info.IsDir() {
		directory = filepath.Dir(directory)
	}

	for {
		goModPath := filepath.Join(directory, "go.mod")
		if _, statErr := os.Lstat(goModPath); statErr == nil {
			return loadModuleMetadata(directory)
		} else if !os.IsNotExist(statErr) {
			return moduleMetadata{}, fmt.Errorf("inspect module metadata: %w", statErr)
		}

		parent := filepath.Dir(directory)
		if parent == directory {
			return moduleMetadata{}, fmt.Errorf("go.mod not found above %s", start)
		}
		directory = parent
	}
}

func loadModuleMetadata(root string) (moduleMetadata, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return moduleMetadata{}, fmt.Errorf("resolve module root: %w", err)
	}
	goModPath := filepath.Join(absoluteRoot, "go.mod")
	info, err := os.Lstat(goModPath)
	if err != nil {
		return moduleMetadata{}, fmt.Errorf("inspect module metadata: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return moduleMetadata{}, fmt.Errorf("module metadata is a symlink: %s", goModPath)
	}
	if !info.Mode().IsRegular() {
		return moduleMetadata{}, fmt.Errorf("module metadata is not a regular file: %s", goModPath)
	}
	contents, err := os.ReadFile(goModPath)
	if err != nil {
		return moduleMetadata{}, fmt.Errorf("read module metadata: %w", err)
	}
	modulePath, err := parseModulePath(contents)
	if err != nil {
		return moduleMetadata{}, fmt.Errorf("parse %s: %w", goModPath, err)
	}
	return moduleMetadata{
		Root:            absoluteRoot,
		Path:            modulePath,
		VNextRoot:       filepath.Join(absoluteRoot, "vnext"),
		VNextImportPath: modulePath + "/vnext",
	}, nil
}

func parseModulePath(contents []byte) (string, error) {
	var modulePath string
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "module" && !strings.HasPrefix(line, "module ") && !strings.HasPrefix(line, "module\t") {
			continue
		}
		rest := strings.TrimSpace(line[len("module"):])
		if rest == "" {
			continue
		}
		if comment := strings.Index(rest, "//"); comment >= 0 {
			rest = strings.TrimSpace(rest[:comment])
		}
		fields := strings.Fields(rest)
		if len(fields) != 1 {
			return "", fmt.Errorf("module directive must contain one path")
		}
		candidate := fields[0]
		if strings.HasPrefix(candidate, "\"") || strings.HasPrefix(candidate, "`") {
			unquoted, err := strconv.Unquote(candidate)
			if err != nil {
				return "", fmt.Errorf("unquote module path: %w", err)
			}
			candidate = unquoted
		}
		if modulePath != "" {
			return "", fmt.Errorf("multiple module directives")
		}
		modulePath = candidate
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan module metadata: %w", err)
	}
	if modulePath == "" {
		return "", fmt.Errorf("module directive not found")
	}
	return modulePath, nil
}

func inspectVNextBoundary(metadata moduleMetadata) ([]boundaryViolation, error) {
	info, err := os.Stat(metadata.VNextRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect vNext root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("vNext root is not a directory: %s", metadata.VNextRoot)
	}

	var violations []boundaryViolation
	err = filepath.WalkDir(metadata.VNextRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath := moduleRelativePath(metadata, path)
		if entry.Type()&os.ModeSymlink != 0 {
			violations = append(violations, boundaryViolation{
				Path:   relativePath,
				Rule:   sourceSymlinkRule,
				Detail: "symlinks prevent complete inspection of the vNext source closure",
			})
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() == "go.mod" {
			violations = append(violations, boundaryViolation{
				Path:   relativePath,
				Rule:   nestedModuleRule,
				Detail: "nested modules can escape the root vNext import policy",
			})
			return nil
		}
		if isForbiddenNativeBuildInput(entry.Name()) {
			violations = append(violations, boundaryViolation{
				Path:   relativePath,
				Rule:   nativeBuildInputRule,
				Detail: "Go-recognized native and object build inputs are outside the bootstrap source policy",
			})
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		violations = append(violations, inspectGoSource(metadata, path, relativePath)...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk vNext source: %w", err)
	}
	sort.Slice(violations, func(left, right int) bool {
		if violations[left].Path != violations[right].Path {
			return violations[left].Path < violations[right].Path
		}
		if violations[left].Rule != violations[right].Rule {
			return violations[left].Rule < violations[right].Rule
		}
		return violations[left].Detail < violations[right].Detail
	})
	return violations, nil
}

func isForbiddenNativeBuildInput(name string) bool {
	switch filepath.Ext(name) {
	case ".c", ".cc", ".cpp", ".cxx", ".m",
		".h", ".hh", ".hpp", ".hxx",
		".f", ".F", ".for", ".f90",
		".s", ".S", ".sx", ".swig", ".swigcxx", ".syso":
		return true
	default:
		return false
	}
}

func inspectGoSource(metadata moduleMetadata, sourcePath, relativePath string) []boundaryViolation {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, sourcePath, nil, parser.ParseComments|parser.AllErrors)
	var violations []boundaryViolation
	if err != nil {
		violations = append(violations, boundaryViolation{
			Path:   relativePath,
			Rule:   sourceParseRule,
			Detail: err.Error(),
		})
	}
	if file == nil {
		return violations
	}

	osAliases := make(map[string]struct{})
	for _, importSpec := range file.Imports {
		importPath, unquoteErr := strconv.Unquote(importSpec.Path.Value)
		if unquoteErr != nil {
			violations = append(violations, boundaryViolation{
				Path:   relativePath,
				Rule:   sourceParseRule,
				Detail: fmt.Sprintf("invalid import path %s: %v", importSpec.Path.Value, unquoteErr),
			})
			continue
		}
		if violation, rejected := inspectImport(metadata, relativePath, importPath, importName(importSpec)); rejected {
			violations = append(violations, violation)
		}
		if importPath != "os" {
			continue
		}
		if importSpec.Name == nil {
			osAliases["os"] = struct{}{}
			continue
		}
		switch importSpec.Name.Name {
		case "_":
		case ".":
			violations = append(violations, boundaryViolation{
				Path:   relativePath,
				Rule:   forbiddenCapabilityRule,
				Detail: "dot-importing os exposes StartProcess without a detectable qualifier",
			})
		default:
			osAliases[importSpec.Name.Name] = struct{}{}
		}
	}

	for _, commentGroup := range file.Comments {
		for _, comment := range commentGroup.List {
			if strings.HasPrefix(comment.Text, "//go:linkname") {
				violations = append(violations, boundaryViolation{
					Path:   relativePath,
					Rule:   forbiddenDirectiveRule,
					Detail: "linkname can reach symbols outside the checked package boundary",
				})
			}
		}
	}

	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "StartProcess" {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, exists := osAliases[qualifier.Name]; exists {
			position := fileSet.Position(selector.Pos())
			violations = append(violations, boundaryViolation{
				Path:   relativePath,
				Rule:   forbiddenCapabilityRule,
				Detail: fmt.Sprintf("os.StartProcess is outside the bootstrap capability policy at line %d", position.Line),
			})
		}
		return true
	})

	return violations
}

const (
	continuitySQLitePackage      = "vnext/continuity/sqlite"
	continuitySQLiteDriverFile   = "vnext/continuity/sqlite/driver.go"
	continuitySQLiteDriverImport = "github.com/ncruces/go-sqlite3/driver"
	ncrucesModuleRoot            = "github.com/ncruces"
	ncrucesModulePrefix          = "github.com/ncruces/"
)

func inspectImport(metadata moduleMetadata, relativePath, importPath, importName string) (boundaryViolation, bool) {
	if importPath == "database/sql" {
		if isContinuitySQLitePackage(relativePath) {
			return boundaryViolation{}, false
		}
		return boundaryViolation{
			Path:   relativePath,
			Rule:   forbiddenImportRule,
			Detail: fmt.Sprintf("%s: database access is outside the bootstrap kernel", importPath),
		}, true
	}
	if isAdmittedSQLiteDriverImport(relativePath, importPath, importName) {
		return boundaryViolation{}, false
	}
	if isNcrucesImport(importPath) {
		return boundaryViolation{
			Path:   relativePath,
			Rule:   thirdPartyImportRule,
			Detail: fmt.Sprintf("%s is not the exact blank sqlite driver import admitted in %s", importPath, continuitySQLiteDriverFile),
		}, true
	}
	if reason, forbidden := forbiddenBootstrapImportReason(importPath); forbidden {
		return boundaryViolation{Path: relativePath, Rule: forbiddenImportRule, Detail: fmt.Sprintf("%s: %s", importPath, reason)}, true
	}
	if importPath == metadata.Path || strings.HasPrefix(importPath, metadata.Path+"/") {
		if importPath == metadata.VNextImportPath || strings.HasPrefix(importPath, metadata.VNextImportPath+"/") {
			return boundaryViolation{}, false
		}
		return boundaryViolation{
			Path:   relativePath,
			Rule:   legacyImportRule,
			Detail: fmt.Sprintf("%s is outside %s", importPath, metadata.VNextImportPath),
		}, true
	}
	if isStandardLibraryImport(importPath) {
		return boundaryViolation{}, false
	}
	return boundaryViolation{
		Path:   relativePath,
		Rule:   thirdPartyImportRule,
		Detail: fmt.Sprintf("%s is not in the standard library or the vNext module subtree", importPath),
	}, true
}

func importName(spec *ast.ImportSpec) string {
	if spec.Name == nil {
		return ""
	}
	return spec.Name.Name
}

func isContinuitySQLitePackage(relativePath string) bool {
	return pathpkg.Dir(relativePath) == continuitySQLitePackage
}

func isAdmittedSQLiteDriverImport(relativePath, importPath, importName string) bool {
	return relativePath == continuitySQLiteDriverFile && importPath == continuitySQLiteDriverImport && importName == "_"
}

func isNcrucesImport(importPath string) bool {
	return importPath == ncrucesModuleRoot || strings.HasPrefix(importPath, ncrucesModulePrefix)
}

func forbiddenBootstrapImportReason(importPath string) (string, bool) {
	switch importPath {
	case "C":
		return "cgo can cross the Go runtime boundary", true
	case "os/exec":
		return "subprocess execution can invoke the legacy runtime", true
	case "plugin":
		return "runtime plugins can load code outside the checked source closure", true
	case "syscall":
		return "raw system calls bypass the bootstrap capability boundary", true
	default:
		return "", false
	}
}

func isStandardLibraryImport(importPath string) bool {
	if importPath == "" || pathpkg.Clean(importPath) != importPath || strings.HasPrefix(importPath, ".") {
		return false
	}
	standardLibraryRoot := filepath.Join(runtime.GOROOT(), "src")
	candidate := filepath.Join(standardLibraryRoot, filepath.FromSlash(importPath))
	relative, err := filepath.Rel(standardLibraryRoot, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	info, err := os.Stat(candidate)
	return err == nil && info.IsDir()
}

func moduleRelativePath(metadata moduleMetadata, path string) string {
	relative, err := filepath.Rel(metadata.Root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

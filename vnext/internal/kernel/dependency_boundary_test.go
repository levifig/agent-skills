package kernel

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	modulePath = "github.com/levifig/loaf"
	vnextPath  = modulePath + "/vnext"
)

type listedPackage struct {
	ImportPath string
	Standard   bool
}

func TestDependencyBoundaryRejectsLegacyRuntimePackages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pkg     listedPackage
		allowed bool
	}{
		{name: "standard library", pkg: listedPackage{ImportPath: "fmt", Standard: true}, allowed: true},
		{name: "vnext package", pkg: listedPackage{ImportPath: vnextPath + "/internal/kernel"}, allowed: true},
		{name: "reviewed external dependency", pkg: listedPackage{ImportPath: "example.com/reviewed/dependency"}, allowed: true},
		{name: "legacy module root", pkg: listedPackage{ImportPath: modulePath}, allowed: false},
		{name: "legacy command", pkg: listedPackage{ImportPath: modulePath + "/cmd/loaf"}, allowed: false},
		{name: "legacy state", pkg: listedPackage{ImportPath: modulePath + "/internal/state"}, allowed: false},
		{name: "legacy crypto", pkg: listedPackage{ImportPath: modulePath + "/internal/crypto"}, allowed: false},
		{name: "legacy sync", pkg: listedPackage{ImportPath: modulePath + "/internal/sync"}, allowed: false},
		{name: "legacy scratchpad", pkg: listedPackage{ImportPath: modulePath + "/internal/scratchpad"}, allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := packageAllowed(test.pkg); got != test.allowed {
				t.Errorf("packageAllowed(%q) = %t, want %t", test.pkg.ImportPath, got, test.allowed)
			}
		})
	}
}

func TestDependencyBoundaryVNextGraphDoesNotReachLegacyRuntime(t *testing.T) {
	t.Parallel()

	packages := listPackages(t, true, "./vnext/...")
	var violations []string
	for _, pkg := range packages {
		if !packageAllowed(pkg) {
			violations = append(violations, pkg.ImportPath)
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("vNext dependency graph reaches legacy runtime packages:\n%s", strings.Join(violations, "\n"))
	}
}

func TestDependencyBoundaryKernelCannotQueryOrInvokeLegacyRuntime(t *testing.T) {
	t.Parallel()

	packages := listPackages(
		t,
		false,
		"./vnext/cmd/loaf",
		"./vnext/internal/command",
		"./vnext/internal/kernel",
	)
	disallowed := map[string]string{
		"database/sql": "query a persistence implementation",
		"os/exec":      "invoke the legacy command as a subprocess",
	}
	for _, pkg := range packages {
		if reason, exists := disallowed[pkg.ImportPath]; exists {
			t.Errorf("bootstrap dependency %q could %s", pkg.ImportPath, reason)
		}
	}
}

func packageAllowed(pkg listedPackage) bool {
	if pkg.Standard {
		return true
	}
	if pkg.ImportPath == vnextPath || strings.HasPrefix(pkg.ImportPath, vnextPath+"/") {
		return true
	}
	return pkg.ImportPath != modulePath && !strings.HasPrefix(pkg.ImportPath, modulePath+"/")
}

func listPackages(t *testing.T, includeTests bool, patterns ...string) []listedPackage {
	t.Helper()

	args := []string{"list", "-deps", "-json"}
	if includeTests {
		args = append(args, "-test")
	}
	args = append(args, patterns...)

	command := exec.Command("go", args...)
	command.Dir = findModuleRoot(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go list dependency graph: %v\n%s", err, output)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []listedPackage
	for {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		packages = append(packages, pkg)
	}
	return packages
}

func findModuleRoot(t *testing.T) string {
	t.Helper()

	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect go.mod: %v", err)
		}

		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("go.mod not found above test working directory")
		}
		directory = parent
	}
}

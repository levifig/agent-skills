package vnextboundary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependencyBoundaryRejectsNativeBuildInputFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
	}{
		{name: "assembly source", filename: "escape.s"},
		{name: "system object", filename: "escape.syso"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := writeBoundaryFixture(t, "alternate.example/loaf", map[string]string{
				"vnext/" + test.filename: "untrusted native build input\n",
			})
			assertBoundaryViolation(t, inspectFixtureBoundary(t, root), nativeBuildInputRule)
		})
	}
}

func TestDependencyBoundaryClassifiesNativeBuildInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		filename string
		want     bool
	}{
		{filename: "source.c", want: true},
		{filename: "source.cc", want: true},
		{filename: "source.cpp", want: true},
		{filename: "source.cxx", want: true},
		{filename: "source.m", want: true},
		{filename: "header.h", want: true},
		{filename: "header.hh", want: true},
		{filename: "header.hpp", want: true},
		{filename: "header.hxx", want: true},
		{filename: "source.f", want: true},
		{filename: "source.F", want: true},
		{filename: "source.for", want: true},
		{filename: "source.f90", want: true},
		{filename: "source.s", want: true},
		{filename: "source.S", want: true},
		{filename: "source.sx", want: true},
		{filename: "source.swig", want: true},
		{filename: "source.swigcxx", want: true},
		{filename: "object.syso", want: true},
		{filename: "source.go", want: false},
		{filename: "notes.md", want: false},
		{filename: "notes.txt", want: false},
		{filename: "source.asm", want: false},
		{filename: "header.inc", want: false},
		{filename: "object.o", want: false},
	}

	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			t.Parallel()
			if got := isForbiddenNativeBuildInput(test.filename); got != test.want {
				t.Errorf("isForbiddenNativeBuildInput(%q) = %t, want %t", test.filename, got, test.want)
			}
		})
	}
}

func TestDependencyBoundaryRejectsSymlinkedModuleAnchor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	actualModuleFile := filepath.Join(root, "actual.mod")
	if err := os.WriteFile(actualModuleFile, []byte("module alternate.example/loaf\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatalf("write module target: %v", err)
	}
	if err := os.Symlink(filepath.Base(actualModuleFile), filepath.Join(root, "go.mod")); err != nil {
		t.Skipf("create module symlink: %v", err)
	}

	_, err := loadModuleMetadata(root)
	if err == nil {
		t.Fatal("loadModuleMetadata() error = nil, want symlink rejection")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("loadModuleMetadata() error = %q, want symlink detail", err)
	}
}

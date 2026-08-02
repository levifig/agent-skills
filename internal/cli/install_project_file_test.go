package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file covers the size half of what a project-file read can refuse. The
// type half — a FIFO or a device at a path Loaf is about to rewrite — needs
// Mkfifo and lives in install_project_file_unix_test.go.
//
// Both halves are the same rule seen twice: the readers that run after
// detection are reading in order to rewrite, so anything they cannot read
// whole they must decline to touch, and say so.

// projectFileTestLimit keeps the direct helper cases cheap. The readers
// themselves are exercised against the real projectFileReadLimit, because a
// limit only means something at the value shipped.
const projectFileTestLimit = 64

// TestReadRegularFileRefusesAFileOverTheLimit pins the refusal against the
// alternative that looks harmless: handing back the first limit bytes. A
// truncated read of a project file is worse than no read, because the caller
// writes what it read back.
func TestReadRegularFileRefusesAFileOverTheLimit(t *testing.T) {
	dir := t.TempDir()

	atLimit := filepath.Join(dir, "at-limit.md")
	writeInstallFile(t, atLimit, strings.Repeat("a", projectFileTestLimit))
	body, err := readRegularFile(atLimit, projectFileTestLimit)
	if err != nil || len(body) != projectFileTestLimit {
		t.Fatalf("readRegularFile(at the limit) = %d bytes, %v, want the whole file", len(body), err)
	}

	overLimit := filepath.Join(dir, "over-limit.md")
	writeInstallFile(t, overLimit, strings.Repeat("a", projectFileTestLimit+1))
	body, err = readRegularFile(overLimit, projectFileTestLimit)
	if !errors.Is(err, errFileTooLarge) {
		t.Fatalf("readRegularFile(over the limit) error = %v, want errFileTooLarge", err)
	}
	if body != nil {
		t.Fatalf("readRegularFile(over the limit) = %d bytes, want nothing; a prefix must never reach a caller that rewrites the file", len(body))
	}
	if !strings.Contains(err.Error(), overLimit) {
		t.Fatalf("readRegularFile(over the limit) error = %q, want the path named", err)
	}
}

// TestInstallFencedSectionRefusesAnOversizedProjectFile is the apply side: the
// managed write refuses on a project file it cannot read whole, exactly as it
// refuses on a fingerprint it cannot parse, and leaves the bytes alone.
func TestInstallFencedSectionRefusesAnOversizedProjectFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "AGENTS.md")
	oversized := oversizedProjectFileBody()
	writeInstallFile(t, target, oversized)

	_, err := installFencedSection(target, "1.2.3", true)

	if err == nil || !errors.Is(err, errFileTooLarge) {
		t.Fatalf("installFencedSection() error = %v, want the too-large refusal", err)
	}
	if !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("installFencedSection() error = %q, want it phrased as a refusal to overwrite", err)
	}
	if got := readFileBytes(t, target); !bytes.Equal(got, []byte(oversized)) {
		t.Fatalf("AGENTS.md changed (%d bytes, want %d); a file Loaf could not read must not be written", len(got), len(oversized))
	}
}

// TestPlanFencedSectionReportsAnOversizedProjectFile is the plan side of the
// same file. The plan may never promise a write the apply path refuses to make.
func TestPlanFencedSectionReportsAnOversizedProjectFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "AGENTS.md")
	writeInstallFile(t, target, oversizedProjectFileBody())

	action, detail := planFencedSection(target, "1.2.3")

	if action != "error" {
		t.Fatalf("planFencedSection() action = %q, want error", action)
	}
	if !strings.Contains(detail, "project read limit") || !strings.Contains(detail, "refusing to overwrite") {
		t.Fatalf("planFencedSection() detail = %q, want the size refusal and the path", detail)
	}
}

// TestReadInstallLoafConfigDocumentRefusesAnOversizedConfig puts a config too
// large to read where the unreadable ones already are: preserved, reported, and
// never merged into.
func TestReadInstallLoafConfigDocumentRefusesAnOversizedConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".agents", "loaf.json")
	oversized := oversizedProjectFileBody()
	writeInstallFile(t, configPath, oversized)

	_, err := readInstallLoafConfigDocument(root)

	var unusable unusableLoafConfigError
	if !errors.As(err, &unusable) || !errors.Is(err, errFileTooLarge) {
		t.Fatalf("readInstallLoafConfigDocument() error = %v, want the unusable-config refusal carrying errFileTooLarge", err)
	}
	if mergeErr := mergeInstallLoafConfigIntegrations(root, map[string]bool{"linear": true}, false); !errors.Is(mergeErr, errFileTooLarge) {
		t.Fatalf("mergeInstallLoafConfigIntegrations() error = %v, want the same refusal", mergeErr)
	}
	if got := readFileBytes(t, configPath); !bytes.Equal(got, []byte(oversized)) {
		t.Fatalf("loaf.json changed (%d bytes, want %d); it must be preserved as written", len(got), len(oversized))
	}
}

// TestMergeMcpConfigRefusesAnOversizedConfig is the size half for the harness
// configs. The merge rewrites the whole file, so a truncated read would hand
// back a document missing every server past the limit and write that back as
// the config.
func TestMergeMcpConfigRefusesAnOversizedConfig(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".cursor", "mcp.json")
	oversized := oversizedProjectFileBody()
	writeInstallFile(t, mcpPath, oversized)

	err := mergeJSONMcpConfig(mcpPath, "mcpServers", "linear", []string{"npx", "-y", "mcp-remote"})

	if !errors.Is(err, errFileTooLarge) {
		t.Fatalf("mergeJSONMcpConfig(oversized) error = %v, want errFileTooLarge", err)
	}
	if !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("mergeJSONMcpConfig(oversized) error = %q, want it phrased as a refusal to overwrite", err)
	}
	if got := readFileBytes(t, mcpPath); !bytes.Equal(got, []byte(oversized)) {
		t.Fatalf("mcp.json changed (%d bytes, want %d); a config Loaf could not read must not be rewritten", len(got), len(oversized))
	}
}

// TestInstallMcpDetectionReportsAnOversizedConfig is the detection half of the
// same file: no signal, and a notice naming the path rather than a silent
// "not configured".
func TestInstallMcpDetectionReportsAnOversizedConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("USERPROFILE", filepath.Join(root, "home"))
	mcpPath := filepath.Join(root, ".cursor", "mcp.json")
	writeInstallFile(t, mcpPath, oversizedProjectFileBody())

	status := detectInstallMcpForTarget(root, "cursor", "linear")

	if status.configured {
		t.Fatalf("status = %#v, want a config that could not be read reported as unconfigured", status)
	}
	if !strings.Contains(status.notice, mcpPath) || !strings.Contains(status.notice, "project read limit") {
		t.Fatalf("status.notice = %q, want the path and the size refusal", status.notice)
	}
}

// TestUpgradeSymlinkPassRefusesAnOversizedLegacyFile pins the migration pass's
// size refusal at the seam the plan and the apply path share: both read the
// legacy file whole in order to merge it, so both refuse the same file.
func TestUpgradeSymlinkPassRefusesAnOversizedLegacyFile(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "AGENTS.md")
	writeInstallFile(t, canonical, "# Project\n")
	legacy := filepath.Join(root, ".agents", "AGENTS.md")
	oversized := oversizedProjectFileBody()
	writeInstallFile(t, legacy, oversized)

	result := ensureRootInstallAgentsFile(root, installSymlinkOptions{AssumeYes: true})

	if result.Action != "error" || !result.Refused {
		t.Fatalf("ensureRootInstallAgentsFile() = %#v, want a refusal that fails the project part", result)
	}
	if !strings.Contains(result.Message, "project read limit") || !strings.Contains(result.Message, "refusing to overwrite") {
		t.Fatalf("result.Message = %q, want the size refusal", result.Message)
	}
	if got := readFileBytes(t, legacy); !bytes.Equal(got, []byte(oversized)) {
		t.Fatalf("legacy file changed (%d bytes, want %d); a refused migration must not move it", len(got), len(oversized))
	}
	if got := readFileBytes(t, canonical); string(got) != "# Project\n" {
		t.Fatalf("AGENTS.md = %q, want it untouched", got)
	}

	action, detail, isError := planRootInstallAgentsFile(root, true)
	if action != "error" || !isError {
		t.Fatalf("planRootInstallAgentsFile() = %q, %q, %v, want the same refusal in the plan", action, detail, isError)
	}
	if !strings.Contains(detail, "project read limit") {
		t.Fatalf("plan detail = %q, want the size refusal", detail)
	}
}

// TestProjectFileReadersAcceptAFileAtTheLimit is the other edge of the same
// boundary. A limit that refused an ordinary project file would be a bug of its
// own, so each reader is shown taking the largest file it is meant to take.
func TestProjectFileReadersAcceptAFileAtTheLimit(t *testing.T) {
	t.Run("fenced apply", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "AGENTS.md")
		writeInstallFile(t, target, projectFileBodyOfSize(projectFileReadLimit))

		result, err := installFencedSection(target, "1.2.3", true)

		if err != nil {
			t.Fatalf("installFencedSection() error = %v, want the section appended", err)
		}
		if result.Action != "appended" {
			t.Fatalf("installFencedSection() action = %q, want appended", result.Action)
		}
		if !strings.Contains(string(readFileBytes(t, target)), fencedStartMarker) {
			t.Fatalf("AGENTS.md missing its managed section after an at-limit read")
		}
	})

	t.Run("fenced plan", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "AGENTS.md")
		writeInstallFile(t, target, projectFileBodyOfSize(projectFileReadLimit))

		action, detail := planFencedSection(target, "1.2.3")

		if action != "appended" {
			t.Fatalf("planFencedSection() = %q, %q, want appended", action, detail)
		}
	})

	t.Run("project config", func(t *testing.T) {
		root := t.TempDir()
		padding := strings.Repeat("x", projectFileReadLimit-len("{\"note\":\"\"}\n"))
		writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), "{\"note\":\""+padding+"\"}\n")

		config, err := readInstallLoafConfigDocument(root)

		if err != nil {
			t.Fatalf("readInstallLoafConfigDocument() error = %v, want the config read", err)
		}
		if len(config["note"].(string)) != len(padding) {
			t.Fatalf("note = %d bytes, want %d; the document must arrive whole", len(config["note"].(string)), len(padding))
		}
	})
}

// oversizedProjectFileBody is one byte past what a project file may be — the
// smallest file that must be refused.
func oversizedProjectFileBody() string {
	return projectFileBodyOfSize(projectFileReadLimit + 1)
}

func projectFileBodyOfSize(size int) string {
	return strings.Repeat("x", size)
}

// assertStillNotARegularFile is shared with the unix cases: whatever a reader
// refused, it must have left exactly as it found it.
func assertStillNotARegularFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(%s) error = %v, want the path left in place", path, err)
	}
	if info.Mode().IsRegular() {
		t.Fatalf("%s is now a regular file; Loaf replaced a path it refused to read", path)
	}
}

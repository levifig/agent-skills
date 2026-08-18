package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	devBuildFixtureCommit  = "abc1234"
	devBuildFixtureVersion = "9.8.7-test.1+gabc1234"
)

func TestRunnerVersionUsesNativePackageMetadata(t *testing.T) {
	root := writeVersionFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
		Executable: distributionFixtureExecutable(root),
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"loaf",
		"9.8.7-test.1",
		"go",
		"Targets:",
		"claude-code",
		"plugins/loaf/",
		"cursor",
		"dist/cursor/",
		"Content:",
		"Skills:  2",
		"Agents:  2",
		"Hooks:   4",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("version output = %q, want %q", output, want)
		}
	}
}

func TestRunnerVersionDoesNotRequireLegacyBridge(t *testing.T) {
	root := writeVersionFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
		Executable: distributionFixtureExecutable(root),
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}
	if strings.Contains(stdout.String(), "args=version") {
		t.Fatalf("version output = %q, want native output instead of legacy bridge", stdout.String())
	}
}

func TestRunnerVersionFlagsDoNotRequireLegacyBridge(t *testing.T) {
	root := writeVersionFixture(t)
	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			var stdout bytes.Buffer

			err := Runner{
				Stdout:     &stdout,
				WorkingDir: root,
				Executable: distributionFixtureExecutable(root),
			}.Run([]string{flag})
			if err != nil {
				t.Fatalf("%s error = %v", flag, err)
			}
			output := stdout.String()
			if !strings.Contains(output, "9.8.7-test.1") || !strings.Contains(output, "Content:") {
				t.Fatalf("%s output = %q, want native version details", flag, output)
			}
		})
	}
}

func TestBuildInfoSuffixFormatsBothPartsDateThenCommit(t *testing.T) {
	got := buildInfoSuffix("abc1234", "2026-06-27T12:00:00Z")
	want := " (built 2026-06-27T12:00:00Z · git abc1234)"
	if got != want {
		t.Fatalf("buildInfoSuffix = %q, want %q", got, want)
	}
}

func TestBuildInfoSuffixEmptyWhenNeitherSet(t *testing.T) {
	if got := buildInfoSuffix("", ""); got != "" {
		t.Fatalf("buildInfoSuffix = %q, want empty string", got)
	}
}

func TestBuildInfoSuffixOnlyCommit(t *testing.T) {
	got := buildInfoSuffix("abc1234", "")
	want := " (git abc1234)"
	if got != want {
		t.Fatalf("buildInfoSuffix = %q, want %q", got, want)
	}
}

func TestBuildInfoSuffixOnlyDate(t *testing.T) {
	got := buildInfoSuffix("", "2026-06-27T12:00:00Z")
	want := " (built 2026-06-27T12:00:00Z)"
	if got != want {
		t.Fatalf("buildInfoSuffix = %q, want %q", got, want)
	}
}

func TestRunnerVersionIncludesBuildInfoWhenSet(t *testing.T) {
	root := writeVersionFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:      &stdout,
		WorkingDir:  root,
		Executable:  distributionFixtureExecutable(root),
		BuildCommit: "abc1234",
		BuildDate:   "2026-06-27T12:00:00Z",
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}

	// ansiBold("loaf") renders as "\x1b[1mloaf\x1b[0m"; assert the exact rendered
	// first line so the build-info suffix sits immediately after the version.
	want := "loaf\x1b[0m 9.8.7-test.1 (built 2026-06-27T12:00:00Z · git abc1234)\n"
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("version output = %q, want to contain %q", stdout.String(), want)
	}
}

func TestRunnerVersionCleanWithoutBuildInfo(t *testing.T) {
	root := writeVersionFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
		Executable: distributionFixtureExecutable(root),
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}

	output := stdout.String()
	cleanLine := "loaf\x1b[0m 9.8.7-test.1\n"
	if !strings.Contains(output, cleanLine) {
		t.Fatalf("version output = %q, want clean version line %q (no regression)", output, cleanLine)
	}
	for _, forbidden := range []string{"(built", "· git", "(git "} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("version output = %q, must not contain %q without build info", output, forbidden)
		}
	}
}

func TestIsDevVersionRecognizesCommitMetadataAndLegacyTimestamps(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{version: "0.2.20"},
		{version: "0.3+gabcdef0"},
		{version: devBuildFixtureVersion, want: true},
		{version: "0.2.20+build.9.gabc1234", want: true},
		{version: "v0.2.20+gabcdef0", want: true},
		{version: "0.2.20+gabc123"},
		{version: "0.2.20+gABCDEFG"},
		{version: "0.2.20+release.1"},
		{version: "0.2.1754593012", want: true},
		{version: "0.2.1000000000", want: true},
		{version: "0.2.999999999"},
		{version: "2.0.0-alpha.19"},
		{version: "0.2.20-dev.1754593012"},
		{version: "not-a-version"},
		{version: ""},
	} {
		t.Run(tc.version, func(t *testing.T) {
			if got := isDevVersion(tc.version); got != tc.want {
				t.Fatalf("isDevVersion(%q) = %t, want %t", tc.version, got, tc.want)
			}
		})
	}
}

func TestDevVersionAppendsTheShortCommitAsBuildMetadata(t *testing.T) {
	got := devVersion("9.8.7-test.1", "ABC1234def5678")
	if got != devBuildFixtureVersion {
		t.Fatalf("devVersion = %q, want %q", got, devBuildFixtureVersion)
	}
	if !isDevVersion(got) {
		t.Fatalf("isDevVersion(%q) = false, want the minted version to satisfy the predicate", got)
	}
}

func TestDevVersionPreservesReleasePrecedence(t *testing.T) {
	dev, ok := parseUpgradeSemver(devVersion("0.2.20", devBuildFixtureCommit))
	if !ok {
		t.Fatal("devVersion did not mint a parseable semver")
	}
	release, ok := parseUpgradeSemver("0.2.20")
	if !ok {
		t.Fatal("parseUpgradeSemver(release) failed")
	}
	if got := compareUpgradeSemver(dev, release); got != 0 {
		t.Fatalf("dev build precedence = %d, want equality with its package version", got)
	}
}

func TestDevVersionFallsBackWithoutAUsableCommit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		release string
		commit  string
		want    string
	}{
		{name: "missing_commit", release: "0.2.20", want: "0.2.20"},
		{name: "short_commit", release: "0.2.20", commit: "abc123", want: "0.2.20"},
		{name: "non_hex_commit", release: "0.2.20", commit: "notasha", want: "0.2.20"},
		{name: "unparseable_release", release: "not-a-version", commit: devBuildFixtureCommit, want: "not-a-version"},
		{name: "unresolvable_distribution", release: packageVersionUnknown, commit: devBuildFixtureCommit, want: packageVersionUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := devVersion(tc.release, tc.commit); got != tc.want {
				t.Fatalf("devVersion(%q, %q) = %q, want %q", tc.release, tc.commit, got, tc.want)
			}
		})
	}
}

func TestRunnerVersionReportsTheDevCommitForDevBuilds(t *testing.T) {
	root := markSourceCheckout(t, writeVersionFixture(t))
	var stdout bytes.Buffer

	err := Runner{
		Stdout:         &stdout,
		WorkingDir:     root,
		Executable:     distributionFixtureExecutable(root),
		DevBuildCommit: devBuildFixtureCommit,
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}

	output := stdout.String()
	want := "loaf\x1b[0m " + devBuildFixtureVersion + " (dev build)\n"
	if !strings.Contains(output, want) {
		t.Fatalf("version output = %q, want to contain %q", output, want)
	}
	if strings.Contains(output, "loaf\x1b[0m 9.8.7-test.1\n") {
		t.Fatalf("version output = %q, want commit metadata on the dev version line", output)
	}
}

// TestRunnerVersionKeepsShippedDistributionsOnTheReleaseVersion is the reason
// missing release metadata cannot be the whole dev signal. The native binaries
// this repository commits are built locally, and the Claude Code plugin
// marketplace serves exactly those bytes at a release tag — so a build time in
// hand means nothing unless the distribution around the binary is the checkout
// that produced it.
func TestRunnerVersionKeepsShippedDistributionsOnTheReleaseVersion(t *testing.T) {
	root := writeVersionFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:         &stdout,
		WorkingDir:     root,
		Executable:     distributionFixtureExecutable(root),
		DevBuildCommit: devBuildFixtureCommit,
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "loaf\x1b[0m 9.8.7-test.1\n") {
		t.Fatalf("version output = %q, want the shipped distribution's release version", output)
	}
	for _, forbidden := range []string{devBuildFixtureVersion, "(dev build)"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("version output = %q, must not contain %q for a shipped distribution", output, forbidden)
		}
	}
}

// TestIsSourceCheckoutSeparatesCheckoutsFromShippedDistributions pins the one
// file the distinction rests on, and the empty root that must never be probed:
// go.mod is a relative path, so an unresolved distribution would otherwise ask
// the working directory whether the binary is a dev build.
func TestIsSourceCheckoutSeparatesCheckoutsFromShippedDistributions(t *testing.T) {
	if isSourceCheckout("") {
		t.Fatal(`isSourceCheckout("") = true, want false for an unresolved distribution`)
	}
	if shipped := writeVersionFixture(t); isSourceCheckout(shipped) {
		t.Fatalf("isSourceCheckout(%q) = true, want false for a shipped distribution", shipped)
	}
	if checkout := markSourceCheckout(t, writeVersionFixture(t)); !isSourceCheckout(checkout) {
		t.Fatalf("isSourceCheckout(%q) = false, want true for a source checkout", checkout)
	}
}

func writeVersionFixture(t *testing.T) string {
	t.Helper()
	root := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(root, "content", "skills", "go-development"))
	mkdirAll(t, filepath.Join(root, "content", "skills", "typescript-development"))
	mkdirAll(t, filepath.Join(root, "content", "agents"))
	mkdirAll(t, filepath.Join(root, "config"))
	mkdirAll(t, filepath.Join(root, "plugins", "loaf"))
	mkdirAll(t, filepath.Join(root, "dist", "cursor"))

	writeFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"9.8.7-test.1"}`)
	writeFile(t, filepath.Join(root, "content", "skills", "README.md"), "# not a skill\n")
	writeFile(t, filepath.Join(root, "content", "agents", "implementer.md"), "# Implementer\n")
	writeFile(t, filepath.Join(root, "content", "agents", "reviewer.md"), "# Reviewer\n")
	writeFile(t, filepath.Join(root, "content", "agents", "reviewer.yaml"), "ignored: true\n")
	writeFile(t, filepath.Join(root, "config", "hooks.yaml"), strings.Join([]string{
		"hooks:",
		"  pre-tool:",
		"    - id: check-secrets",
		"    - id: session-nudge",
		"  post-tool:",
		"    - id: capture-result",
		"  session:",
		"    - id: pre-compact",
		"  pre-commit:",
		"    - id: ignored-by-version-command",
	}, "\n"))
	return root
}

// markSourceCheckout adds the one file that separates a checkout from a shipped
// distribution: the Go module that builds the binary.
func markSourceCheckout(t *testing.T, root string) string {
	t.Helper()
	writeFile(t, filepath.Join(root, "go.mod"), "module github.com/levifig/loaf\n\ngo 1.25.0\n")
	return root
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

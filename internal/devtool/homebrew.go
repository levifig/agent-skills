package devtool

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// HomebrewOptions drive UpdateHomebrewFormula.
type HomebrewOptions struct {
	FormulaPath   string
	ChecksumsPath string
	Version       string
	Repo          string
}

var homebrewTargets = []string{"darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64"}

// UpdateHomebrewFormula rewrites the tap formula from a release's
// checksums.txt. The keg installs the whole distribution into libexec so the
// binary can serve as its own Claude Code marketplace and carries the authored
// vnext Flow content.
func UpdateHomebrewFormula(options HomebrewOptions) error {
	if options.FormulaPath == "" || options.ChecksumsPath == "" || options.Version == "" {
		return fmt.Errorf("--formula, --checksums, and --version are required.")
	}
	repo := options.Repo
	if repo == "" {
		repo = "levifig/loaf"
	}
	checksums, err := ReadChecksums(options.ChecksumsPath, options.Version)
	if err != nil {
		return err
	}
	for _, target := range homebrewTargets {
		if checksums[target] == "" {
			return fmt.Errorf("checksums file missing %s archive.", target)
		}
	}
	if err := os.WriteFile(options.FormulaPath, []byte(HomebrewFormula(options.Version, repo, checksums)), 0o644); err != nil {
		return err
	}
	return nil
}

var checksumLine = regexp.MustCompile(`^([a-f0-9]{64})\s+loaf_(.+)_(.+)\.tar\.gz$`)

// ReadChecksums parses checksums.txt into target -> sha256, refusing lines
// from another version.
func ReadChecksums(path, version string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		match := checksumLine.FindStringSubmatch(trimmed)
		if match == nil {
			return nil, fmt.Errorf("invalid checksum line: %s", line)
		}
		if match[2] != version {
			return nil, fmt.Errorf("checksum line version %s does not match %s", match[2], version)
		}
		result[match[3]] = match[1]
	}
	return result, nil
}

// HomebrewFormula renders the formula body.
func HomebrewFormula(version, repo string, checksums map[string]string) string {
	url := func(target string) string {
		return fmt.Sprintf("https://github.com/%s/releases/download/v%s/loaf_%s_%s.tar.gz", repo, version, version, target)
	}
	return fmt.Sprintf(`class Loaf < Formula
  desc "Opinionated agentic framework for AI coding assistants"
  homepage "https://github.com/%s"
  version "%s"
  license "MIT"

  depends_on "git"

  on_macos do
    if Hardware::CPU.arm?
      url "%s"
      sha256 "%s"
    else
      url "%s"
      sha256 "%s"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "%s"
      sha256 "%s"
    else
      url "%s"
      sha256 "%s"
    end
  end

  def install
    libexec.install "bin", "package.json", "config", "content", "vnext", "dist", "plugins", ".claude-plugin"
    bin.write_exec_script libexec/"bin/loaf"
  end

  test do
    output = shell_output("#{bin}/loaf --version")
    assert_match "loaf", output
    assert_match version.to_s, output
  end
end
`, repo, version,
		url("darwin-arm64"), checksums["darwin-arm64"],
		url("darwin-x64"), checksums["darwin-x64"],
		url("linux-arm64"), checksums["linux-arm64"],
		url("linux-x64"), checksums["linux-x64"])
}

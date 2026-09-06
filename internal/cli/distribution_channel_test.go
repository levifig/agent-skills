package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// homebrewKegFixture builds the layout `brew install loaf` leaves behind: the
// receipt sits at the keg root and the payload — what
// resolveInstalledDistributionRoot returns — lives under libexec/.
func homebrewKegFixture(t *testing.T, formula string, version string) string {
	t.Helper()
	keg := filepath.Join(t.TempDir(), "Cellar", formula, version)
	distribution := filepath.Join(keg, "libexec")
	mkdirAll(t, distribution)
	writeFile(t, filepath.Join(keg, "INSTALL_RECEIPT.json"), `{"installed_as_dependency":false}`)
	writeFile(t, filepath.Join(distribution, "package.json"), `{"name":"loaf","version":"`+version+`"}`)
	return distribution
}

// npmGlobalFixture builds the `npm install -g` layout: <prefix>/lib/node_modules/<pkg>.
func npmGlobalFixture(t *testing.T, name string) string {
	t.Helper()
	distribution := filepath.Join(t.TempDir(), "lib", "node_modules", "loaf")
	mkdirAll(t, distribution)
	writeFile(t, filepath.Join(distribution, "package.json"), `{"name":"`+name+`","version":"2.0.0-alpha.17"}`)
	return distribution
}

// devCheckoutFixture builds a source checkout: a package tree with git above it.
func devCheckoutFixture(t *testing.T) string {
	t.Helper()
	distribution := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(distribution, ".git"))
	writeFile(t, filepath.Join(distribution, "package.json"), `{"name":"loaf","version":"2.0.0-alpha.17"}`)
	return distribution
}

func TestResolveInstallChannelRecognizesAHomebrewKeg(t *testing.T) {
	channel := resolveInstallChannel(homebrewKegFixture(t, "loaf", "2.0.0-alpha.17"))
	if channel.Kind != installChannelHomebrew || channel.UpgradeCommand != "brew upgrade loaf" {
		t.Fatalf("channel = %v/%q, want homebrew/brew upgrade loaf", channel.Kind, channel.UpgradeCommand)
	}
	// The formula name is read from the keg path, so a versioned or renamed
	// formula still prints a command that resolves.
	versioned := resolveInstallChannel(homebrewKegFixture(t, "loaf@2", "2.0.0-alpha.17"))
	if versioned.UpgradeCommand != "brew upgrade loaf@2" {
		t.Fatalf("versioned formula command = %q, want brew upgrade loaf@2", versioned.UpgradeCommand)
	}
}

// TestResolveInstallChannelRequiresBothHalvesOfTheKegSignature keeps the two
// probes joined: a stray receipt outside the Cellar, and a Cellar-shaped path
// with nothing installed into it, are both short of proof.
func TestResolveInstallChannelRequiresBothHalvesOfTheKegSignature(t *testing.T) {
	strayReceipt := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(strayReceipt, "libexec"))
	writeFile(t, filepath.Join(strayReceipt, "INSTALL_RECEIPT.json"), "{}")
	if channel := resolveInstallChannel(filepath.Join(strayReceipt, "libexec")); channel.Kind != installChannelUnknown {
		t.Fatalf("channel = %v, want unknown for a receipt outside the Cellar", channel.Kind)
	}

	receiptless := filepath.Join(realpath(t, t.TempDir()), "Cellar", "loaf", "2.0.0", "libexec")
	mkdirAll(t, receiptless)
	if channel := resolveInstallChannel(receiptless); channel.Kind != installChannelUnknown {
		t.Fatalf("channel = %v, want unknown for a keg path with no receipt", channel.Kind)
	}
}

func TestResolveInstallChannelRecognizesAGlobalNpmTree(t *testing.T) {
	channel := resolveInstallChannel(npmGlobalFixture(t, "loaf"))
	if channel.Kind != installChannelNpm || channel.UpgradeCommand != "npm update -g loaf" {
		t.Fatalf("channel = %v/%q, want npm/npm update -g loaf", channel.Kind, channel.UpgradeCommand)
	}
	// The command names whatever the package publishes as, not a constant.
	scoped := resolveInstallChannel(npmGlobalFixture(t, "@levifig/loaf"))
	if scoped.UpgradeCommand != "npm update -g @levifig/loaf" {
		t.Fatalf("scoped command = %q, want npm update -g @levifig/loaf", scoped.UpgradeCommand)
	}

	// The flatter prefix layout — node_modules beside bin/, with no package
	// above it — is global too.
	prefix := realpath(t, t.TempDir())
	distribution := filepath.Join(prefix, "node_modules", "loaf")
	mkdirAll(t, distribution)
	mkdirAll(t, filepath.Join(prefix, "bin"))
	writeFile(t, filepath.Join(distribution, "package.json"), `{"name":"loaf","version":"2.0.0-alpha.17"}`)
	if channel := resolveInstallChannel(distribution); channel.Kind != installChannelNpm {
		t.Fatalf("channel = %v, want npm for a prefix-root node_modules", channel.Kind)
	}
}

// TestResolveInstallChannelIgnoresAProjectLocalNodeModules is the false
// positive worth preventing: `npm update -g` would not touch a copy installed
// as somebody's dependency, so the honest answer is no advisory at all.
func TestResolveInstallChannelIgnoresAProjectLocalNodeModules(t *testing.T) {
	project := realpath(t, t.TempDir())
	distribution := filepath.Join(project, "node_modules", "loaf")
	mkdirAll(t, distribution)
	mkdirAll(t, filepath.Join(project, "bin"))
	writeFile(t, filepath.Join(project, "package.json"), `{"name":"consumer","version":"1.0.0"}`)
	writeFile(t, filepath.Join(distribution, "package.json"), `{"name":"loaf","version":"2.0.0-alpha.17"}`)

	if channel := resolveInstallChannel(distribution); channel.Kind != installChannelUnknown {
		t.Fatalf("channel = %v, want unknown for a project-local dependency", channel.Kind)
	}
}

func TestResolveInstallChannelRecognizesADevCheckout(t *testing.T) {
	channel := resolveInstallChannel(devCheckoutFixture(t))
	if channel.Kind != installChannelDev || channel.UpgradeCommand != "git pull && make build" {
		t.Fatalf("channel = %v/%q, want dev/git pull && make build", channel.Kind, channel.UpgradeCommand)
	}

	// A linked worktree carries .git as a file, and the checkout may sit below
	// the repo root.
	worktree := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(worktree, ".git"), "gitdir: /elsewhere/.git/worktrees/feat\n")
	nested := filepath.Join(worktree, "packages", "loaf")
	mkdirAll(t, nested)
	writeFile(t, filepath.Join(nested, "package.json"), `{"name":"loaf","version":"2.0.0-alpha.17"}`)
	if channel := resolveInstallChannel(nested); channel.Kind != installChannelDev {
		t.Fatalf("channel = %v, want dev for a linked worktree", channel.Kind)
	}
}

func TestResolveInstallChannelIsUnknownWithoutASignature(t *testing.T) {
	bare := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(bare, "package.json"), `{"name":"loaf","version":"2.0.0-alpha.17"}`)

	channel := resolveInstallChannel(bare)
	if channel.Kind != installChannelUnknown || channel.UpgradeCommand != "" {
		t.Fatalf("channel = %v/%q, want unknown with no command", channel.Kind, channel.UpgradeCommand)
	}
	if channel.Kind.String() != "unknown" {
		t.Fatalf("kind.String() = %q, want unknown", channel.Kind.String())
	}
}

func TestResolveInstallChannelRecognizesTheInstallScriptLayout(t *testing.T) {
	home := t.TempDir()
	release := filepath.Join(home, "loaf", "releases", "1.2.3")
	if err := os.MkdirAll(filepath.Join(release, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(release, "package.json"), []byte(`{"name":"loaf","version":"1.2.3"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A release directory without the current link is just an unpacked archive.
	if got := resolveInstallChannel(release); got.Kind != installChannelUnknown {
		t.Fatalf("channel without current link = %v, want unknown", got.Kind)
	}
	if err := os.Symlink(release, filepath.Join(home, "loaf", "current")); err != nil {
		t.Fatal(err)
	}
	got := resolveInstallChannel(release)
	if got.Kind != installChannelScript || !strings.Contains(got.UpgradeCommand, "install.sh") {
		t.Fatalf("channel = %#v, want the script channel with the installer one-liner", got)
	}
	// A sibling release the link does not point at is not current, so it gets no
	// upgrade advice that would silently re-run the installer over it.
	stale := filepath.Join(home, "loaf", "releases", "1.0.0")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolveInstallChannel(stale); got.Kind != installChannelUnknown {
		t.Fatalf("channel for a non-current release = %v, want unknown", got.Kind)
	}
}

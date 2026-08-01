package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// upgrade_advisory.go closes `loaf upgrade` with the one thing that command
// cannot do for itself: replace the binary running it. It classifies the
// install channel (distribution_channel.go), asks GitHub Releases whether a
// newer version exists, and prints the exact command. It never runs it.
//
// The check is advisory and hard-bounded. One second is the entire budget, and
// every failure — unknown channel, no network, a slow API, an unparseable
// version on either side, a release feed with nothing comparable in it —
// produces no line at all. Upgrade's output and exit code are then exactly what
// they would be if this file did not exist, which is what keeps the network
// optional rather than load-bearing.

// upgradeCurrencyBudget is the hard total the check may spend, wall clock,
// including the request and everything done with its answer.
const upgradeCurrencyBudget = time.Second

// upgradeReleasesEndpoint is the single source of truth for every channel.
// Homebrew formulae, the npm package, and the dev checkout all trail the same
// releases, so one feed answers for all three; asking each channel's own
// registry would mean three protocols and three staleness stories.
const upgradeReleasesEndpoint = "https://api.github.com/repos/levifig/loaf/releases?per_page=30"

// upgradeCurrencySource resolves the newest published version. It is an
// interface so tests can supply version sets, failures, and latency without a
// network, and so the budget can be proven rather than trusted.
type upgradeCurrencySource interface {
	LatestVersion(ctx context.Context, includePrereleases bool) (string, error)
}

// upgradeCurrency is the source the command uses. Tests replace it to drive the
// advisory through a real `loaf upgrade` invocation.
var upgradeCurrency upgradeCurrencySource = githubReleasesCurrency{endpoint: upgradeReleasesEndpoint}

// writeUpgradeCurrencyAdvisory is upgrade's whole insertion point. It is called
// only on the applied human run: the dry-run plan — including its JSON document
// and the schema consumers read — returns before this and stays untouched.
func writeUpgradeCurrencyAdvisory(out io.Writer, distributionRoot string, version string) {
	advisory := upgradeCurrencyAdvisory(context.Background(), upgradeCurrency, resolveInstallChannel(distributionRoot), version)
	if advisory == "" {
		return
	}
	fmt.Fprintf(out, "  %s %s\n\n", ansiYellow("⚠"), advisory)
}

func upgradeCurrencyAdvisory(ctx context.Context, source upgradeCurrencySource, channel installChannel, version string) string {
	return upgradeCurrencyAdvisoryWithin(ctx, upgradeCurrencyBudget, source, channel, version)
}

// upgradeCurrencyAdvisoryWithin returns the advisory text, or "" for every case
// that must stay silent. The budget is a parameter so the timeout is testable
// without a one-second test.
func upgradeCurrencyAdvisoryWithin(ctx context.Context, budget time.Duration, source upgradeCurrencySource, channel installChannel, version string) string {
	// An unrecognized channel has no command to offer, so there is nothing to
	// ask about — the source is never consulted and no request is made.
	if channel.Kind == installChannelUnknown || source == nil {
		return ""
	}
	current, ok := parseUpgradeSemver(version)
	if !ok {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	// A prerelease binary is asking about the line it is already on, so
	// prereleases count as available for it; a stable binary is not offered a
	// prerelease it never opted into.
	latest, err := source.LatestVersion(ctx, current.prerelease != "")
	if err != nil {
		return ""
	}
	// The budget covers the check, not just the wire: an answer that arrives
	// after the deadline is as unusable as one that never arrives.
	if ctx.Err() != nil {
		return ""
	}

	available, ok := parseUpgradeSemver(latest)
	if !ok || compareUpgradeSemver(available, current) <= 0 {
		return ""
	}
	// The "stable compares against stable only" half of the rule, enforced here
	// as well as in the source's selection: a stable binary is never nudged
	// onto a prerelease line, whatever a source answers.
	if current.prerelease == "" && available.prerelease != "" {
		return ""
	}
	return fmt.Sprintf("Update available: loaf %s → %s. Run: %s", normalizeUpgradeVersion(version), normalizeUpgradeVersion(latest), channel.UpgradeCommand)
}

type githubReleasesCurrency struct {
	endpoint string
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// upgradeCurrencyResponseLimit caps what a currency check will read. The feed is
// a few kilobytes; the limit exists so a hostile or broken response cannot turn
// an advisory into a memory problem.
const upgradeCurrencyResponseLimit = 1 << 20

func (source githubReleasesCurrency) LatestVersion(ctx context.Context, includePrereleases bool) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.endpoint, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases responded %s", response.Status)
	}
	var releases []githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, upgradeCurrencyResponseLimit)).Decode(&releases); err != nil {
		return "", err
	}
	tag, ok := selectLatestReleaseTag(releases, includePrereleases)
	if !ok {
		return "", fmt.Errorf("no comparable release in the feed")
	}
	return tag, nil
}

// selectLatestReleaseTag picks the highest release the caller may be offered.
// The feed arrives newest-created first, which is not the same as highest, so
// the winner is chosen by precedence: drafts are unpublished, prereleases are
// filtered for a stable binary, and a tag that is not semver cannot be compared
// and is skipped rather than guessed at.
func selectLatestReleaseTag(releases []githubRelease, includePrereleases bool) (string, bool) {
	var bestTag string
	var best releaseSemver
	for _, release := range releases {
		if release.Draft {
			continue
		}
		parsed, ok := parseUpgradeSemver(release.TagName)
		if !ok {
			continue
		}
		if !includePrereleases && (release.Prerelease || parsed.prerelease != "") {
			continue
		}
		if bestTag == "" || compareUpgradeSemver(parsed, best) > 0 {
			bestTag, best = release.TagName, parsed
		}
	}
	return bestTag, bestTag != ""
}

// parseUpgradeSemver normalizes the shapes a version arrives in — a leading "v"
// on a release tag, build metadata that semver excludes from precedence —
// before handing the core to the release parser both sides of this comparison
// share.
func parseUpgradeSemver(value string) (releaseSemver, bool) {
	core, _, _ := strings.Cut(normalizeUpgradeVersion(value), "+")
	return parseReleaseSemver(core)
}

func normalizeUpgradeVersion(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "v")
}

func compareUpgradeSemver(a releaseSemver, b releaseSemver) int {
	if result := cmp.Compare(a.major, b.major); result != 0 {
		return result
	}
	if result := cmp.Compare(a.minor, b.minor); result != 0 {
		return result
	}
	if result := cmp.Compare(a.patch, b.patch); result != 0 {
		return result
	}
	return compareUpgradePrerelease(a.prerelease, b.prerelease)
}

// compareUpgradePrerelease implements semver precedence for prerelease
// identifiers: a release outranks any prerelease of the same core, identifiers
// compare left to right, numeric ones compare numerically and rank below
// alphanumeric ones, and a longer identifier list wins when every shared field
// is equal. Without this, alpha.10 would sort below alpha.9 and every double
// digit prerelease would stop advertising its successors.
func compareUpgradePrerelease(a string, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	left := strings.Split(a, ".")
	right := strings.Split(b, ".")
	for i := 0; i < len(left) && i < len(right); i++ {
		if result := compareUpgradePrereleaseIdentifier(left[i], right[i]); result != 0 {
			return result
		}
	}
	return cmp.Compare(len(left), len(right))
}

func compareUpgradePrereleaseIdentifier(a string, b string) int {
	leftNumber, leftNumeric := numericPrereleaseIdentifier(a)
	rightNumber, rightNumeric := numericPrereleaseIdentifier(b)
	switch {
	case leftNumeric && rightNumeric:
		return cmp.Compare(leftNumber, rightNumber)
	case leftNumeric:
		return -1
	case rightNumeric:
		return 1
	default:
		return cmp.Compare(a, b)
	}
}

func numericPrereleaseIdentifier(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return number, true
}

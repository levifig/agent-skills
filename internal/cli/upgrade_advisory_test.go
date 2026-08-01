package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubCurrencySource stands in for GitHub Releases. It records what it was
// asked so the prerelease rule can be observed, and it can be slow or rude
// about the deadline so the budget can be proven.
type stubCurrencySource struct {
	latest         string
	err            error
	delay          time.Duration
	blockOnContext bool
	calls          int
	sawPrereleases bool
}

func (source *stubCurrencySource) LatestVersion(ctx context.Context, includePrereleases bool) (string, error) {
	source.calls++
	source.sawPrereleases = includePrereleases
	if source.blockOnContext {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if source.delay > 0 {
		time.Sleep(source.delay)
	}
	return source.latest, source.err
}

// TestUpgradeCurrencyAdvisoryPerChannel is the golden matrix: for every
// recognized channel, a stale binary gets one line naming both versions and
// that channel's command, and both a current binary and an unreachable source
// get nothing at all.
func TestUpgradeCurrencyAdvisoryPerChannel(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		channel installChannel
		want    string
	}{
		{
			name:    "homebrew",
			channel: resolveInstallChannel(homebrewKegFixture(t, "loaf", "2.0.0-alpha.17")),
			want:    "Update available: loaf 2.0.0-alpha.17 → 2.0.0-alpha.18. Run: brew upgrade loaf",
		},
		{
			name:    "npm",
			channel: resolveInstallChannel(npmGlobalFixture(t, "loaf")),
			want:    "Update available: loaf 2.0.0-alpha.17 → 2.0.0-alpha.18. Run: npm update -g loaf",
		},
		{
			name:    "dev",
			channel: resolveInstallChannel(devCheckoutFixture(t)),
			want:    "Update available: loaf 2.0.0-alpha.17 → 2.0.0-alpha.18. Run: git pull && npm run build",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			stale := upgradeCurrencyAdvisory(context.Background(), &stubCurrencySource{latest: "v2.0.0-alpha.18"}, testCase.channel, "2.0.0-alpha.17")
			if stale != testCase.want {
				t.Fatalf("stale advisory = %q, want %q", stale, testCase.want)
			}
			current := upgradeCurrencyAdvisory(context.Background(), &stubCurrencySource{latest: "2.0.0-alpha.17"}, testCase.channel, "2.0.0-alpha.17")
			if current != "" {
				t.Fatalf("current advisory = %q, want silence", current)
			}
			ahead := upgradeCurrencyAdvisory(context.Background(), &stubCurrencySource{latest: "2.0.0-alpha.16"}, testCase.channel, "2.0.0-alpha.17")
			if ahead != "" {
				t.Fatalf("ahead-of-release advisory = %q, want silence", ahead)
			}
			offline := upgradeCurrencyAdvisory(context.Background(), &stubCurrencySource{err: errors.New("dial tcp: no route to host")}, testCase.channel, "2.0.0-alpha.17")
			if offline != "" {
				t.Fatalf("offline advisory = %q, want silence", offline)
			}
		})
	}
}

func TestUpgradeCurrencyAdvisorySkipsUnknownChannelsWithoutAsking(t *testing.T) {
	bare := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(bare, "package.json"), `{"name":"loaf","version":"2.0.0-alpha.17"}`)
	source := &stubCurrencySource{latest: "9.9.9"}

	if advisory := upgradeCurrencyAdvisory(context.Background(), source, resolveInstallChannel(bare), "2.0.0-alpha.17"); advisory != "" {
		t.Fatalf("advisory = %q, want silence for an unknown channel", advisory)
	}
	if source.calls != 0 {
		t.Fatalf("source consulted %d times, want no request for an unknown channel", source.calls)
	}
}

func TestUpgradeCurrencyAdvisoryStaysSilentOnUnparseableVersions(t *testing.T) {
	channel := resolveInstallChannel(homebrewKegFixture(t, "loaf", "2.0.0-alpha.17"))

	unparseableLocal := &stubCurrencySource{latest: "2.0.0"}
	if advisory := upgradeCurrencyAdvisory(context.Background(), unparseableLocal, channel, "not-a-version"); advisory != "" {
		t.Fatalf("advisory = %q, want silence for an unparseable current version", advisory)
	}
	if unparseableLocal.calls != 0 {
		t.Fatalf("source consulted %d times, want no request when the local version cannot be compared", unparseableLocal.calls)
	}

	unparseableRemote := &stubCurrencySource{latest: "banana"}
	if advisory := upgradeCurrencyAdvisory(context.Background(), unparseableRemote, channel, "2.0.0-alpha.17"); advisory != "" {
		t.Fatalf("advisory = %q, want silence for an unparseable available version", advisory)
	}
}

// TestUpgradeCurrencyAdvisoryHonorsThePrereleaseRule pins both halves: a
// prerelease binary asks about the line it is already on, and a stable binary
// is never moved onto a prerelease — not even by a source that offers one.
func TestUpgradeCurrencyAdvisoryHonorsThePrereleaseRule(t *testing.T) {
	channel := resolveInstallChannel(homebrewKegFixture(t, "loaf", "2.0.0-alpha.17"))

	prerelease := &stubCurrencySource{latest: "2.0.0-alpha.18"}
	if advisory := upgradeCurrencyAdvisory(context.Background(), prerelease, channel, "2.0.0-alpha.17"); advisory == "" {
		t.Fatal("prerelease binary got no advisory, want the newer prerelease offered")
	}
	if !prerelease.sawPrereleases {
		t.Fatal("prerelease binary asked for stable releases only, want prereleases included")
	}

	stable := &stubCurrencySource{latest: "2.1.0"}
	if advisory := upgradeCurrencyAdvisory(context.Background(), stable, channel, "2.0.0"); advisory == "" {
		t.Fatal("stable binary got no advisory, want the newer stable offered")
	}
	if stable.sawPrereleases {
		t.Fatal("stable binary asked for prereleases, want stable releases only")
	}

	offered := &stubCurrencySource{latest: "2.1.0-beta.1"}
	if advisory := upgradeCurrencyAdvisory(context.Background(), offered, channel, "2.0.0"); advisory != "" {
		t.Fatalf("advisory = %q, want a stable binary never nudged onto a prerelease", advisory)
	}
}

// TestUpgradeCurrencyAdvisoryStopsAtItsBudget covers both ways a source can be
// too slow: one that waits for the deadline, and one that ignores it and
// answers late. Neither may produce a line, and neither may hold up the
// command.
func TestUpgradeCurrencyAdvisoryStopsAtItsBudget(t *testing.T) {
	channel := resolveInstallChannel(homebrewKegFixture(t, "loaf", "2.0.0-alpha.17"))

	started := time.Now()
	blocking := &stubCurrencySource{blockOnContext: true}
	if advisory := upgradeCurrencyAdvisoryWithin(context.Background(), 40*time.Millisecond, blocking, channel, "2.0.0-alpha.17"); advisory != "" {
		t.Fatalf("advisory = %q, want silence when the source never answers", advisory)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("check took %s, want it abandoned at its 40ms budget", elapsed)
	}

	late := &stubCurrencySource{latest: "9.9.9", delay: 60 * time.Millisecond}
	if advisory := upgradeCurrencyAdvisoryWithin(context.Background(), 10*time.Millisecond, late, channel, "2.0.0-alpha.17"); advisory != "" {
		t.Fatalf("advisory = %q, want an answer that arrives after the budget discarded", advisory)
	}
}

func TestUpgradeCurrencyBudgetIsOneSecond(t *testing.T) {
	if upgradeCurrencyBudget != time.Second {
		t.Fatalf("upgradeCurrencyBudget = %s, want the contract's one-second hard total", upgradeCurrencyBudget)
	}
}

func TestCompareUpgradeSemverFollowsPrereleasePrecedence(t *testing.T) {
	for _, testCase := range []struct {
		left  string
		right string
		want  int
	}{
		{"2.0.0", "1.9.9", 1},
		{"2.1.0", "2.0.9", 1},
		{"2.0.1", "2.0.0", 1},
		{"2.0.0", "2.0.0-alpha.1", 1},
		{"2.0.0-alpha.2", "2.0.0-alpha.1", 1},
		{"2.0.0-alpha.10", "2.0.0-alpha.9", 1},
		{"2.0.0-beta.1", "2.0.0-alpha.1", 1},
		{"2.0.0-alpha.1.1", "2.0.0-alpha.1", 1},
		{"2.0.0-alpha", "2.0.0-1", 1},
		{"2.0.0+build.5", "2.0.0", 0},
		{"v2.0.0", "2.0.0", 0},
	} {
		left, leftOK := parseUpgradeSemver(testCase.left)
		right, rightOK := parseUpgradeSemver(testCase.right)
		if !leftOK || !rightOK {
			t.Fatalf("parseUpgradeSemver(%q, %q) failed to parse", testCase.left, testCase.right)
		}
		if got := compareUpgradeSemver(left, right); got != testCase.want {
			t.Fatalf("compare(%q, %q) = %d, want %d", testCase.left, testCase.right, got, testCase.want)
		}
		if got := compareUpgradeSemver(right, left); got != -testCase.want {
			t.Fatalf("compare(%q, %q) = %d, want %d", testCase.right, testCase.left, got, -testCase.want)
		}
	}
	if _, ok := parseUpgradeSemver("2.0"); ok {
		t.Fatal("parseUpgradeSemver(2.0) parsed, want a version short of three fields rejected")
	}
}

// TestSelectLatestReleaseTagPicksByPrecedenceNotOrder guards the assumption
// that would otherwise be invisible: the feed is ordered by creation, so a
// patch published after a minor appears first and must still lose.
func TestSelectLatestReleaseTagPicksByPrecedenceNotOrder(t *testing.T) {
	feed := []githubRelease{
		{TagName: "nightly", Prerelease: true},
		{TagName: "v2.1.1", Draft: true},
		{TagName: "v2.0.9"},
		{TagName: "v2.1.0-alpha.2", Prerelease: true},
		{TagName: "v2.1.0"},
		{TagName: "v2.1.0-alpha.10", Prerelease: true},
	}

	stable, ok := selectLatestReleaseTag(feed, false)
	if !ok || stable != "v2.1.0" {
		t.Fatalf("stable selection = %q/%v, want v2.1.0 (drafts, prereleases, and unparseable tags skipped)", stable, ok)
	}
	withPrereleases, ok := selectLatestReleaseTag(feed, true)
	if !ok || withPrereleases != "v2.1.0" {
		t.Fatalf("prerelease selection = %q/%v, want v2.1.0 outranking its own prereleases", withPrereleases, ok)
	}
	if tag, ok := selectLatestReleaseTag([]githubRelease{{TagName: "nightly"}, {TagName: "v3.0.0", Draft: true}}, true); ok {
		t.Fatalf("selection = %q, want no comparable release", tag)
	}
	if tag, ok := selectLatestReleaseTag([]githubRelease{{TagName: "v2.1.0-alpha.2", Prerelease: true}}, false); ok {
		t.Fatalf("stable selection = %q, want a prerelease-only feed to offer a stable binary nothing", tag)
	}
}

func TestGithubReleasesCurrencyReadsTheFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"tag_name":"v2.1.0-alpha.3","prerelease":true},{"tag_name":"v2.0.0"}]`)
	}))
	t.Cleanup(server.Close)
	source := githubReleasesCurrency{endpoint: server.URL}

	stable, err := source.LatestVersion(context.Background(), false)
	if err != nil || stable != "v2.0.0" {
		t.Fatalf("LatestVersion(stable) = %q, %v, want v2.0.0", stable, err)
	}
	prerelease, err := source.LatestVersion(context.Background(), true)
	if err != nil || prerelease != "v2.1.0-alpha.3" {
		t.Fatalf("LatestVersion(prereleases) = %q, %v, want v2.1.0-alpha.3", prerelease, err)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	t.Cleanup(failing.Close)
	if _, err := (githubReleasesCurrency{endpoint: failing.URL}).LatestVersion(context.Background(), true); err == nil {
		t.Fatal("LatestVersion() error = nil, want a non-200 reported as a failure")
	}
}

// TestRunnerUpgradeAppendsTheAdvisoryToTheHumanRun drives the advisory through
// a real invocation. The same fixture is upgraded three times, differing only
// in what the currency source says: the stale run is the current run plus one
// line, and the offline run is byte-identical to the current one — the
// contract's "identical output and exit code" written as an assertion.
func TestRunnerUpgradeAppendsTheAdvisoryToTheHumanRun(t *testing.T) {
	root, _ := setupUpgradeFixture(t)
	distribution := homebrewKegFixture(t, "loaf", "2.0.0-alpha.17")

	current := runUpgradeWithCurrency(t, root, distribution, &stubCurrencySource{latest: "v2.0.0-alpha.17"})
	stale := runUpgradeWithCurrency(t, root, distribution, &stubCurrencySource{latest: "v2.0.0-alpha.18"})
	offline := runUpgradeWithCurrency(t, root, distribution, &stubCurrencySource{err: errors.New("dial tcp: no route to host")})

	advisory := "  " + ansiYellow("⚠") + " Update available: loaf 2.0.0-alpha.17 → 2.0.0-alpha.18. Run: brew upgrade loaf\n\n"
	if stale != current+advisory {
		t.Fatalf("stale run = %q, want the current run plus %q", stale, advisory)
	}
	if offline != current {
		t.Fatalf("offline run = %q, want it identical to a run with nothing newer (%q)", offline, current)
	}
}

// TestRunnerUpgradeJSONPlanNeverCarriesTheAdvisory keeps the plan document a
// plan document: no advisory text, and no request made while producing it.
func TestRunnerUpgradeJSONPlanNeverCarriesTheAdvisory(t *testing.T) {
	root, _ := setupUpgradeFixture(t)
	distribution := homebrewKegFixture(t, "loaf", "2.0.0-alpha.17")
	source := &stubCurrencySource{latest: "v9.9.9"}

	output := runUpgradeWithCurrency(t, root, distribution, source, "--dry-run", "--json")

	var plan map[string]any
	if err := json.Unmarshal([]byte(output), &plan); err != nil {
		t.Fatalf("plan JSON = %v\n%s", err, output)
	}
	if strings.Contains(output, "Update available") {
		t.Fatalf("plan JSON = %s, want no advisory in the plan document", output)
	}
	if source.calls != 0 {
		t.Fatalf("source consulted %d times, want the plan surface to make no request", source.calls)
	}
}

// runUpgradeWithCurrency runs `loaf upgrade` against a distribution fixture
// with the currency source replaced for the duration of the test.
func runUpgradeWithCurrency(t *testing.T, projectRoot string, distribution string, source upgradeCurrencySource, args ...string) string {
	t.Helper()
	restore := upgradeCurrency
	upgradeCurrency = source
	t.Cleanup(func() { upgradeCurrency = restore })

	executable := filepath.Join(distribution, "bin", "loaf")
	var stdout bytes.Buffer
	runner := Runner{
		Stdout:     &stdout,
		WorkingDir: projectRoot,
		Executable: func() (string, error) { return executable, nil },
	}
	if err := runner.Run(append([]string{"upgrade", "--yes"}, args...)); err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	return stdout.String()
}

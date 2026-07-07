package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunGHShim is the argv[0] dispatch entry point. cmd/loaf/main.go calls it
// when the running binary was invoked as "gh" (via the symlink created by
// `loaf shim enable gh`). It always ends the process: on the success and
// fall-through paths it execs the real gh (replacing this process image), and
// on the rare resolution failures below that it returns an exit code for
// main() to use instead.
func RunGHShim(argv []string, environ []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	self, err := os.Executable()
	if err != nil {
		self = ""
	}

	plan, err := planGHShimExec(argv, environ, cwd, self, defaultRealGHResolver, defaultGHTokenResolver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loaf gh-shim: %v\n", err)
		return 127
	}
	if plan.StderrNote != "" {
		fmt.Fprint(os.Stderr, plan.StderrNote)
	}
	if execErr := execRealGH(plan.Path, plan.Args, plan.Env); execErr != nil {
		fmt.Fprintf(os.Stderr, "loaf gh-shim: exec %s failed: %v\n", plan.Path, execErr)
		return 126
	}
	return 0
}

// ghShimExecPlan is what RunGHShim will hand to execRealGH. Keeping the
// decision (this struct) separate from the side effect (the actual exec)
// makes the failure matrix unit-testable without spawning processes.
type ghShimExecPlan struct {
	Path       string
	Args       []string
	Env        []string
	StderrNote string
}

// planGHShimExec resolves what to exec without performing the exec.
//
// Fall-through is the default: any resolution failure (no configured
// account, no keychain entry, cwd unreadable) execs the real gh with the
// caller's argv and env completely untouched — the shim must be invisible
// outside identity-configured Loaf projects. Only a successfully resolved
// named-account token adds GH_TOKEN.
func planGHShimExec(
	argv []string,
	environ []string,
	cwd string,
	selfPath string,
	resolveRealGH func() (string, error),
	resolveToken func(realGH string, account string) (string, error),
) (ghShimExecPlan, error) {
	realGH, err := resolveRealGH()
	if err != nil {
		return ghShimExecPlan{}, err
	}
	if selfPath != "" && isSameExecutable(realGH, selfPath) {
		return ghShimExecPlan{}, fmt.Errorf("resolved gh %q is the loaf shim itself; refusing to recurse", realGH)
	}

	plain := ghShimExecPlan{
		Path: realGH,
		Args: append([]string{realGH}, argv[1:]...),
		Env:  environ,
	}

	if cwd == "" {
		return plain, nil
	}
	account, err := findConfiguredGitHubAccountUpward(cwd)
	if err != nil || account == "" {
		return plain, nil
	}

	token, err := resolveToken(realGH, account)
	if err != nil {
		plain.StderrNote = fmt.Sprintf("loaf gh-shim: token for %q unavailable; running unshimmed\n", account)
		return plain, nil
	}

	shimmed := plain
	shimmed.Env = setGHTokenEnv(environ, token)
	return shimmed, nil
}

// findConfiguredGitHubAccountUpward walks from startDir toward its git root
// (or the filesystem root, when startDir isn't inside a git repo), checking
// for .agents/loaf.json's integrations.github.account at every level. The
// git-root boundary (a ".git" entry) keeps a shimmed invocation inside one
// repo from ever picking up a loaf.json belonging to an unrelated ancestor
// directory.
func findConfiguredGitHubAccountUpward(startDir string) (string, error) {
	dir := filepath.Clean(startDir)
	for {
		account, err := configuredGitHubAccount(dir)
		if err != nil {
			return "", err
		}
		if account != "" {
			return account, nil
		}
		if isGitDirBoundary(dir) {
			return "", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func isGitDirBoundary(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

// defaultRealGHResolver reads the real gh path recorded at `loaf shim enable
// gh` time. It deliberately does not re-walk PATH on the normal path — PATH
// re-walking is the recursion hazard the recorded config exists to avoid.
// The PATH-minus-shim-dir walk only runs as a last resort when the recorded
// config is unusable (missing entry, or a real_path that no longer exists on
// disk); `loaf doctor` flags both cases so they don't go unnoticed.
func defaultRealGHResolver() (string, error) {
	cfg, ok, err := readShimUserConfig()
	if err == nil && ok && cfg.Shims.GH != nil && pathIsExecutableFile(cfg.Shims.GH.RealPath) {
		return cfg.Shims.GH.RealPath, nil
	}

	shimDir, _ := shimSymlinkDir()
	fallback, ferr := findRealGHOnPATH(shimDir)
	if ferr != nil || fallback == "" {
		return "", fmt.Errorf("no usable gh shim configuration and no gh found on PATH")
	}
	return fallback, nil
}

// defaultGHTokenResolver reads the token for a specific named account. It
// never inspects the active-account pointer (no `gh auth status --active`) —
// only the named read that the design accepted as its v1 token source.
func defaultGHTokenResolver(realGH string, account string) (string, error) {
	cmd := exec.Command(realGH, "auth", "token", "--hostname", githubAccountHostname, "--user", account)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", fmt.Errorf("empty token for %q", account)
	}
	return token, nil
}

// setGHTokenEnv returns environ with any existing GH_TOKEN entries replaced
// by token — the contract's "GH_TOKEN and GH_TOKEN only, no other env
// surgery" promise, applied without risking a duplicate-key ambiguity if the
// caller's environment already carried one.
func setGHTokenEnv(environ []string, token string) []string {
	out := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if strings.HasPrefix(kv, "GH_TOKEN=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "GH_TOKEN="+token)
}

// findRealGHOnPATH walks PATH for an executable named "gh", skipping
// excludeDir (the shim's own directory) so the walk can never resolve back
// to the shim.
func findRealGHOnPATH(excludeDir string) (string, error) {
	excludeClean := ""
	if excludeDir != "" {
		excludeClean = filepath.Clean(excludeDir)
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		if excludeClean != "" && filepath.Clean(dir) == excludeClean {
			continue
		}
		candidate := filepath.Join(dir, "gh")
		if pathIsExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("gh not found on PATH")
}

func pathIsExecutableFile(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

// isSameExecutable reports whether a and b resolve to the same file on disk,
// following symlinks on both sides. It's the recursion guard: a shim config
// that (accidentally or otherwise) points real_path at the shim symlink
// itself, or at the loaf binary directly, must never be exec'd.
func isSameExecutable(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		ra = a
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		rb = b
	}
	infoA, err := os.Stat(ra)
	if err != nil {
		return false
	}
	infoB, err := os.Stat(rb)
	if err != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}

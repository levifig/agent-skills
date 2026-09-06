package devtool

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// BuildGoOptions drive one native build.
type BuildGoOptions struct {
	RootDir string
	Env     Env
	Runner  Runner
	Stdout  io.Writer
	Stderr  io.Writer
	// Builder compiles one target into dest. Nil means `go build`; tests inject
	// a stand-in that writes fixture bytes.
	Builder func(dest string, target Target, env Env) error
	// Linker publishes the dev launcher pointer after a successful non-release
	// build. Nil means RefreshDevBuildLink; tests inject a recorder.
	Linker func(launcher string) DevLinkResult
	// HostTarget overrides the platform bin/loaf is copied from. Nil means the
	// running process's GOOS/GOARCH.
	HostTarget *Target
}

// GoBuildArgs is the single source of truth for how the public loaf binary is
// compiled. Every channel (dev, release, verification) must agree with it.
//
// Release metadata is linked via -X main.buildCommit/buildDate only when the
// release workflow sets LOAF_BUILD_COMMIT / LOAF_BUILD_DATE; their absence is
// half of what marks a binary as a dev build (see internal/cli/version.go).
// -buildvcs=true stamps vcs.revision and vcs.modified, which a dev build
// reports as <version>+g<short-sha>[.dirty]. go.mod pins a toolchain that
// stamps linked worktrees; go1.26.6 did not.
func GoBuildArgs(output string, env Env) []string {
	return []string{"build", "-trimpath", "-buildvcs=true", "-ldflags", GoLdflags(env), "-o", output, "./cmd/loaf"}
}

// GoLdflags renders the linker flags for GoBuildArgs.
func GoLdflags(env Env) string {
	parts := []string{"-buildid="}
	if commit := strings.TrimSpace(env["LOAF_BUILD_COMMIT"]); commit != "" {
		parts = append(parts, "-X main.buildCommit="+commit)
	}
	if date := strings.TrimSpace(env["LOAF_BUILD_DATE"]); date != "" {
		parts = append(parts, "-X main.buildDate="+date)
	}
	return strings.Join(parts, " ")
}

// IsReleaseBuild reports whether release metadata is present.
func IsReleaseBuild(env Env) bool {
	return strings.TrimSpace(env["LOAF_BUILD_COMMIT"]) != "" || strings.TrimSpace(env["LOAF_BUILD_DATE"]) != ""
}

// BuildGo compiles the requested targets (LOAF_BUILD_TARGETS, else the host)
// into bin/native/<target>/, staging every binary first so a failing target
// leaves the previously published set intact. bin/loaf becomes a copy of the
// host platform's binary: it is the distribution's entry point, and since the
// Node launcher is gone it is the native binary itself. Non-release builds
// then point Loaf's user-local dev launcher at it.
func BuildGo(options BuildGoOptions) error {
	root := options.RootDir
	stdout := options.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := options.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	env := options.Env.With(map[string]string{"CGO_ENABLED": "0"}).With(PinnedToolchainEnv(root))
	runner := options.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	builder := options.Builder
	if builder == nil {
		builder = func(dest string, target Target, env Env) error {
			return runner.Run(root, env.With(map[string]string{"GOOS": target.GOOS, "GOARCH": target.GOARCH}), "go", GoBuildArgs(dest, env)...)
		}
	}
	linker := options.Linker
	if linker == nil {
		linker = func(launcher string) DevLinkResult {
			return RefreshDevBuildLink(launcher, DevLinkOptions{Env: env, Warn: func(message string) { fmt.Fprintln(stderr, message) }})
		}
	}

	targets, err := requestedTargets(env, root, runner, "LOAF_BUILD_TARGETS", "LOAF_NATIVE_TARGETS")
	if err != nil {
		return err
	}
	host := options.HostTarget
	if host == nil {
		if resolved, err := TargetForGo(runtime.GOOS, runtime.GOARCH); err == nil {
			host = &resolved
		}
	}
	dryRun := env["LOAF_NATIVE_ARTIFACT_DRY_RUN"] == "1"

	nativeRoot := filepath.Join(root, "bin", "native")
	staging := filepath.Join(nativeRoot, ".staging")
	launcher := filepath.Join(root, "bin", "loaf")
	if dryRun {
		fmt.Fprintf(stdout, "DRY RUN: would copy the host binary to %s\n", launcher)
		for _, target := range targets {
			fmt.Fprintf(stdout, "DRY RUN: would build %s at %s\n", target.RuntimeID, filepath.Join(nativeRoot, target.RuntimeID, target.BinaryName()))
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		return err
	}
	_ = os.RemoveAll(staging)
	defer os.RemoveAll(staging)

	type staged struct{ from, to string }
	var built []staged
	for _, target := range targets {
		dest := filepath.Join(staging, target.RuntimeID, target.BinaryName())
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := builder(dest, target, env); err != nil {
			return fmt.Errorf("go build for %s failed: %w", target.RuntimeID, err)
		}
		if err := os.Chmod(dest, 0o755); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ Built Go front controller (%s): %s\n", target.RuntimeID, filepath.Join(nativeRoot, target.RuntimeID, target.BinaryName()))
		built = append(built, staged{from: dest, to: filepath.Join(nativeRoot, target.RuntimeID, target.BinaryName())})
	}
	for _, artifact := range built {
		if err := os.MkdirAll(filepath.Dir(artifact.to), 0o755); err != nil {
			return err
		}
		if err := os.Rename(artifact.from, artifact.to); err != nil {
			_ = os.Remove(artifact.to)
			if err := os.Rename(artifact.from, artifact.to); err != nil {
				return err
			}
		}
	}

	// bin/loaf is the host platform's binary. A build that did not compile the
	// host keeps whatever bin/loaf already exists.
	if host != nil {
		hostBinary := filepath.Join(nativeRoot, host.RuntimeID, host.BinaryName())
		if containsTarget(targets, *host) {
			if err := copyFile(hostBinary, launcher, 0o755); err != nil {
				return fmt.Errorf("publish bin/loaf: %w", err)
			}
			fmt.Fprintf(stdout, "✓ Published Loaf entry point: %s\n", launcher)
		}
	}
	// The plugin shim's `package.json` sibling is gone; keep the old file from
	// lingering in checkouts that predate the change.
	_ = os.Remove(filepath.Join(root, "bin", "package.json"))

	if runtime.GOOS != "windows" && !IsReleaseBuild(env) && env["LOAF_DEV_LINK"] != "0" {
		if _, err := os.Stat(launcher); err == nil {
			result := linker(launcher)
			if result.Status == DevLinkLinked {
				fmt.Fprintf(stdout, "✓ Linked latest dev build: %s -> %s\n", result.Link, launcher)
			}
		}
	}
	return nil
}

func containsTarget(targets []Target, want Target) bool {
	for _, target := range targets {
		if target.RuntimeID == want.RuntimeID {
			return true
		}
	}
	return false
}

func copyFile(from, to string, mode os.FileMode) error {
	body, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	tmp := to + ".tmp"
	if err := os.WriteFile(tmp, body, mode); err != nil {
		return err
	}
	return os.Rename(tmp, to)
}

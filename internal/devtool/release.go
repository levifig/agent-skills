package devtool

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BuildOptions drive the full build pipeline.
type BuildOptions struct {
	RootDir string
	Env     Env
	Runner  Runner
	Stdout  io.Writer
	Stderr  io.Writer
}

// Build is `make build`: compile the native binaries, regenerate the CLI
// reference, build every content target with the fresh binary, and verify the
// result. It is the sequence the npm `build` script used to chain.
func Build(options BuildOptions) error {
	root := options.RootDir
	stdout := options.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := options.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	runner := options.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	if err := BuildGo(BuildGoOptions{RootDir: root, Env: options.Env, Runner: runner, Stdout: stdout, Stderr: stderr}); err != nil {
		return err
	}
	loaf := filepath.Join(root, "bin", "loaf")
	if err := runner.Run(root, options.Env, loaf, "__generate-cli-ref"); err != nil {
		return fmt.Errorf("generate CLI reference: %w", err)
	}
	if err := runner.Run(root, options.Env, loaf, "build"); err != nil {
		return fmt.Errorf("build content targets: %w", err)
	}
	return VerifyArtifacts(VerifyOptions{RootDir: root, Env: options.Env, Runner: runner, Stdout: stdout})
}

// ReleaseTargetsFromEnv resolves the release target list: LOAF_RELEASE_TARGETS,
// then LOAF_BUILD_TARGETS, else every release platform.
func ReleaseTargetsFromEnv(env Env) ([]Target, error) {
	for _, name := range []string{"LOAF_RELEASE_TARGETS", "LOAF_BUILD_TARGETS"} {
		if value := strings.TrimSpace(env[name]); value != "" {
			targets, err := ParseTargetList(value)
			if err != nil {
				return nil, err
			}
			if len(targets) == 0 {
				return nil, fmt.Errorf("release build target list is empty")
			}
			return targets, nil
		}
	}
	return ReleaseTargets(), nil
}

// Release builds every release platform (or the requested subset) and
// verifies the same set. LOAF_RELEASE_DRY_RUN=1 reports the plan instead.
func Release(options BuildOptions) error {
	stdout := options.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	targets, err := ReleaseTargetsFromEnv(options.Env)
	if err != nil {
		return err
	}
	ids := RuntimeIDs(targets)
	verify := strings.TrimSpace(options.Env["LOAF_VERIFY_TARGETS"])
	if verify == "" {
		verify = ids
	}
	env := options.Env.With(map[string]string{"LOAF_BUILD_TARGETS": ids, "LOAF_VERIFY_TARGETS": verify})
	if env["LOAF_RELEASE_DRY_RUN"] == "1" {
		fmt.Fprintf(stdout, "DRY RUN: would run loafdev build for native release targets: %s\n", ids)
		fmt.Fprintf(stdout, "DRY RUN: LOAF_VERIFY_TARGETS=%s\n", verify)
		return nil
	}
	options.Env = env
	return Build(options)
}

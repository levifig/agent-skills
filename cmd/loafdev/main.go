// loafdev is Loaf's development tool: it builds the native binaries, runs the
// full build pipeline, verifies artifacts, packages releases, updates the
// Homebrew formula, and classifies release tags. It replaces the npm scripts
// that used to do this and is invoked through `go run ./cmd/loafdev` or the
// Makefile. It is never shipped.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/levifig/loaf/internal/devtool"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(usage)
		return nil
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	env := devtool.EnvFromOS()
	build := devtool.BuildOptions{RootDir: root, Env: env, Stdout: os.Stdout, Stderr: os.Stderr}
	switch args[0] {
	case "build-go":
		return devtool.BuildGo(devtool.BuildGoOptions{RootDir: root, Env: env, Stdout: os.Stdout, Stderr: os.Stderr})
	case "build":
		return devtool.Build(build)
	case "verify-artifacts":
		return devtool.VerifyArtifacts(devtool.VerifyOptions{RootDir: root, Env: env, Stdout: os.Stdout})
	case "release":
		return devtool.Release(build)
	case "package":
		return devtool.Package(devtool.PackageOptions{RootDir: root, Env: env, Stdout: os.Stdout})
	case "classify-tag":
		if len(args) != 2 {
			return fmt.Errorf("classify-tag requires exactly one tag")
		}
		return devtool.WriteTagClassification(os.Stdout, os.Stderr, args[1])
	case "homebrew-formula":
		options, err := parseHomebrewArgs(args[1:])
		if err != nil {
			return err
		}
		if err := devtool.UpdateHomebrewFormula(options); err != nil {
			return err
		}
		fmt.Printf("✓ Updated %s for Loaf %s\n", options.FormulaPath, options.Version)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

func parseHomebrewArgs(args []string) (devtool.HomebrewOptions, error) {
	var options devtool.HomebrewOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			return options, fmt.Errorf("unexpected argument %s", arg)
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			return options, fmt.Errorf("%s requires a value", arg)
		}
		value := args[i+1]
		i++
		switch arg {
		case "--formula":
			options.FormulaPath = value
		case "--checksums":
			options.ChecksumsPath = value
		case "--version":
			options.Version = value
		case "--repo":
			options.Repo = value
		default:
			return options, fmt.Errorf("unknown option %s", arg)
		}
	}
	return options, nil
}

const usage = `Usage: go run ./cmd/loafdev <command>

Commands:
  build-go            Compile bin/native/<target>/loaf for LOAF_BUILD_TARGETS (default: this host)
                      and publish bin/loaf; non-release builds relink the dev launcher pointer
  build               build-go, then regenerate the CLI reference, build content targets, verify
  verify-artifacts    Check bin/ and the Claude Code plugin shim are present and consistent
  release             build for every release platform (or LOAF_RELEASE_TARGETS)
  package             Write dist/release archives and checksums.txt from bin/native
  classify-tag <tag>  Print tag=/ref=/dev= for the release workflow; malformed tags fail
  homebrew-formula    --formula <path> --checksums <path> --version <X.Y.Z> [--repo owner/name]

Environment:
  LOAF_BUILD_TARGETS, LOAF_VERIFY_TARGETS, LOAF_RELEASE_TARGETS   comma-separated target lists
  LOAF_BUILD_COMMIT, LOAF_BUILD_DATE                              release metadata (release workflow only)
  LOAF_DEV_LINK=0                                                 do not touch the dev launcher pointer
  LOAF_NATIVE_ARTIFACT_DRY_RUN=1, LOAF_RELEASE_DRY_RUN=1          report instead of acting
`

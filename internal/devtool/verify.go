package devtool

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// VerifyOptions drive VerifyArtifacts.
type VerifyOptions struct {
	RootDir string
	Env     Env
	Runner  Runner
	Stdout  io.Writer
}

// VerifyArtifacts checks that a build left the distribution consistent: the
// bin/loaf entry point and a native binary per requested target exist, and
// the Claude Code plugin ships exactly its shim (equal to the embedded source)
// with no native runtime of its own. Binaries carry a -buildvcs stamp, so two
// builds of the same source differ by design; nothing here compares bytes
// against a rebuild.
func VerifyArtifacts(options VerifyOptions) error {
	root := options.RootDir
	stdout := options.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	runner := options.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	env := options.Env.With(map[string]string{"CGO_ENABLED": "0"}).With(PinnedToolchainEnv(root))
	targets, err := requestedTargets(env, root, runner, "LOAF_VERIFY_TARGETS", "LOAF_BUILD_TARGETS", "LOAF_NATIVE_TARGETS")
	if err != nil {
		return err
	}

	required := []string{
		"internal/cli/claude_plugin_shim.sh",
		"bin/loaf",
		"plugins/loaf/bin/loaf",
	}
	for _, target := range targets {
		required = append(required, filepath.ToSlash(filepath.Join("bin", "native", target.RuntimeID, target.BinaryName())))
	}
	if env["LOAF_NATIVE_ARTIFACT_DRY_RUN"] == "1" {
		for _, rel := range required {
			fmt.Fprintf(stdout, "DRY RUN: would verify %s\n", rel)
		}
		return nil
	}
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("missing required artifact: %s", rel)
		}
	}
	shim, err := os.ReadFile(filepath.Join(root, "internal", "cli", "claude_plugin_shim.sh"))
	if err != nil {
		return err
	}
	published, err := os.ReadFile(filepath.Join(root, "plugins", "loaf", "bin", "loaf"))
	if err != nil {
		return err
	}
	if !bytes.Equal(shim, published) {
		return fmt.Errorf("plugins/loaf/bin/loaf is stale; run make build")
	}
	for _, rel := range []string{"plugins/loaf/bin/native", "plugins/loaf/bin/package.json", "bin/package.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			return fmt.Errorf("%s must not exist; the plugin ships no native runtime and the Node launcher is gone. Run make build", rel)
		}
	}
	fmt.Fprintln(stdout, "Go command artifacts are present and synchronized.")
	return nil
}

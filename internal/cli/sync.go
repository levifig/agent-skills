package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
	"github.com/levifig/loaf/internal/sync"
)

type syncOptions struct {
	bundlePath string
	jsonOutput     bool
}

func (r Runner) runClientSync(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeClientSyncHelp(out)
		return nil
	}
	opts, err := parseClientSyncArgs(args)
	if err != nil {
		return err
	}
	root, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	ctx := context.Background()
	resolver := state.PathResolver{StateHome: r.StateHome}
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return err
	}
	store, err := state.OpenStore(databasePath)
	if err != nil {
		return err
	}
	defer store.Close()

	rawBundle, err := os.ReadFile(opts.bundlePath)
	if err != nil {
		return fmt.Errorf("read sync bundle: %w", err)
	}
	bundle, err := crypto.DecodeBundledClientCredential(strings.TrimSpace(string(rawBundle)))
	if err != nil {
		return fmt.Errorf("decode sync bundle: %w", err)
	}

	engine, err := sync.NewEngine(sync.Config{Store: store, Credential: bundle})
	if err != nil {
		return err
	}
	result, err := engine.Sync(ctx)
	if err != nil {
		return err
	}
	if opts.jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	fmt.Fprintf(out, "sync ok: queued=%d pushed=%d duplicates=%d pulled=%d cursor=%d\n",
		result.Queued, result.Pushed, result.Duplicates, result.Pulled, result.Cursor)
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "warning: %s\n", warning)
	}
	return nil
}

func writeClientSyncHelp(out io.Writer) {
	writeUsageHelp(out, "loaf sync --bundle <path> [--json]", "Push and pull grow-only facts through the sync relay.",
		"--bundle <path>  Bundled project client bundle file path",
		"--json               Emit structured sync result")
}

func parseClientSyncArgs(args []string) (syncOptions, error) {
	opts := syncOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--bundle":
			if i+1 >= len(args) {
				return syncOptions{}, fmt.Errorf("sync --bundle requires a value")
			}
			i++
			opts.bundlePath = strings.TrimSpace(args[i])
		case "--json":
			opts.jsonOutput = true
		default:
			return syncOptions{}, fmt.Errorf("sync: unknown option %q", args[i])
		}
	}
	if opts.bundlePath == "" {
		return syncOptions{}, fmt.Errorf("sync requires --bundle")
	}
	return opts, nil
}

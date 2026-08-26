package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func (r Runner) runAttach(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 1 && isHelpArg(args) {
		writeUsageHelp(out, "loaf attach [--name <connection>] [--json]", "Run the unattended attach ceremony from conf ID and LOAF_CLIENT_TOKEN.",
			"--name <connection>  Connection name recorded in attach state (default: LOAF_CONNECTION_NAME or token id)",
			"--json               Emit structured attach refusal on failure")
		return nil
	}
	name := strings.TrimSpace(os.Getenv("LOAF_CONNECTION_NAME"))
	jsonOutput := false
	allowHTTP := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 >= len(args) {
				return fmt.Errorf("attach --name requires a value")
			}
			i++
			name = strings.TrimSpace(args[i])
		case "--allow-insecure-http":
			allowHTTP = true
		case "--json":
			jsonOutput = true
		default:
			return fmt.Errorf("attach: unknown option %q", args[i])
		}
	}
	root, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	authStore, err := r.authStore()
	if err != nil {
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	dbPath, err := resolver.DatabasePath(root)
	if err != nil {
		return err
	}
	projectStore, err := state.OpenStore(dbPath)
	if err != nil {
		return err
	}
	defer projectStore.Close()
	if err := projectStore.ApplyMigrations(context.Background()); err != nil {
		return err
	}

	result, err := auth.UnattendedAttach(context.Background(), auth.AttachInput{
		Root:              root,
		Store:             authStore,
		ClientWire:        os.Getenv(auth.ClientTokenEnv),
		ConnectionName:    name,
		ProbeStore:        projectStore,
		AllowInsecureHTTP: allowHTTP,
	})
	if err != nil {
		errOut := r.Stderr
		if errOut == nil {
			errOut = os.Stderr
		}
		if jsonOutput {
			if payload, jerr := auth.AttachRefusalJSON(err); jerr == nil {
				fmt.Fprintln(errOut, string(payload))
			}
		}
		var refusal *auth.RefusalError
		if errors.As(err, &refusal) {
			return writeAttachRefusal(errOut, refusal, jsonOutput)
		}
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "attached project %s as %q via %s\n", result.ProjectID, result.ConnectionName, result.Endpoint)
	return nil
}

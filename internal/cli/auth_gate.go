package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func (r Runner) enforceAttachGate(args []string, runtime state.Runtime, errOut io.Writer) error {
	if len(args) == 0 || auth.IsAuthNamespace(args) || !auth.CommandRequiresAttach(args) {
		return nil
	}
	dataHome, err := r.resolveDataHome()
	if err != nil {
		return err
	}
	if !auth.SubstrateModeEnabled(dataHome) {
		return nil
	}
	root, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return nil
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return nil
	}
	store, err := state.OpenStore(databasePath)
	if err != nil {
		return nil
	}
	defer store.Close()
	identity, err := store.ProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		return nil
	}
	command := strings.Join(args, " ")
	if err := auth.CheckAttached(dataHome, identity.ID, command); err != nil {
		if !hasFlag(args, "--json") {
			fmt.Fprintln(errOut, err.Error())
		}
		return err
	}
	return nil
}

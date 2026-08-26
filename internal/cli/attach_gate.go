package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/state"
)

func (r Runner) authStore() (auth.Store, error) {
	dir, err := state.PathResolver{StateHome: r.StateHome}.AuthDir()
	if err != nil {
		return auth.Store{}, err
	}
	return auth.NewStore(dir), nil
}

func (r Runner) enforceAttachGate(args []string, errOut io.Writer) error {
	if argsRequestHelp(args) || !auth.CommandRequiresAttach(args) {
		return nil
	}
	store, err := r.authStore()
	if err != nil {
		return err
	}
	active, err := store.EnforcementActive()
	if err != nil {
		return err
	}
	if !active {
		return nil
	}
	attached, err := store.IsAttached()
	if err != nil {
		return err
	}
	if attached {
		return nil
	}
	refusal := auth.NewUnattachedRefusal(strings.Join(args, " "))
	if hasFlag(args, "--json") {
		payload, err := refusal.JSON()
		if err != nil {
			return err
		}
		fmt.Fprintln(errOut, string(payload))
	} else {
		fmt.Fprintln(errOut, refusal.Error())
		if suggestion := strings.TrimSpace(refusal.Suggestion); suggestion != "" {
			fmt.Fprintln(errOut, suggestion)
		}
	}
	return ExitError{Code: 1}
}

func argsRequestHelp(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--help", "-h", "help":
			return true
		}
	}
	return false
}

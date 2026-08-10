package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// hook_state.go opens the user-scoped state hook reconciliation reads. The
// records are host-local and per target, not per project, but they live in the
// same global database as everything else Loaf remembers, so they are reached
// through the same resolver the rest of the CLI uses.
//
// Two resolvers exist because the two callers make different promises. Apply
// may create the database, because a host that has never run Loaf still has to
// be able to record an absorption marker. A plan may not: a dry run that
// created a database would be a write, however small, and the plan surface
// promises none.

// hookStateResolver is the reconciler's handle on the store. It is a function
// so that a target which never reconciles a hooks file never opens a database,
// and so that the open happens once per run rather than once per target.
type hookStateResolver func() (*state.Store, error)

func (r Runner) hookStateForApply(projectRoot string) (hookStateResolver, func()) {
	return r.hookStateResolver(projectRoot, true)
}

func (r Runner) hookStateForPlan(projectRoot string) (hookStateResolver, func()) {
	return r.hookStateResolver(projectRoot, false)
}

func (r Runner) hookStateResolver(projectRoot string, create bool) (hookStateResolver, func()) {
	var (
		store    *state.Store
		openErr  error
		resolved bool
	)
	resolve := func() (*state.Store, error) {
		if !resolved {
			resolved = true
			store, openErr = r.openHookStateStore(projectRoot, create)
		}
		return store, openErr
	}
	release := func() {
		if store != nil {
			store.Close()
			store = nil
		}
	}
	return resolve, release
}

func (r Runner) openHookStateStore(projectRoot string, create bool) (*state.Store, error) {
	root, err := project.ResolveRoot(projectRoot)
	if err != nil {
		return nil, err
	}
	path, err := state.PathResolver{StateHome: r.StateHome}.DatabasePath(root)
	if err != nil {
		return nil, err
	}
	if !create {
		return openHookStateStoreReadOnly(path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create state database directory: %w", err)
	}
	store, err := state.OpenStore(path)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	bootstrapped, err := store.BootstrapIfEmpty(ctx)
	if err != nil {
		store.Close()
		return nil, err
	}
	if !bootstrapped {
		if err := store.RequireCurrentSchema(ctx); err != nil {
			store.Close()
			return nil, fmt.Errorf("hook enablement state at %s cannot be read: %w", path, err)
		}
	}
	return store, nil
}

// openHookStateStoreReadOnly is the plan's half of the resolver, and every
// branch of it leaves the database exactly as it found it: a missing file is
// not created, an uninitialized one is not bootstrapped, and the connection is
// opened read-only so not even the journal mode moves. Both of those states
// mean the same thing to a plan — nothing has been recorded, so every hook
// reads enabled — and it says so rather than writing to find out.
func openHookStateStoreReadOnly(path string) (*state.Store, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect hook enablement state at %s: %w", path, err)
	}
	store, err := state.OpenStoreReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("hook enablement state at %s cannot be read: %w", path, err)
	}
	ctx := context.Background()
	// A database that has never been initialized carries no records, which is a
	// perfectly good answer for a plan. A database that merely fails to read as
	// Loaf's is not the same thing and must not be reported as one: an ordinary
	// schema read fails identically on both, so the emptiness question is asked
	// on its own rather than inferred from the failure.
	uninitialized, err := store.IsUninitialized(ctx)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("hook enablement state at %s cannot be read: %w", path, err)
	}
	if uninitialized {
		store.Close()
		return nil, nil
	}
	if err := store.RequireCurrentSchema(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("hook enablement state at %s cannot be read: %w", path, err)
	}
	return store, nil
}

// hookRecords is the reconciler's read view of the user-scoped records for one
// target. Absence of an enablement row means enabled, so an empty view — the
// state of a host with no database yet — projects every catalog hook.
type hookRecords struct {
	enablement map[string]state.HookEnablement
	absorbed   bool
	trusted    []string
}

func loadHookRecords(ctx context.Context, store *state.Store, target string) (hookRecords, error) {
	records := hookRecords{enablement: map[string]state.HookEnablement{}}
	if store == nil {
		return records, nil
	}
	rows, err := store.ListHookEnablements(ctx, target)
	if err != nil {
		return hookRecords{}, err
	}
	for _, row := range rows {
		records.enablement[hookRecordKey(row.Event, row.HookID)] = row
	}
	_, marked, err := store.GetHookAbsorptionMarker(ctx, target)
	if err != nil {
		return hookRecords{}, err
	}
	records.absorbed = marked
	paths, err := store.ListHookTrustedPaths(ctx, target)
	if err != nil {
		return hookRecords{}, err
	}
	for _, path := range paths {
		records.trusted = append(records.trusted, path.Path)
	}
	return records, nil
}

// enabled answers the projection question for one identity. Absence means
// enabled; a row means whatever the operator last said.
func (r hookRecords) enabled(event string, hookID string) bool {
	row, recorded := r.enablement[hookRecordKey(event, hookID)]
	return !recorded || row.Enablement == state.HookEnablementEnabled
}

func (r hookRecords) recorded(event string, hookID string) bool {
	_, ok := r.enablement[hookRecordKey(event, hookID)]
	return ok
}

func hookRecordKey(event string, hookID string) string {
	return event + "/" + hookID
}

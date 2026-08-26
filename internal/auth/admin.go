package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/syncserver"
)

// AdminContext holds decoded admin authority and an open sync server store.
type AdminContext struct {
	Wire       crypto.AccountAdminCredential
	ServerDB   string
	Server     *syncserver.Store
}

// OpenAdminContext loads the local admin wire and opens the configured sync server DB.
func OpenAdminContext(ctx context.Context, store Store, serverDBOverride string) (AdminContext, func(), error) {
	wireRaw, err := store.LoadAdminWire()
	if err != nil {
		return AdminContext{}, nil, err
	}
	wire, err := crypto.DecodeAccountAdminCredential(wireRaw)
	if err != nil {
		return AdminContext{}, nil, fmt.Errorf("decode admin wire: %w", err)
	}
	local, err := store.LoadLocalConfig()
	if err != nil {
		return AdminContext{}, nil, err
	}
	serverDB := strings.TrimSpace(serverDBOverride)
	if serverDB == "" {
		serverDB = strings.TrimSpace(local.ServerDB)
	}
	if serverDB == "" {
		return AdminContext{}, nil, fmt.Errorf("sync server database path is required (--server-db or prior setup)")
	}
	server, err := syncserver.OpenStore(serverDB)
	if err != nil {
		return AdminContext{}, nil, err
	}
	cleanup := func() { _ = server.Close() }
	if err := server.AuthenticateAdmin(ctx, wire.AccountAccessKey, wire.AccountAccessSecret); err != nil {
		cleanup()
		return AdminContext{}, nil, fmt.Errorf("admin authentication failed: %w", err)
	}
	return AdminContext{Wire: wire, ServerDB: serverDB, Server: server}, cleanup, nil
}

// AdminEndpoint returns the configured sync endpoint from stored admin wire.
func AdminEndpoint(store Store) (string, error) {
	wireRaw, err := store.LoadAdminWire()
	if err != nil {
		return "", err
	}
	wire, err := crypto.DecodeAccountAdminCredential(wireRaw)
	if err != nil {
		return "", fmt.Errorf("decode admin wire: %w", err)
	}
	return strings.TrimSpace(wire.Endpoint), nil
}


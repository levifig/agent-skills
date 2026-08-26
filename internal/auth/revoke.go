package auth

import (
	"context"
	"fmt"
	"strings"
)

// RevokeConnection revokes a named connection token. Effective at the client's next server contact.
func RevokeConnection(ctx context.Context, store Store, name, serverDB string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("auth revoke: connection name is required")
	}
	adminCtx, cleanup, err := OpenAdminContext(ctx, store, serverDB)
	if err != nil {
		return err
	}
	defer cleanup()

	revoked, err := adminCtx.Server.RevokeConnectionToken(ctx, adminCtx.Wire.AccountAccessKey, adminCtx.Wire.AccountAccessSecret, name)
	if err != nil {
		return fmt.Errorf("revoke connection token: %w", err)
	}
	if !revoked {
		return fmt.Errorf("auth revoke: connection %q not found", name)
	}
	return nil
}

package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/levifig/loaf/internal/syncserver"
)

// ConnectionSummary is a listed connection token row.
type ConnectionSummary struct {
	Name       string     `json:"name"`
	ProjectID  string     `json:"project_id"`
	TokenID    string     `json:"token_id"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// ListConnections returns named connection tokens for the configured admin account.
func ListConnections(ctx context.Context, store Store, serverDB string) ([]ConnectionSummary, error) {
	adminCtx, cleanup, err := OpenAdminContext(ctx, store, serverDB)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	tokens, err := adminCtx.Server.ListConnectionTokens(ctx, adminCtx.Wire.AccountAccessKey, adminCtx.Wire.AccountAccessSecret)
	if err != nil {
		return nil, fmt.Errorf("list connection tokens: %w", err)
	}
	out := make([]ConnectionSummary, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, summarizeConnection(token))
	}
	return out, nil
}

func summarizeConnection(token syncserver.ConnectionToken) ConnectionSummary {
	return ConnectionSummary{
		Name:       token.Name,
		ProjectID:  token.ProjectID,
		TokenID:    token.TokenID,
		CreatedAt:  token.CreatedAt,
		LastSeenAt: token.LastSeenAt,
		RevokedAt:  token.RevokedAt,
	}
}

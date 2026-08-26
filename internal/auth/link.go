package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/crypto"
)

// LinkInput mints a named connection token and bundled client wire.
type LinkInput struct {
	Name      string
	ProjectID string
	ServerDB  string
}

// LinkResult is the minted client bundle and connection metadata.
type LinkResult struct {
	Name              string `json:"name"`
	ProjectID         string `json:"project_id"`
	ConnectionTokenID string `json:"connection_token_id"`
	ClientWire        string `json:"client_wire"`
}

// Link mints a project-scoped connection token and emits the one-string bundled client wire.
func Link(ctx context.Context, store Store, in LinkInput) (LinkResult, error) {
	name := strings.TrimSpace(in.Name)
	projectID := strings.TrimSpace(in.ProjectID)
	if name == "" {
		return LinkResult{}, fmt.Errorf("auth link: connection name is required")
	}
	if projectID == "" {
		return LinkResult{}, fmt.Errorf("auth link: project id is required")
	}
	adminCtx, cleanup, err := OpenAdminContext(ctx, store, in.ServerDB)
	if err != nil {
		return LinkResult{}, err
	}
	defer cleanup()

	token, tokenSecret, err := adminCtx.Server.MintConnectionToken(
		ctx,
		adminCtx.Wire.AccountAccessKey,
		adminCtx.Wire.AccountAccessSecret,
		name,
		projectID,
	)
	if err != nil {
		return LinkResult{}, fmt.Errorf("mint connection token: %w", err)
	}
	ring := crypto.NewProjectKeyRing(adminCtx.Wire.MasterKey, projectID)
	projectKey, err := ring.WriteKey()
	if err != nil {
		return LinkResult{}, fmt.Errorf("derive project key: %w", err)
	}
	bundle := crypto.BundledClientCredential{
		Endpoint:        adminCtx.Wire.Endpoint,
		ConnectionToken: token.TokenID + ":" + tokenSecret,
		ProjectID:       projectID,
		KeyGeneration:   ring.WriteGeneration,
		ProjectKey:      projectKey[:],
	}
	clientWire, err := crypto.EncodeBundledClientCredential(bundle)
	if err != nil {
		return LinkResult{}, fmt.Errorf("encode client wire: %w", err)
	}
	return LinkResult{
		Name:              name,
		ProjectID:         projectID,
		ConnectionTokenID: token.TokenID,
		ClientWire:        clientWire,
	}, nil
}

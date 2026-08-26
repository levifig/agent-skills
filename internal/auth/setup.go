package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/syncserver"
)

// SetupInput configures first-time admin account provisioning.
type SetupInput struct {
	Endpoint string
	ServerDB string
}

// SetupResult summarizes a successful setup.
type SetupResult struct {
	Endpoint         string `json:"endpoint"`
	AccountAccessKey string `json:"account_access_key"`
	EmergencyKit     string `json:"emergency_kit"`
	MasterFingerprint string `json:"master_fingerprint"`
	ServerDB         string `json:"server_db"`
}

// Setup creates a zero-PII sync account, mints a master key, stores admin wire locally, and returns the one-time Emergency Kit.
func Setup(ctx context.Context, store Store, in SetupInput) (SetupResult, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(in.Endpoint), "/")
	serverDB := strings.TrimSpace(in.ServerDB)
	if endpoint == "" {
		return SetupResult{}, fmt.Errorf("auth setup: endpoint is required")
	}
	if serverDB == "" {
		return SetupResult{}, fmt.Errorf("auth setup: server database path is required")
	}
	server, err := syncserver.OpenStore(serverDB)
	if err != nil {
		return SetupResult{}, err
	}
	defer server.Close()

	account, accessSecret, err := server.CreateAccountWithKey(ctx, "")
	if err != nil {
		return SetupResult{}, fmt.Errorf("create sync account: %w", err)
	}
	master, kit, err := crypto.EmergencyKitSetup()
	if err != nil {
		return SetupResult{}, fmt.Errorf("mint emergency kit: %w", err)
	}
	admin := crypto.AccountAdminCredential{
		Endpoint:            endpoint,
		AccountAccessKey:    account.AccessKeyID,
		AccountAccessSecret: accessSecret,
		MasterKey:           master,
	}
	wire, err := crypto.EncodeAccountAdminCredential(admin)
	if err != nil {
		return SetupResult{}, fmt.Errorf("encode admin wire: %w", err)
	}
	if err := store.SaveAdminWire(wire); err != nil {
		return SetupResult{}, err
	}
	if err := store.SaveLocalConfig(LocalConfig{ServerDB: serverDB}); err != nil {
		return SetupResult{}, err
	}
	return SetupResult{
		Endpoint:          endpoint,
		AccountAccessKey:  account.AccessKeyID,
		EmergencyKit:      kit,
		MasterFingerprint: crypto.MasterKeyFingerprint(master),
		ServerDB:          serverDB,
	}, nil
}

// MarkAttached records successful attach for this environment.
func MarkAttached(store Store, endpoint, connectionName string) error {
	return store.SaveAttachState(AttachState{
		Attached:       true,
		Endpoint:       strings.TrimSpace(endpoint),
		ConnectionName: strings.TrimSpace(connectionName),
		AttachedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	})
}

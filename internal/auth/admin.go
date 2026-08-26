package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/syncserver"
)

// AdminConfig names machine-local auth storage and the sync relay database.
type AdminConfig struct {
	DataHome string
	ServerDB string
}

// SetupResult summarizes a new account bootstrap.
type SetupResult struct {
	AccessKeyID  string `json:"access_key_id"`
	AccessSecret string `json:"access_secret,omitempty"`
	EmergencyKit string `json:"emergency_kit"`
	Endpoint     string `json:"endpoint"`
	ServerDB     string `json:"server_db"`
}

// LinkOptions mints a named project-scoped client credential.
type LinkOptions struct {
	Name      string
	ProjectID string
}

// LinkResult carries the bundled client credential string.
type LinkResult struct {
	Name                string `json:"name"`
	ProjectID           string `json:"project_id"`
	ConnectionTokenID   string `json:"connection_token_id"`
	BundledCredential   string `json:"bundled_credential"`
	KeyGeneration       int    `json:"key_generation"`
}

// SetupAccount creates a zero-PII relay account, mints a master key, stores the admin credential, and prints the Emergency Kit once.
func SetupAccount(ctx context.Context, cfg AdminConfig, endpoint string) (SetupResult, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return SetupResult{}, fmt.Errorf("auth setup: endpoint is empty")
	}
	serverDB, err := resolveServerDB(cfg)
	if err != nil {
		return SetupResult{}, err
	}
	store, err := syncserver.OpenStore(serverDB)
	if err != nil {
		return SetupResult{}, err
	}
	defer store.Close()

	account, accessSecret, err := store.CreateAccountWithKey(ctx, "")
	if err != nil {
		return SetupResult{}, fmt.Errorf("auth setup: create account: %w", err)
	}
	master, emergencyKit, err := crypto.EmergencyKitSetup()
	if err != nil {
		return SetupResult{}, fmt.Errorf("auth setup: emergency kit: %w", err)
	}
	adminCred := crypto.AccountAdminCredential{
		Endpoint:            endpoint,
		AccountAccessKey:    account.AccessKeyID,
		AccountAccessSecret: accessSecret,
		MasterKey:           master,
	}
	if err := SaveAdminCredential(cfg.DataHome, adminCred); err != nil {
		return SetupResult{}, err
	}
	return SetupResult{
		AccessKeyID:  account.AccessKeyID,
		AccessSecret: accessSecret,
		EmergencyKit: emergencyKit,
		Endpoint:     endpoint,
		ServerDB:     serverDB,
	}, nil
}

// LinkConnection mints a named connection token and emits the bundled client credential.
func LinkConnection(ctx context.Context, cfg AdminConfig, opts LinkOptions) (LinkResult, error) {
	opts.Name = strings.TrimSpace(opts.Name)
	opts.ProjectID = strings.TrimSpace(opts.ProjectID)
	if opts.Name == "" {
		return LinkResult{}, fmt.Errorf("auth link: connection name is empty")
	}
	if opts.ProjectID == "" {
		return LinkResult{}, fmt.Errorf("auth link: project id is empty")
	}
	adminCred, err := LoadAdminCredential(cfg.DataHome)
	if err != nil {
		return LinkResult{}, err
	}
	serverDB, err := resolveServerDB(cfg)
	if err != nil {
		return LinkResult{}, err
	}
	store, err := syncserver.OpenStore(serverDB)
	if err != nil {
		return LinkResult{}, err
	}
	defer store.Close()

	token, tokenSecret, err := store.MintConnectionToken(ctx, adminCred.AccountAccessKey, adminCred.AccountAccessSecret, opts.Name, opts.ProjectID)
	if err != nil {
		return LinkResult{}, fmt.Errorf("auth link: mint connection token: %w", err)
	}
	ring := crypto.NewProjectKeyRing(adminCred.MasterKey, opts.ProjectID)
	projectKey, err := ring.WriteKey()
	if err != nil {
		return LinkResult{}, fmt.Errorf("auth link: derive project key: %w", err)
	}
	endpoint := strings.TrimRight(strings.TrimSpace(adminCred.Endpoint), "/")
	bundled, err := crypto.EncodeBundledClientCredential(crypto.BundledClientCredential{
		Endpoint:        endpoint,
		ConnectionToken: token.TokenID + ":" + tokenSecret,
		ProjectID:       opts.ProjectID,
		KeyGeneration:   ring.WriteGeneration,
		ProjectKey:      projectKey[:],
	})
	if err != nil {
		return LinkResult{}, fmt.Errorf("auth link: encode bundled credential: %w", err)
	}
	return LinkResult{
		Name:              opts.Name,
		ProjectID:         opts.ProjectID,
		ConnectionTokenID: token.TokenID,
		BundledCredential: bundled,
		KeyGeneration:     ring.WriteGeneration,
	}, nil
}

// ListConnections returns named connection tokens for the stored admin account.
func ListConnections(ctx context.Context, cfg AdminConfig) ([]syncserver.ConnectionToken, error) {
	adminCred, err := LoadAdminCredential(cfg.DataHome)
	if err != nil {
		return nil, err
	}
	serverDB, err := resolveServerDB(cfg)
	if err != nil {
		return nil, err
	}
	store, err := syncserver.OpenStore(serverDB)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.ListConnectionTokens(ctx, adminCred.AccountAccessKey, adminCred.AccountAccessSecret)
}

// RevokeConnection revokes a named connection token. Revocation takes effect at the client's next server contact.
func RevokeConnection(ctx context.Context, cfg AdminConfig, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, fmt.Errorf("auth revoke: connection name is empty")
	}
	adminCred, err := LoadAdminCredential(cfg.DataHome)
	if err != nil {
		return false, err
	}
	serverDB, err := resolveServerDB(cfg)
	if err != nil {
		return false, err
	}
	store, err := syncserver.OpenStore(serverDB)
	if err != nil {
		return false, err
	}
	defer store.Close()
	return store.RevokeConnectionToken(ctx, adminCred.AccountAccessKey, adminCred.AccountAccessSecret, name)
}

// SaveAdminCredential persists the account admin credential for this machine.
func SaveAdminCredential(dataHome string, cred crypto.AccountAdminCredential) error {
	encoded, err := crypto.EncodeAccountAdminCredential(cred)
	if err != nil {
		return fmt.Errorf("encode admin credential: %w", err)
	}
	path := AdminCredentialPath(dataHome)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create auth directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		return fmt.Errorf("write admin credential: %w", err)
	}
	return nil
}

// LoadAdminCredential reads the machine-local admin credential.
func LoadAdminCredential(dataHome string) (crypto.AccountAdminCredential, error) {
	path := AdminCredentialPath(dataHome)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return crypto.AccountAdminCredential{}, fmt.Errorf("admin credential not found at %s; run `loaf auth setup` first", path)
		}
		return crypto.AccountAdminCredential{}, fmt.Errorf("read admin credential: %w", err)
	}
	return crypto.DecodeAccountAdminCredential(strings.TrimSpace(string(raw)))
}

func resolveServerDB(cfg AdminConfig) (string, error) {
	if strings.TrimSpace(cfg.ServerDB) != "" {
		return filepath.Clean(cfg.ServerDB), nil
	}
	if strings.TrimSpace(cfg.DataHome) == "" {
		return "", fmt.Errorf("auth: server database path is required when data home is unset")
	}
	return SyncServerDBPath(cfg.DataHome), nil
}

// SubstrateModeEnabled reports whether this machine has completed auth setup.
func SubstrateModeEnabled(dataHome string) bool {
	_, err := LoadAdminCredential(dataHome)
	return err == nil
}

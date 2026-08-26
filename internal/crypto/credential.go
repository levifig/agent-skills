package crypto

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	bundledClientCredentialVersion = 1
	accountAdminCredentialVersion  = 1
)

// BundledClientCredential is a paste-able project-scoped client secret.
// It never carries the master key or account admin secret — only the
// derived project key for one project channel.
type BundledClientCredential struct {
	Kind            string `json:"kind"`
	Endpoint        string `json:"endpoint"`
	ConnectionToken string `json:"connection_token"`
	ProjectID       string `json:"project_id"`
	KeyGeneration   int    `json:"key_generation"`
	ProjectKey      []byte `json:"project_key"`
}

// AccountAdminCredential is the trusted-machine admin credential.
// It is a distinct type from BundledClientCredential and cannot be
// encoded or decoded through the client credential wire format.
type AccountAdminCredential struct {
	Kind                string    `json:"kind"`
	Endpoint            string    `json:"endpoint"`
	AccountAccessKey    string    `json:"account_access_key"`
	AccountAccessSecret string    `json:"account_access_secret"`
	MasterKey           MasterKey `json:"master_key"`
}

const (
	credentialKindClient = "project_client"
	credentialKindAdmin  = "account_admin"
)

// EncodeBundledClientCredential renders a project client credential as one string.
func EncodeBundledClientCredential(c BundledClientCredential) (string, error) {
	if strings.TrimSpace(c.Endpoint) == "" {
		return "", fmt.Errorf("encode bundled credential: endpoint is empty")
	}
	if strings.TrimSpace(c.ConnectionToken) == "" {
		return "", fmt.Errorf("encode bundled credential: connection token is empty")
	}
	if strings.TrimSpace(c.ProjectID) == "" {
		return "", fmt.Errorf("encode bundled credential: project id is empty")
	}
	if len(c.ProjectKey) != projectKeySize {
		return "", fmt.Errorf("encode bundled credential: project key must be %d bytes", projectKeySize)
	}
	c.Kind = credentialKindClient
	payload, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode bundled credential: %w", err)
	}
	return fmt.Sprintf("loafclient%d://%s", bundledClientCredentialVersion, base64.RawURLEncoding.EncodeToString(payload)), nil
}

// DecodeBundledClientCredential parses a project client credential string.
// Structurally refuses admin wire formats and admin-only JSON fields.
func DecodeBundledClientCredential(raw string) (BundledClientCredential, error) {
	raw = strings.TrimSpace(raw)
	prefix := fmt.Sprintf("loafclient%d://", bundledClientCredentialVersion)
	if strings.HasPrefix(raw, fmt.Sprintf("loafadmin%d://", accountAdminCredentialVersion)) {
		return BundledClientCredential{}, fmt.Errorf("decode bundled credential: account admin credential is not a project client credential")
	}
	if !strings.HasPrefix(raw, prefix) {
		return BundledClientCredential{}, fmt.Errorf("decode bundled credential: unsupported prefix")
	}
	body, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(raw, prefix))
	if err != nil {
		return BundledClientCredential{}, fmt.Errorf("decode bundled credential: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return BundledClientCredential{}, fmt.Errorf("decode bundled credential: %w", err)
	}
	for _, forbidden := range []string{"master_key", "account_access_key", "account_access_secret"} {
		if _, ok := fields[forbidden]; ok {
			return BundledClientCredential{}, fmt.Errorf("decode bundled credential: admin field %q is not allowed on project client credentials", forbidden)
		}
	}
	var c BundledClientCredential
	if err := json.Unmarshal(body, &c); err != nil {
		return BundledClientCredential{}, fmt.Errorf("decode bundled credential: %w", err)
	}
	if c.Kind != "" && c.Kind != credentialKindClient {
		return BundledClientCredential{}, fmt.Errorf("decode bundled credential: unexpected kind %q", c.Kind)
	}
	if len(c.ProjectKey) != projectKeySize {
		return BundledClientCredential{}, fmt.Errorf("decode bundled credential: project key must be %d bytes", projectKeySize)
	}
	c.Kind = credentialKindClient
	return c, nil
}

// EncodeAccountAdminCredential renders an admin credential as a distinct wire string.
func EncodeAccountAdminCredential(c AccountAdminCredential) (string, error) {
	if strings.TrimSpace(c.Endpoint) == "" {
		return "", fmt.Errorf("encode admin credential: endpoint is empty")
	}
	if strings.TrimSpace(c.AccountAccessKey) == "" {
		return "", fmt.Errorf("encode admin credential: account access key is empty")
	}
	if strings.TrimSpace(c.AccountAccessSecret) == "" {
		return "", fmt.Errorf("encode admin credential: account access secret is empty")
	}
	c.Kind = credentialKindAdmin
	payload, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode admin credential: %w", err)
	}
	return fmt.Sprintf("loafadmin%d://%s", accountAdminCredentialVersion, base64.RawURLEncoding.EncodeToString(payload)), nil
}

// DecodeAccountAdminCredential parses an account admin credential string.
func DecodeAccountAdminCredential(raw string) (AccountAdminCredential, error) {
	raw = strings.TrimSpace(raw)
	prefix := fmt.Sprintf("loafadmin%d://", accountAdminCredentialVersion)
	if strings.HasPrefix(raw, fmt.Sprintf("loafclient%d://", bundledClientCredentialVersion)) {
		return AccountAdminCredential{}, fmt.Errorf("decode admin credential: project client credential is not an account admin credential")
	}
	if !strings.HasPrefix(raw, prefix) {
		return AccountAdminCredential{}, fmt.Errorf("decode admin credential: unsupported prefix")
	}
	body, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(raw, prefix))
	if err != nil {
		return AccountAdminCredential{}, fmt.Errorf("decode admin credential: %w", err)
	}
	var c AccountAdminCredential
	if err := json.Unmarshal(body, &c); err != nil {
		return AccountAdminCredential{}, fmt.Errorf("decode admin credential: %w", err)
	}
	if c.Kind != "" && c.Kind != credentialKindAdmin {
		return AccountAdminCredential{}, fmt.Errorf("decode admin credential: unexpected kind %q", c.Kind)
	}
	c.Kind = credentialKindAdmin
	return c, nil
}

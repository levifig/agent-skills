package crypto

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestBundledClientCredentialRoundTrip(t *testing.T) {
	master, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	projectID := "proj_client_cred_00000001"
	ring := NewProjectKeyRing(master, projectID)
	projectKey, err := ring.WriteKey()
	if err != nil {
		t.Fatalf("WriteKey() error = %v", err)
	}
	cred := BundledClientCredential{
		Endpoint:        "https://sync.example.test/v1",
		ConnectionToken: "project-token-scope",
		ProjectID:       projectID,
		KeyGeneration:   ring.WriteGeneration,
		ProjectKey:      projectKey[:],
	}
	encoded, err := EncodeBundledClientCredential(cred)
	if err != nil {
		t.Fatalf("EncodeBundledClientCredential() error = %v", err)
	}
	if !strings.HasPrefix(encoded, "loafclient1://") {
		t.Fatalf("client prefix = %q, want loafclient1://", encoded)
	}
	decoded, err := DecodeBundledClientCredential(encoded)
	if err != nil {
		t.Fatalf("DecodeBundledClientCredential() error = %v", err)
	}
	if decoded.Endpoint != cred.Endpoint || decoded.ConnectionToken != cred.ConnectionToken || decoded.ProjectID != cred.ProjectID {
		t.Fatalf("decoded credential = %#v, want %#v", decoded, cred)
	}
	if decoded.Kind != credentialKindClient {
		t.Fatalf("decoded kind = %q, want %q", decoded.Kind, credentialKindClient)
	}
}

func TestClientCredentialStructurallyExcludesAdminFields(t *testing.T) {
	master, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	ring := NewProjectKeyRing(master, "proj_struct_00000001")
	projectKey, err := ring.WriteKey()
	if err != nil {
		t.Fatalf("WriteKey() error = %v", err)
	}
	encoded, err := EncodeBundledClientCredential(BundledClientCredential{
		Endpoint:        "https://sync.example.test/v1",
		ConnectionToken: "project-token-scope",
		ProjectID:       ring.ProjectID,
		KeyGeneration:   0,
		ProjectKey:      projectKey[:],
	})
	if err != nil {
		t.Fatalf("EncodeBundledClientCredential() error = %v", err)
	}
	body, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "loafclient1://"))
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, forbidden := range []string{"master_key", "account_access_key", "account_access_secret"} {
		if _, ok := fields[forbidden]; ok {
			t.Fatalf("client credential unexpectedly contains admin field %q", forbidden)
		}
	}
	if _, ok := fields["project_key"]; !ok {
		t.Fatal("client credential missing project_key")
	}
}

func TestDecodeClientRejectsAdminWireAndAdminFields(t *testing.T) {
	master, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	adminEncoded, err := EncodeAccountAdminCredential(AccountAdminCredential{
		Endpoint:            "https://sync.example.test/v1",
		AccountAccessKey:    "ak_live",
		AccountAccessSecret: "sk_live",
		MasterKey:           master,
	})
	if err != nil {
		t.Fatalf("EncodeAccountAdminCredential() error = %v", err)
	}
	if _, err := DecodeBundledClientCredential(adminEncoded); err == nil {
		t.Fatal("DecodeBundledClientCredential(admin) error = nil, want refusal")
	}

	// Even if someone re-prefixes an admin JSON body as a client wire string,
	// structural field checks must refuse.
	adminJSON, err := json.Marshal(map[string]any{
		"kind":                  credentialKindClient,
		"endpoint":              "https://sync.example.test/v1",
		"connection_token":      "token",
		"project_id":            "proj_x",
		"key_generation":        0,
		"project_key":           make([]byte, projectKeySize),
		"account_access_secret": "sk_live",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	fake := "loafclient1://" + base64.RawURLEncoding.EncodeToString(adminJSON)
	if _, err := DecodeBundledClientCredential(fake); err == nil {
		t.Fatal("DecodeBundledClientCredential(admin fields) error = nil, want refusal")
	}
}

func TestAccountAdminCredentialRoundTripAndSeparation(t *testing.T) {
	master, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	admin := AccountAdminCredential{
		Endpoint:            "https://sync.example.test/v1",
		AccountAccessKey:    "ak_live",
		AccountAccessSecret: "sk_live",
		MasterKey:           master,
	}
	encoded, err := EncodeAccountAdminCredential(admin)
	if err != nil {
		t.Fatalf("EncodeAccountAdminCredential() error = %v", err)
	}
	if !strings.HasPrefix(encoded, "loafadmin1://") {
		t.Fatalf("admin prefix = %q, want loafadmin1://", encoded)
	}
	decoded, err := DecodeAccountAdminCredential(encoded)
	if err != nil {
		t.Fatalf("DecodeAccountAdminCredential() error = %v", err)
	}
	if decoded.MasterKey != master || decoded.AccountAccessSecret != admin.AccountAccessSecret {
		t.Fatalf("decoded admin = %#v", decoded)
	}
	clientEncoded, err := EncodeBundledClientCredential(BundledClientCredential{
		Endpoint:        admin.Endpoint,
		ConnectionToken: "project-token",
		ProjectID:       "proj_x",
		ProjectKey:      make([]byte, projectKeySize),
	})
	if err != nil {
		t.Fatalf("EncodeBundledClientCredential() error = %v", err)
	}
	if _, err := DecodeAccountAdminCredential(clientEncoded); err == nil {
		t.Fatal("DecodeAccountAdminCredential(client) error = nil, want refusal")
	}
}

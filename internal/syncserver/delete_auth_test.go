package syncserver_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/levifig/loaf/internal/syncserver"
)

func TestRelayDeleteIsScopedToOwningAccount(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	defer store.Close()
	srv := syncserver.NewServer(store)
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	projectID := "proj_owned_by_a"
	ownerAdmin, ownerClient := bootstrapAccount(t, store, projectID, "owner-token", "owner-secret")
	otherAdmin, _ := bootstrapAccount(t, store, "proj_owned_by_b", "other-token", "other-secret")

	factID := "018f5c2a-0000-7000-8000-0000000000aa"
	blob := []byte(`{"version":1,"fact_id":"` + factID + `","ciphertext":"opaque"}`)
	postPush(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts", ownerClient, pushRequest{
		Blobs: []pushBlob{{FactID: factID, Blob: base64.StdEncoding.EncodeToString(blob)}},
	})

	cross := deleteFact(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts/"+factID, otherAdmin)
	if cross != http.StatusForbidden {
		t.Fatalf("cross-account delete status = %d, want 403", cross)
	}

	bogus := deleteFact(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts/"+factID, "LoafAdmin ak_missing:nope")
	if bogus != http.StatusUnauthorized {
		t.Fatalf("invalid admin delete status = %d, want 401", bogus)
	}

	stillThere := getPull(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts?since=0", ownerClient)
	if len(stillThere.Facts) != 1 || stillThere.Facts[0].FactID != factID {
		t.Fatalf("pull after forbidden delete = %#v, want the fact still present", stillThere.Facts)
	}

	revokeBody, err := json.Marshal(map[string]string{"name": "owner-token"})
	if err != nil {
		t.Fatalf("marshal revoke: %v", err)
	}
	revokeReq, err := http.NewRequest(http.MethodDelete, httpSrv.URL+"/v1/admin/tokens", bytes.NewReader(revokeBody))
	if err != nil {
		t.Fatalf("revoke request: %v", err)
	}
	revokeReq.Header.Set("Authorization", ownerAdmin)
	revokeReq.Header.Set("Content-Type", "application/json")
	revokeResp, err := http.DefaultClient.Do(revokeReq)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_ = revokeResp.Body.Close()
	if revokeResp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want 204", revokeResp.StatusCode)
	}

	owned := deleteFact(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts/"+factID, ownerAdmin)
	if owned != http.StatusNoContent {
		t.Fatalf("owning admin delete after revoke status = %d, want 204", owned)
	}
	missing := deleteFact(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts/"+factID, ownerAdmin)
	if missing != http.StatusNotFound {
		t.Fatalf("repeat delete status = %d, want 404", missing)
	}
}

func bootstrapAccount(t *testing.T, store *syncserver.Store, projectID, tokenName, accountSecret string) (adminAuth, clientAuth string) {
	t.Helper()
	account, accessKeyID, err := store.CreateAccount(t.Context(), accountSecret)
	if err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	_ = account
	adminAuth = "LoafAdmin " + accessKeyID + ":" + accountSecret

	token, tokenSecret, err := store.MintConnectionToken(t.Context(), accessKeyID, accountSecret, tokenName, projectID)
	if err != nil {
		t.Fatalf("MintConnectionToken() error = %v", err)
	}
	clientAuth = "LoafToken " + token.TokenID + ":" + tokenSecret
	return adminAuth, clientAuth
}

func deleteFact(t *testing.T, url, auth string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	req.Header.Set("Authorization", auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

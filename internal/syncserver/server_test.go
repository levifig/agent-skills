package syncserver_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/syncserver"
)

func TestRelayHealthAndKeyMaterialAudit(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	defer store.Close()

	hasKeys, err := store.SchemaHasKeyMaterial(context.Background())
	if err != nil {
		t.Fatalf("SchemaHasKeyMaterial() error = %v", err)
	}
	if hasKeys {
		t.Fatal("SchemaHasKeyMaterial() = true, want false")
	}

	srv := syncserver.NewServer(store)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/health", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", resp.Code)
	}
}

func TestRelayPushPullDeleteAndIdempotency(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	defer store.Close()
	srv := syncserver.NewServer(store)
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	projectID := "proj_test_channel"
	adminAuth, clientAuth := bootstrapCredentials(t, store, projectID, "ci-runner")

	factA := "018f5c2a-0000-7000-8000-000000000001"
	factB := "018f5c2a-0000-7000-8000-000000000002"
	bodyA := []byte(`{"version":1,"fact_id":"` + factA + `","nonce":"abc","ciphertext":"opaque"}`)
	bodyB := []byte(`{"version":1,"fact_id":"` + factB + `","nonce":"def","ciphertext":"opaque2"}`)

	pushResp := postPush(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts", clientAuth, pushRequest{
		Blobs: []pushBlob{
			{FactID: factA, Blob: base64.StdEncoding.EncodeToString(bodyA)},
			{FactID: factB, Blob: base64.StdEncoding.EncodeToString(bodyB)},
		},
	})
	if len(pushResp.Accepted) != 2 || pushResp.Accepted[0].ArrivalSeq != 1 || pushResp.Accepted[1].ArrivalSeq != 2 {
		t.Fatalf("first push = %#v, want accepted seq 1 then 2", pushResp.Accepted)
	}

	dupResp := postPush(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts", clientAuth, pushRequest{
		Blobs: []pushBlob{{FactID: factA, Blob: base64.StdEncoding.EncodeToString(bodyA)}},
	})
	if len(dupResp.Duplicates) != 1 || dupResp.Duplicates[0] != factA {
		t.Fatalf("duplicate push = %#v, want factA duplicate", dupResp.Duplicates)
	}

	pullResp := getPull(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts?since=0", clientAuth)
	if pullResp.Cursor != 2 || len(pullResp.Facts) != 2 {
		t.Fatalf("pull all = cursor %d facts %d, want cursor 2 and 2 facts", pullResp.Cursor, len(pullResp.Facts))
	}
	if pullResp.Facts[0].FactID != factA || pullResp.Facts[1].FactID != factB {
		t.Fatalf("pull order = %#v, want factA then factB", pullResp.Facts)
	}

	partial := getPull(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts?since=1", clientAuth)
	if len(partial.Facts) != 1 || partial.Facts[0].FactID != factB {
		t.Fatalf("partial pull = %#v, want factB only", partial.Facts)
	}

	req, _ := http.NewRequest(http.MethodDelete, httpSrv.URL+"/v1/projects/"+projectID+"/facts/"+factA, nil)
	req.Header.Set("Authorization", adminAuth)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", delResp.StatusCode)
	}

	afterDelete := getPull(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts?since=0", clientAuth)
	if len(afterDelete.Facts) != 1 || afterDelete.Facts[0].FactID != factB {
		t.Fatalf("pull after delete = %#v, want factB only", afterDelete.Facts)
	}
}

func TestRelayAuthBoundaries(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	defer store.Close()
	srv := syncserver.NewServer(store)
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	projectID := "proj_scope_test"
	adminAuth, clientAuth := bootstrapCredentials(t, store, projectID, "scope-token")

	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/facts", nil))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status = %d, want 401", resp.Code)
	}

	wrongProject := getPullStatus(t, httpSrv.URL+"/v1/projects/other-project/facts?since=0", clientAuth)
	if wrongProject != http.StatusUnauthorized {
		t.Fatalf("wrong project status = %d, want 401", wrongProject)
	}

	revokeBody, _ := json.Marshal(map[string]string{"name": "scope-token"})
	revokeReq, _ := http.NewRequest(http.MethodDelete, httpSrv.URL+"/v1/admin/tokens", bytes.NewReader(revokeBody))
	revokeReq.Header.Set("Authorization", adminAuth)
	revokeReq.Header.Set("Content-Type", "application/json")
	revokeResp, err := http.DefaultClient.Do(revokeReq)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_ = revokeResp.Body.Close()
	if revokeResp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want 204", revokeResp.StatusCode)
	}

	afterRevoke := getPullStatus(t, httpSrv.URL+"/v1/projects/"+projectID+"/facts?since=0", clientAuth)
	if afterRevoke != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, want 401", afterRevoke)
	}
}

func openTestStore(t *testing.T) *syncserver.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sync.sqlite")
	store, err := syncserver.OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	return store
}

func bootstrapCredentials(t *testing.T, store *syncserver.Store, projectID, tokenName string) (adminAuth, clientAuth string) {
	t.Helper()
	ctx := context.Background()
	accountSecret := "admin-secret-for-tests"
	account, accessKeyID, err := store.CreateAccount(ctx, accountSecret)
	if err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	_ = account
	adminAuth = fmt.Sprintf("LoafAdmin %s:%s", accessKeyID, accountSecret)

	token, tokenSecret, err := store.MintConnectionToken(ctx, accessKeyID, accountSecret, tokenName, projectID)
	if err != nil {
		t.Fatalf("MintConnectionToken() error = %v", err)
	}
	clientAuth = fmt.Sprintf("LoafToken %s:%s", token.TokenID, tokenSecret)
	return adminAuth, clientAuth
}

type pushRequest struct {
	Blobs []pushBlob `json:"blobs"`
}

type pushBlob struct {
	FactID string `json:"fact_id"`
	Blob   string `json:"blob"`
}

type pushResponse struct {
	Accepted   []struct {
		FactID     string `json:"fact_id"`
		ArrivalSeq int64  `json:"arrival_seq"`
	} `json:"accepted"`
	Duplicates []string `json:"duplicates"`
}

type pullResponse struct {
	Cursor int64 `json:"cursor"`
	Facts  []struct {
		FactID     string `json:"fact_id"`
		ArrivalSeq int64  `json:"arrival_seq"`
		Blob       string `json:"blob"`
	} `json:"facts"`
}

func postPush(t *testing.T, url, auth string, req pushRequest) pushResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal push: %v", err)
	}
	httpReq, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	httpReq.Header.Set("Authorization", auth)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("post push: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("push status = %d body = %s", resp.StatusCode, string(b))
	}
	var out pushResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode push: %v", err)
	}
	return out
}

func getPull(t *testing.T, url, auth string) pullResponse {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("pull status = %d body = %s", resp.StatusCode, string(b))
	}
	var out pullResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode pull: %v", err)
	}
	return out
}

func getPullStatus(t *testing.T, url, auth string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("pull status request: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

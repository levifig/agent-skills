package syncserver_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/levifig/loaf/internal/syncserver"
)

func TestScratchpadPruneIsScopedToOwningAccount(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	defer store.Close()
	srv := syncserver.NewServer(store)
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	projectID := "proj_scratchpad_owned_by_a"
	channel := "effort-prune"
	ownerAdmin, ownerClient := bootstrapAccount(t, store, projectID, "owner-scratchpad", "owner-scratchpad-secret")
	otherAdmin, _ := bootstrapAccount(t, store, "proj_scratchpad_owned_by_b", "other-scratchpad", "other-scratchpad-secret")

	publishScratchpad(t, httpSrv.URL+"/v1/projects/"+projectID+"/scratchpad/"+channel, ownerClient, []byte(`{"text":"keep-me"}`))

	cross := pruneScratchpad(t, httpSrv.URL+"/v1/projects/"+projectID+"/scratchpad/"+channel, otherAdmin)
	if cross != http.StatusForbidden {
		t.Fatalf("cross-account prune status = %d, want 403", cross)
	}

	bogus := pruneScratchpad(t, httpSrv.URL+"/v1/projects/"+projectID+"/scratchpad/"+channel, "LoafAdmin ak_missing:nope")
	if bogus != http.StatusUnauthorized {
		t.Fatalf("invalid admin prune status = %d, want 401", bogus)
	}

	clientPrune := pruneScratchpad(t, httpSrv.URL+"/v1/projects/"+projectID+"/scratchpad/"+channel, ownerClient)
	if clientPrune != http.StatusUnauthorized {
		t.Fatalf("client-token prune status = %d, want 401", clientPrune)
	}

	stillThere := pollScratchpad(t, httpSrv.URL+"/v1/projects/"+projectID+"/scratchpad/"+channel+"/poll?since=0&timeout_ms=10", ownerClient)
	if len(stillThere) != 1 {
		t.Fatalf("poll after forbidden prune = %d messages, want 1 still present", len(stillThere))
	}

	revokeBody, err := json.Marshal(map[string]string{"name": "owner-scratchpad"})
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

	owned := pruneScratchpad(t, httpSrv.URL+"/v1/projects/"+projectID+"/scratchpad/"+channel, ownerAdmin)
	if owned != http.StatusOK {
		t.Fatalf("owning admin prune after revoke status = %d, want 200", owned)
	}

	gone, err := store.ListScratchpadSince(t.Context(), projectID, channel, 0)
	if err != nil {
		t.Fatalf("list after owning prune: %v", err)
	}
	if len(gone) != 0 {
		t.Fatalf("store list after owning prune = %d messages, want 0", len(gone))
	}
}

func publishScratchpad(t *testing.T, url, auth string, payload []byte) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"payload": base64.StdEncoding.EncodeToString(payload)})
	if err != nil {
		t.Fatalf("marshal scratchpad publish: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("publish request: %v", err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("publish status = %d body = %s", resp.StatusCode, raw)
	}
}

func pruneScratchpad(t *testing.T, url, auth string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("prune request: %v", err)
	}
	req.Header.Set("Authorization", auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func pollScratchpad(t *testing.T, url, auth string) []struct {
	Seq     int64  `json:"seq"`
	Payload string `json:"payload"`
} {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("poll request: %v", err)
	}
	req.Header.Set("Authorization", auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("poll status = %d body = %s", resp.StatusCode, raw)
	}
	var decoded struct {
		Messages []struct {
			Seq     int64  `json:"seq"`
			Payload string `json:"payload"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode poll: %v", err)
	}
	return decoded.Messages
}

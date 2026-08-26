package sync

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type relayClient struct {
	baseURL string
	auth    string
	http    *http.Client
}

type pushBlobItem struct {
	FactID string `json:"fact_id"`
	Blob   string `json:"blob"`
}

type pushRequest struct {
	Blobs []pushBlobItem `json:"blobs"`
}

type pushAcceptedItem struct {
	FactID     string `json:"fact_id"`
	ArrivalSeq int64  `json:"arrival_seq"`
}

type pushResponse struct {
	Accepted   []pushAcceptedItem `json:"accepted"`
	Duplicates []string           `json:"duplicates"`
}

type pullFactItem struct {
	FactID     string `json:"fact_id"`
	ArrivalSeq int64  `json:"arrival_seq"`
	Blob       string `json:"blob"`
}

type pullResponse struct {
	Cursor int64          `json:"cursor"`
	Facts  []pullFactItem `json:"facts"`
}

func newRelayClient(endpoint, connectionToken string, httpClient *http.Client) (*relayClient, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, fmt.Errorf("sync client: endpoint is empty")
	}
	token := strings.TrimSpace(connectionToken)
	if token == "" {
		return nil, fmt.Errorf("sync client: connection token is empty")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &relayClient{
		baseURL: endpoint,
		auth:    "LoafToken " + token,
		http:    httpClient,
	}, nil
}

func (c *relayClient) push(ctx context.Context, projectID string, blobs []pushBlobItem) (pushResponse, error) {
	body, err := json.Marshal(pushRequest{Blobs: blobs})
	if err != nil {
		return pushResponse{}, err
	}
	url := c.baseURL + "/v1/projects/" + projectID + "/facts"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return pushResponse{}, err
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return pushResponse{}, fmt.Errorf("push facts: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return pushResponse{}, fmt.Errorf("push facts: unauthorized (check project scope and token)")
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return pushResponse{}, fmt.Errorf("push facts: status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var out pushResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return pushResponse{}, fmt.Errorf("decode push response: %w", err)
	}
	return out, nil
}

func (c *relayClient) pull(ctx context.Context, projectID string, since int64) (pullResponse, error) {
	url := fmt.Sprintf("%s/v1/projects/%s/facts?since=%d", c.baseURL, projectID, since)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return pullResponse{}, err
	}
	req.Header.Set("Authorization", c.auth)
	resp, err := c.http.Do(req)
	if err != nil {
		return pullResponse{}, fmt.Errorf("pull facts: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return pullResponse{}, fmt.Errorf("pull facts: unauthorized (check project scope and token)")
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return pullResponse{}, fmt.Errorf("pull facts: status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var out pullResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return pullResponse{}, fmt.Errorf("decode pull response: %w", err)
	}
	return out, nil
}

func encodeBlob(raw []byte) string {
	return base64.StdEncoding.EncodeToString(raw)
}

func decodeBlob(raw string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode blob: %w", err)
	}
	return decoded, nil
}

package sync

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/state"
)

// ProbeConfig validates remote reachability, auth, decrypt, capability, and HLC skew.
type ProbeConfig struct {
	Credential   crypto.BundledClientCredential
	HTTPClient   *http.Client
	MaxHLCSkewMS int64
	// Receive validates one opened fact envelope (capability + HLC skew). When the
	// remote stream is empty, Receive is not called.
	Receive func(ctx context.Context, envelope state.FactEnvelope) error
}

// ProbeResult summarizes a successful remote attach probe.
type ProbeResult struct {
	Pulled    int
	Decrypted int
}

// ProbeRemote pulls the project fact stream from cursor zero, decrypts each blob,
// and invokes Receive for every opened envelope.
func ProbeRemote(ctx context.Context, cfg ProbeConfig) (ProbeResult, error) {
	cred := cfg.Credential
	projectID := strings.TrimSpace(cred.ProjectID)
	if projectID == "" {
		return ProbeResult{}, fmt.Errorf("attach probe: credential project id is empty")
	}
	var projectKey [32]byte
	if len(cred.ProjectKey) != len(projectKey) {
		return ProbeResult{}, fmt.Errorf("attach probe: credential project key must be %d bytes", len(projectKey))
	}
	copy(projectKey[:], cred.ProjectKey)
	readKeys := make([][32]byte, 0, cred.KeyGeneration+1)
	for gen := 0; gen <= cred.KeyGeneration; gen++ {
		readKeys = append(readKeys, projectKey)
	}
	relay, err := newRelayClient(cred.Endpoint, cred.ConnectionToken, cfg.HTTPClient)
	if err != nil {
		return ProbeResult{}, err
	}

	result := ProbeResult{}
	cursor := int64(0)
	for {
		resp, err := relay.pull(ctx, projectID, cursor)
		if err != nil {
			return result, err
		}
		if len(resp.Facts) == 0 {
			break
		}
		for _, item := range resp.Facts {
			result.Pulled++
			raw, err := decodeBlob(item.Blob)
			if err != nil {
				return result, fmt.Errorf("attach probe: decode blob: %w", err)
			}
			envelope, err := openFactBlob(readKeys, projectID, raw)
			if err != nil {
				return result, fmt.Errorf("attach probe: decrypt fact %q: %w", item.FactID, err)
			}
			result.Decrypted++
			if cfg.Receive != nil {
				if err := cfg.Receive(ctx, envelope); err != nil {
					return result, err
				}
			}
		}
		if resp.Cursor <= cursor {
			break
		}
		cursor = resp.Cursor
	}
	return result, nil
}

// CheckRelayHealth verifies the sync server responds over HTTP(S).
func CheckRelayHealth(ctx context.Context, endpoint string, httpClient *http.Client) error {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return fmt.Errorf("attach probe: endpoint is empty")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/health", nil)
	if err != nil {
		return fmt.Errorf("attach probe: build health request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("attach probe: reach server: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("attach probe: health check status %d", resp.StatusCode)
	}
	return nil
}

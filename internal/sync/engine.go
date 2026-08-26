package sync

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/state"
)

// Config configures the client sync engine.
type Config struct {
	Store        *state.Store
	Credential   crypto.BundledClientCredential
	HTTPClient   *http.Client
	MaxHLCSkewMS int64
}

// Engine queues local facts, pushes sealed blobs, pulls remote facts, and refreshes projections.
type Engine struct {
	store        *state.Store
	credential   crypto.BundledClientCredential
	projectKey   [32]byte
	readKeys     [][32]byte
	relay        *relayClient
	maxHLCSkewMS int64
}

// Result summarizes one sync pass.
type Result struct {
	Queued     int         `json:"queued"`
	Pushed     int         `json:"pushed"`
	Duplicates int         `json:"duplicates"`
	Pulled     int         `json:"pulled"`
	Cursor     int64       `json:"cursor"`
	Gaps       []EnvSeqGap `json:"gaps,omitempty"`
	Warnings   []string    `json:"warnings,omitempty"`
}

// NewEngine constructs a sync engine from a bundled client credential.
func NewEngine(cfg Config) (*Engine, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("sync engine: store is required")
	}
	cred := cfg.Credential
	if strings.TrimSpace(cred.ProjectID) == "" {
		return nil, fmt.Errorf("sync engine: credential project id is empty")
	}
	var projectKey [32]byte
	if len(cred.ProjectKey) != len(projectKey) {
		return nil, fmt.Errorf("sync engine: credential project key must be %d bytes", len(projectKey))
	}
	copy(projectKey[:], cred.ProjectKey)
	readKeys := make([][32]byte, 0, cred.KeyGeneration+1)
	for gen := 0; gen <= cred.KeyGeneration; gen++ {
		readKeys = append(readKeys, projectKey)
	}
	relay, err := newRelayClient(cred.Endpoint, cred.ConnectionToken, cfg.HTTPClient)
	if err != nil {
		return nil, err
	}
	return &Engine{
		store:        cfg.Store,
		credential:   cred,
		projectKey:   projectKey,
		readKeys:     readKeys,
		relay:        relay,
		maxHLCSkewMS: cfg.MaxHLCSkewMS,
	}, nil
}

// Sync pushes pending local facts, pulls remote facts since cursor, refreshes projections, and reports gaps.
func (e *Engine) Sync(ctx context.Context) (Result, error) {
	projectID := strings.TrimSpace(e.credential.ProjectID)
	result := Result{}
	pending, err := countPendingOutbound(ctx, e.store, projectID)
	if err != nil {
		return result, err
	}
	result.Queued = pending

	pushed, duplicates, err := e.pushPending(ctx, projectID)
	if err != nil {
		return result, err
	}
	result.Pushed = pushed
	result.Duplicates = duplicates

	pulled, cursor, err := e.pullSinceCursor(ctx, projectID)
	if err != nil {
		return result, err
	}
	result.Pulled = pulled
	result.Cursor = cursor

	if err := refreshProjections(ctx, e.store, projectID); err != nil {
		return result, err
	}

	gaps, err := DetectEnvSeqGaps(ctx, e.store, projectID)
	if err != nil {
		return result, err
	}
	result.Gaps = gaps
	result.Warnings = formatGapWarnings(gaps)
	return result, nil
}

func (e *Engine) pushPending(ctx context.Context, projectID string) (pushed, duplicates int, err error) {
	pendingIDs, err := listPendingOutboundFacts(ctx, e.store, projectID)
	if err != nil {
		return 0, 0, err
	}
	if len(pendingIDs) == 0 {
		return 0, 0, nil
	}
	blobs := make([]pushBlobItem, 0, len(pendingIDs))
	pushedIDs := make([]string, 0, len(pendingIDs))
	for _, factID := range pendingIDs {
		fact, ok, err := state.GetFactByID(ctx, e.store, factID)
		if err != nil {
			return 0, 0, err
		}
		if !ok {
			continue
		}
		sealed, err := sealFact(e.projectKey, e.credential.KeyGeneration, fact)
		if err != nil {
			return 0, 0, err
		}
		blobs = append(blobs, pushBlobItem{FactID: factID, Blob: encodeBlob(sealed)})
		pushedIDs = append(pushedIDs, factID)
	}
	if len(blobs) == 0 {
		return 0, 0, nil
	}
	resp, err := e.relay.push(ctx, projectID, blobs)
	if err != nil {
		return 0, 0, err
	}
	pushed = len(resp.Accepted)
	duplicates = len(resp.Duplicates)
	accepted := map[string]struct{}{}
	for _, item := range resp.Accepted {
		accepted[item.FactID] = struct{}{}
	}
	for _, dup := range resp.Duplicates {
		accepted[dup] = struct{}{}
	}
	toMark := make([]string, 0, len(pushedIDs))
	for _, factID := range pushedIDs {
		if _, ok := accepted[factID]; ok {
			toMark = append(toMark, factID)
		}
	}
	if err := markOutboundFactsPushed(ctx, e.store, projectID, toMark); err != nil {
		return pushed, duplicates, err
	}
	return pushed, duplicates, nil
}

func (e *Engine) pullSinceCursor(ctx context.Context, projectID string) (pulled int, cursor int64, err error) {
	cursor, err = readArrivalCursor(ctx, e.store, projectID)
	if err != nil {
		return 0, 0, err
	}
	for {
		resp, err := e.relay.pull(ctx, projectID, cursor)
		if err != nil {
			return pulled, cursor, err
		}
		if len(resp.Facts) == 0 {
			if resp.Cursor > cursor {
				cursor = resp.Cursor
			}
			break
		}
		for _, item := range resp.Facts {
			raw, err := decodeBlob(item.Blob)
			if err != nil {
				return pulled, cursor, err
			}
			envelope, err := openFactBlob(e.readKeys, projectID, raw)
			if err != nil {
				return pulled, cursor, err
			}
			inserted, err := state.ReceiveFact(ctx, e.store, envelope, state.ReceiveFactOptions{MaxHLCSkewMS: e.maxHLCSkewMS})
			if err != nil {
				return pulled, cursor, err
			}
			if inserted {
				pulled++
			}
		}
		if resp.Cursor <= cursor {
			break
		}
		cursor = resp.Cursor
	}
	if err := writeArrivalCursor(ctx, e.store, projectID, cursor); err != nil {
		return pulled, cursor, err
	}
	return pulled, cursor, nil
}

func refreshProjections(ctx context.Context, store *state.Store, projectID string) error {
	if _, err := state.RebuildJournalProjectionForProject(ctx, store, projectID); err != nil {
		return err
	}
	// Rebuilds spark/idea/handoff/release plus refs, worktrees, and verification.
	_, err := state.RebuildMutableCoreProjectionsForProject(ctx, store, projectID)
	return err
}

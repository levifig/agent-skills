package syncserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	maxBatchSize = 256
	maxBlobBytes = 8 << 20
)

type Config struct {
	Store *Store
}

type Server struct {
	store *Store
}

type pushRequest struct {
	Blobs []pushBlobItem `json:"blobs"`
}

type pushBlobItem struct {
	FactID string `json:"fact_id"`
	Blob   string `json:"blob"`
}

type pushHTTPResponse struct {
	Accepted   []pushAcceptedItem `json:"accepted"`
	Duplicates []string           `json:"duplicates,omitempty"`
}

type pushAcceptedItem struct {
	FactID     string `json:"fact_id"`
	ArrivalSeq int64  `json:"arrival_seq"`
}

type pullHTTPResponse struct {
	Cursor int64          `json:"cursor"`
	Facts  []pullFactItem `json:"facts"`
}

type pullFactItem struct {
	FactID     string `json:"fact_id"`
	ArrivalSeq int64  `json:"arrival_seq"`
	Blob       string `json:"blob"`
}

type revokeTokenRequest struct {
	Name string `json:"name"`
}

type healthResponse struct {
	Status string `json:"status"`
}

func NewServer(store *Store) *Server {
	return &Server{store: store}
}

func NewServerFromConfig(cfg Config) (*Server, error) {
	if cfg.Store == nil {
		return nil, errors.New("sync server store is required")
	}
	return &Server{store: cfg.Store}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /v1/projects/{project_id}/facts", s.handlePush)
	mux.HandleFunc("GET /v1/projects/{project_id}/facts", s.handlePull)
	mux.HandleFunc("DELETE /v1/projects/{project_id}/facts/{fact_id}", s.handleDelete)
	mux.HandleFunc("POST /v1/projects/{project_id}/scratchpad/{channel}", s.handleScratchpadPublish)
	mux.HandleFunc("GET /v1/projects/{project_id}/scratchpad/{channel}/poll", s.handleScratchpadPoll)
	mux.HandleFunc("GET /v1/projects/{project_id}/scratchpad/{channel}/stream", s.handleScratchpadStream)
	mux.HandleFunc("DELETE /v1/projects/{project_id}/scratchpad/{channel}", s.handleScratchpadPrune)
	mux.HandleFunc("DELETE /v1/admin/tokens", s.handleRevokeToken)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	if err := s.authorizeClient(r.Context(), r, projectID); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBlobBytes*maxBatchSize))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("read push body: %w", err))
		return
	}
	var req pushRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode push body: %w", err))
		return
	}
	if len(req.Blobs) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("push requires at least one blob"))
		return
	}
	if len(req.Blobs) > maxBatchSize {
		writeError(w, http.StatusBadRequest, fmt.Errorf("push batch exceeds %d blobs", maxBatchSize))
		return
	}
	inputs := make([]PushInput, 0, len(req.Blobs))
	for _, blob := range req.Blobs {
		raw, err := base64.StdEncoding.DecodeString(blob.Blob)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("decode blob for fact %q: %w", blob.FactID, err))
			return
		}
		if len(raw) > maxBlobBytes {
			writeError(w, http.StatusBadRequest, fmt.Errorf("blob for fact %q exceeds %d bytes", blob.FactID, maxBlobBytes))
			return
		}
		inputs = append(inputs, PushInput{FactID: blob.FactID, Blob: raw})
	}
	results, err := s.store.PushFacts(r.Context(), projectID, inputs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp := pushHTTPResponse{Accepted: make([]pushAcceptedItem, 0), Duplicates: make([]string, 0)}
	for _, result := range results {
		if result.Duplicate {
			resp.Duplicates = append(resp.Duplicates, result.FactID)
			continue
		}
		resp.Accepted = append(resp.Accepted, pushAcceptedItem{
			FactID:     result.FactID,
			ArrivalSeq: result.ArrivalSeq,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	if err := s.authorizeClient(r.Context(), r, projectID); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	cursor, err := parseCursor(r.URL.Query().Get("since"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	facts, nextCursor, err := s.store.PullFactsSince(r.Context(), projectID, cursor)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp := pullHTTPResponse{Cursor: nextCursor, Facts: make([]pullFactItem, 0, len(facts))}
	for _, fact := range facts {
		resp.Facts = append(resp.Facts, pullFactItem{
			FactID:     fact.FactID,
			ArrivalSeq: fact.ArrivalSeq,
			Blob:       base64.StdEncoding.EncodeToString(fact.Blob),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	factID := strings.TrimSpace(r.PathValue("fact_id"))
	if err := s.authorizeAdmin(r.Context(), r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	deleted, err := s.store.DeleteFact(r.Context(), projectID, factID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, errors.New("fact not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	accessKeyID, accessSecret, err := parseAdminAuthorization(r.Header.Get("Authorization"))
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req revokeTokenRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode revoke token body: %w", err))
		return
	}
	revoked, err := s.store.RevokeConnectionToken(r.Context(), accessKeyID, accessSecret, req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !revoked {
		writeError(w, http.StatusNotFound, errors.New("connection token not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authorizeClient(ctx context.Context, r *http.Request, projectID string) error {
	tokenID, tokenSecret, err := parseConnectionAuthorization(r.Header.Get("Authorization"))
	if err != nil {
		return err
	}
	return s.store.AuthenticateConnectionToken(ctx, tokenID, tokenSecret, projectID)
}

func (s *Server) authorizeAdmin(ctx context.Context, r *http.Request) error {
	accessKeyID, accessSecret, err := parseAdminAuthorization(r.Header.Get("Authorization"))
	if err != nil {
		return err
	}
	return s.store.AuthenticateAdmin(ctx, accessKeyID, accessSecret)
}

func parseConnectionAuthorization(authHeader string) (string, string, error) {
	authHeader = strings.TrimSpace(authHeader)
	const prefix = "LoafToken "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", "", errors.New("missing connection token authorization")
	}
	return parseCredentialPair(strings.TrimPrefix(authHeader, prefix))
}

func parseAdminAuthorization(authHeader string) (string, string, error) {
	authHeader = strings.TrimSpace(authHeader)
	const prefix = "LoafAdmin "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", "", errors.New("missing admin authorization")
	}
	return parseCredentialPair(strings.TrimPrefix(authHeader, prefix))
}

func parseCredentialPair(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", errors.New("invalid credential format")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func parseCursor(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	var cursor int64
	_, err := fmt.Sscan(raw, &cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor %q: %w", raw, err)
	}
	if cursor < 0 {
		return 0, errors.New("cursor must be non-negative")
	}
	return cursor, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

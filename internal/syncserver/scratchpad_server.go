package syncserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type scratchpadPublishRequest struct {
	Payload string `json:"payload"`
}

type scratchpadPollResponse struct {
	Messages []scratchpadMessageItem `json:"messages"`
}

type scratchpadMessageItem struct {
	Seq       int64  `json:"seq"`
	Payload   string `json:"payload"`
	CreatedAt string `json:"created_at"`
}

func (s *Server) handleScratchpadPublish(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	channel := strings.TrimSpace(r.PathValue("channel"))
	if err := s.authorizeClient(r.Context(), r, projectID); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxScratchpadPayloadBytes+4096))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req scratchpadPublishRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode scratchpad publish body: %w", err))
		return
	}
	raw, err := base64.StdEncoding.DecodeString(req.Payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode scratchpad payload: %w", err))
		return
	}
	msg, err := s.store.PublishScratchpadMessage(r.Context(), projectID, channel, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, scratchpadMessageItem{
		Seq:       msg.Seq,
		Payload:   base64.StdEncoding.EncodeToString(msg.Payload),
		CreatedAt: msg.CreatedAt,
	})
}

func (s *Server) handleScratchpadPoll(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	channel := strings.TrimSpace(r.PathValue("channel"))
	if err := s.authorizeClient(r.Context(), r, projectID); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	since, err := parseScratchpadSince(r.URL.Query().Get("since"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	timeoutMS, err := parseScratchpadTimeout(r.URL.Query().Get("timeout_ms"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	if timeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
		defer cancel()
	}
	messages, err := s.store.WaitScratchpadSince(ctx, projectID, channel, since)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, scratchpadPollResponse{Messages: encodeScratchpadItems(messages)})
}

func (s *Server) handleScratchpadStream(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	channel := strings.TrimSpace(r.PathValue("channel"))
	if err := s.authorizeClient(r.Context(), r, projectID); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	since, err := parseScratchpadSince(r.URL.Query().Get("since"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	for {
		messages, err := s.store.WaitScratchpadSince(ctx, projectID, channel, since)
		if err != nil {
			return
		}
		for _, msg := range messages {
			item := encodeScratchpadItems([]ScratchpadMessage{msg})[0]
			raw, _ := json.Marshal(item)
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", raw)
			flusher.Flush()
			since = msg.Seq
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func encodeScratchpadItems(messages []ScratchpadMessage) []scratchpadMessageItem {
	items := make([]scratchpadMessageItem, 0, len(messages))
	for _, msg := range messages {
		items = append(items, scratchpadMessageItem{
			Seq:       msg.Seq,
			Payload:   base64.StdEncoding.EncodeToString(msg.Payload),
			CreatedAt: msg.CreatedAt,
		})
	}
	return items
}

func parseScratchpadSince(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	since, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || since < 0 {
		return 0, fmt.Errorf("invalid scratchpad since cursor")
	}
	return since, nil
}

func parseScratchpadTimeout(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 25_000, nil
	}
	timeout, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || timeout < 0 {
		return 0, fmt.Errorf("invalid scratchpad timeout_ms")
	}
	if timeout > 60_000 {
		timeout = 60_000
	}
	return timeout, nil
}

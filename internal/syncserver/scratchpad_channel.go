package syncserver

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const maxScratchpadPayloadBytes = 64 << 10

// ScratchpadMessage is an opaque scratchpad payload stored on the relay.
type ScratchpadMessage struct {
	ProjectID string `json:"project_id"`
	Channel   string `json:"channel"`
	Seq       int64  `json:"seq"`
	Payload   []byte `json:"payload"`
	CreatedAt string `json:"created_at"`
}

// PublishScratchpadMessage appends one opaque message to a channel.
func (s *Store) PublishScratchpadMessage(ctx context.Context, projectID, channel string, payload []byte) (ScratchpadMessage, error) {
	projectID = strings.TrimSpace(projectID)
	channel = normalizeScratchpadChannel(channel)
	if projectID == "" || channel == "" {
		return ScratchpadMessage{}, fmt.Errorf("scratchpad publish requires project_id and channel")
	}
	if len(payload) == 0 {
		return ScratchpadMessage{}, fmt.Errorf("scratchpad payload cannot be empty")
	}
	if len(payload) > maxScratchpadPayloadBytes {
		return ScratchpadMessage{}, fmt.Errorf("scratchpad payload exceeds %d bytes", maxScratchpadPayloadBytes)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ScratchpadMessage{}, err
	}
	defer tx.Rollback()
	var seq int64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(seq), 0) + 1 FROM scratchpad_messages WHERE project_id = ? AND channel = ?
`, projectID, channel).Scan(&seq); err != nil {
		return ScratchpadMessage{}, fmt.Errorf("allocate scratchpad seq: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO scratchpad_messages (project_id, channel, seq, payload, created_at)
VALUES (?, ?, ?, ?, ?)
`, projectID, channel, seq, payload, now); err != nil {
		return ScratchpadMessage{}, fmt.Errorf("insert scratchpad message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ScratchpadMessage{}, err
	}
	return ScratchpadMessage{ProjectID: projectID, Channel: channel, Seq: seq, Payload: append([]byte(nil), payload...), CreatedAt: now}, nil
}

// ListScratchpadSince returns messages with seq greater than since.
func (s *Store) ListScratchpadSince(ctx context.Context, projectID, channel string, since int64) ([]ScratchpadMessage, error) {
	projectID = strings.TrimSpace(projectID)
	channel = normalizeScratchpadChannel(channel)
	if projectID == "" || channel == "" {
		return nil, fmt.Errorf("scratchpad list requires project_id and channel")
	}
	if since < 0 {
		return nil, fmt.Errorf("scratchpad since must be >= 0")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT seq, payload, created_at
FROM scratchpad_messages
WHERE project_id = ? AND channel = ? AND seq > ?
ORDER BY seq ASC
`, projectID, channel, since)
	if err != nil {
		return nil, fmt.Errorf("list scratchpad messages: %w", err)
	}
	defer rows.Close()
	var messages []ScratchpadMessage
	for rows.Next() {
		var msg ScratchpadMessage
		msg.ProjectID = projectID
		msg.Channel = channel
		if err := rows.Scan(&msg.Seq, &msg.Payload, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan scratchpad message: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

// WaitScratchpadSince blocks until a message arrives with seq > since or the deadline elapses.
func (s *Store) WaitScratchpadSince(ctx context.Context, projectID, channel string, since int64) ([]ScratchpadMessage, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(25 * time.Second)
	}
	for {
		messages, err := s.ListScratchpadSince(ctx, projectID, channel, since)
		if err != nil {
			return nil, err
		}
		if len(messages) > 0 {
			return messages, nil
		}
		if time.Now().After(deadline) {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func normalizeScratchpadChannel(channel string) string {
	return strings.TrimSpace(channel)
}

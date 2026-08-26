package scratchpad

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type IntroPayload struct {
	Channel    string `json:"channel"`
	InstanceID string `json:"instance_id"`
	EnvID      string `json:"env_id"`
	Who        string `json:"who"`
	WorkingRef string `json:"working_ref,omitempty"`
}

type MessagePayload struct {
	Channel    string `json:"channel"`
	InstanceID string `json:"instance_id"`
	EnvID      string `json:"env_id"`
	Text       string `json:"text"`
}

type ClaimPayload struct {
	Channel    string `json:"channel"`
	InstanceID string `json:"instance_id"`
	EnvID      string `json:"env_id"`
	Resource   string `json:"resource"`
	ExpiresAt  string `json:"expires_at"`
}

type ClosePayload struct {
	Channel    string `json:"channel"`
	InstanceID string `json:"instance_id"`
	EnvID      string `json:"env_id"`
}

type ReleasePayload struct {
	Channel    string `json:"channel"`
	InstanceID string `json:"instance_id"`
	EnvID      string `json:"env_id"`
	Resource   string `json:"resource"`
}

type Entry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	HLC       string    `json:"hlc"`
	CreatedAt time.Time `json:"created_at"`
	Payload   any       `json:"payload"`
}

type RosterMember struct {
	InstanceID string `json:"instance_id"`
	EnvID      string `json:"env_id"`
	Who        string `json:"who"`
	WorkingRef string `json:"working_ref,omitempty"`
	LastSeen   string `json:"last_seen"`
}

type ActiveClaim struct {
	Resource   string `json:"resource"`
	InstanceID string `json:"instance_id"`
	EnvID      string `json:"env_id"`
	ExpiresAt  string `json:"expires_at"`
	ClaimedAt  string `json:"claimed_at"`
}

func encodePayload(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode scratchpad payload: %w", err)
	}
	return string(raw), nil
}

func decodeIntro(raw string) (IntroPayload, error) {
	var payload IntroPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return IntroPayload{}, fmt.Errorf("decode scratchpad intro: %w", err)
	}
	return payload, nil
}

func decodeMessage(raw string) (MessagePayload, error) {
	var payload MessagePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return MessagePayload{}, fmt.Errorf("decode scratchpad message: %w", err)
	}
	return payload, nil
}

func decodeClaim(raw string) (ClaimPayload, error) {
	var payload ClaimPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ClaimPayload{}, fmt.Errorf("decode scratchpad claim: %w", err)
	}
	return payload, nil
}

func decodeRelease(raw string) (ReleasePayload, error) {
	var payload ReleasePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ReleasePayload{}, fmt.Errorf("decode scratchpad release: %w", err)
	}
	return payload, nil
}

func decodeClose(raw string) (ClosePayload, error) {
	var payload ClosePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ClosePayload{}, fmt.Errorf("decode scratchpad close: %w", err)
	}
	return payload, nil
}

func normalizeChannel(channel string) (string, error) {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return "", fmt.Errorf("scratchpad channel is required")
	}
	return channel, nil
}

func normalizeInstanceID(instanceID string) string {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID != "" {
		return instanceID
	}
	return fmt.Sprintf("instance-%d", time.Now().UTC().UnixNano())
}

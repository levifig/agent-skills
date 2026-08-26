package sync

import (
	"encoding/json"
	"fmt"

	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/state"
)

func sealFact(projectKey [32]byte, keyGen int, fact state.FactEnvelope) ([]byte, error) {
	plaintext := crypto.FactPlaintext{
		Kind:      fact.Kind,
		Payload:   fact.Payload,
		EnvID:     fact.EnvID,
		Seq:       fact.Seq,
		HLC:       fact.HLC,
		EnvelopeV: fact.EnvelopeV,
		KeyGen:    keyGen,
	}
	envelope, err := crypto.EncryptFactEnvelope(projectKey, fact.ID, plaintext)
	if err != nil {
		return nil, fmt.Errorf("seal fact %q: %w", fact.ID, err)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal wire envelope: %w", err)
	}
	return raw, nil
}

func openFactBlob(readKeys [][32]byte, projectID string, blob []byte) (state.FactEnvelope, error) {
	var wire crypto.WireEnvelope
	if err := json.Unmarshal(blob, &wire); err != nil {
		return state.FactEnvelope{}, fmt.Errorf("decode wire envelope: %w", err)
	}
	plaintext, err := crypto.DecryptFactEnvelope(readKeys, wire)
	if err != nil {
		return state.FactEnvelope{}, fmt.Errorf("open fact blob: %w", err)
	}
	permanence, ok := state.FactPermanenceClass(plaintext.Kind)
	if !ok {
		return state.FactEnvelope{}, fmt.Errorf("open fact blob: unknown kind %q", plaintext.Kind)
	}
	return state.FactEnvelope{
		ID:         wire.FactID,
		ProjectID:  projectID,
		Kind:       plaintext.Kind,
		Payload:    plaintext.Payload,
		EnvID:      plaintext.EnvID,
		Seq:        plaintext.Seq,
		HLC:        plaintext.HLC,
		EnvelopeV:  plaintext.EnvelopeV,
		Permanence: permanence,
	}, nil
}

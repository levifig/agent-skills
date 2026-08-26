package crypto

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	masterKeySize   = 32
	projectKeySize  = 32
	projectKeyLabel = "loaf-substrate-v1/project-key"
)

// MasterKey is a full-entropy operator secret used only on trusted machines.
type MasterKey [masterKeySize]byte

// GenerateMasterKey mints a fresh master key.
func GenerateMasterKey() (MasterKey, error) {
	var key MasterKey
	if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
		return MasterKey{}, fmt.Errorf("generate master key: %w", err)
	}
	return key, nil
}

// MasterKeyFromBytes copies raw key material.
func MasterKeyFromBytes(raw []byte) (MasterKey, error) {
	if len(raw) != masterKeySize {
		return MasterKey{}, fmt.Errorf("master key must be %d bytes, got %d", masterKeySize, len(raw))
	}
	var key MasterKey
	copy(key[:], raw)
	return key, nil
}

// Bytes returns a copy of the raw master key bytes.
func (k MasterKey) Bytes() []byte {
	out := make([]byte, masterKeySize)
	copy(out, k[:])
	return out
}

// Hex returns lowercase hex encoding of the master key.
func (k MasterKey) Hex() string {
	return hex.EncodeToString(k[:])
}

// ProjectKeyRing derives per-project keys from a master key and tracks generations.
type ProjectKeyRing struct {
	Master          MasterKey
	ProjectID       string
	WriteGeneration int
}

// NewProjectKeyRing starts a ring at generation zero.
func NewProjectKeyRing(master MasterKey, projectID string) ProjectKeyRing {
	return ProjectKeyRing{
		Master:          master,
		ProjectID:       strings.TrimSpace(projectID),
		WriteGeneration: 0,
	}
}

// DeriveProjectKey returns the AEAD key for a specific generation.
func DeriveProjectKey(master MasterKey, projectID string, generation int) ([projectKeySize]byte, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return [projectKeySize]byte{}, fmt.Errorf("derive project key: project id is empty")
	}
	if generation < 0 {
		return [projectKeySize]byte{}, fmt.Errorf("derive project key: generation %d is invalid", generation)
	}
	info := projectKeyLabel + "/" + projectID + "/gen/" + strconv.Itoa(generation)
	raw, err := hkdf.Key(sha256.New, master[:], nil, info, projectKeySize)
	if err != nil {
		return [projectKeySize]byte{}, fmt.Errorf("derive project key: %w", err)
	}
	var key [projectKeySize]byte
	copy(key[:], raw)
	return key, nil
}

// WriteKey returns the current write-generation project key.
func (r ProjectKeyRing) WriteKey() ([projectKeySize]byte, error) {
	return DeriveProjectKey(r.Master, r.ProjectID, r.WriteGeneration)
}

// ReadKeys returns keys for every generation from zero through the write generation.
func (r ProjectKeyRing) ReadKeys() ([][projectKeySize]byte, error) {
	keys := make([][projectKeySize]byte, 0, r.WriteGeneration+1)
	for gen := 0; gen <= r.WriteGeneration; gen++ {
		key, err := DeriveProjectKey(r.Master, r.ProjectID, gen)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// Rotate bumps the write generation while preserving read access to prior keys.
func (r *ProjectKeyRing) Rotate() {
	r.WriteGeneration++
}

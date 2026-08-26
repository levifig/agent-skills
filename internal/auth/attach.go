package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const UnattachedCode = "environment-unattached"

// AttachRecord marks one project as attached for this environment.
type AttachRecord struct {
	ProjectID  string `json:"project_id"`
	EnvID      string `json:"env_id"`
	InstanceID string `json:"instance_id,omitempty"`
	Endpoint   string `json:"endpoint"`
	AttachedAt string `json:"attached_at"`
}

// IsAttached reports whether the project has a persisted attach record.
func IsAttached(dataHome, projectID string) (bool, error) {
	_, err := LoadAttachRecord(dataHome, projectID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// LoadAttachRecord reads the attach record for a project.
func LoadAttachRecord(dataHome, projectID string) (AttachRecord, error) {
	path := AttachRecordPath(dataHome, projectID)
	raw, err := os.ReadFile(path)
	if err != nil {
		return AttachRecord{}, err
	}
	var record AttachRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return AttachRecord{}, fmt.Errorf("decode attach record: %w", err)
	}
	if strings.TrimSpace(record.ProjectID) == "" {
		return AttachRecord{}, fmt.Errorf("attach record missing project_id")
	}
	return record, nil
}

// SaveAttachRecord persists an attach record for a project.
func SaveAttachRecord(dataHome string, record AttachRecord) error {
	record.ProjectID = strings.TrimSpace(record.ProjectID)
	if record.ProjectID == "" {
		return fmt.Errorf("attach record project_id is empty")
	}
	if strings.TrimSpace(record.AttachedAt) == "" {
		record.AttachedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	path := AttachRecordPath(dataHome, record.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create attach directory: %w", err)
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode attach record: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write attach record: %w", err)
	}
	return nil
}

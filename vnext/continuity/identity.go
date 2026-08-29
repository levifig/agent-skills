package continuity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewFactID mints an opaque fact identity suitable for retained retries.
func NewFactID() (FactID, error) {
	value, err := mintOpaqueID("fact_")
	return FactID(value), err
}

// NewProjectID mints an opaque project identity.
func NewProjectID() (ProjectID, error) {
	value, err := mintOpaqueID("project_")
	return ProjectID(value), err
}

// NewSubjectID mints an opaque continuity-subject identity.
func NewSubjectID() (SubjectID, error) {
	value, err := mintOpaqueID("subject_")
	return SubjectID(value), err
}

// NewEnvironmentID mints an opaque fact-producing environment identity.
func NewEnvironmentID() (EnvironmentID, error) {
	value, err := mintOpaqueID("environment_")
	return EnvironmentID(value), err
}

func mintOpaqueID(prefix string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("mint continuity identity: %w", err)
	}
	return prefix + hex.EncodeToString(random), nil
}

package auth

import (
	"path/filepath"
	"strings"
)

const (
	adminCredentialFile = "admin.credential"
	attachDirName       = "attach"
	syncDBFileName      = "sync.sqlite"
)

// AdminCredentialPath returns the machine-local admin credential file path.
func AdminCredentialPath(dataHome string) string {
	return filepath.Join(loafDataDir(dataHome), adminCredentialFile)
}

// AttachRecordPath returns the attach status file for one project.
func AttachRecordPath(dataHome, projectID string) string {
	projectID = strings.TrimSpace(projectID)
	return filepath.Join(loafDataDir(dataHome), attachDirName, projectID+".json")
}

// SyncServerDBPath returns the default sync relay database path.
func SyncServerDBPath(dataHome string) string {
	return filepath.Join(loafDataDir(dataHome), syncDBFileName)
}

func loafDataDir(dataHome string) string {
	dataHome = strings.TrimSpace(dataHome)
	if dataHome == "" {
		return filepath.Join("loaf")
	}
	return filepath.Join(dataHome, "loaf")
}

package auth

import "errors"

var (
	// ErrAdminNotConfigured means no admin wire is stored locally.
	ErrAdminNotConfigured = errors.New("admin is not configured; run loaf auth setup first")
)

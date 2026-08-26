package auth

import "errors"

var (
	// ErrAdminNotConfigured means no admin wire is stored locally.
	ErrAdminNotConfigured = errors.New("admin is not configured; run loaf auth setup first")
	// ErrClientNotConfigured means no bundled client credential is available.
	ErrClientNotConfigured = errors.New("client credential is not configured")
)

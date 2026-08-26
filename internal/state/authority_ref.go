package state

import (
	"fmt"
	"strings"
)

const (
	AuthorityProviderLinear = "linear"
	AuthorityProviderBranch = "branch"
	AuthorityProviderPR     = "pr"
)

var v1AuthorityProviders = map[string]bool{
	AuthorityProviderLinear: true,
	AuthorityProviderBranch: true,
	AuthorityProviderPR:     true,
}

var futureAuthorityProviders = map[string]bool{
	"github":  true,
	"gitlab":  true,
	"gitea":   true,
	"forgejo": true,
}

// AuthorityRef is a provider-qualified work authority identifier.
type AuthorityRef struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
}

func (r AuthorityRef) String() string {
	if r.Provider == "" && r.Key == "" {
		return ""
	}
	return r.Provider + ":" + r.Key
}

// IsAuthorityRef reports whether raw uses provider-qualified addressing.
func IsAuthorityRef(raw string) bool {
	provider, key, ok := splitAuthorityRef(raw)
	return ok && provider != "" && key != ""
}

// ParseAuthorityRef parses a provider-qualified authority ref for v1 machinery.
// Unsupported future providers refuse actionably and never fall back to issue rows.
func ParseAuthorityRef(raw string) (AuthorityRef, error) {
	provider, key, ok := splitAuthorityRef(raw)
	if !ok || provider == "" || key == "" {
		return AuthorityRef{}, fmt.Errorf("authority ref must be provider-qualified (linear:, branch:, or pr:); got %q", strings.TrimSpace(raw))
	}
	provider = strings.ToLower(provider)
	if futureAuthorityProviders[provider] {
		return AuthorityRef{}, &UnsupportedAuthorityError{Provider: provider}
	}
	if !v1AuthorityProviders[provider] {
		return AuthorityRef{}, fmt.Errorf("unknown authority provider %q; v1 supports linear, branch, and pr", provider)
	}
	return AuthorityRef{Provider: provider, Key: key}, nil
}

// UnsupportedAuthorityError is returned when a future tracker adapter is named
// but not implemented in v1.
type UnsupportedAuthorityError struct {
	Provider string
}

func (e *UnsupportedAuthorityError) Error() string {
	if e == nil {
		return "authority provider is not supported in v1"
	}
	provider := strings.TrimSpace(e.Provider)
	if provider == "" {
		provider = "unknown"
	}
	return fmt.Sprintf("%s is not a v1 authority provider; use linear:, branch:, or pr: refs instead (future adapters are tracked separately; internal issue rows are not a fallback)", provider)
}

func splitAuthorityRef(raw string) (provider, key string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	idx := strings.Index(trimmed, ":")
	if idx <= 0 || idx >= len(trimmed)-1 {
		return "", "", false
	}
	return trimmed[:idx], trimmed[idx+1:], true
}

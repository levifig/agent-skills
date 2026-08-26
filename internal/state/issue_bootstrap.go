package state

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// BootstrapTrackerMode is the steering lane selected during bootstrap.
type BootstrapTrackerMode string

const (
	BootstrapTrackerModeLinear      BootstrapTrackerMode = "linear"
	BootstrapTrackerModeTrackerless BootstrapTrackerMode = "trackerless"
)

// BootstrapTrackerSteeringResult reports the bootstrap lane and follow-on guidance.
type BootstrapTrackerSteeringResult struct {
	Mode      BootstrapTrackerMode `json:"mode"`
	Authority string               `json:"authority"`
	Prefix    string               `json:"prefix,omitempty"`
	Guidance  string               `json:"guidance"`
	Applied   bool                 `json:"applied"`
}

// UnsupportedIssueAuthorityBootstrapError refuses tracker adapters that have no v1 bootstrap path.
type UnsupportedIssueAuthorityBootstrapError struct {
	Authority string
}

func (e *UnsupportedIssueAuthorityBootstrapError) Error() string {
	if e == nil {
		return "issue authority bootstrap is not supported"
	}
	provider := strings.TrimSpace(e.Authority)
	if provider == "" {
		provider = "unknown"
	}
	ref := &UnsupportedAuthorityError{Provider: provider}
	return fmt.Sprintf("issue.authority %q is not bootstrappable in v1; %s", provider, ref.Error())
}

// BootstrapTrackerSteering selects Linear when a tracker is configured and leaves
// trackerless projects on branch:/pr: refs with explicit guidance.
func BootstrapTrackerSteering(ctx context.Context, root project.Root, resolver PathResolver) (BootstrapTrackerSteeringResult, error) {
	cfg, err := LoadIssueProjectConfig(root.Path())
	if err != nil {
		return BootstrapTrackerSteeringResult{}, err
	}
	if cfg.Authority == IssueAuthorityGitHub {
		return BootstrapTrackerSteeringResult{}, &UnsupportedIssueAuthorityBootstrapError{Authority: cfg.Authority}
	}
	for provider := range futureAuthorityProviders {
		if cfg.Authority == provider {
			return BootstrapTrackerSteeringResult{}, &UnsupportedIssueAuthorityBootstrapError{Authority: cfg.Authority}
		}
	}

	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BootstrapTrackerSteeringResult{}, err
	}
	defer store.Close()

	linearReady, err := trackerBootstrapLinearReady(root.Path())
	if err != nil {
		return BootstrapTrackerSteeringResult{}, err
	}

	if linearReady {
		prefix := strings.TrimSpace(cfg.Prefix)
		if prefix == "" {
			identity, ok, err := store.LookupIssueIdentity(ctx, root)
			if err != nil {
				return BootstrapTrackerSteeringResult{}, err
			}
			if ok {
				prefix = identity.Prefix
			}
		}
		opts := IssueIdentityOptions{Authority: IssueAuthorityLinear}
		if prefix != "" {
			opts.Prefix = prefix
		}
		identity, err := store.SetIssueIdentity(ctx, root, opts)
		if err != nil {
			return BootstrapTrackerSteeringResult{}, err
		}
		if err := persistIssueProjectConfig(root.Path(), IssueAuthorityLinear, identity.Prefix); err != nil {
			return BootstrapTrackerSteeringResult{}, err
		}
		guidance := "tracker-backed: create work with `loaf issue new` (mints Linear) or address contracts as linear:<KEY>; unsupported providers refuse — use branch: or pr: only when trackerless"
		if identity.Prefix == "" {
			guidance = "Linear is configured but team prefix is missing; set `loaf issue identity --prefix <TEAM>` then re-run bootstrap"
		}
		return BootstrapTrackerSteeringResult{
			Mode:      BootstrapTrackerModeLinear,
			Authority: identity.Authority,
			Prefix:    identity.Prefix,
			Guidance:  guidance,
			Applied:   true,
		}, nil
	}

	identity, ok, err := store.LookupIssueIdentity(ctx, root)
	if err != nil {
		return BootstrapTrackerSteeringResult{}, err
	}
	authority := IssueAuthorityLocal
	prefix := ""
	if ok {
		authority = identity.Authority
		prefix = identity.Prefix
	} else if cfg.Authority != "" {
		authority = cfg.Authority
		prefix = cfg.Prefix
	}
	return BootstrapTrackerSteeringResult{
		Mode:      BootstrapTrackerModeTrackerless,
		Authority: authority,
		Prefix:    prefix,
		Guidance:  "trackerless: use branch:<name> or pr:<number> authority refs and `loaf issue render-out`; internal LOAF-* rows are legacy — do not mint new tracker work as internal issues",
		Applied:   false,
	}, nil
}

// RefuseUnsupportedIssueBootstrap blocks issue creation when bootstrap would mint internal rows for unsupported tracker kinds.
func RefuseUnsupportedIssueBootstrap(projectRoot string) error {
	cfg, err := LoadIssueProjectConfig(projectRoot)
	if err != nil {
		return err
	}
	if cfg.Authority == IssueAuthorityGitHub {
		return &UnsupportedIssueAuthorityBootstrapError{Authority: cfg.Authority}
	}
	for provider := range futureAuthorityProviders {
		if cfg.Authority == provider {
			return &UnsupportedIssueAuthorityBootstrapError{Authority: cfg.Authority}
		}
	}
	return nil
}

func trackerBootstrapLinearReady(projectRoot string) (bool, error) {
	if strings.TrimSpace(os.Getenv("LINEAR_API_KEY")) != "" {
		return true, nil
	}
	if enabled, err := linearIntegrationEnabled(projectRoot); err != nil {
		return false, err
	} else if enabled {
		return true, nil
	}
	if _, err := LinearClientFromEnv(); err == nil {
		return true, nil
	}
	return false, nil
}

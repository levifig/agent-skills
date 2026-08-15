package cli

import (
	"context"

	"github.com/levifig/loaf/internal/state"
)

const (
	readinessLabelAgent = "ready-for-agent"
	readinessLabelHuman = "ready-for-human"
)

// ReadinessPublication is the tracker-facing readiness signal.
type ReadinessPublication struct {
	IssueID  string `json:"issue_id"`
	IssueRef string `json:"issue_ref"`
	Label    string `json:"label"`
	Reason   string `json:"reason,omitempty"`
}

// ReadinessPublisher publishes derived readiness to a bound tracker.
// The only production implementation today is a no-op; a later task
// supplies Linear/GitHub adapters. Tests inject a fake.
type ReadinessPublisher interface {
	Publish(ctx context.Context, publication ReadinessPublication) error
}

type noopReadinessPublisher struct{}

func (noopReadinessPublisher) Publish(context.Context, ReadinessPublication) error {
	return nil
}

var defaultReadinessPublisher ReadinessPublisher = noopReadinessPublisher{}

func trackerAuthority(authority string) bool {
	return authority == state.IssueAuthorityLinear || authority == state.IssueAuthorityGitHub
}

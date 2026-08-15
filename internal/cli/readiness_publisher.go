package cli

import (
	"context"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

const (
	readinessLabelAgent = "ready-for-agent"
	readinessLabelHuman = "ready-for-human"
)

// ReadinessPublication is the tracker-facing readiness signal.
type ReadinessPublication struct {
	IssueID     string `json:"issue_id"`
	IssueRef    string `json:"issue_ref"`
	Label       string `json:"label"`
	Reason      string `json:"reason,omitempty"`
	Authority   string `json:"authority,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
	StateHome   string `json:"-"`
}

// ReadinessPublisher publishes derived readiness to a bound tracker.
// Linear-authority projects apply ready-for-agent / ready-for-human
// through the Linear adapter. Tests inject a fake.
type ReadinessPublisher interface {
	Publish(ctx context.Context, publication ReadinessPublication) error
}

type noopReadinessPublisher struct{}

func (noopReadinessPublisher) Publish(context.Context, ReadinessPublication) error {
	return nil
}

type linearReadinessPublisher struct{}

func (linearReadinessPublisher) Publish(ctx context.Context, publication ReadinessPublication) error {
	if publication.Authority != state.IssueAuthorityLinear {
		return nil
	}
	client, err := state.LinearClientFromEnv()
	if err != nil {
		return err
	}
	root, err := project.ResolveRoot(publication.ProjectPath)
	if err != nil {
		return err
	}
	return state.PublishLinearReadiness(ctx, root, state.PathResolver{StateHome: publication.StateHome}, client, publication.IssueID, publication.Label, publication.Reason)
}

var defaultReadinessPublisher ReadinessPublisher = linearReadinessPublisher{}

func trackerAuthority(authority string) bool {
	return authority == state.IssueAuthorityLinear || authority == state.IssueAuthorityGitHub
}

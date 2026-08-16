package state

import (
	"context"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// NormalizeIssueLinkType maps CLI relationship names onto the stored
// vocabulary: blocks and relates_to. relates-to is accepted as an alias.
func NormalizeIssueLinkType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "blocks":
		return IssueRelationshipBlocks, nil
	case "relates-to", "relates_to":
		return IssueRelationshipRelatesTo, nil
	case "blocked_by", "blocked-by":
		return IssueRelationshipBlockedBy, nil
	default:
		return "", &IssueValidationError{Field: "type", Err: fmt.Errorf("must be blocks or relates-to")}
	}
}

// CreateIssueLink writes an issue-to-issue relationship through the shared
// relationships table.
func CreateIssueLink(ctx context.Context, root project.Root, resolver PathResolver, from, relationshipType, to string) (LinkMutationResult, error) {
	normalized, err := NormalizeIssueLinkType(relationshipType)
	if err != nil {
		return LinkMutationResult{}, err
	}
	if normalized != IssueRelationshipBlocks && normalized != IssueRelationshipRelatesTo {
		return LinkMutationResult{}, &IssueValidationError{Field: "type", Err: fmt.Errorf("must be blocks or relates-to")}
	}
	return CreateLink(ctx, root, resolver, LinkMutationOptions{
		From:   from,
		To:     to,
		Type:   normalized,
		Reason: "recorded by issue link",
	})
}

// RemoveIssueLink removes an issue-to-issue relationship.
func RemoveIssueLink(ctx context.Context, root project.Root, resolver PathResolver, from, relationshipType, to string) (LinkMutationResult, error) {
	normalized, err := NormalizeIssueLinkType(relationshipType)
	if err != nil {
		return LinkMutationResult{}, err
	}
	if normalized != IssueRelationshipBlocks && normalized != IssueRelationshipRelatesTo {
		return LinkMutationResult{}, &IssueValidationError{Field: "type", Err: fmt.Errorf("must be blocks or relates-to")}
	}
	return RemoveLink(ctx, root, resolver, LinkMutationOptions{
		From: from,
		To:   to,
		Type: normalized,
	})
}

package devtool

import (
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/cli"
)

// TagClassification is the release workflow's gate on a pushed tag.
type TagClassification struct {
	Status  string // invalid, dev, release
	Tag     string
	Version string
	Error   string
}

// ClassifyReleaseTag validates strict SemVer first, then decides whether a
// valid identity is a dev build (+g<sha> or a legacy timestamp patch) or a
// release. Malformed tags fail instead of being skipped as dev.
func ClassifyReleaseTag(tag string) TagClassification {
	if !strings.HasPrefix(tag, "v") {
		return TagClassification{Status: "invalid", Error: "Release tag must start with v."}
	}
	if !cli.IsStrictSemver(tag) {
		return TagClassification{Status: "invalid", Error: "Release tag is not a valid SemVer identity."}
	}
	version := strings.TrimPrefix(tag, "v")
	if cli.IsDevVersion(tag) {
		return TagClassification{Status: "dev", Tag: tag, Version: version}
	}
	return TagClassification{Status: "release", Tag: tag, Version: version}
}

// WriteTagClassification prints the workflow's key=value outputs. Invalid tags
// return an error; dev tags print a guardrail notice to stderr and still emit
// dev=true so the workflow can skip cleanly.
func WriteTagClassification(stdout, stderr io.Writer, tag string) error {
	result := ClassifyReleaseTag(tag)
	if result.Status == "invalid" {
		return fmt.Errorf("%s", result.Error)
	}
	if result.Status == "dev" {
		fmt.Fprintf(stderr, "Release ceremony guardrail: %s carries a dev build identity. Skipping the release.\n", result.Tag)
	}
	fmt.Fprintf(stdout, "tag=%s\n", result.Tag)
	fmt.Fprintf(stdout, "ref=refs/tags/%s\n", result.Tag)
	fmt.Fprintf(stdout, "dev=%t\n", result.Status == "dev")
	return nil
}

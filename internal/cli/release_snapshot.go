package cli

import "fmt"

// resolveReleaseSnapshot is the single derivation every release consumer reads:
// one immutable snapshot of version-file state, bump, candidate, and the commit
// range those fields were resolved from.
//
// Non-post-merge cuts key CurrentVersion on HEAD blobs so a refused prepare's
// worktree candidate bumps do not shift the candidate on resume. Post-merge
// still keys on the prepared worktree/HEAD version (they match on a clean tree).
func resolveReleaseSnapshot(root string, options releaseOptions) (releaseSnapshot, error) {
	configOverrides, err := releaseConfigVersionFiles(root)
	if err != nil {
		return releaseSnapshot{}, err
	}
	versionOverrides := options.versionFile
	if len(versionOverrides) == 0 {
		versionOverrides = configOverrides
	}
	versionFiles, err := detectReleaseVersionFiles(root, versionOverrides)
	if err != nil {
		return releaseSnapshot{}, err
	}
	if len(versionFiles) == 0 {
		return releaseSnapshot{}, fmt.Errorf("no version files detected")
	}
	if !options.postMerge {
		versionFiles = releaseVersionFilesWithHEADBaseline(root, versionFiles)
	}
	current := versionFiles[0].CurrentVersion
	for _, file := range versionFiles {
		if file.CurrentVersion != current {
			return releaseSnapshot{}, fmt.Errorf("inconsistent version files: %s vs %s", current, file.CurrentVersion)
		}
	}
	baseRef, err := releaseCandidateBaseRef(root, options)
	if err != nil {
		return releaseSnapshot{}, err
	}
	commits := releaseCommitsSince(root, baseRef)
	snap := releaseSnapshot{
		VersionFiles:   versionFiles,
		CurrentVersion: current,
		BaseRef:        baseRef,
		Commits:        commits,
	}
	if options.postMerge {
		// Post-merge keys on the prepared version at HEAD: a prepared prerelease
		// publishes through the valve; a prepared stable tags. Bump says
		// "release" only when this run finalizes stable — a prerelease publish
		// performs no bump, and the field must not claim one.
		snap.Candidate = current
		if !releaseVersionIsPrerelease(current) {
			snap.Bump = "release"
		}
		return guardReleaseCeremony(snap)
	}
	bump := effectiveReleaseBumpFrom(options, commits)
	if bump == "" {
		// Nothing unreleased: the executor stops before cutting anything, so the
		// candidate is the version the repository already carries.
		snap.Candidate = current
		return guardReleaseCeremony(snap)
	}
	next := bumpReleaseVersion(current, bump)
	if next == "" {
		return releaseSnapshot{}, fmt.Errorf("cannot bump %q with %q", current, bump)
	}
	snap.Bump = bump
	snap.Candidate = next
	return guardReleaseCeremony(snap)
}

// guardReleaseCeremony refuses to hand back a snapshot whose candidate is a dev
// build's identity. Dev builds mint a Unix timestamp in the patch slot
// (isDevVersion), and the ceremony a release runs — changelog entry, release
// build, packaged GitHub Release, Homebrew bump — is meaningless for a number
// that names a build clock rather than a published version.
//
// It sits on the snapshot because that is the one derivation every release
// consumer reads, so dry-run, apply, and post-merge are covered by a single
// refusal. Cheaper acts never pass through here and stay available: commits,
// lightweight tags, and prerelease-marked uploads. What it judges is the
// candidate, because that is the number a run would publish.
func guardReleaseCeremony(snap releaseSnapshot) (releaseSnapshot, error) {
	if !isDevVersion(snap.Candidate) {
		return snap, nil
	}
	parsed, _ := parseUpgradeSemver(snap.Candidate)
	return releaseSnapshot{}, fmt.Errorf("release ceremony guardrail: %s is a dev build identity (a Unix timestamp in the patch slot), not a release version; cut releases from a plain %d.%d.X version",
		snap.Candidate, parsed.major, parsed.minor)
}

// releaseVersionFilesWithHEADBaseline rewrites CurrentVersion from HEAD blobs
// when available so snapshot derivation ignores uncommitted candidate bumps.
func releaseVersionFilesWithHEADBaseline(root string, files []releaseVersionFile) []releaseVersionFile {
	out := make([]releaseVersionFile, len(files))
	for i, file := range files {
		out[i] = file
		headBody, err := releaseGitShowPath(root, "HEAD", file.RelativePath)
		if err != nil {
			continue
		}
		version, format, err := parseReleaseVersion(file.RelativePath, headBody)
		if err != nil {
			continue
		}
		out[i].CurrentVersion = version
		out[i].Format = format
	}
	return out
}

// assertReleaseSnapshotStillCurrent re-reads the snapshot's version files and
// blocks when any has drifted from the version the candidate was resolved from.
// An uncommitted worktree already at Candidate is allowed when HEAD still
// carries CurrentVersion (resume after an evidence-gate refusal).
func assertReleaseSnapshotStillCurrent(root string, snapshot releaseSnapshot) error {
	if snapshot.CurrentVersion == "" {
		return fmt.Errorf("release blocked: release snapshot was not resolved before apply")
	}
	for _, file := range snapshot.VersionFiles {
		fresh, err := loadReleaseVersionFile(root, file.RelativePath, true)
		if err != nil {
			return fmt.Errorf("release blocked: cannot re-read version file %s: %w", file.RelativePath, err)
		}
		if fresh.CurrentVersion == snapshot.CurrentVersion {
			continue
		}
		if snapshot.Candidate != "" && fresh.CurrentVersion == snapshot.Candidate {
			if headBody, err := releaseGitShowPath(root, "HEAD", file.RelativePath); err == nil {
				if headVersion, _, parseErr := parseReleaseVersion(file.RelativePath, headBody); parseErr == nil && headVersion == snapshot.CurrentVersion {
					continue
				}
			}
		}
		return fmt.Errorf("release blocked: version drifted from %s to %s since preflight; re-run release", snapshot.CurrentVersion, fresh.CurrentVersion)
	}
	return nil
}

// effectiveReleaseBump resolves the bump the release will actually apply: the
// explicit --bump flag when given, otherwise the bump the unreleased commits
// suggest. It returns "" when there is nothing to release.
func effectiveReleaseBump(root string, options releaseOptions) (string, error) {
	if options.bump != "" {
		return options.bump, nil
	}
	baseRef, err := releaseCandidateBaseRef(root, options)
	if err != nil {
		return "", err
	}
	return effectiveReleaseBumpFrom(options, releaseCommitsSince(root, baseRef)), nil
}

// effectiveReleaseBumpFrom resolves the same bump from an already-loaded commit
// range, so the executor never re-derives it from different inputs.
func effectiveReleaseBumpFrom(options releaseOptions, commits []releaseCommit) string {
	if options.bump != "" {
		return options.bump
	}
	if len(commits) == 0 {
		return ""
	}
	return suggestReleaseBump(commits)
}

// releaseCandidateBaseRef resolves the commit range the suggested bump reads,
// mirroring the executor: an explicit --base wins, --pre-merge auto-detects its
// base, and everything else measures from the last tag.
func releaseCandidateBaseRef(root string, options releaseOptions) (string, error) {
	base := options.base
	if base == "" && options.preMerge {
		detected, _, err := detectReleaseBase(root)
		if err != nil {
			return "", err
		}
		base = detected
	}
	if base == "" {
		return releaseLastTag(root), nil
	}
	return validateReleaseBaseRef(root, base)
}

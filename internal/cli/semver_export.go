package cli

// IsStrictSemver reports whether value (optionally prefixed with v) is a valid
// SemVer identity. It backs the release tag classification in the devtool,
// which must apply exactly the same parsing as the upgrade advisory.
func IsStrictSemver(value string) bool {
	_, ok := parseUpgradeSemver(value)
	return ok
}

// IsDevVersion reports whether a valid version carries a dev build identity:
// +g<short-sha> build metadata or a legacy timestamp patch.
func IsDevVersion(value string) bool {
	return isDevVersion(value)
}

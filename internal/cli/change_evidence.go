package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// ChangeEvidenceDigestSpec is the domain-separation / serialization version
// recorded on every v2 receipt as digest_spec. Bumping it expires all receipts.
const ChangeEvidenceDigestSpec = "v1"

const changeEvidenceDigestDomain = "loaf/change-evidence-digest\nv1\n"

// ChangeEvidenceReceiptMasks exclude every change's receipt surface from the
// scope digest so cohort members' receipts never stale each other.
var ChangeEvidenceReceiptMasks = []string{
	"docs/changes/*/receipts/**",
}

// ChangeEvidenceReportMasks exclude authored report boards from the scope digest.
var ChangeEvidenceReportMasks = []string{
	"docs/changes/*/reports/**",
}

// ReleaseMetadataAllowlist names paths the release/promotion ceremony may rewrite
// without changing receipt-bound content. The promotion Change imports this
// constant for its designation diff check so designation-legal ≡ receipt-neutral.
//
// Composition obligation: masking regenerated outputs (dist/**, plugins/**, bin/**)
// means the promotion designation check must independently prove they are the
// deterministic rebuild of source.
var ReleaseMetadataAllowlist = []string{
	"package.json",
	".claude-plugin/marketplace.json",
	"CHANGELOG.md",
	"dist/**",
	"plugins/**",
	"bin/**",
}

// ChangeEvidenceExclusions is the full exclusion set for scopeDigest:
// receipts ∪ reports ∪ ReleaseMetadataAllowlist. Exported so the promotion
// Change can import the same boundary without redefining it.
func ChangeEvidenceExclusions() []string {
	out := make([]string, 0, len(ChangeEvidenceReceiptMasks)+len(ChangeEvidenceReportMasks)+len(ReleaseMetadataAllowlist))
	out = append(out, ChangeEvidenceReceiptMasks...)
	out = append(out, ChangeEvidenceReportMasks...)
	out = append(out, ReleaseMetadataAllowlist...)
	return out
}

// changeTreeEntry is one ls-tree blob/commit entry used for digest construction.
type changeTreeEntry struct {
	Mode string
	OID  string
	Path string
}

// changeScopeDigestResult holds the masked root digest and per-top-level-directory
// sub-digests derived from the same filtered, sorted entry stream.
type changeScopeDigestResult struct {
	Digest   string
	Sections map[string]string
}

// scopeDigest computes the content digest for treeish under exclusions.
// Serialization (pinned): over every git ls-tree -r -z --full-tree entry whose
// path matches no glob in exclusions, emit path\0mode\0oid\n, byte-sort
// ascending, prefix the domain header, SHA-256 hex.
//
// Glob grammar (component-anchored, byte-exact, case-sensitive):
//   - literal segments match exactly
//   - * matches exactly one path segment
//   - ** as a trailing segment matches zero or more remaining segments
//
// Paths come from ls-tree only — never the filesystem — so quotePath, autocrlf,
// and case-folding cannot change the digest.
func scopeDigest(rootPath, treeish string, exclusions []string, outputCommand changeGitOutput) (changeScopeDigestResult, error) {
	if outputCommand == nil {
		outputCommand = commandOutput
	}
	raw, err := outputCommand(rootPath, "git", "ls-tree", "-r", "-z", "--full-tree", treeish)
	if err != nil {
		return changeScopeDigestResult{}, fmt.Errorf("ls-tree %s: %w", treeish, err)
	}
	entries, err := parseLSTreeNUL(raw)
	if err != nil {
		return changeScopeDigestResult{}, err
	}
	filtered := make([]changeTreeEntry, 0, len(entries))
	for _, e := range entries {
		if evidencePathExcluded(e.Path, exclusions) {
			continue
		}
		filtered = append(filtered, e)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Path < filtered[j].Path
	})
	digest := hashEvidenceEntries(filtered)
	sections := map[string]string{}
	bySection := map[string][]changeTreeEntry{}
	for _, e := range filtered {
		section := evidenceTopLevelSection(e.Path)
		bySection[section] = append(bySection[section], e)
	}
	for section, sectionEntries := range bySection {
		sections[section] = hashEvidenceEntries(sectionEntries)
	}
	return changeScopeDigestResult{Digest: digest, Sections: sections}, nil
}

func hashEvidenceEntries(entries []changeTreeEntry) string {
	var b strings.Builder
	b.WriteString(changeEvidenceDigestDomain)
	for _, e := range entries {
		b.WriteString(e.Path)
		b.WriteByte(0)
		b.WriteString(e.Mode)
		b.WriteByte(0)
		b.WriteString(e.OID)
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func evidenceTopLevelSection(path string) string {
	if i := strings.IndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	return path
}

// evidencePathExcluded reports whether path matches any exclusion glob.
func evidencePathExcluded(path string, exclusions []string) bool {
	for _, pattern := range exclusions {
		if matchEvidenceGlob(path, pattern) {
			return true
		}
	}
	return false
}

// matchEvidenceGlob matches a git tree path against a component-anchored glob.
// Matching is byte-exact and case-sensitive; * is one segment; trailing ** is
// zero-or-more remaining segments.
func matchEvidenceGlob(path, pattern string) bool {
	pathParts := splitPathSegments(path)
	patternParts := splitPathSegments(pattern)
	return matchEvidenceParts(pathParts, patternParts)
}

func splitPathSegments(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func matchEvidenceParts(pathParts, patternParts []string) bool {
	pi, pti := 0, 0
	for pti < len(patternParts) {
		pat := patternParts[pti]
		if pat == "**" {
			if pti == len(patternParts)-1 {
				return true
			}
			// Non-trailing ** is not used by the exclusion set; treat as "match
			// any prefix then resume" for completeness.
			rest := patternParts[pti+1:]
			for skip := 0; skip <= len(pathParts)-pi; skip++ {
				if matchEvidenceParts(pathParts[pi+skip:], rest) {
					return true
				}
			}
			return false
		}
		if pi >= len(pathParts) {
			return false
		}
		if pat != "*" && pat != pathParts[pi] {
			return false
		}
		pi++
		pti++
	}
	return pi == len(pathParts)
}

// parseLSTreeNUL parses `git ls-tree -z` output into entries.
// Each record is: <mode> SP <type> SP <object> TAB <file> NUL
func parseLSTreeNUL(raw string) ([]changeTreeEntry, error) {
	if raw == "" {
		return nil, nil
	}
	records := strings.Split(raw, "\x00")
	entries := make([]changeTreeEntry, 0, len(records))
	for _, rec := range records {
		if rec == "" {
			continue
		}
		tab := strings.IndexByte(rec, '\t')
		if tab < 0 {
			return nil, fmt.Errorf("ls-tree record missing tab: %q", rec)
		}
		meta := rec[:tab]
		path := rec[tab+1:]
		parts := strings.SplitN(meta, " ", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("ls-tree record malformed meta: %q", rec)
		}
		mode, typ, oid := parts[0], parts[1], parts[2]
		if typ != "blob" && typ != "commit" {
			// -r lists blobs (and submodule commits); skip unexpected types.
			continue
		}
		entries = append(entries, changeTreeEntry{Mode: mode, OID: oid, Path: path})
	}
	return entries, nil
}

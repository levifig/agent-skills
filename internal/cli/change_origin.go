package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

const (
	ChangeOriginCodeNotFound                  = "change-not-found"
	ChangeOriginCodeAmbiguous                 = "change-ambiguous"
	ChangeOriginCodeOutsideCanonicalDirectory = "change-outside-canonical-directory"
	ChangeOriginCodeIdentityMismatch          = "change-identity-mismatch"
	ChangeOriginCodeEvidenceUnavailable       = "change-evidence-unavailable"
)

// ChangeOriginError is a stable, typed failure from local Change evidence
// resolution. Ref and Path retain the caller's selector and the best-known
// filesystem path without making the error depend on mutable state later.
type ChangeOriginError struct {
	Code string
	Ref  string
	Path string
	Err  error
}

// changeOriginOps is a per-call seam for deterministic local race tests. It
// deliberately has no package-global hooks: production calls use the real Git
// and filesystem operations, while tests can advance HEAD or swap a path at
// the exact evidence-capture boundaries.
type changeOriginOps struct {
	gitOutputBytes            func(string, ...string) ([]byte, error)
	afterHeadCapture          func()
	afterOpenBeforeRevalidate func()
}

func normalizeChangeOriginOps(ops changeOriginOps) changeOriginOps {
	if ops.gitOutputBytes == nil {
		ops.gitOutputBytes = originGitOutputBytes
	}
	if ops.afterHeadCapture == nil {
		ops.afterHeadCapture = func() {}
	}
	if ops.afterOpenBeforeRevalidate == nil {
		ops.afterOpenBeforeRevalidate = func() {}
	}
	return ops
}

func (e *ChangeOriginError) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{e.Code}
	if e.Ref != "" {
		parts = append(parts, fmt.Sprintf("ref %q", e.Ref))
	}
	if e.Path != "" {
		parts = append(parts, fmt.Sprintf("path %q", e.Path))
	}
	if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}
	return strings.Join(parts, ": ")
}

func (e *ChangeOriginError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ResolveChangeOrigin captures a self-contained local Change origin envelope.
// The selector is either a retained Change slug or a path to its canonical
// folder/change.md. No network or harness metadata is consulted.
func ResolveChangeOrigin(rootPath, ref string) (state.JournalOriginInput, error) {
	return resolveChangeOriginWithOps(rootPath, ref, changeOriginOps{})
}

// ResolveManualJournalOrigin captures the local Git context that is available
// for a manual journal write. Git is contextual rather than required here:
// journal logging remains useful outside a repository and in repositories
// without a commit, so unavailable fields stay empty instead of being guessed.
func ResolveManualJournalOrigin(rootPath, sourceEvent string) state.JournalOriginInput {
	origin := state.JournalOriginInput{
		EnvelopeVersion:  state.JournalOriginEnvelopeVersion,
		CaptureMechanism: state.JournalOriginMechanismManual,
		SourceEvent:      sourceEvent,
	}
	if strings.TrimSpace(rootPath) == "" {
		rootPath = "."
	}
	worktreeBytes, err := originGitOutputBytes(rootPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return origin
	}
	worktree := strings.TrimSpace(string(worktreeBytes))
	if worktree == "" {
		return origin
	}
	if absolute, absErr := filepath.Abs(worktree); absErr == nil {
		worktree = absolute
	}
	if evaluated, evalErr := filepath.EvalSymlinks(worktree); evalErr == nil {
		worktree = evaluated
	}
	origin.Worktree = worktree

	if headBytes, headErr := originGitOutputBytes(worktree, "rev-parse", "--verify", "HEAD"); headErr == nil {
		origin.Head = strings.TrimSpace(string(headBytes))
	}
	if branchBytes, branchErr := originGitOutputBytes(worktree, "symbolic-ref", "--quiet", "--short", "HEAD"); branchErr == nil {
		origin.Branch = strings.TrimSpace(string(branchBytes))
	}
	return origin
}

func resolveChangeOriginWithOps(rootPath, ref string, rawOps changeOriginOps) (state.JournalOriginInput, error) {
	ops := normalizeChangeOriginOps(rawOps)
	gitRoot, head, branch, err := resolveChangeGitContextWithOps(rootPath, ref, ops)
	if err != nil {
		return state.JournalOriginInput{}, err
	}

	changeFile, relPath, err := resolveCanonicalChangePath(gitRoot, ref)
	if err != nil {
		return state.JournalOriginInput{}, err
	}
	content, err := readValidatedChange(gitRoot, ref, changeFile, relPath, ops)
	if err != nil {
		return state.JournalOriginInput{}, err
	}
	if err := validateChangeOriginIdentity(content, filepath.Base(filepath.Dir(changeFile)), ref, relPath); err != nil {
		return state.JournalOriginInput{}, err
	}

	digest := sha256.Sum256(content)
	dirty := true
	reconstructable := false
	if headContent, showErr := ops.gitOutputBytes(gitRoot, "show", head+":"+filepath.ToSlash(relPath)); showErr == nil {
		dirty = !equalBytes(headContent, content)
		if !dirty && head != "" {
			reconstructable = true
		}
	}

	return state.JournalOriginInput{
		EnvelopeVersion:  state.JournalOriginEnvelopeVersion,
		CaptureMechanism: state.JournalOriginMechanismManual,
		SourceEvent:      "journal.defer",
		Branch:           branch,
		Worktree:         gitRoot,
		Head:             head,
		ChangePath:       filepath.ToSlash(relPath),
		ChangeSHA256:     hex.EncodeToString(digest[:]),
		Dirty:            boolPointer(dirty),
		Reconstructable:  boolPointer(reconstructable),
	}, nil
}

// resolveChangeOrigin is kept package-local for the journal command, while
// ResolveChangeOrigin is available to other internal CLI wiring and tests.
func resolveChangeOrigin(rootPath, ref string) (state.JournalOriginInput, error) {
	return ResolveChangeOrigin(rootPath, ref)
}

func resolveChangeGitContext(rootPath, ref string) (string, string, string, error) {
	return resolveChangeGitContextWithOps(rootPath, ref, normalizeChangeOriginOps(changeOriginOps{}))
}

func resolveChangeGitContextWithOps(rootPath, ref string, ops changeOriginOps) (string, string, string, error) {
	ops = normalizeChangeOriginOps(ops)
	if strings.TrimSpace(rootPath) == "" {
		rootPath = "."
	}
	outputBytes, err := ops.gitOutputBytes(rootPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", "", changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, rootPath, "resolve git worktree", err)
	}
	gitRoot := strings.TrimSpace(string(outputBytes))
	if gitRoot == "" {
		return "", "", "", changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, rootPath, "resolve git worktree", errors.New("git returned an empty worktree"))
	}
	gitRoot, err = filepath.Abs(gitRoot)
	if err != nil {
		return "", "", "", changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, rootPath, "make git worktree absolute", err)
	}
	if evaluated, evalErr := filepath.EvalSymlinks(gitRoot); evalErr == nil {
		gitRoot = evaluated
	}

	headBytes, err := ops.gitOutputBytes(gitRoot, "rev-parse", "--verify", "HEAD")
	head := strings.TrimSpace(string(headBytes))
	if err != nil || head == "" {
		if err == nil {
			err = errors.New("git returned an empty HEAD")
		}
		return "", "", "", changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, gitRoot, "resolve HEAD", err)
	}
	ops.afterHeadCapture()
	branchBytes, branchErr := ops.gitOutputBytes(gitRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	if branchErr != nil {
		branchBytes = nil
	}
	branch := strings.TrimSpace(string(branchBytes))
	return gitRoot, head, branch, nil
}

func resolveCanonicalChangePath(gitRoot, ref string) (string, string, error) {
	base := filepath.Join(gitRoot, "docs", "changes")
	baseReal, baseErr := filepath.EvalSymlinks(base)
	if baseErr != nil {
		baseReal = base
	} else if !pathWithin(gitRoot, baseReal) {
		return "", "", changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, ref, base, "docs/changes resolves outside the git worktree", nil)
	}

	if isChangeSlugSelector(ref) {
		return resolveChangeSlug(gitRoot, base, baseReal, ref)
	}
	return resolveExplicitChangePath(gitRoot, base, baseReal, ref)
}

func isChangeSlugSelector(ref string) bool {
	return changeSlugRE.MatchString(ref) && changeFolderRE.FindStringSubmatch(ref) == nil
}

func resolveChangeSlug(gitRoot, base, baseReal, slug string) (string, string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", changeOriginFailure(ChangeOriginCodeNotFound, slug, filepath.Join("docs", "changes"), "no retained Change matches slug", nil)
		}
		return "", "", changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, slug, base, "list retained Changes", err)
	}
	var matches []canonicalChangePath
	for _, entry := range entries {
		folderMatch := changeFolderRE.FindStringSubmatch(entry.Name())
		if folderMatch == nil || folderMatch[2] != slug {
			continue
		}
		folderPath := filepath.Join(base, entry.Name())
		if folderTarget, folderErr := filepath.EvalSymlinks(folderPath); folderErr == nil && (!pathWithin(gitRoot, folderTarget) || !pathWithin(baseReal, folderTarget)) {
			return "", "", changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, slug, folderTarget, "Change folder resolves outside docs/changes", nil)
		}
		candidate, candidateErr := resolveCanonicalChangeFile(gitRoot, base, baseReal, filepath.Join(folderPath, "change.md"))
		if candidateErr != nil {
			var typed *ChangeOriginError
			if errors.As(candidateErr, &typed) && typed.Code == ChangeOriginCodeNotFound {
				continue
			}
			return "", "", candidateErr
		}
		resolvedFolder := changeFolderRE.FindStringSubmatch(filepath.Base(filepath.Dir(candidate.absolute)))
		if resolvedFolder == nil || resolvedFolder[2] != slug {
			resolvedSlug := ""
			if resolvedFolder != nil {
				resolvedSlug = resolvedFolder[2]
			}
			return "", "", changeOriginFailure(ChangeOriginCodeIdentityMismatch, slug, candidate.relative, fmt.Sprintf("resolved folder slug %q does not match requested slug %q", resolvedSlug, slug), nil)
		}
		matches = append(matches, candidate)
	}
	if len(matches) == 0 {
		return "", "", changeOriginFailure(ChangeOriginCodeNotFound, slug, filepath.Join("docs", "changes"), "no retained Change matches slug", nil)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].relative < matches[j].relative })
	if len(matches) > 1 {
		paths := make([]string, len(matches))
		for i, match := range matches {
			paths[i] = match.relative
		}
		return "", "", changeOriginFailure(ChangeOriginCodeAmbiguous, slug, filepath.Join("docs", "changes"), "matches "+strings.Join(paths, ", "), nil)
	}
	return matches[0].absolute, matches[0].relative, nil
}

func resolveExplicitChangePath(gitRoot, base, baseReal, ref string) (string, string, error) {
	path := ref
	if !filepath.IsAbs(path) {
		if changeFolderRE.FindStringSubmatch(path) != nil {
			path = filepath.Join(gitRoot, "docs", "changes", path)
		} else {
			path = filepath.Join(gitRoot, path)
		}
	}
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", "", changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, ref, path, "clean Change path", err)
	}

	if info, statErr := os.Stat(path); statErr == nil {
		if info.IsDir() {
			path = filepath.Join(path, "change.md")
		}
	} else if !os.IsNotExist(statErr) {
		return "", "", changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, path, "inspect Change path", statErr)
	} else if filepath.Base(path) != "change.md" {
		path = filepath.Join(path, "change.md")
	}

	resolved, err := resolveCanonicalChangeFile(gitRoot, base, baseReal, path)
	if err != nil {
		return "", "", err
	}
	return resolved.absolute, resolved.relative, nil
}

type canonicalChangePath struct {
	absolute string
	relative string
}

func resolveCanonicalChangeFile(gitRoot, base, baseReal, candidate string) (canonicalChangePath, error) {
	lexical, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, candidate, candidate, "clean Change path", err)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		if !pathWithinExistingAncestor(gitRoot, lexical) {
			return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, candidate, lexical, "Change path is outside the git worktree", nil)
		}
		if os.IsNotExist(err) {
			if !canonicalChangeLexicalShape(lexical) {
				return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, candidate, lexical, "Change path must be docs/changes/YYYYMMDD-slug/change.md", nil)
			}
			return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeNotFound, candidate, candidate, "Change file does not exist", err)
		}
		return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, candidate, candidate, "resolve Change path", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, candidate, candidate, "make Change path absolute", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeNotFound, candidate, candidate, "stat Change file", err)
	}
	if info.IsDir() {
		return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, candidate, resolved, "Change path is a directory", nil)
	}

	rel, err := filepath.Rel(baseReal, resolved)
	if err != nil || !pathWithin(baseReal, resolved) {
		return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, candidate, resolved, "Change path is outside docs/changes", err)
	}
	parts := splitPath(rel)
	if len(parts) != 2 || parts[1] != "change.md" || changeFolderRE.FindStringSubmatch(parts[0]) == nil {
		return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, candidate, resolved, "Change path must be docs/changes/YYYYMMDD-slug/change.md", nil)
	}
	if !pathWithin(gitRoot, resolved) {
		return canonicalChangePath{}, changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, candidate, resolved, "Change path resolves outside the git worktree", nil)
	}
	// base is intentionally retained in the signature: it documents and checks
	// the lexical canonical root for missing-path diagnostics and future callers.
	_ = base
	return canonicalChangePath{absolute: resolved, relative: filepath.ToSlash(relFromRoot(gitRoot, resolved))}, nil
}

func canonicalChangeLexicalShape(path string) bool {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] != "docs" || parts[i+1] != "changes" {
			continue
		}
		return len(parts[i+2:]) == 2 && parts[len(parts)-1] == "change.md" && changeFolderRE.FindStringSubmatch(parts[i+2]) != nil
	}
	return false
}

func readValidatedChange(gitRoot, ref, changeFile, relPath string, ops changeOriginOps) ([]byte, error) {
	opened, err := os.Open(changeFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, changeOriginFailure(ChangeOriginCodeNotFound, ref, relPath, "open working Change", err)
		}
		return nil, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, relPath, "open working Change", err)
	}
	defer opened.Close()

	openedInfo, err := opened.Stat()
	if err != nil {
		return nil, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, relPath, "stat opened Change", err)
	}
	if openedInfo.IsDir() {
		return nil, changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, ref, relPath, "opened Change path is a directory", nil)
	}
	ops.afterOpenBeforeRevalidate()

	revalidatedFile, revalidatedPath, revalidateErr := resolveCanonicalChangePath(gitRoot, ref)
	if revalidateErr != nil {
		var typed *ChangeOriginError
		if errors.As(revalidateErr, &typed) && (typed.Code == ChangeOriginCodeOutsideCanonicalDirectory || typed.Code == ChangeOriginCodeIdentityMismatch) {
			return nil, revalidateErr
		}
		return nil, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, relPath, "revalidate opened Change path", revalidateErr)
	}
	requestedSlug := ""
	if isChangeSlugSelector(ref) {
		requestedSlug = ref
	}
	if requestedSlug != "" {
		resolvedFolder := changeFolderRE.FindStringSubmatch(filepath.Base(filepath.Dir(revalidatedFile)))
		if resolvedFolder == nil || resolvedFolder[2] != requestedSlug {
			return nil, changeOriginFailure(ChangeOriginCodeIdentityMismatch, ref, revalidatedPath, "revalidated folder slug does not match requested slug", nil)
		}
	}

	revalidatedInfo, err := os.Stat(revalidatedFile)
	if err != nil {
		return nil, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, relPath, "stat revalidated Change path", err)
	}
	if !os.SameFile(openedInfo, revalidatedInfo) {
		return nil, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, relPath, "opened Change inode changed during revalidation", nil)
	}

	content, err := io.ReadAll(opened)
	if err != nil {
		return nil, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, relPath, "read opened Change", err)
	}
	readInfo, err := opened.Stat()
	if err != nil {
		return nil, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, relPath, "restat opened Change", err)
	}
	pathInfo, err := os.Stat(revalidatedFile)
	if err != nil {
		return nil, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, relPath, "restat revalidated Change path", err)
	}
	if !os.SameFile(readInfo, pathInfo) {
		return nil, changeOriginFailure(ChangeOriginCodeEvidenceUnavailable, ref, relPath, "opened Change inode changed after read", nil)
	}
	return content, nil
}

func pathWithinExistingAncestor(root, path string) bool {
	for current := path; ; current = filepath.Dir(current) {
		if evaluated, err := filepath.EvalSymlinks(current); err == nil {
			return pathWithin(root, evaluated)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
	}
}

func validateChangeOriginIdentity(content []byte, folderName, ref, relPath string) error {
	folderMatch := changeFolderRE.FindStringSubmatch(folderName)
	if folderMatch == nil {
		return changeOriginFailure(ChangeOriginCodeOutsideCanonicalDirectory, ref, relPath, "malformed Change folder", nil)
	}
	parsed := parseChangeFrontmatter(string(content))
	if !parsed.AtByteOne {
		return changeOriginFailure(ChangeOriginCodeIdentityMismatch, ref, relPath, "Change frontmatter is missing", nil)
	}
	found := false
	for _, field := range parsed.Fields {
		if !strings.EqualFold(field.Key, "slug") && !strings.EqualFold(field.Key, "change") {
			continue
		}
		found = true
		if field.Value != folderMatch[2] {
			return changeOriginFailure(ChangeOriginCodeIdentityMismatch, ref, relPath, fmt.Sprintf("frontmatter %s %q does not match folder slug %q", field.Key, field.Value, folderMatch[2]), nil)
		}
	}
	if !found {
		return changeOriginFailure(ChangeOriginCodeIdentityMismatch, ref, relPath, "frontmatter has no slug identity", nil)
	}
	return nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func splitPath(path string) []string {
	path = filepath.Clean(path)
	if path == "." || path == string(filepath.Separator) {
		return nil
	}
	return strings.Split(filepath.ToSlash(path), "/")
}

func originGitOutput(cwd string, args ...string) (string, error) {
	output, err := originGitOutputBytes(cwd, args...)
	return string(output), err
}

func originGitOutputBytes(cwd string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	return cmd.Output()
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func boolPointer(value bool) *bool {
	return &value
}

func changeOriginFailure(code, ref, path, message string, cause error) *ChangeOriginError {
	var err error
	if message != "" {
		err = errors.New(message)
	}
	if cause != nil {
		if err != nil {
			err = fmt.Errorf("%w: %v", err, cause)
		} else {
			err = cause
		}
	}
	return &ChangeOriginError{Code: code, Ref: ref, Path: path, Err: err}
}

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/state"
)

func TestResolveChangeOriginCommittedAndExplicitSelectors(t *testing.T) {
	repo, changeFile, content := committedOriginFixture(t, "auth-token-rotation", "20260711")

	bySlug, err := ResolveChangeOrigin(repo, "auth-token-rotation")
	if err != nil {
		t.Fatalf("resolve by slug: %v", err)
	}
	byFolder, err := ResolveChangeOrigin(repo, filepath.Join("docs", "changes", "20260711-auth-token-rotation"))
	if err != nil {
		t.Fatalf("resolve by folder: %v", err)
	}
	byFile, err := ResolveChangeOrigin(repo, changeFile)
	if err != nil {
		t.Fatalf("resolve by file: %v", err)
	}

	for name, origin := range map[string]state.JournalOriginInput{"slug": bySlug, "folder": byFolder, "file": byFile} {
		if origin.EnvelopeVersion != state.JournalOriginEnvelopeVersion || origin.CaptureMechanism != state.JournalOriginMechanismManual || origin.SourceEvent != "journal.defer" {
			t.Errorf("%s envelope = %#v", name, origin)
		}
		if origin.ChangePath != "docs/changes/20260711-auth-token-rotation/change.md" {
			t.Errorf("%s ChangePath = %q", name, origin.ChangePath)
		}
		if origin.Worktree != evalPath(t, repo) {
			t.Errorf("%s Worktree = %q, want %q", name, origin.Worktree, evalPath(t, repo))
		}
		if *origin.Dirty || !*origin.Reconstructable {
			t.Errorf("%s dirty/reconstructable = %v/%v", name, *origin.Dirty, *origin.Reconstructable)
		}
		if origin.Branch != "main" || origin.Head == "" {
			t.Errorf("%s branch/head = %q/%q", name, origin.Branch, origin.Head)
		}
		digest := sha256.Sum256(content)
		if origin.ChangeSHA256 != hex.EncodeToString(digest[:]) {
			t.Errorf("%s digest = %q", name, origin.ChangeSHA256)
		}
	}
}

func TestResolveChangeOriginDirtyAndUnrelatedWorkingTreeChanges(t *testing.T) {
	repo, changeFile, committed := committedOriginFixture(t, "dirty-change", "20260711")

	if err := os.WriteFile(changeFile, []byte("---\nslug: dirty-change\n---\nchanged working bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := ResolveChangeOrigin(repo, "dirty-change")
	if err != nil {
		t.Fatalf("resolve dirty Change: %v", err)
	}
	if !*dirty.Dirty || *dirty.Reconstructable {
		t.Fatalf("dirty/reconstructable = %v/%v", *dirty.Dirty, *dirty.Reconstructable)
	}

	if err := os.WriteFile(changeFile, committed, 0o644); err != nil {
		t.Fatal(err)
	}
	unrelatedFile := filepath.Join(repo, "unrelated.txt")
	if err := os.WriteFile(unrelatedFile, []byte("unrelated dirty bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clean, err := ResolveChangeOrigin(repo, "dirty-change")
	if err != nil {
		t.Fatalf("resolve dirty Change with unrelated file: %v", err)
	}
	if *clean.Dirty || !*clean.Reconstructable {
		t.Fatalf("unrelated dirty file changed evidence = %v/%v", *clean.Dirty, *clean.Reconstructable)
	}
}

func TestResolveChangeOriginUntrackedDetachedAndSelfContained(t *testing.T) {
	repo, _, _ := committedOriginFixture(t, "tracked-change", "20260711")
	untrackedFolder := filepath.Join(repo, "docs", "changes", "20260712-untracked-change")
	untrackedFile := filepath.Join(untrackedFolder, "change.md")
	writeOriginChange(t, untrackedFile, "untracked-change")
	untracked, err := ResolveChangeOrigin(repo, "untracked-change")
	if err != nil {
		t.Fatalf("resolve untracked Change: %v", err)
	}
	if !*untracked.Dirty || *untracked.Reconstructable {
		t.Fatalf("untracked dirty/reconstructable = %v/%v", *untracked.Dirty, *untracked.Reconstructable)
	}

	if err := originGitCLI(repo, "checkout", "--detach"); err != nil {
		t.Fatal(err)
	}
	detached, err := ResolveChangeOrigin(repo, "tracked-change")
	if err != nil {
		t.Fatalf("resolve detached Change: %v", err)
	}
	if detached.Branch != "" {
		t.Fatalf("detached branch = %q, want empty", detached.Branch)
	}

	snapshot := detached
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	if snapshot.ChangePath != detached.ChangePath || snapshot.ChangeSHA256 != detached.ChangeSHA256 || snapshot.Worktree != detached.Worktree || snapshot.Head != detached.Head {
		t.Fatal("origin changed after source worktree removal")
	}
}

func TestResolveChangeOriginRejectsAmbiguousAndNonCanonicalSelectors(t *testing.T) {
	repo, _, _ := committedOriginFixture(t, "ambiguous-change", "20260711")
	secondFile := filepath.Join(repo, "docs", "changes", "20260712-ambiguous-change", "change.md")
	writeOriginChange(t, secondFile, "ambiguous-change")
	if _, err := ResolveChangeOrigin(repo, "ambiguous-change"); !hasChangeOriginCode(err, ChangeOriginCodeAmbiguous) {
		t.Fatalf("ambiguous error = %v, want %s", err, ChangeOriginCodeAmbiguous)
	}

	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		outside,
		filepath.Join(repo, "docs", "changes", "20260711-ambiguous-change", "nested", "change.md"),
		filepath.Join(repo, "docs", "changes", "20260711-ambiguous-change", "..", "..", "outside.md"),
	}
	for _, ref := range cases {
		if _, err := ResolveChangeOrigin(repo, ref); !hasChangeOriginCode(err, ChangeOriginCodeOutsideCanonicalDirectory) {
			t.Errorf("noncanonical ref %q error = %v", ref, err)
		}
	}

	escapeFolder := filepath.Join(repo, "docs", "changes", "20260713-escape-change")
	if err := os.Symlink(t.TempDir(), escapeFolder); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveChangeOrigin(repo, "escape-change"); !hasChangeOriginCode(err, ChangeOriginCodeOutsideCanonicalDirectory) {
		t.Fatalf("symlink escape error = %v", err)
	}
}

func TestResolveChangeOriginIdentityAndEvidenceErrors(t *testing.T) {
	repo, changeFile, _ := committedOriginFixture(t, "identity-change", "20260711")
	if err := os.WriteFile(changeFile, []byte("---\nslug: other-change\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveChangeOrigin(repo, "identity-change"); !hasChangeOriginCode(err, ChangeOriginCodeIdentityMismatch) {
		t.Fatalf("identity mismatch error = %v", err)
	}
	if err := os.Remove(changeFile); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveChangeOrigin(repo, changeFile); !hasChangeOriginCode(err, ChangeOriginCodeNotFound) {
		t.Fatalf("removed Change error = %v", err)
	}

	noGit := t.TempDir()
	if _, err := ResolveChangeOrigin(noGit, "missing-change"); !hasChangeOriginCode(err, ChangeOriginCodeEvidenceUnavailable) {
		t.Fatalf("no git error = %v", err)
	}
	emptyGit := t.TempDir()
	if err := originGitCLI(emptyGit, "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	emptyFile := filepath.Join(emptyGit, "docs", "changes", "20260711-empty-change", "change.md")
	writeOriginChange(t, emptyFile, "empty-change")
	if _, err := ResolveChangeOrigin(emptyGit, "empty-change"); !hasChangeOriginCode(err, ChangeOriginCodeEvidenceUnavailable) {
		t.Fatalf("no HEAD error = %v", err)
	}
}

func TestResolveChangeOriginUsesCapturedHeadForBlobEvidence(t *testing.T) {
	repo, changeFile, contentA := committedOriginFixture(t, "captured-head", "20260711")
	headA := strings.TrimSpace(mustOriginGitOutput(t, repo, "rev-parse", "HEAD"))
	contentB := []byte("---\nslug: captured-head\n---\ncommit B bytes\n")
	origin, err := resolveChangeOriginWithOps(repo, "captured-head", changeOriginOps{
		afterHeadCapture: func() {
			if writeErr := os.WriteFile(changeFile, contentB, 0o644); writeErr != nil {
				t.Fatalf("write commit B Change: %v", writeErr)
			}
			if gitErr := originGitCLI(repo, "add", filepath.ToSlash(filepath.Join("docs", "changes", "20260711-captured-head", "change.md"))); gitErr != nil {
				t.Fatalf("stage commit B Change: %v", gitErr)
			}
			if gitErr := originGitCLI(repo, "-c", "commit.gpgsign=false", "commit", "-m", "commit B"); gitErr != nil {
				t.Fatalf("commit B: %v", gitErr)
			}
		},
	})
	if err != nil {
		t.Fatalf("resolve after advancing HEAD: %v", err)
	}
	if origin.Head != headA {
		t.Fatalf("captured Head = %q, want A %q", origin.Head, headA)
	}
	if !*origin.Dirty || *origin.Reconstructable {
		t.Fatalf("evidence against captured A = dirty %v/reconstructable %v", *origin.Dirty, *origin.Reconstructable)
	}
	digestA := sha256.Sum256(contentA)
	digestB := sha256.Sum256(contentB)
	if origin.ChangeSHA256 != hex.EncodeToString(digestB[:]) || origin.ChangeSHA256 == hex.EncodeToString(digestA[:]) {
		t.Fatalf("working digest = %q, want commit B bytes %q", origin.ChangeSHA256, hex.EncodeToString(digestB[:]))
	}
	if headB := strings.TrimSpace(mustOriginGitOutput(t, repo, "rev-parse", "HEAD")); headB == headA {
		t.Fatal("head-advance seam did not create commit B")
	}
}

func TestResolveChangeOriginRejectsSlugSymlinkAlias(t *testing.T) {
	repo, _, _ := committedOriginFixture(t, "canonical-b", "20260712")
	canonicalFolder := filepath.Join(repo, "docs", "changes", "20260712-canonical-b")
	aliasFolder := filepath.Join(repo, "docs", "changes", "20260711-alias-a")
	if err := os.Symlink(canonicalFolder, aliasFolder); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveChangeOrigin(repo, "alias-a"); !hasChangeOriginCode(err, ChangeOriginCodeIdentityMismatch) {
		t.Fatalf("slug alias error = %v, want %s", err, ChangeOriginCodeIdentityMismatch)
	}
	if _, err := ResolveChangeOrigin(repo, filepath.Join("docs", "changes", "20260712-canonical-b")); err != nil {
		t.Fatalf("explicit canonical target after alias = %v", err)
	}
}

func TestResolveChangeOriginRevalidatesOpenedInodeBeforeReading(t *testing.T) {
	repo, changeFile, _ := committedOriginFixture(t, "inode-race", "20260711")
	folder := filepath.Dir(changeFile)
	backup := folder + ".held-open"
	external := filepath.Join(t.TempDir(), "external-change")
	writeOriginChange(t, filepath.Join(external, "change.md"), "inode-race")
	var swapped bool
	origin, err := resolveChangeOriginWithOps(repo, "inode-race", changeOriginOps{
		afterOpenBeforeRevalidate: func() {
			if renameErr := os.Rename(folder, backup); renameErr != nil {
				t.Fatalf("move opened Change folder: %v", renameErr)
			}
			if symlinkErr := os.Symlink(external, folder); symlinkErr != nil {
				t.Fatalf("swap Change folder with external symlink: %v", symlinkErr)
			}
			swapped = true
		},
	})
	if swapped {
		if removeErr := os.Remove(folder); removeErr != nil {
			t.Fatalf("remove external symlink: %v", removeErr)
		}
		if renameErr := os.Rename(backup, folder); renameErr != nil {
			t.Fatalf("restore opened Change folder: %v", renameErr)
		}
	}
	if origin != (state.JournalOriginInput{}) {
		t.Fatalf("origin returned after path swap = %#v", origin)
	}
	if !hasChangeOriginCode(err, ChangeOriginCodeOutsideCanonicalDirectory) && !hasChangeOriginCode(err, ChangeOriginCodeEvidenceUnavailable) {
		t.Fatalf("path swap error = %v, want canonical rejection", err)
	}
}

func committedOriginFixture(t *testing.T, slug, date string) (string, string, []byte) {
	t.Helper()
	repo := t.TempDir()
	if err := originGitCLI(repo, "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	if err := originGitCLI(repo, "config", "user.name", "Loaf Test"); err != nil {
		t.Fatal(err)
	}
	if err := originGitCLI(repo, "config", "user.email", "loaf@example.test"); err != nil {
		t.Fatal(err)
	}
	changeFile := filepath.Join(repo, "docs", "changes", date+"-"+slug, "change.md")
	content := []byte("---\nslug: " + slug + "\n---\ncommitted bytes\n")
	if err := os.MkdirAll(filepath.Dir(changeFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changeFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := originGitCLI(repo, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := originGitCLI(repo, "-c", "commit.gpgsign=false", "commit", "-m", "initial"); err != nil {
		t.Fatal(err)
	}
	return repo, changeFile, content
}

func writeOriginChange(t *testing.T, path, slug string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nslug: "+slug+"\n---\nworking bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func originGitCLI(dir string, args ...string) error {
	cmd := originExecCommand(dir, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(output)))
	}
	return nil
}

func mustOriginGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := originGitOutput(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return output
}

func originExecCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}

func hasChangeOriginCode(err error, code string) bool {
	var typed *ChangeOriginError
	return errors.As(err, &typed) && typed.Code == code
}

func evalPath(t *testing.T, path string) string {
	t.Helper()
	evaluated, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return evaluated
}

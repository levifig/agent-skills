package state

import (
	"context"
	"strings"
	"testing"
)

func TestRecordReleasePersistsMembersAsFacts(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Landed work"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	recorded, err := store.RecordRelease(ctx, root, RecordReleaseOptions{
		Version:      "0.3.0",
		Tag:          "v0.3.0",
		TaggedCommit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Notes:        "## [0.3.0]\n",
		IssueIDs:     []string{issue.ID},
	})
	if err != nil {
		t.Fatalf("RecordRelease() error = %v", err)
	}
	if recorded.Version != "0.3.0" || recorded.Tag != "v0.3.0" || recorded.TaggedCommit != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("release = %#v", recorded)
	}
	if len(recorded.Members) != 1 || recorded.Members[0].Kind != ReleaseMemberKindIssue || recorded.Members[0].MemberID != issue.ID {
		t.Fatalf("members = %#v, want one issue member", recorded.Members)
	}

	loaded, err := store.GetRelease(ctx, root, "v0.3.0")
	if err != nil {
		t.Fatalf("GetRelease(tag) error = %v", err)
	}
	if loaded.ID != recorded.ID {
		t.Fatalf("GetRelease(tag) = %q, want %q", loaded.ID, recorded.ID)
	}
	byVersion, err := store.GetRelease(ctx, root, "0.3.0")
	if err != nil || byVersion.ID != recorded.ID {
		t.Fatalf("GetRelease(version) = %#v %v", byVersion, err)
	}
}

func TestRecordReleaseRejectsUnknownIssueMember(t *testing.T) {
	root, store := issueTestFixture(t)
	_, err := store.RecordRelease(context.Background(), root, RecordReleaseOptions{
		Version:      "0.3.1",
		Tag:          "v0.3.1",
		TaggedCommit: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		IssueIDs:     []string{"issue_missing"},
	})
	if err == nil || !strings.Contains(err.Error(), "issue_missing") {
		t.Fatalf("error = %v, want unknown issue", err)
	}
	listed, err := store.ListReleases(context.Background(), root)
	if err != nil {
		t.Fatalf("ListReleases() error = %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("failed write left %d releases", len(listed))
	}
}

func TestRecordReleaseIncludesPrereleaseByReference(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	pre, err := store.RecordRelease(ctx, root, RecordReleaseOptions{
		Version:      "0.3.0-alpha.1",
		Tag:          "v0.3.0-alpha.1",
		TaggedCommit: "cccccccccccccccccccccccccccccccccccccccc",
	})
	if err != nil {
		t.Fatalf("RecordRelease(prerelease) error = %v", err)
	}
	stable, err := store.RecordRelease(ctx, root, RecordReleaseOptions{
		Version:      "0.3.0",
		Tag:          "v0.3.0",
		TaggedCommit: "dddddddddddddddddddddddddddddddddddddddd",
		IncludedIDs:  []string{pre.ID},
	})
	if err != nil {
		t.Fatalf("RecordRelease(stable) error = %v", err)
	}
	if len(stable.Members) != 1 || stable.Members[0].Kind != ReleaseMemberKindRelease || stable.Members[0].MemberID != pre.ID {
		t.Fatalf("stable members = %#v, want prerelease reference", stable.Members)
	}
}

func TestRecordReleaseRejectsUnknownIncludedRelease(t *testing.T) {
	root, store := issueTestFixture(t)
	_, err := store.RecordRelease(context.Background(), root, RecordReleaseOptions{
		Version:      "1.0.0",
		Tag:          "v1.0.0",
		TaggedCommit: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		IncludedIDs:  []string{"rel_missing"},
	})
	if err == nil || !strings.Contains(err.Error(), "rel_missing") {
		t.Fatalf("error = %v, want unknown included release", err)
	}
}

func TestRecordReleaseRejectsDuplicateVersion(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	if _, err := store.RecordRelease(ctx, root, RecordReleaseOptions{
		Version:      "0.4.0",
		Tag:          "v0.4.0",
		TaggedCommit: "ffffffffffffffffffffffffffffffffffffffff",
	}); err != nil {
		t.Fatalf("first RecordRelease() error = %v", err)
	}
	_, err := store.RecordRelease(ctx, root, RecordReleaseOptions{
		Version:      "0.4.0",
		Tag:          "v0.4.1",
		TaggedCommit: "1111111111111111111111111111111111111111",
	})
	if err == nil {
		t.Fatal("duplicate version must fail")
	}
}

func TestRecordReleaseIdempotentOnIdenticalRetry(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Landed"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	options := RecordReleaseOptions{
		Version:      "0.5.0",
		Tag:          "v0.5.0",
		TaggedCommit: "2222222222222222222222222222222222222222",
		IssueIDs:     []string{issue.ID},
	}
	first, err := store.RecordRelease(ctx, root, options)
	if err != nil {
		t.Fatalf("first RecordRelease() error = %v", err)
	}
	second, err := store.RecordRelease(ctx, root, options)
	if err != nil {
		t.Fatalf("identical retry error = %v", err)
	}
	if second.ID != first.ID || second.TaggedCommit != first.TaggedCommit {
		t.Fatalf("retry = %#v, want %#v", second, first)
	}
	listed, err := store.ListReleases(ctx, root)
	if err != nil {
		t.Fatalf("ListReleases() error = %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("releases = %d, want 1 after identical retry", len(listed))
	}
}

func TestRecordReleaseRejectsSameVersionDifferentCommit(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	if _, err := store.RecordRelease(ctx, root, RecordReleaseOptions{
		Version:      "0.6.0",
		Tag:          "v0.6.0",
		TaggedCommit: "3333333333333333333333333333333333333333",
	}); err != nil {
		t.Fatalf("first RecordRelease() error = %v", err)
	}
	_, err := store.RecordRelease(ctx, root, RecordReleaseOptions{
		Version:      "0.6.0",
		Tag:          "v0.6.0",
		TaggedCommit: "4444444444444444444444444444444444444444",
	})
	if err == nil {
		t.Fatal("same version/tag with different commit must fail")
	}
}

func TestRecordReleaseIdempotentWhenMembersAndNotesMatch(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	firstIssue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "First"})
	if err != nil {
		t.Fatalf("CreateIssue(first) error = %v", err)
	}
	secondIssue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Second"})
	if err != nil {
		t.Fatalf("CreateIssue(second) error = %v", err)
	}
	pre, err := store.RecordRelease(ctx, root, RecordReleaseOptions{
		Version:      "0.7.0-alpha.1",
		Tag:          "v0.7.0-alpha.1",
		TaggedCommit: "5555555555555555555555555555555555555555",
	})
	if err != nil {
		t.Fatalf("RecordRelease(prerelease) error = %v", err)
	}
	options := RecordReleaseOptions{
		Version:      "0.7.0",
		Tag:          "v0.7.0",
		TaggedCommit: "6666666666666666666666666666666666666666",
		Notes:        "## [0.7.0]\nlanded work\n",
		IssueIDs:     []string{firstIssue.ID, secondIssue.ID},
		IncludedIDs:  []string{pre.ID},
	}
	first, err := store.RecordRelease(ctx, root, options)
	if err != nil {
		t.Fatalf("first RecordRelease() error = %v", err)
	}
	retry := options
	retry.IssueIDs = []string{secondIssue.ID, firstIssue.ID, firstIssue.ID, " "}
	retry.IncludedIDs = []string{pre.ID, pre.ID}
	second, err := store.RecordRelease(ctx, root, retry)
	if err != nil {
		t.Fatalf("identical member/notes retry error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("retry id = %q, want %q", second.ID, first.ID)
	}
	listed, err := store.ListReleases(ctx, root)
	if err != nil {
		t.Fatalf("ListReleases() error = %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("releases = %d, want 2 (prerelease + stable)", len(listed))
	}
}

func TestRecordReleaseRejectsDivergentContentOnRetry(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	firstIssue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "First"})
	if err != nil {
		t.Fatalf("CreateIssue(first) error = %v", err)
	}
	secondIssue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Second"})
	if err != nil {
		t.Fatalf("CreateIssue(second) error = %v", err)
	}
	pre, err := store.RecordRelease(ctx, root, RecordReleaseOptions{
		Version:      "0.8.0-alpha.1",
		Tag:          "v0.8.0-alpha.1",
		TaggedCommit: "7777777777777777777777777777777777777777",
	})
	if err != nil {
		t.Fatalf("RecordRelease(prerelease) error = %v", err)
	}
	other, err := store.RecordRelease(ctx, root, RecordReleaseOptions{
		Version:      "0.8.0-alpha.2",
		Tag:          "v0.8.0-alpha.2",
		TaggedCommit: "8888888888888888888888888888888888888888",
	})
	if err != nil {
		t.Fatalf("RecordRelease(other) error = %v", err)
	}
	base := RecordReleaseOptions{
		Version:      "0.8.0",
		Tag:          "v0.8.0",
		TaggedCommit: "9999999999999999999999999999999999999999",
		Notes:        "original notes",
		IssueIDs:     []string{firstIssue.ID},
		IncludedIDs:  []string{pre.ID},
	}
	if _, err := store.RecordRelease(ctx, root, base); err != nil {
		t.Fatalf("first RecordRelease() error = %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*RecordReleaseOptions)
		wantErr string
	}{
		{
			name: "issue members",
			mutate: func(options *RecordReleaseOptions) {
				options.IssueIDs = []string{firstIssue.ID, secondIssue.ID}
			},
			wantErr: "issue members",
		},
		{
			name: "included members",
			mutate: func(options *RecordReleaseOptions) {
				options.IncludedIDs = []string{other.ID}
			},
			wantErr: "included members",
		},
		{
			name: "notes",
			mutate: func(options *RecordReleaseOptions) {
				options.Notes = "changed notes"
			},
			wantErr: "notes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			retry := base
			tc.mutate(&retry)
			_, err := store.RecordRelease(ctx, root, retry)
			if err == nil || !strings.Contains(err.Error(), "divergent") || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want divergent %s", err, tc.wantErr)
			}
			listed, err := store.ListReleases(ctx, root)
			if err != nil {
				t.Fatalf("ListReleases() error = %v", err)
			}
			if len(listed) != 3 {
				t.Fatalf("divergent retry wrote a release; count = %d", len(listed))
			}
		})
	}
}

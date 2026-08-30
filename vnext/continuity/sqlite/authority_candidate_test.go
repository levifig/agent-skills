package sqlite

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestSyncAuthorityCandidateStagesReplaysReopensReadiesAndDiscards(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "authority-candidate-lifecycle")
	store, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	projectID := continuity.ProjectID("project-authority-candidate-lifecycle")
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(5)
	environments := syncAuthorityCandidateTestEnvironmentsV2(5)
	firstPage := syncAuthorityCandidatePageV2("", environments[:2], true)

	first, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, firstPage)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(first) error = %v", err)
	}
	if first.ProjectID != projectID || first.CandidateID == ([32]byte{}) || first.Snapshot != snapshot ||
		first.PageCount != 1 || first.EnvironmentCount != 2 || first.ThroughEnvironmentID != environments[1].EnvironmentID ||
		first.RollingEnvironmentDigest == ([32]byte{}) || first.Ready || first.AuthorityDigestVersion != 2 ||
		first.AuthorityDigest != ([32]byte{}) {
		t.Fatalf("first candidate = %#v", first)
	}
	current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
	if err != nil || !found || current != first {
		t.Fatalf("CurrentSyncAuthorityCandidate() = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, first)
	}
	replayed, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, firstPage)
	if err != nil || replayed != first {
		t.Fatalf("exact first replay = (%#v, %v), want (%#v, nil)", replayed, err, first)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	store, err = Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	current, found, err = store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
	if err != nil || !found || current != first {
		t.Fatalf("CurrentSyncAuthorityCandidate(reopen) = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, first)
	}

	secondPage := syncAuthorityCandidatePageV2(first.ThroughEnvironmentID, environments[2:4], true)
	second, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, secondPage)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(second) error = %v", err)
	}
	if second.PageCount != 2 || second.EnvironmentCount != 4 || second.ThroughEnvironmentID != environments[3].EnvironmentID || second.Ready {
		t.Fatalf("second candidate = %#v", second)
	}
	finalPage := syncAuthorityCandidatePageV2(second.ThroughEnvironmentID, environments[4:], false)
	ready, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, finalPage)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(final) error = %v", err)
	}
	if !ready.Ready || ready.PageCount != 3 || ready.EnvironmentCount != 5 || ready.AuthorityDigestVersion != 2 || ready.AuthorityDigest == ([32]byte{}) {
		t.Fatalf("ready candidate = %#v", ready)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(ready) error = %v", err)
	}
	store, err = Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(ready replay) error = %v", err)
	}
	for name, page := range map[string]SyncAuthorityPage{"first": firstPage, "second": secondPage, "final": finalPage} {
		replayed, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
		if err != nil || replayed != ready {
			t.Fatalf("%s exact replay after ready = (%#v, %v), want (%#v, nil)", name, replayed, err, ready)
		}
	}

	altered := finalPage
	altered.Environments = cloneSyncAuthorityCandidateEnvironmentsV2(finalPage.Environments)
	altered.Environments[0].CertificateBytes[0] ^= 0xff
	if _, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, altered); err == nil {
		t.Fatal("altered retry after ready error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	current, found, err = store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
	if err != nil || !found || current != ready {
		t.Fatalf("candidate changed after altered retry: (%#v, %v, %v)", current, found, err)
	}

	if _, err := store.CurrentSyncAuthority(context.Background(), projectID); err == nil {
		t.Fatal("candidate staging installed canonical authority")
	} else {
		assertSyncErrorCode(t, err, SyncErrorNotFound)
	}
	if err := store.DiscardSyncAuthorityCandidate(context.Background(), projectID, first.Checkpoint()); err == nil {
		t.Fatal("discard with stale checkpoint error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	if err := store.DiscardSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("DiscardSyncAuthorityCandidate() error = %v", err)
	}
	if _, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID); err != nil || found {
		t.Fatalf("CurrentSyncAuthorityCandidate(after discard) = (_, %v, %v), want (_, false, nil)", found, err)
	}
	if err := store.DiscardSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("DiscardSyncAuthorityCandidate(retry) error = %v", err)
	}
}

func TestSyncAuthorityCandidateStreamsThreeHundredEnvironments(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-300")
	projectID := continuity.ProjectID("project-authority-candidate-300")
	const environmentCount = 300
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(environmentCount)
	environments := syncAuthorityCandidateManyEnvironmentsV2(environmentCount)
	after := ""
	var candidate SyncAuthorityCandidate
	for offset := 0; offset < len(environments); offset += maximumSyncAuthorityCandidatePageEnvironments {
		end := offset + maximumSyncAuthorityCandidatePageEnvironments
		if end > len(environments) {
			end = len(environments)
		}
		page := syncAuthorityCandidatePageV2(after, environments[offset:end], end < len(environments))
		var err error
		candidate, err = store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
		if err != nil {
			t.Fatalf("stage environments %d:%d error = %v", offset, end, err)
		}
		after = page.ThroughEnvironmentID
	}
	if !candidate.Ready || candidate.EnvironmentCount != environmentCount || candidate.PageCount != 75 || candidate.AuthorityDigest == ([32]byte{}) {
		t.Fatalf("300-environment candidate = %#v", candidate)
	}
	var pages, persistedEnvironments, membershipEvents int
	if err := store.db.QueryRow(`
SELECT
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_pages WHERE project_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_environments WHERE project_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_membership_events WHERE project_id = ?)`,
		string(projectID), string(projectID), string(projectID),
	).Scan(&pages, &persistedEnvironments, &membershipEvents); err != nil {
		t.Fatalf("count streamed candidate rows: %v", err)
	}
	if pages != 75 || persistedEnvironments != environmentCount || membershipEvents != environmentCount {
		t.Fatalf("streamed row counts = (%d, %d, %d), want (75, 300, 300)", pages, persistedEnvironments, membershipEvents)
	}
}

func TestSyncAuthorityCandidateSubsequentPageQueriesUseBoundedIndexes(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-query-plans")
	projectID := continuity.ProjectID("project-authority-candidate-query-plans")
	candidateID := sha256.Sum256([]byte("authority-candidate-query-plan"))

	tests := []struct {
		name  string
		query string
		args  []any
		want  []string
	}{
		{
			name:  "candidate first cursor",
			query: syncAuthorityCandidateFirstPageByCursorQueryV2,
			args:  []any{string(projectID), candidateID[:]},
			want: []string{
				"SEARCH continuity_sync_authority_candidate_pages USING INDEX sqlite_autoindex_continuity_sync_authority_candidate_pages_2 (project_id=? AND candidate_id=? AND after_environment_id=?)",
			},
		},
		{
			name:  "candidate cursor equality",
			query: syncAuthorityCandidateSubsequentPageByCursorQueryV2,
			args:  []any{string(projectID), candidateID[:], "environment:0150"},
			want: []string{
				"SEARCH continuity_sync_authority_candidate_pages USING INDEX sqlite_autoindex_continuity_sync_authority_candidate_pages_2 (project_id=? AND candidate_id=? AND after_environment_id=?)",
			},
		},
		{
			name:  "canonical first bounded range",
			query: canonicalSyncAuthorityFirstPageRangeQueryV2,
			args:  []any{string(projectID), "environment:0004", maximumSyncAuthorityCandidateBoundedReadRowsV2},
			want: []string{
				"SEARCH continuity_sync_environment_certificates USING PRIMARY KEY (project_id=? AND environment_id<?)",
			},
		},
		{
			name:  "canonical bounded range",
			query: canonicalSyncAuthoritySubsequentPageRangeQueryV2,
			args:  []any{string(projectID), "environment:0150", "environment:0154", maximumSyncAuthorityCandidateBoundedReadRowsV2},
			want: []string{
				"SEARCH continuity_sync_environment_certificates USING PRIMARY KEY (project_id=? AND environment_id>? AND environment_id<?)",
			},
		},
		{
			name:  "canonical first final range",
			query: canonicalSyncAuthorityFirstFinalPageRangeQueryV2,
			args:  []any{string(projectID), maximumSyncAuthorityCandidateBoundedReadRowsV2},
			want: []string{
				"SEARCH continuity_sync_environment_certificates USING PRIMARY KEY (project_id=?)",
			},
		},
		{
			name:  "canonical final suffix",
			query: canonicalSyncAuthoritySubsequentFinalPageRangeQueryV2,
			args:  []any{string(projectID), "environment:0150", maximumSyncAuthorityCandidateBoundedReadRowsV2},
			want: []string{
				"SEARCH continuity_sync_environment_certificates USING PRIMARY KEY (project_id=? AND environment_id>?)",
			},
		},
		{
			name:  "candidate ordinal interval",
			query: syncAuthorityCandidatePageEnvironmentsByOrdinalRangeQueryV2,
			args:  []any{string(projectID), candidateID[:], 150, 154, maximumSyncAuthorityCandidateBoundedReadRowsV2},
			want: []string{
				"SEARCH continuity_sync_authority_candidate_environments USING INDEX sqlite_autoindex_continuity_sync_authority_candidate_environments_2 (project_id=? AND candidate_id=? AND environment_ordinal>? AND environment_ordinal<?)",
			},
		},
		{
			name:  "membership event equality",
			query: syncAuthorityCandidateMembershipEventsByEnvironmentQueryV2,
			args:  []any{string(projectID), candidateID[:], "environment:0150", "join"},
			want: []string{
				"SEARCH continuity_sync_authority_candidate_membership_events USING INDEX sqlite_autoindex_continuity_sync_authority_candidate_membership_events_2 (project_id=? AND candidate_id=? AND environment_id=? AND event_kind=?)",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := syncAuthorityCandidateQueryPlanV2(t, store, test.query, test.args...)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("query plan = %#v, want %#v", got, test.want)
			}
		})
	}
	for name, query := range map[string]string{
		"first":             canonicalSyncAuthorityFirstPageRangeQueryV2,
		"subsequent":        canonicalSyncAuthoritySubsequentPageRangeQueryV2,
		"first final":       canonicalSyncAuthorityFirstFinalPageRangeQueryV2,
		"subsequent final":  canonicalSyncAuthoritySubsequentFinalPageRangeQueryV2,
		"candidate ordinal": syncAuthorityCandidatePageEnvironmentsByOrdinalRangeQueryV2,
	} {
		if !strings.Contains(query, "LIMIT ?") {
			t.Errorf("%s canonical authority range query has no fixed row limit", name)
		}
	}
}

func TestSyncAuthorityCandidateCanonicalRangeUsesOneOmissionSentinel(t *testing.T) {
	tests := []struct {
		name           string
		canonicalCount int
		candidateCount int
		wantConflict   bool
	}{
		{name: "exact four", canonicalCount: 4, candidateCount: 4},
		{name: "omitted fifth", canonicalCount: 5, candidateCount: 4, wantConflict: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "authority-candidate-canonical-sentinel-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-authority-candidate-canonical-sentinel-" + syncSlug(test.name))
			environments := syncAuthorityCandidateManyEnvironmentsV2(test.canonicalCount)
			authority := SyncAuthority{
				ChannelID:            testSyncChannelID("authority-candidate-sentinel-channel"),
				RelayGeneration:      sha256.Sum256([]byte("authority-candidate-sentinel-relay")),
				AdminPublicKey:       sha256.Sum256([]byte("authority-candidate-sentinel-admin")),
				MembershipGeneration: uint32(test.canonicalCount),
				Environments:         environments,
			}
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
				t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
			}
			baseDigest, err := frozenSyncAuthorityDigestV1(projectID, authority)
			if err != nil {
				t.Fatalf("frozenSyncAuthorityDigestV1() error = %v", err)
			}
			snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, 1, baseDigest)
			candidate, err := store.StageVerifiedSyncAuthorityCandidatePage(
				context.Background(), projectID, snapshot,
				syncAuthorityCandidatePageV2("", environments[:test.candidateCount], false),
			)
			if test.wantConflict {
				assertSyncAuthorityCandidateConflictV2(t, err)
				if candidate != (SyncAuthorityCandidate{}) {
					t.Fatalf("omitted canonical environment returned candidate %#v", candidate)
				}
				return
			}
			if err != nil || !candidate.Ready || candidate.EnvironmentCount != int64(test.candidateCount) {
				t.Fatalf("exact canonical range = (%#v, %v)", candidate, err)
			}
		})
	}
}

func TestSyncAuthorityCandidateRejectsInvalidPagesWithoutMutation(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-invalid-pages")
	projectID := continuity.ProjectID("project-authority-candidate-invalid-pages")
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(5)
	environments := syncAuthorityCandidateTestEnvironmentsV2(5)
	protectedBefore := syncAuthorityCandidateProtectedCountsV2(t, store)

	tests := []struct {
		name   string
		page   SyncAuthorityPage
		mutate func(*SyncAuthoritySnapshot)
	}{
		{name: "empty", page: SyncAuthorityPage{}},
		{name: "too many", page: syncAuthorityCandidatePageV2("", environments, true)},
		{name: "first with after", page: syncAuthorityCandidatePageV2("not-empty", environments[:1], true)},
		{name: "wrong through", page: SyncAuthorityPage{ThroughEnvironmentID: "environment:z", Environments: environments[:1], More: true}},
		{name: "unsorted", page: syncAuthorityCandidatePageV2("", []SyncEnvironmentCertificate{environments[1], environments[0]}, true)},
		{name: "duplicate certificate", page: func() SyncAuthorityPage {
			value := cloneSyncAuthorityCandidateEnvironmentsV2(environments[:2])
			value[1].CertificateID = value[0].CertificateID
			return syncAuthorityCandidatePageV2("", value, true)
		}()},
		{name: "membership above header", page: func() SyncAuthorityPage {
			value := cloneSyncAuthorityCandidateEnvironmentsV2(environments[:1])
			value[0].JoinMembershipGeneration = 6
			return syncAuthorityCandidatePageV2("", value, true)
		}()},
		{name: "bad retirement relay", page: func() SyncAuthorityPage {
			value := cloneSyncAuthorityCandidateEnvironmentsV2(environments[:1])
			value[0].Retirement = &SyncEnvironmentRetirement{RelayGeneration: sha256.Sum256([]byte("other")), MembershipGeneration: 2, RetirementID: sha256.Sum256([]byte("retire")), RetirementBytes: []byte("retirement")}
			return syncAuthorityCandidatePageV2("", value, true)
		}()},
		{name: "malformed base", page: syncAuthorityCandidatePageV2("", environments[:1], true), mutate: func(value *SyncAuthoritySnapshot) { value.BaseAuthorityDigestVersion = 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateSnapshot := snapshot
			if test.mutate != nil {
				test.mutate(&candidateSnapshot)
			}
			if _, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, candidateSnapshot, test.page); err == nil {
				t.Fatal("invalid page error = nil")
			}
			if _, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID); err != nil || found {
				t.Fatalf("invalid page left candidate = (_, %v, %v)", found, err)
			}
			if protectedAfter := syncAuthorityCandidateProtectedCountsV2(t, store); !reflect.DeepEqual(protectedAfter, protectedBefore) {
				t.Fatalf("invalid page changed protected tables: got %#v, want %#v", protectedAfter, protectedBefore)
			}
		})
	}
}

func TestSyncAuthorityCandidateBindsAndPreservesCanonicalBase(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SyncAuthoritySnapshot, *[]SyncEnvironmentCertificate)
		wantOK bool
	}{
		{name: "append environment", wantOK: true},
		{name: "wrong base digest", mutate: func(snapshot *SyncAuthoritySnapshot, _ *[]SyncEnvironmentCertificate) {
			snapshot.BaseAuthorityDigest[0] ^= 0xff
		}},
		{name: "missing base", mutate: func(snapshot *SyncAuthoritySnapshot, _ *[]SyncEnvironmentCertificate) {
			snapshot.BaseAuthorityDigestVersion = 0
			snapshot.BaseAuthorityDigest = [32]byte{}
		}},
		{name: "changed certificate", mutate: func(_ *SyncAuthoritySnapshot, environments *[]SyncEnvironmentCertificate) {
			(*environments)[0].CertificateBytes[0] ^= 0xff
		}},
		{name: "omitted canonical environment", mutate: func(_ *SyncAuthoritySnapshot, environments *[]SyncEnvironmentCertificate) {
			*environments = (*environments)[1:]
		}},
		{name: "new environment at old membership", mutate: func(_ *SyncAuthoritySnapshot, environments *[]SyncEnvironmentCertificate) {
			(*environments)[len(*environments)-1].JoinMembershipGeneration = 3
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "authority-candidate-base-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-authority-candidate-base-" + syncSlug(test.name))
			canonical := testSyncAuthority()
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, canonical); err != nil {
				t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
			}
			baseDigest, err := frozenSyncAuthorityDigestV1(projectID, canonical)
			if err != nil {
				t.Fatalf("frozenSyncAuthorityDigestV1() error = %v", err)
			}
			snapshot := syncAuthoritySnapshotFromAuthorityV2(canonical, 1, baseDigest)
			snapshot.MembershipGeneration = 4
			snapshot.InventoryArrivalHead = 1
			environments := cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments)
			environments = append(environments, SyncEnvironmentCertificate{
				EnvironmentID:            "environment-c",
				CertificateID:            sha256.Sum256([]byte("candidate-base-environment-c")),
				CertificateBytes:         []byte("candidate base environment c"),
				Mode:                     SyncEnvironmentTrusted,
				JoinMembershipGeneration: 4,
			})
			if test.mutate != nil {
				test.mutate(&snapshot, &environments)
			}
			before, err := store.CurrentSyncAuthority(context.Background(), projectID)
			if err != nil {
				t.Fatalf("CurrentSyncAuthority(before) error = %v", err)
			}
			_, stageErr := store.StageVerifiedSyncAuthorityCandidatePage(
				context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", environments, false),
			)
			if test.wantOK {
				if stageErr != nil {
					t.Fatalf("StageVerifiedSyncAuthorityCandidatePage() error = %v", stageErr)
				}
			} else if stageErr == nil {
				t.Fatal("StageVerifiedSyncAuthorityCandidatePage() error = nil, want refusal")
			}
			after, err := store.CurrentSyncAuthority(context.Background(), projectID)
			if err != nil {
				t.Fatalf("CurrentSyncAuthority(after) error = %v", err)
			}
			if !syncAuthorityEqual(before, after) {
				t.Fatalf("canonical authority changed: got %#v, want %#v", after, before)
			}
			if !test.wantOK {
				if _, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID); err != nil || found {
					t.Fatalf("refused candidate persisted = (_, %v, %v)", found, err)
				}
			}
		})
	}
}

func TestSyncAuthorityCandidateAllowsOnlyExactAuthorityAtSameHead(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-same-head")
	projectID := continuity.ProjectID("project-authority-candidate-same-head")
	canonical := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, canonical); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	baseDigest, err := frozenSyncAuthorityDigestV1(projectID, canonical)
	if err != nil {
		t.Fatalf("frozenSyncAuthorityDigestV1() error = %v", err)
	}
	snapshot := syncAuthoritySnapshotFromAuthorityV2(canonical, 1, baseDigest)
	ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", canonical.Environments, false),
	)
	if err != nil || !ready.Ready {
		t.Fatalf("exact same-head candidate = (%#v, %v)", ready, err)
	}
	if err := store.DiscardSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("discard exact same-head candidate: %v", err)
	}

	changed := cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments)
	changed[0].CertificateBytes[0] ^= 0xff
	if _, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", changed, false),
	); err == nil {
		t.Fatal("changed same-head candidate error = nil")
	}
}

func TestSyncAuthorityCandidateResumesMultiPageCanonicalBaseAfterReopen(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "authority-candidate-canonical-resume")
	store, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	projectID := continuity.ProjectID("project-authority-candidate-canonical-resume")
	canonicalEnvironments := syncAuthorityCandidateManyEnvironmentsV2(6)
	canonical := SyncAuthority{
		ChannelID:            testSyncChannelID("authority-candidate-channel"),
		RelayGeneration:      sha256.Sum256([]byte("authority-candidate-relay")),
		AdminPublicKey:       sha256.Sum256([]byte("authority-candidate-admin")),
		MembershipGeneration: 6,
		Environments:         canonicalEnvironments,
	}
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, canonical); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	baseDigest, err := frozenSyncAuthorityDigestV1(projectID, canonical)
	if err != nil {
		t.Fatalf("frozenSyncAuthorityDigestV1() error = %v", err)
	}
	snapshot := syncAuthoritySnapshotFromAuthorityV2(canonical, 1, baseDigest)
	snapshot.MembershipGeneration = 7
	snapshot.InventoryArrivalHead = 1
	target := cloneSyncAuthorityCandidateEnvironmentsV2(canonicalEnvironments)
	target = append(target, SyncEnvironmentCertificate{
		EnvironmentID:            "environment:0007",
		CertificateID:            sha256.Sum256([]byte("candidate-certificate:environment:0007")),
		CertificateBytes:         []byte("candidate certificate bytes for environment:0007"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: 7,
	})
	firstPage := syncAuthorityCandidatePageV2("", target[:4], true)
	first, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, firstPage)
	if err != nil {
		t.Fatalf("stage first canonical page: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	store, err = Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2(first.ThroughEnvironmentID, target[4:], false),
	)
	if err != nil || !ready.Ready || ready.EnvironmentCount != 7 || ready.PageCount != 2 {
		t.Fatalf("resume canonical candidate = (%#v, %v)", ready, err)
	}
}

func TestCurrentAndDiscardSyncAuthorityCandidateRecoverAfterCanonicalAdvance(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "authority-candidate-stale-base-recovery")
	store, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	projectID := continuity.ProjectID("project-authority-candidate-stale-base-recovery")
	canonical := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, canonical); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(initial) error = %v", err)
	}
	baseDigest, err := frozenSyncAuthorityDigestV1(projectID, canonical)
	if err != nil {
		t.Fatalf("frozenSyncAuthorityDigestV1() error = %v", err)
	}
	snapshot := syncAuthoritySnapshotFromAuthorityV2(canonical, 1, baseDigest)
	snapshot.MembershipGeneration = 4
	snapshot.InventoryArrivalHead = 1
	target := cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments)
	target = append(target, SyncEnvironmentCertificate{
		EnvironmentID:            "environment-c",
		CertificateID:            sha256.Sum256([]byte("stale-base-environment-c")),
		CertificateBytes:         []byte("stale base environment c"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: 4,
	})
	firstPage := syncAuthorityCandidatePageV2("", target[:1], true)
	first, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, firstPage)
	if err != nil {
		t.Fatalf("stage partial candidate: %v", err)
	}
	advancedCanonical := cloneSyncAuthority(canonical)
	advancedCanonical.MembershipGeneration = 4
	advancedCanonical.Environments = target
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, advancedCanonical); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(advance) error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	store, err = Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
	if err != nil || !found || current != first {
		t.Fatalf("CurrentSyncAuthorityCandidate(stale base) = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, first)
	}
	if _, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, firstPage); err == nil {
		t.Fatal("exact replay against stale base error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	if _, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2(first.ThroughEnvironmentID, target[1:], false),
	); err == nil {
		t.Fatal("resume against stale base error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	if err := store.DiscardSyncAuthorityCandidate(context.Background(), projectID, first.Checkpoint()); err != nil {
		t.Fatalf("DiscardSyncAuthorityCandidate(stale base) error = %v", err)
	}
	persistedCanonical, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil || !syncAuthorityEqual(persistedCanonical, advancedCanonical) {
		t.Fatalf("canonical after stale discard = (%#v, %v), want %#v", persistedCanonical, err, advancedCanonical)
	}
}

func TestSyncAuthorityCandidateFinalPageProofFailuresRollBack(t *testing.T) {
	tests := []struct {
		name       string
		membership int
		first      []SyncEnvironmentCertificate
		final      []SyncEnvironmentCertificate
	}{
		{
			name:       "membership gap",
			membership: 4,
			first:      syncAuthorityCandidateManyEnvironmentsV2(2),
			final:      syncAuthorityCandidateManyEnvironmentsV2(3)[2:],
		},
		{
			name:       "duplicate certificate across pages",
			membership: 3,
			first:      syncAuthorityCandidateManyEnvironmentsV2(2),
			final: func() []SyncEnvironmentCertificate {
				value := syncAuthorityCandidateManyEnvironmentsV2(3)[2:]
				value[0].CertificateID = syncAuthorityCandidateManyEnvironmentsV2(1)[0].CertificateID
				return value
			}(),
		},
		{
			name:       "duplicate membership across pages",
			membership: 3,
			first:      syncAuthorityCandidateManyEnvironmentsV2(2),
			final: func() []SyncEnvironmentCertificate {
				value := syncAuthorityCandidateManyEnvironmentsV2(3)[2:]
				value[0].JoinMembershipGeneration = 2
				return value
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "authority-candidate-final-proof-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-authority-candidate-final-proof-" + syncSlug(test.name))
			snapshot := syncAuthorityCandidateBootstrapSnapshotV2(test.membership)
			firstPage := syncAuthorityCandidatePageV2("", test.first, true)
			first, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, firstPage)
			if err != nil {
				t.Fatalf("stage first page: %v", err)
			}
			protectedBefore := syncAuthorityCandidateProtectedCountsV2(t, store)
			if _, err := store.StageVerifiedSyncAuthorityCandidatePage(
				context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2(first.ThroughEnvironmentID, test.final, false),
			); err == nil {
				t.Fatal("invalid final page error = nil")
			}
			current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
			if err != nil || !found || current != first {
				t.Fatalf("failed final page changed candidate = (%#v, %v, %v), want %#v", current, found, err, first)
			}
			if protectedAfter := syncAuthorityCandidateProtectedCountsV2(t, store); !reflect.DeepEqual(protectedAfter, protectedBefore) {
				t.Fatalf("failed final page changed protected tables: got %#v, want %#v", protectedAfter, protectedBefore)
			}
		})
	}
}

func TestSyncAuthorityCandidateFullAuditsFailClosedOnExtraUnjoinedRows(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *Store, continuity.ProjectID, SyncAuthorityCandidate)
	}{
		{
			name: "childless page",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, candidate SyncAuthorityCandidate) {
				t.Helper()
				pageDigest := sha256.Sum256([]byte("extra childless page digest"))
				through := candidate.ThroughEnvironmentID + ":extra"
				if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_pages(
  project_id, candidate_id, page_number, after_environment_id,
  through_environment_id, environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
) VALUES(?, ?, ?, ?, ?, 1, 1, ?, ?, ?)`,
					string(projectID), candidate.CandidateID[:], candidate.PageCount+1,
					candidate.ThroughEnvironmentID, through, pageDigest[:],
					candidate.EnvironmentCount+1, candidate.RollingEnvironmentDigest[:],
				); err != nil {
					t.Fatalf("insert extra childless page: %v", err)
				}
			},
		},
		{
			name: "orphan environment",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, candidate SyncAuthorityCandidate) {
				t.Helper()
				if _, err := store.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
					t.Fatalf("disable foreign keys for corruption fixture: %v", err)
				}
				certificateID := sha256.Sum256([]byte("orphan candidate environment"))
				if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_environments(
  project_id, candidate_id, environment_id, environment_ordinal, page_number,
  certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation
) VALUES(?, ?, 'environment:orphan', ?, ?, ?, X'01', 'trusted', 0, 1)`,
					string(projectID), candidate.CandidateID[:], candidate.EnvironmentCount+1,
					candidate.PageCount+1, certificateID[:],
				); err != nil {
					t.Fatalf("insert orphan candidate environment: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "authority-candidate-extra-row-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-authority-candidate-extra-row-" + syncSlug(test.name))
			snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
			page := syncAuthorityCandidatePageV2("", syncAuthorityCandidateManyEnvironmentsV2(1), false)
			ready, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
			if err != nil {
				t.Fatalf("stage ready fixture: %v", err)
			}
			test.mutate(t, store, projectID, ready)
			if _, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID); err == nil || found {
				t.Fatalf("CurrentSyncAuthorityCandidate(extra row) = (_, %v, %v), want fail closed", found, err)
			} else {
				assertSyncErrorCode(t, err, SyncErrorStore)
			}
			replayed, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
			if err != nil || replayed != ready {
				t.Fatalf("bounded ready replay with unrelated extra row = (%#v, %v), want (%#v, nil)", replayed, err, ready)
			}
			if err := store.DiscardSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
				t.Fatal("discard with extra row error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorStore)
			}
		})
	}
}

func TestSyncAuthorityCandidatePublicValidation(t *testing.T) {
	projectID := continuity.ProjectID("project-authority-candidate-validation")
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
	page := syncAuthorityCandidatePageV2("", syncAuthorityCandidateManyEnvironmentsV2(1), false)
	var nilStore *Store
	if _, err := nilStore.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page); err == nil {
		t.Fatal("StageVerifiedSyncAuthorityCandidatePage(nil store) error = nil")
	}
	if _, _, err := nilStore.CurrentSyncAuthorityCandidate(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncAuthorityCandidate(nil store) error = nil")
	}
	if err := nilStore.DiscardSyncAuthorityCandidate(context.Background(), projectID, SyncAuthorityCandidateCheckpoint{}); err == nil {
		t.Fatal("DiscardSyncAuthorityCandidate(invalid checkpoint) error = nil")
	}
	store := openSyncStore(t, "authority-candidate-public-validation")
	if _, err := store.StageVerifiedSyncAuthorityCandidatePage(nil, projectID, snapshot, page); err == nil {
		t.Fatal("StageVerifiedSyncAuthorityCandidatePage(nil context) error = nil")
	}
	if _, _, err := store.CurrentSyncAuthorityCandidate(nil, projectID); err == nil {
		t.Fatal("CurrentSyncAuthorityCandidate(nil context) error = nil")
	}
	invalidCheckpoints := []SyncAuthorityCandidateCheckpoint{
		{},
		{CandidateID: sha256.Sum256([]byte("candidate")), PageCount: 1, EnvironmentCount: 1, ThroughEnvironmentID: "environment:1", RollingEnvironmentDigest: sha256.Sum256([]byte("rolling")), Ready: true},
		{CandidateID: sha256.Sum256([]byte("candidate")), PageCount: 1, EnvironmentCount: 1, ThroughEnvironmentID: "environment:1", RollingEnvironmentDigest: sha256.Sum256([]byte("rolling")), AuthorityDigest: sha256.Sum256([]byte("authority"))},
	}
	for _, checkpoint := range invalidCheckpoints {
		if err := store.DiscardSyncAuthorityCandidate(context.Background(), projectID, checkpoint); err == nil {
			t.Fatalf("DiscardSyncAuthorityCandidate(%#v) error = nil", checkpoint)
		}
	}
}

func TestSyncAuthorityCandidateRetirementIsAppendOnlyAfterBaseMembership(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-retirement")
	projectID := continuity.ProjectID("project-authority-candidate-retirement")
	canonical := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, canonical); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	baseDigest, err := frozenSyncAuthorityDigestV1(projectID, canonical)
	if err != nil {
		t.Fatalf("frozenSyncAuthorityDigestV1() error = %v", err)
	}
	snapshot := syncAuthoritySnapshotFromAuthorityV2(canonical, 1, baseDigest)
	snapshot.MembershipGeneration = 4
	snapshot.InventoryArrivalHead = 1
	environments := cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments)
	environments[1].Retirement = &SyncEnvironmentRetirement{
		RelayGeneration:          canonical.RelayGeneration,
		MembershipGeneration:     4,
		FinalEnvironmentSequence: 8,
		FinalEnvelopeDigest:      sha256.Sum256([]byte("candidate-retirement-final")),
		RetirementID:             sha256.Sum256([]byte("candidate-retirement-id")),
		RetirementBytes:          []byte("candidate retirement bytes"),
	}
	ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil || !ready.Ready {
		t.Fatalf("append retirement candidate = (%#v, %v)", ready, err)
	}
	if err := store.DiscardSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("discard appended retirement: %v", err)
	}

	changed := cloneSyncAuthorityCandidateEnvironmentsV2(environments)
	changed[0].Retirement.RetirementBytes[0] ^= 0xff
	if _, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", changed, false),
	); err == nil {
		t.Fatal("changed canonical retirement error = nil")
	}
}

func TestSyncAuthorityCandidateRejectsInventoryHeadRegressionFromV2Base(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-head-regression")
	projectID := continuity.ProjectID("project-authority-candidate-head-regression")
	canonical := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, canonical); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	canonical.InventoryArrivalHead = 5
	baseDigest := syncAuthorityCandidateAuthorityDigestForTestV2(t, projectID, canonical)
	if _, err := store.db.Exec(`
UPDATE continuity_sync_authorities
SET digest_version = 2, authority_digest = ?, inventory_arrival_head = 5
WHERE project_id = ?`, baseDigest[:], string(projectID)); err != nil {
		t.Fatalf("seed v2 canonical metadata: %v", err)
	}
	snapshot := syncAuthoritySnapshotFromAuthorityV2(canonical, 2, baseDigest)
	snapshot.InventoryArrivalHead = 4
	if _, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", canonical.Environments, false),
	); err == nil {
		t.Fatal("inventory head regression error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
}

func TestSyncAuthorityCandidateReadersFailClosedOnPersistedCorruption(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *Store, continuity.ProjectID)
	}{
		{
			name: "header",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				t.Helper()
				value := sha256.Sum256([]byte("corrupt candidate admin"))
				if _, err := store.db.Exec(`UPDATE continuity_sync_authority_candidates SET admin_public_key = ? WHERE project_id = ?`, value[:], string(projectID)); err != nil {
					t.Fatalf("corrupt candidate header: %v", err)
				}
			},
		},
		{
			name: "page",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				t.Helper()
				value := sha256.Sum256([]byte("corrupt page digest"))
				if _, err := store.db.Exec(`UPDATE continuity_sync_authority_candidate_pages SET page_digest = ? WHERE project_id = ?`, value[:], string(projectID)); err != nil {
					t.Fatalf("corrupt candidate page: %v", err)
				}
			},
		},
		{
			name: "environment",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				t.Helper()
				if _, err := store.db.Exec(`UPDATE continuity_sync_authority_candidate_environments SET certificate_bytes = X'01' WHERE project_id = ?`, string(projectID)); err != nil {
					t.Fatalf("corrupt candidate environment: %v", err)
				}
			},
		},
		{
			name: "membership event",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				t.Helper()
				if _, err := store.db.Exec(`UPDATE continuity_sync_authority_candidate_membership_events SET event_kind = 'retirement' WHERE project_id = ? AND membership_generation = 1`, string(projectID)); err != nil {
					t.Fatalf("corrupt candidate membership event: %v", err)
				}
			},
		},
		{
			name: "missing membership event",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				t.Helper()
				if _, err := store.db.Exec(`DELETE FROM continuity_sync_authority_candidate_membership_events WHERE project_id = ? AND membership_generation = 1`, string(projectID)); err != nil {
					t.Fatalf("delete candidate membership event: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "authority-candidate-corrupt-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-authority-candidate-corrupt-" + syncSlug(test.name))
			snapshot := syncAuthorityCandidateBootstrapSnapshotV2(3)
			environments := syncAuthorityCandidateManyEnvironmentsV2(3)
			first, err := store.StageVerifiedSyncAuthorityCandidatePage(
				context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", environments[:2], true),
			)
			if err != nil {
				t.Fatalf("stage corruption fixture: %v", err)
			}
			test.mutate(t, store, projectID)
			if _, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID); err == nil || found {
				t.Fatalf("CurrentSyncAuthorityCandidate(corrupt) = (_, %v, %v), want (_, false, store error)", found, err)
			} else {
				assertSyncErrorCode(t, err, SyncErrorStore)
			}
			if _, err := store.StageVerifiedSyncAuthorityCandidatePage(
				context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2(first.ThroughEnvironmentID, environments[2:], false),
			); err == nil {
				t.Fatal("stage after corruption error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorStore)
			}
		})
	}
}

func TestSyncAuthorityCandidateReadyReplayAuditsOnlyBoundedPageNeighborhood(t *testing.T) {
	tests := []struct {
		name            string
		corruptIndex    int
		replayPageIndex int
		wantReplayStore bool
	}{
		{name: "unrelated later middle page", corruptIndex: 160, replayPageIndex: 0},
		{name: "unrelated earlier middle page", corruptIndex: 4, replayPageIndex: 50},
		{name: "last checkpoint page", corruptIndex: 299, replayPageIndex: 0, wantReplayStore: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "authority-candidate-ready-lazy-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-authority-candidate-ready-lazy-" + syncSlug(test.name))
			snapshot, environments, pages, ready := stageReadySyncAuthorityCandidateV2(t, store, projectID, 300)
			if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidate_environments
SET certificate_bytes = ?
WHERE project_id = ? AND environment_id = ?`, []byte("valid bounded but digest-conflicting certificate"), string(projectID), environments[test.corruptIndex].EnvironmentID); err != nil {
				t.Fatalf("corrupt unrelated candidate environment: %v", err)
			}
			beforeRows := syncAuthorityCandidatePersistedRowsV2(t, store, projectID)
			beforeProtected := syncAuthorityCandidateProtectedCountsV2(t, store)
			replayed, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, pages[test.replayPageIndex])
			if test.wantReplayStore {
				assertSyncErrorCode(t, err, SyncErrorStore)
				if replayed != (SyncAuthorityCandidate{}) {
					t.Fatalf("replay across corrupt last checkpoint = %#v, want zero", replayed)
				}
			} else if err != nil || replayed != ready {
				t.Fatalf("bounded exact replay = (%#v, %v), want (%#v, nil)", replayed, err, ready)
			}
			assertSyncAuthorityCandidatePersistedStateV2(t, store, projectID, beforeRows, beforeProtected)
			if _, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID); err == nil || found {
				t.Fatalf("CurrentSyncAuthorityCandidate(corrupt unrelated page) = (_, %v, %v), want full-audit store error", found, err)
			} else {
				assertSyncErrorCode(t, err, SyncErrorStore)
			}
		})
	}
}

func TestSyncAuthorityCandidateReadyReplayRejectsRequestedAndHeaderCorruptionWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *Store, continuity.ProjectID, SyncAuthorityCandidate, []SyncEnvironmentCertificate, []SyncAuthorityPage)
	}{
		{
			name: "requested page header",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, candidate SyncAuthorityCandidate, _ []SyncEnvironmentCertificate, _ []SyncAuthorityPage) {
				digest := sha256.Sum256([]byte("different valid requested page digest"))
				if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidate_pages
SET page_digest = ?
WHERE project_id = ? AND candidate_id = ? AND page_number = 2`, digest[:], string(projectID), candidate.CandidateID[:]); err != nil {
					t.Fatalf("corrupt requested page header: %v", err)
				}
			},
		},
		{
			name: "requested environment child",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, _ SyncAuthorityCandidate, environments []SyncEnvironmentCertificate, _ []SyncAuthorityPage) {
				if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidate_environments
SET certificate_bytes = ?
WHERE project_id = ? AND environment_id = ?`, []byte("different valid requested certificate bytes"), string(projectID), environments[4].EnvironmentID); err != nil {
					t.Fatalf("corrupt requested environment child: %v", err)
				}
			},
		},
		{
			name: "requested membership event",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, candidate SyncAuthorityCandidate, _ []SyncEnvironmentCertificate, _ []SyncAuthorityPage) {
				if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidate_membership_events
SET event_kind = 'retirement'
WHERE project_id = ? AND candidate_id = ? AND membership_generation = 5`, string(projectID), candidate.CandidateID[:]); err != nil {
					t.Fatalf("corrupt requested membership event: %v", err)
				}
			},
		},
		{
			name: "previous checkpoint",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, candidate SyncAuthorityCandidate, _ []SyncEnvironmentCertificate, _ []SyncAuthorityPage) {
				digest := sha256.Sum256([]byte("different valid previous rolling digest"))
				if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidate_pages
SET resulting_rolling_digest = ?
WHERE project_id = ? AND candidate_id = ? AND page_number = 1`, digest[:], string(projectID), candidate.CandidateID[:]); err != nil {
					t.Fatalf("corrupt previous checkpoint: %v", err)
				}
			},
		},
		{
			name: "ready header final digest",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, candidate SyncAuthorityCandidate, _ []SyncEnvironmentCertificate, _ []SyncAuthorityPage) {
				digest := sha256.Sum256([]byte("different valid ready authority digest"))
				if digest == candidate.AuthorityDigest {
					t.Fatal("corruption digest unexpectedly equals ready authority digest")
				}
				if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidates
SET authority_digest = ?
WHERE project_id = ? AND candidate_id = ?`, digest[:], string(projectID), candidate.CandidateID[:]); err != nil {
					t.Fatalf("corrupt ready header authority digest: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "authority-candidate-ready-requested-corruption-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-authority-candidate-ready-requested-corruption-" + syncSlug(test.name))
			snapshot, environments, pages, ready := stageReadySyncAuthorityCandidateV2(t, store, projectID, 12)
			test.mutate(t, store, projectID, ready, environments, pages)
			beforeRows := syncAuthorityCandidatePersistedRowsV2(t, store, projectID)
			beforeProtected := syncAuthorityCandidateProtectedCountsV2(t, store)
			replayed, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, pages[1])
			assertSyncErrorCode(t, err, SyncErrorStore)
			if replayed != (SyncAuthorityCandidate{}) {
				t.Fatalf("corrupt requested replay = %#v, want zero", replayed)
			}
			assertSyncAuthorityCandidatePersistedStateV2(t, store, projectID, beforeRows, beforeProtected)
		})
	}
}

func TestSyncAuthorityCandidateReadyReplayReadsOnlyRequestedCanonicalRange(t *testing.T) {
	tests := []struct {
		name         string
		corruptIndex int
		replayIndex  int
		wantConflict bool
	}{
		{name: "unrelated canonical range", corruptIndex: 8, replayIndex: 0},
		{name: "requested canonical range", corruptIndex: 4, replayIndex: 1, wantConflict: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "authority-candidate-ready-canonical-range-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-authority-candidate-ready-canonical-range-" + syncSlug(test.name))
			environments := syncAuthorityCandidateManyEnvironmentsV2(12)
			authority := SyncAuthority{
				ChannelID:            testSyncChannelID("authority-candidate-ready-canonical-range-channel"),
				RelayGeneration:      sha256.Sum256([]byte("authority-candidate-ready-canonical-range-relay")),
				AdminPublicKey:       sha256.Sum256([]byte("authority-candidate-ready-canonical-range-admin")),
				MembershipGeneration: 12,
				Environments:         environments,
			}
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
				t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
			}
			baseDigest, err := frozenSyncAuthorityDigestV1(projectID, authority)
			if err != nil {
				t.Fatalf("frozenSyncAuthorityDigestV1() error = %v", err)
			}
			snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, 1, baseDigest)
			pages := make([]SyncAuthorityPage, 0, 3)
			after := ""
			var ready SyncAuthorityCandidate
			for offset := 0; offset < len(environments); offset += maximumSyncAuthorityCandidatePageEnvironments {
				end := offset + maximumSyncAuthorityCandidatePageEnvironments
				page := syncAuthorityCandidatePageV2(after, environments[offset:end], end < len(environments))
				ready, err = store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
				if err != nil {
					t.Fatalf("stage canonical range page %d: %v", len(pages)+1, err)
				}
				pages = append(pages, page)
				after = page.ThroughEnvironmentID
			}
			if !ready.Ready {
				t.Fatalf("canonical range candidate = %#v, want ready", ready)
			}
			if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_certificates
SET certificate_bytes = ?
WHERE project_id = ? AND environment_id = ?`, []byte("valid but authority-digest-conflicting certificate"), string(projectID), environments[test.corruptIndex].EnvironmentID); err != nil {
				t.Fatalf("corrupt canonical range: %v", err)
			}
			beforeRows := syncAuthorityCandidatePersistedRowsV2(t, store, projectID)
			beforeProtected := syncAuthorityCandidateProtectedCountsV2(t, store)
			replayed, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, pages[test.replayIndex])
			if test.wantConflict {
				assertSyncErrorCode(t, err, SyncErrorConflict)
				if replayed != (SyncAuthorityCandidate{}) {
					t.Fatalf("requested corrupt canonical replay = %#v, want zero", replayed)
				}
			} else if err != nil || replayed != ready {
				t.Fatalf("unrelated corrupt canonical replay = (%#v, %v), want (%#v, nil)", replayed, err, ready)
			}
			assertSyncAuthorityCandidatePersistedStateV2(t, store, projectID, beforeRows, beforeProtected)
			if _, err := store.CurrentSyncAuthority(context.Background(), projectID); err == nil {
				t.Fatal("CurrentSyncAuthority(corrupt canonical range) error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorStore)
			}
		})
	}
}

func TestSyncAuthorityCandidateConcurrentExactPageConvergesAndCompetingPageConflicts(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		stateRoot := filepath.Join(testTempDir(t), "authority-candidate-concurrent-exact")
		stores := openSyncAuthorityCandidateStoresV2(t, stateRoot)
		projectID := continuity.ProjectID("project-authority-candidate-concurrent-exact")
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(2)
		page := syncAuthorityCandidatePageV2("", syncAuthorityCandidateManyEnvironmentsV2(2), false)
		results := stageSyncAuthorityCandidateConcurrentlyV2(stores, projectID, snapshot, []SyncAuthorityPage{page, page})
		if results[0].err != nil || results[1].err != nil || results[0].candidate != results[1].candidate || !results[0].candidate.Ready {
			t.Fatalf("concurrent exact results = %#v", results)
		}
	})

	t.Run("competing", func(t *testing.T) {
		stateRoot := filepath.Join(testTempDir(t), "authority-candidate-concurrent-competing")
		stores := openSyncAuthorityCandidateStoresV2(t, stateRoot)
		projectID := continuity.ProjectID("project-authority-candidate-concurrent-competing")
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(2)
		pageA := syncAuthorityCandidatePageV2("", syncAuthorityCandidateManyEnvironmentsV2(2), false)
		pageB := pageA
		pageB.Environments = cloneSyncAuthorityCandidateEnvironmentsV2(pageA.Environments)
		pageB.Environments[0].CertificateBytes[0] ^= 0xff
		results := stageSyncAuthorityCandidateConcurrentlyV2(stores, projectID, snapshot, []SyncAuthorityPage{pageA, pageB})
		successes := 0
		conflicts := 0
		for _, result := range results {
			if result.err == nil {
				successes++
				continue
			}
			var problem *SyncError
			if errors.As(result.err, &problem) && problem.Code == SyncErrorConflict {
				conflicts++
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("concurrent competing results = %#v, want one success and one conflict", results)
		}
	})
}

func TestSyncAuthorityCandidateIdentityAndRowsAreProjectScoped(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-project-scope")
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(2)
	page := syncAuthorityCandidatePageV2("", syncAuthorityCandidateManyEnvironmentsV2(2), false)
	first, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), "project-authority-candidate-scope-a", snapshot, page)
	if err != nil {
		t.Fatalf("stage project A: %v", err)
	}
	second, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), "project-authority-candidate-scope-b", snapshot, page)
	if err != nil {
		t.Fatalf("stage project B: %v", err)
	}
	if first.CandidateID == second.CandidateID || first.AuthorityDigest == second.AuthorityDigest {
		t.Fatalf("project-scoped identities collided: A=%#v B=%#v", first, second)
	}
}

func TestDiscardSyncAuthorityCandidateRefusesPromotedReceipt(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-promoted-discard")
	projectID := continuity.ProjectID("project-authority-candidate-promoted-discard")
	ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, syncAuthorityCandidateBootstrapSnapshotV2(1),
		syncAuthorityCandidatePageV2("", syncAuthorityCandidateManyEnvironmentsV2(1), false),
	)
	if err != nil {
		t.Fatalf("stage ready candidate: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE continuity_sync_authority_candidates SET state = 'promoted' WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("seed promoted receipt: %v", err)
	}
	if err := store.DiscardSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
		t.Fatal("discard promoted receipt error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	var promoted int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_candidates WHERE project_id = ? AND state = 'promoted'`, string(projectID)).Scan(&promoted); err != nil {
		t.Fatalf("count promoted receipt: %v", err)
	}
	if promoted != 1 {
		t.Fatalf("promoted receipt count = %d, want 1", promoted)
	}
}

func syncAuthorityCandidateBootstrapSnapshotV2(membershipGeneration int) SyncAuthoritySnapshot {
	return SyncAuthoritySnapshot{
		ChannelID:            testSyncChannelID("authority-candidate-channel"),
		RelayGeneration:      sha256.Sum256([]byte("authority-candidate-relay")),
		AdminPublicKey:       sha256.Sum256([]byte("authority-candidate-admin")),
		MembershipGeneration: uint32(membershipGeneration),
		InventoryArrivalHead: 1,
	}
}

func syncAuthorityCandidateManyEnvironmentsV2(count int) []SyncEnvironmentCertificate {
	environments := make([]SyncEnvironmentCertificate, count)
	for index := range environments {
		environmentID := fmt.Sprintf("environment:%04d", index+1)
		environments[index] = SyncEnvironmentCertificate{
			EnvironmentID:            environmentID,
			CertificateID:            sha256.Sum256([]byte("candidate-certificate:" + environmentID)),
			CertificateBytes:         []byte("candidate certificate bytes for " + environmentID),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: uint32(index + 1),
		}
	}
	return environments
}

func syncAuthorityCandidatePageV2(after string, environments []SyncEnvironmentCertificate, more bool) SyncAuthorityPage {
	through := ""
	if len(environments) > 0 {
		through = environments[len(environments)-1].EnvironmentID
	}
	return SyncAuthorityPage{
		AfterEnvironmentID:   after,
		ThroughEnvironmentID: through,
		Environments:         cloneSyncAuthorityCandidateEnvironmentsV2(environments),
		More:                 more,
	}
}

func cloneSyncAuthorityCandidateEnvironmentsV2(environments []SyncEnvironmentCertificate) []SyncEnvironmentCertificate {
	cloned := make([]SyncEnvironmentCertificate, len(environments))
	for index, environment := range environments {
		cloned[index] = environment
		cloned[index].CertificateBytes = append([]byte(nil), environment.CertificateBytes...)
		if environment.Retirement != nil {
			retirement := *environment.Retirement
			retirement.RetirementBytes = append([]byte(nil), environment.Retirement.RetirementBytes...)
			cloned[index].Retirement = &retirement
		}
	}
	return cloned
}

func syncAuthorityCandidateAuthorityDigestForTestV2(t *testing.T, projectID continuity.ProjectID, authority SyncAuthority) [32]byte {
	t.Helper()
	snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, 0, [32]byte{})
	_, headerDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, snapshot)
	if err != nil {
		t.Fatalf("derive v2 authority header: %v", err)
	}
	rolling, err := syncAuthorityCandidateRollingSeedV2(headerDigest)
	if err != nil {
		t.Fatalf("derive v2 authority rolling seed: %v", err)
	}
	for index, environment := range authority.Environments {
		rolling, _, err = advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, int64(index+1), environment)
		if err != nil {
			t.Fatalf("derive v2 authority rolling step: %v", err)
		}
	}
	digest, err := finalizeSyncAuthorityDigestV2(headerDigest, int64(len(authority.Environments)), rolling)
	if err != nil {
		t.Fatalf("derive v2 authority digest: %v", err)
	}
	return digest
}

func openSyncAuthorityCandidateStoresV2(t *testing.T, stateRoot string) [2]*Store {
	t.Helper()
	var stores [2]*Store
	for index := range stores {
		store, err := Open(stateRoot, "environment-local")
		if err != nil {
			t.Fatalf("Open(store %d) error = %v", index, err)
		}
		stores[index] = store
		t.Cleanup(func() { store.Close() })
	}
	return stores
}

type syncAuthorityCandidateConcurrentResultV2 struct {
	candidate SyncAuthorityCandidate
	err       error
}

func stageSyncAuthorityCandidateConcurrentlyV2(
	stores [2]*Store,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
	pages []SyncAuthorityPage,
) [2]syncAuthorityCandidateConcurrentResultV2 {
	var results [2]syncAuthorityCandidateConcurrentResultV2
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := range stores {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			results[index].candidate, results[index].err = stores[index].StageVerifiedSyncAuthorityCandidatePage(
				context.Background(), projectID, snapshot, pages[index],
			)
		}(index)
	}
	close(start)
	group.Wait()
	return results
}

func syncAuthorityCandidateProtectedCountsV2(t *testing.T, store *Store) map[string]int64 {
	t.Helper()
	tables := []string{
		"continuity_facts",
		"continuity_sync_projects",
		"continuity_sync_authorities",
		"continuity_sync_environment_certificates",
		"continuity_sync_terminal_candidates",
		"continuity_sync_terminal_candidate_frames",
		"continuity_sync_inbox",
		"continuity_sync_receipts",
		"continuity_sync_environment_heads",
		"continuity_sync_outbox",
		"continuity_sync_tombstones",
	}
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err := store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count protected table %s: %v", table, err)
		}
		counts[table] = count
	}
	return counts
}

func stageReadySyncAuthorityCandidateV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	environmentCount int,
) (SyncAuthoritySnapshot, []SyncEnvironmentCertificate, []SyncAuthorityPage, SyncAuthorityCandidate) {
	t.Helper()
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(environmentCount)
	environments := syncAuthorityCandidateManyEnvironmentsV2(environmentCount)
	pages := make([]SyncAuthorityPage, 0, (environmentCount+maximumSyncAuthorityCandidatePageEnvironments-1)/maximumSyncAuthorityCandidatePageEnvironments)
	after := ""
	var candidate SyncAuthorityCandidate
	for offset := 0; offset < len(environments); offset += maximumSyncAuthorityCandidatePageEnvironments {
		end := offset + maximumSyncAuthorityCandidatePageEnvironments
		if end > len(environments) {
			end = len(environments)
		}
		page := syncAuthorityCandidatePageV2(after, environments[offset:end], end < len(environments))
		var err error
		candidate, err = store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
		if err != nil {
			t.Fatalf("stage ready authority candidate page %d: %v", len(pages)+1, err)
		}
		pages = append(pages, page)
		after = page.ThroughEnvironmentID
	}
	if !candidate.Ready || candidate.EnvironmentCount != int64(environmentCount) || candidate.PageCount != int64(len(pages)) {
		t.Fatalf("ready authority candidate = %#v, pages=%d environments=%d", candidate, len(pages), environmentCount)
	}
	return snapshot, environments, pages, candidate
}

func syncAuthorityCandidatePersistedRowsV2(t *testing.T, store *Store, projectID continuity.ProjectID) []string {
	t.Helper()
	queries := []struct {
		table string
		order string
	}{
		{table: "continuity_sync_authority_candidates", order: "candidate_id"},
		{table: "continuity_sync_authority_candidate_pages", order: "candidate_id, page_number"},
		{table: "continuity_sync_authority_candidate_environments", order: "candidate_id, environment_ordinal"},
		{table: "continuity_sync_authority_candidate_membership_events", order: "candidate_id, membership_generation"},
	}
	var result []string
	for _, query := range queries {
		rows, err := store.db.Query("SELECT * FROM "+query.table+" WHERE project_id = ? ORDER BY "+query.order, string(projectID))
		if err != nil {
			t.Fatalf("read exact %s rows: %v", query.table, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatalf("read %s columns: %v", query.table, err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for index := range values {
				pointers[index] = &values[index]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatalf("scan exact %s row: %v", query.table, err)
			}
			row := query.table
			for index, value := range values {
				switch value := value.(type) {
				case []byte:
					row += fmt.Sprintf("|%s=bytes:%x", columns[index], value)
				case string:
					row += fmt.Sprintf("|%s=string:%q", columns[index], value)
				case int64:
					row += fmt.Sprintf("|%s=int64:%d", columns[index], value)
				case nil:
					row += "|" + columns[index] + "=nil"
				default:
					rows.Close()
					t.Fatalf("unexpected %s.%s type %T", query.table, columns[index], value)
				}
			}
			result = append(result, row)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("iterate exact %s rows: %v", query.table, err)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close exact %s rows: %v", query.table, err)
		}
	}
	return result
}

func assertSyncAuthorityCandidatePersistedStateV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	wantRows []string,
	wantProtected map[string]int64,
) {
	t.Helper()
	if got := syncAuthorityCandidatePersistedRowsV2(t, store, projectID); !reflect.DeepEqual(got, wantRows) {
		t.Fatalf("authority candidate rows changed:\n got %#v\nwant %#v", got, wantRows)
	}
	if got := syncAuthorityCandidateProtectedCountsV2(t, store); !reflect.DeepEqual(got, wantProtected) {
		t.Fatalf("protected row counts changed: got %#v, want %#v", got, wantProtected)
	}
}

func syncAuthorityCandidateQueryPlanV2(t *testing.T, store *Store, query string, arguments ...any) []string {
	t.Helper()
	rows, err := store.db.Query("EXPLAIN QUERY PLAN\n"+query, arguments...)
	if err != nil {
		t.Fatalf("explain authority candidate query: %v", err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan authority candidate query plan: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read authority candidate query plan: %v", err)
	}
	return details
}

func assertSyncAuthorityCandidateEqualV2(t *testing.T, got, want SyncAuthorityCandidate) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate = %#v, want %#v", got, want)
	}
}

func assertSyncAuthorityCandidateConflictV2(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want conflict")
	}
	var problem *SyncError
	if !errors.As(err, &problem) || problem.Code != SyncErrorConflict {
		t.Fatalf("error = %v, want conflict", err)
	}
}

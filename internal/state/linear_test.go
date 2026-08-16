package state

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMapLinearStateType(t *testing.T) {
	cases := map[string]string{
		"triage":    IssueStatusTriage,
		"backlog":   IssueStatusBacklog,
		"unstarted": IssueStatusTodo,
		"started":   IssueStatusActive,
		"completed": IssueStatusDone,
		"canceled":  IssueStatusCancelled,
		"cancelled": IssueStatusCancelled,
		"unknown":   IssueStatusTriage,
	}
	for input, want := range cases {
		if got := MapLinearStateType(input); got != want {
			t.Fatalf("MapLinearStateType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLinearClientTeamAndIssueRoundTrip(t *testing.T) {
	fake := NewLinearFake()
	parent := fake.SeedIssue("ENG-1", "Parent", "root body", "started", "")
	fake.SeedIssue("ENG-2", "Child", "child body", "unstarted", "ENG-1")
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	client := NewLinearClient(server.URL, "test-key")
	ctx := context.Background()
	team, err := client.TeamByKey(ctx, "ENG")
	if err != nil {
		t.Fatalf("TeamByKey() error = %v", err)
	}
	if team.Key != "ENG" || len(team.States) == 0 {
		t.Fatalf("team = %#v", team)
	}
	issue, err := client.Issue(ctx, "ENG-1")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if issue.ID != parent.ID || issue.Identifier != "ENG-1" || MapLinearStateType(issue.State.Type) != IssueStatusActive {
		t.Fatalf("issue = %#v", issue)
	}
	if len(issue.ChildKeys) != 1 || issue.ChildKeys[0] != "ENG-2" {
		t.Fatalf("children = %#v", issue.ChildKeys)
	}

	created, err := client.CreateIssue(ctx, LinearCreateIssueInput{TeamID: team.ID, Title: "Minted", Description: "from loaf"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if created.Identifier != "ENG-3" {
		t.Fatalf("minted identifier = %q, want ENG-3", created.Identifier)
	}
}

func TestLinearClientReleasesUnsupported(t *testing.T) {
	fake := NewLinearFake()
	fake.SupportsReleases = false
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	client := NewLinearClient(server.URL, "test-key")
	ok, err := client.ReleasesSupported(context.Background())
	if err != nil {
		t.Fatalf("ReleasesSupported() error = %v", err)
	}
	if ok {
		t.Fatal("ReleasesSupported() = true, want false")
	}
}

func TestPullLinearIssueTreeKeepsParentEdges(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Authority: IssueAuthorityLinear}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	fake := NewLinearFake()
	fake.SeedIssue("ENG-1", "Root", "", "triage", "")
	fake.SeedIssue("ENG-2", "Child", "", "unstarted", "ENG-1")
	fake.SeedIssue("ENG-3", "Grandchild", "", "started", "ENG-2")
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	client := NewLinearClient(server.URL, "test-key")

	rootIssue, err := store.adoptLinearIssue(ctx, root, client, "ENG-1", "")
	if err != nil {
		t.Fatalf("adopt root error = %v", err)
	}
	tree := []Issue{rootIssue}
	if err := store.pullLinearChildren(ctx, root, client, "ENG-1", &tree); err != nil {
		t.Fatalf("pull children error = %v", err)
	}
	if len(tree) != 3 {
		t.Fatalf("tree = %#v, want 3 issues", tree)
	}
	byAlias := map[string]Issue{}
	for _, issue := range tree {
		byAlias[issue.Alias] = issue
	}
	if byAlias["ENG-1"].ParentID != "" {
		t.Fatalf("root parent = %q, want empty", byAlias["ENG-1"].ParentID)
	}
	if byAlias["ENG-2"].ParentID != byAlias["ENG-1"].ID {
		t.Fatalf("child parent = %q, want %q", byAlias["ENG-2"].ParentID, byAlias["ENG-1"].ID)
	}
	if byAlias["ENG-3"].ParentID != byAlias["ENG-2"].ID {
		t.Fatalf("grandchild parent = %q, want %q", byAlias["ENG-3"].ParentID, byAlias["ENG-2"].ID)
	}
	if byAlias["ENG-3"].Status != IssueStatusActive {
		t.Fatalf("grandchild status = %q, want active", byAlias["ENG-3"].Status)
	}
	identity, err := store.GetIssueIdentity(ctx, root)
	if err != nil {
		t.Fatalf("GetIssueIdentity() error = %v", err)
	}
	if identity.NextNumber != 1 {
		t.Fatalf("next_number = %d, want 1", identity.NextNumber)
	}
}

func TestLinearMintErrorNamesOfflinePath(t *testing.T) {
	err := &LinearMintError{Err: context.Canceled}
	if !strings.Contains(err.Error(), "loaf spark") || !strings.Contains(err.Error(), "loaf idea") {
		t.Fatalf("error = %q, want spark/idea offline path", err)
	}
}

func TestLinearOrphanErrorNamesPullRecovery(t *testing.T) {
	err := &LinearOrphanError{Identifier: "ENG-88", Err: context.Canceled}
	if !strings.Contains(err.Error(), "ENG-88") || !strings.Contains(err.Error(), "loaf issue pull ENG-88") {
		t.Fatalf("error = %q, want orphan key and pull recovery", err)
	}
}

func TestPullLinearIssueTreeReattachesExistingChild(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Authority: IssueAuthorityLinear}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	fake := NewLinearFake()
	fake.SeedIssue("ENG-1", "Root", "", "triage", "")
	fake.SeedIssue("ENG-2", "Child", "", "unstarted", "ENG-1")
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	client := NewLinearClient(server.URL, "test-key")

	child, err := store.adoptLinearIssue(ctx, root, client, "ENG-2", "")
	if err != nil {
		t.Fatalf("adopt child error = %v", err)
	}
	if child.ParentID != "" {
		t.Fatalf("solo child parent = %q, want empty", child.ParentID)
	}

	rootIssue, err := store.adoptLinearIssue(ctx, root, client, "ENG-1", "")
	if err != nil {
		t.Fatalf("adopt root error = %v", err)
	}
	tree := []Issue{rootIssue}
	if err := store.pullLinearChildren(ctx, root, client, "ENG-1", &tree); err != nil {
		t.Fatalf("pull --tree after solo child error = %v", err)
	}
	byAlias := map[string]Issue{}
	for _, issue := range tree {
		byAlias[issue.Alias] = issue
	}
	if byAlias["ENG-2"].ParentID != byAlias["ENG-1"].ID {
		t.Fatalf("reattached child parent = %q, want %q", byAlias["ENG-2"].ParentID, byAlias["ENG-1"].ID)
	}
	shown, err := store.ShowIssue(ctx, root, byAlias["ENG-1"].ID)
	if err != nil {
		t.Fatalf("ShowIssue() error = %v", err)
	}
	if len(shown.Children) != 1 || shown.Children[0].ID != byAlias["ENG-2"].ID {
		t.Fatalf("root children = %#v, want depth-2 edge to ENG-2", shown.Children)
	}
}

func TestWriteLinearTeamConfigReplacesRoleRow(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		t.Fatalf("projectID() error = %v", err)
	}
	if err := store.upsertBackendMapping(ctx, root, backendMapping{
		EntityKind:   "project",
		EntityID:     projectID,
		ExternalKind: linearExternalKindTeam,
		ExternalID:   "ENG",
		SyncStatus:   linearSyncLinked,
	}); err != nil {
		t.Fatalf("write ENG team error = %v", err)
	}
	if err := store.upsertBackendMapping(ctx, root, backendMapping{
		EntityKind:   "project",
		EntityID:     projectID,
		ExternalKind: linearExternalKindTeam,
		ExternalID:   "OPS",
		SyncStatus:   linearSyncLinked,
	}); err != nil {
		t.Fatalf("write OPS team error = %v", err)
	}
	cfg, err := store.LoadLinearAdapterConfig(ctx, root)
	if err != nil {
		t.Fatalf("LoadLinearAdapterConfig() error = %v", err)
	}
	if cfg.TeamKey != "OPS" {
		t.Fatalf("TeamKey = %q, want OPS", cfg.TeamKey)
	}
	var n int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM backend_mappings
WHERE project_id = ? AND backend = ? AND entity_kind = 'project' AND external_kind = ?
`, projectID, linearBackend, linearExternalKindTeam).Scan(&n); err != nil {
		t.Fatalf("count team rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("team rows = %d, want 1", n)
	}
}

func TestEnsureLabelRetriesAfterCreateRace(t *testing.T) {
	fake := NewLinearFake()
	existing := fake.SeedLabel(fake.Team.ID, "ready-for-agent")
	fake.SkipLabelLookups = 1
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	client := NewLinearClient(server.URL, "test-key")
	id, err := client.EnsureLabel(context.Background(), fake.Team.ID, "ready-for-agent")
	if err != nil {
		t.Fatalf("EnsureLabel() error = %v", err)
	}
	if id != existing.ID {
		t.Fatalf("EnsureLabel() = %q, want raced existing %q", id, existing.ID)
	}
}

func TestFindLabelIDIsTeamScoped(t *testing.T) {
	fake := NewLinearFake()
	other := fake.SeedLabel("team_other", "ready-for-agent")
	want := fake.SeedLabel(fake.Team.ID, "ready-for-agent")
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	client := NewLinearClient(server.URL, "test-key")
	id, err := client.FindLabelID(context.Background(), fake.Team.ID, "ready-for-agent")
	if err != nil {
		t.Fatalf("FindLabelID() error = %v", err)
	}
	if id != want.ID {
		t.Fatalf("FindLabelID() = %q, want team-scoped %q (other=%q)", id, want.ID, other.ID)
	}
}

func TestLinearFakeRejectsIssueUpdateTitle(t *testing.T) {
	fake := NewLinearFake()
	issue := fake.SeedIssue("ENG-1", "Name", "", "triage", "")
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	payload, err := json.Marshal(map[string]any{
		"operationName": "IssueUpdate",
		"variables": map[string]any{
			"id":    issue.ID,
			"input": map[string]any{"title": "must not land"},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("IssueUpdate POST error = %v", err)
	}
	defer resp.Body.Close()
	var decoded struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Errors) == 0 || !strings.Contains(decoded.Errors[0].Message, "title") {
		t.Fatalf("errors = %#v, want title rejection", decoded.Errors)
	}
	got, ok := fake.Issue("ENG-1")
	if !ok || got.Title != "Name" {
		t.Fatalf("title mutated: %#v", got)
	}
}

func TestReconcileLinearIssueDescriptionUsesPushRender(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Authority: IssueAuthorityLinear}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	fake := NewLinearFake()
	fake.SeedIssue("ENG-1", "Shaped", "raw tracker body", "unstarted", "")
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	client := NewLinearClient(server.URL, "test-key")
	adopted, err := store.adoptLinearIssue(ctx, root, client, "ENG-1", "")
	if err != nil {
		t.Fatalf("adopt error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: adopted.ID, Body: "local shaping", SetBody: true}); err != nil {
		t.Fatalf("edit body error = %v", err)
	}
	shown, err := store.ShowIssue(ctx, root, adopted.ID)
	if err != nil {
		t.Fatalf("ShowIssue() error = %v", err)
	}
	rendered := RenderIssueMarkdown(shown)
	if _, err := store.PushLinearIssue(ctx, root, client, adopted.ID, rendered); err != nil {
		t.Fatalf("PushLinearIssue() error = %v", err)
	}
	afterPush, err := store.ReconcileLinearIssue(ctx, root, client, adopted.ID, false, false)
	if err != nil {
		t.Fatalf("reconcile after push error = %v", err)
	}
	for _, conflict := range afterPush.Conflicts {
		if conflict.Field == "description" {
			t.Fatalf("false description drift after push: %#v", afterPush.Conflicts)
		}
	}
	fake.SetIssueDescription("ENG-1", "tracker edited the render", time.Now().UTC())
	afterEdit, err := store.ReconcileLinearIssue(ctx, root, client, adopted.ID, false, false)
	if err != nil {
		t.Fatalf("reconcile after remote edit error = %v", err)
	}
	found := false
	for _, conflict := range afterEdit.Conflicts {
		if conflict.Field == "description" {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflicts = %#v, want description drift after remote edit", afterEdit.Conflicts)
	}
}

package state

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LinearFake is an in-memory Linear GraphQL workspace for hermetic tests.
// It implements http.Handler and speaks the same operation names as LinearClient.
type LinearFake struct {
	mu                   sync.Mutex
	Team                 LinearFakeTeam
	Issues               map[string]*LinearFakeIssue
	Releases             map[string]*LinearFakeRelease
	Labels               map[string]*LinearFakeLabel
	Comments             []LinearFakeComment
	SupportsReleases     bool
	Unreachable          bool
	SkipLabelLookups     int
	ReleaseMutationError string
	nextIssue            int
	nextID               int
}

// LinearFakeTeam is the single team the fake workspace serves.
type LinearFakeTeam struct {
	ID     string
	Key    string
	States []LinearState
}

// LinearFakeIssue is one issue in the fake workspace.
type LinearFakeIssue struct {
	ID          string
	Identifier  string
	Title       string
	Description string
	URL         string
	UpdatedAt   time.Time
	State       LinearState
	ParentID    string
	LabelIDs    []string
	ReleaseID   string
}

// LinearFakeRelease is one release in the fake workspace.
type LinearFakeRelease struct {
	ID   string
	Name string
}

// LinearFakeLabel is one label in the fake workspace.
type LinearFakeLabel struct {
	ID     string
	Name   string
	TeamID string
}

// LinearFakeComment is one comment recorded by the fake.
type LinearFakeComment struct {
	ID      string
	IssueID string
	Body    string
}

type linearFakeRequest struct {
	Query         string          `json:"query"`
	OperationName string          `json:"operationName"`
	Variables     json.RawMessage `json:"variables"`
}

// NewLinearFake returns a workspace with team ENG and the seven-type states.
func NewLinearFake() *LinearFake {
	return &LinearFake{
		Team: LinearFakeTeam{
			ID:  "team_eng",
			Key: "ENG",
			States: []LinearState{
				{ID: "state_triage", Name: "Triage", Type: "triage"},
				{ID: "state_backlog", Name: "Backlog", Type: "backlog"},
				{ID: "state_todo", Name: "Todo", Type: "unstarted"},
				{ID: "state_active", Name: "In Progress", Type: "started"},
				{ID: "state_done", Name: "Done", Type: "completed"},
				{ID: "state_cancelled", Name: "Canceled", Type: "canceled"},
			},
		},
		Issues:           map[string]*LinearFakeIssue{},
		Releases:         map[string]*LinearFakeRelease{},
		Labels:           map[string]*LinearFakeLabel{},
		SupportsReleases: true,
		nextIssue:        1,
	}
}

func (f *LinearFake) nextOpaque(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s_%d", prefix, f.nextID)
}

func (f *LinearFake) stateByID(id string) LinearState {
	for _, state := range f.Team.States {
		if state.ID == id {
			return state
		}
	}
	return LinearState{}
}

func (f *LinearFake) stateByType(typeName string) LinearState {
	for _, state := range f.Team.States {
		if state.Type == typeName {
			return state
		}
	}
	return LinearState{}
}

func (f *LinearFake) issueByID(id string) *LinearFakeIssue {
	if issue, ok := f.Issues[id]; ok {
		return issue
	}
	for _, issue := range f.Issues {
		if issue.ID == id || issue.Identifier == id {
			return issue
		}
	}
	return nil
}

func (f *LinearFake) releaseByRef(ref string) *LinearFakeRelease {
	if release, ok := f.Releases[ref]; ok {
		return release
	}
	for _, release := range f.Releases {
		if release.ID == ref || release.Name == ref {
			return release
		}
	}
	return nil
}

func (f *LinearFake) childrenOf(parentID string) []*LinearFakeIssue {
	var children []*LinearFakeIssue
	for _, issue := range f.Issues {
		if issue.ParentID == parentID {
			children = append(children, issue)
		}
	}
	return children
}

func (f *LinearFake) labelNames(ids []string) []map[string]string {
	nodes := []map[string]string{}
	for _, id := range ids {
		if label, ok := f.Labels[id]; ok {
			nodes = append(nodes, map[string]string{"id": label.ID, "name": label.Name})
		}
	}
	return nodes
}

func (f *LinearFake) issuePayload(issue *LinearFakeIssue) map[string]any {
	var parent any
	if issue.ParentID != "" {
		if parentIssue := f.issueByID(issue.ParentID); parentIssue != nil {
			parent = map[string]string{"id": parentIssue.ID, "identifier": parentIssue.Identifier}
		}
	}
	children := []map[string]string{}
	for _, child := range f.childrenOf(issue.ID) {
		children = append(children, map[string]string{"id": child.ID, "identifier": child.Identifier})
	}
	return map[string]any{
		"id":          issue.ID,
		"identifier":  issue.Identifier,
		"title":       issue.Title,
		"description": issue.Description,
		"url":         issue.URL,
		"updatedAt":   issue.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"state":       issue.State,
		"parent":      parent,
		"children":    map[string]any{"nodes": children},
		"labels":      map[string]any{"nodes": f.labelNames(issue.LabelIDs)},
	}
}

func (f *LinearFake) releasePayload(release *LinearFakeRelease) map[string]any {
	nodes := []map[string]string{}
	for _, issue := range f.Issues {
		if issue.ReleaseID == release.ID {
			nodes = append(nodes, map[string]string{"id": issue.ID, "identifier": issue.Identifier})
		}
	}
	return map[string]any{
		"id":     release.ID,
		"name":   release.Name,
		"issues": map[string]any{"nodes": nodes},
	}
}

// SeedLabel inserts a team-scoped label with a known name.
func (f *LinearFake) SeedLabel(teamID, name string) *LinearFakeLabel {
	f.mu.Lock()
	defer f.mu.Unlock()
	if teamID == "" {
		teamID = f.Team.ID
	}
	label := &LinearFakeLabel{ID: f.nextOpaque("lbl"), Name: name, TeamID: teamID}
	f.Labels[label.ID] = label
	return label
}

// SeedIssue inserts an issue with a known identifier. Identifier defaults to ENG-N.
func (f *LinearFake) SeedIssue(identifier, title, description, stateType, parentKey string) *LinearFakeIssue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seedIssueLocked(identifier, title, description, stateType, parentKey)
}

func (f *LinearFake) seedIssueLocked(identifier, title, description, stateType, parentKey string) *LinearFakeIssue {
	if identifier == "" {
		identifier = fmt.Sprintf("%s-%d", f.Team.Key, f.nextIssue)
		f.nextIssue++
	} else if n := linearIdentifierNumber(identifier); n >= f.nextIssue {
		f.nextIssue = n + 1
	}
	state := f.stateByType(stateType)
	if state.ID == "" {
		state = f.stateByType("triage")
	}
	var parentID string
	if parentKey != "" {
		if parent := f.issueByID(parentKey); parent != nil {
			parentID = parent.ID
		}
	}
	issue := &LinearFakeIssue{
		ID:          f.nextOpaque("iss"),
		Identifier:  identifier,
		Title:       title,
		Description: description,
		URL:         "https://linear.app/loaf/issue/" + identifier,
		UpdatedAt:   time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		State:       state,
		ParentID:    parentID,
	}
	f.Issues[issue.ID] = issue
	return issue
}

func linearIdentifierNumber(identifier string) int {
	parts := strings.Split(identifier, "-")
	if len(parts) < 2 {
		return 0
	}
	n, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}
	return n
}

// SetIssueState moves an issue to the first workflow state of typeName.
func (f *LinearFake) SetIssueState(ref, typeName string, updatedAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	issue := f.issueByID(ref)
	if issue == nil {
		return
	}
	if state := f.stateByType(typeName); state.ID != "" {
		issue.State = state
	}
	if !updatedAt.IsZero() {
		issue.UpdatedAt = updatedAt
	} else {
		issue.UpdatedAt = time.Now().UTC()
	}
}

// SetIssueTitle sets the tracker-owned title.
func (f *LinearFake) SetIssueTitle(ref, title string, updatedAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	issue := f.issueByID(ref)
	if issue == nil {
		return
	}
	issue.Title = title
	if !updatedAt.IsZero() {
		issue.UpdatedAt = updatedAt
	}
}

// SetIssueDescription sets the Linear description.
func (f *LinearFake) SetIssueDescription(ref, description string, updatedAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	issue := f.issueByID(ref)
	if issue == nil {
		return
	}
	issue.Description = description
	if !updatedAt.IsZero() {
		issue.UpdatedAt = updatedAt
	}
}

// Issue returns a copy of a seeded or created issue.
func (f *LinearFake) Issue(ref string) (LinearFakeIssue, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	issue := f.issueByID(ref)
	if issue == nil {
		return LinearFakeIssue{}, false
	}
	return *issue, true
}

// Release returns a copy of a created release.
func (f *LinearFake) Release(ref string) (LinearFakeRelease, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	release := f.releaseByRef(ref)
	if release == nil {
		return LinearFakeRelease{}, false
	}
	return *release, true
}

// ReleaseIssueKeys returns Linear identifiers attached to a release.
func (f *LinearFake) ReleaseIssueKeys(ref string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	release := f.releaseByRef(ref)
	if release == nil {
		return nil
	}
	var keys []string
	for _, issue := range f.Issues {
		if issue.ReleaseID == release.ID {
			keys = append(keys, issue.Identifier)
		}
	}
	return keys
}

// IssueLabelNames returns labels currently on an issue.
func (f *LinearFake) IssueLabelNames(ref string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	issue := f.issueByID(ref)
	if issue == nil {
		return nil
	}
	var names []string
	for _, id := range issue.LabelIDs {
		if label, ok := f.Labels[id]; ok {
			names = append(names, label.Name)
		}
	}
	return names
}

// IssueComments returns comments recorded on an issue.
func (f *LinearFake) IssueComments(ref string) []LinearFakeComment {
	f.mu.Lock()
	defer f.mu.Unlock()
	issue := f.issueByID(ref)
	if issue == nil {
		return nil
	}
	var comments []LinearFakeComment
	for _, comment := range f.Comments {
		if comment.IssueID == issue.ID {
			comments = append(comments, comment)
		}
	}
	return comments
}

// ServeHTTP implements Linear's GraphQL POST endpoint.
func (f *LinearFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	f.mu.Lock()
	unreachable := f.Unreachable
	f.mu.Unlock()
	if unreachable {
		http.Error(w, "linear unreachable", http.StatusBadGateway)
		return
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		http.Error(w, "missing authorization", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req linearFakeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	operation := req.OperationName
	if operation == "" {
		operation = linearFakeOperationFromQuery(req.Query)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	data, gqlErr := f.dispatch(operation, req.Variables)
	response := map[string]any{}
	if gqlErr != "" {
		response["errors"] = []map[string]string{{"message": gqlErr}}
	} else {
		response["data"] = data
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func linearFakeOperationFromQuery(query string) string {
	for _, name := range []string{
		"TeamByKey", "IssueCreate", "IssueUpdate", "IssueLabels", "IssueLabelCreate",
		"CommentCreate", "ReleaseCreate", "ReleaseUpdate", "Releases", "Release", "Issue",
	} {
		if strings.Contains(query, name) {
			return name
		}
	}
	return ""
}

func (f *LinearFake) dispatch(operation string, rawVars json.RawMessage) (any, string) {
	switch operation {
	case "TeamByKey":
		return f.handleTeamByKey(rawVars)
	case "Issue":
		return f.handleIssue(rawVars)
	case "IssueCreate":
		return f.handleIssueCreate(rawVars)
	case "IssueUpdate":
		return f.handleIssueUpdate(rawVars)
	case "Releases":
		return f.handleReleases()
	case "Release":
		return f.handleRelease(rawVars)
	case "ReleaseCreate":
		return f.handleReleaseCreate(rawVars)
	case "ReleaseUpdate":
		return f.handleReleaseUpdate(rawVars)
	case "IssueLabels":
		return f.handleIssueLabels(rawVars)
	case "IssueLabelCreate":
		return f.handleIssueLabelCreate(rawVars)
	case "CommentCreate":
		return f.handleCommentCreate(rawVars)
	default:
		return nil, fmt.Sprintf("unknown operation %q", operation)
	}
}

func decodeFakeVars(raw json.RawMessage, dest any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dest)
}

func (f *LinearFake) handleTeamByKey(raw json.RawMessage) (any, string) {
	var vars struct {
		Key string `json:"key"`
	}
	if err := decodeFakeVars(raw, &vars); err != nil {
		return nil, err.Error()
	}
	if vars.Key != f.Team.Key {
		return map[string]any{"teams": map[string]any{"nodes": []any{}}}, ""
	}
	return map[string]any{
		"teams": map[string]any{
			"nodes": []any{
				map[string]any{
					"id":     f.Team.ID,
					"key":    f.Team.Key,
					"states": map[string]any{"nodes": f.Team.States},
				},
			},
		},
	}, ""
}

func (f *LinearFake) handleIssue(raw json.RawMessage) (any, string) {
	var vars struct {
		ID string `json:"id"`
	}
	if err := decodeFakeVars(raw, &vars); err != nil {
		return nil, err.Error()
	}
	issue := f.issueByID(vars.ID)
	if issue == nil {
		return map[string]any{"issue": nil}, ""
	}
	return map[string]any{"issue": f.issuePayload(issue)}, ""
}

func (f *LinearFake) handleIssueCreate(raw json.RawMessage) (any, string) {
	var vars struct {
		Input struct {
			TeamID      string `json:"teamId"`
			Title       string `json:"title"`
			Description string `json:"description"`
			ParentID    string `json:"parentId"`
			StateID     string `json:"stateId"`
		} `json:"input"`
	}
	if err := decodeFakeVars(raw, &vars); err != nil {
		return nil, err.Error()
	}
	if vars.Input.TeamID != f.Team.ID {
		return nil, "unknown team"
	}
	state := f.stateByType("triage")
	if vars.Input.StateID != "" {
		if found := f.stateByID(vars.Input.StateID); found.ID != "" {
			state = found
		}
	}
	var parentID string
	if vars.Input.ParentID != "" {
		if parent := f.issueByID(vars.Input.ParentID); parent != nil {
			parentID = parent.ID
		}
	}
	identifier := fmt.Sprintf("%s-%d", f.Team.Key, f.nextIssue)
	f.nextIssue++
	issue := &LinearFakeIssue{
		ID:          f.nextOpaque("iss"),
		Identifier:  identifier,
		Title:       vars.Input.Title,
		Description: vars.Input.Description,
		URL:         "https://linear.app/loaf/issue/" + identifier,
		UpdatedAt:   time.Now().UTC(),
		State:       state,
		ParentID:    parentID,
	}
	f.Issues[issue.ID] = issue
	return map[string]any{
		"issueCreate": map[string]any{"success": true, "issue": f.issuePayload(issue)},
	}, ""
}

func issueUpdateCarriesTitle(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return false
	}
	_, ok := fields["title"]
	return ok
}

func (f *LinearFake) handleIssueUpdate(raw json.RawMessage) (any, string) {
	var vars struct {
		ID    string          `json:"id"`
		Input json.RawMessage `json:"input"`
	}
	if err := decodeFakeVars(raw, &vars); err != nil {
		return nil, err.Error()
	}
	if issueUpdateCarriesTitle(vars.Input) {
		return nil, "issue update must not include title; Linear owns the name"
	}
	var input struct {
		Description   *string  `json:"description"`
		StateID       string   `json:"stateId"`
		ReleaseID     string   `json:"releaseId"`
		AddedLabelIDs []string `json:"addedLabelIds"`
	}
	if len(vars.Input) > 0 {
		if err := json.Unmarshal(vars.Input, &input); err != nil {
			return nil, err.Error()
		}
	}
	issue := f.issueByID(vars.ID)
	if issue == nil {
		return nil, "issue not found"
	}
	if input.Description != nil {
		issue.Description = *input.Description
	}
	if input.StateID != "" {
		if state := f.stateByID(input.StateID); state.ID != "" {
			issue.State = state
		}
	}
	if input.ReleaseID != "" {
		if release := f.releaseByRef(input.ReleaseID); release != nil {
			issue.ReleaseID = release.ID
		}
	}
	for _, labelID := range input.AddedLabelIDs {
		if _, ok := f.Labels[labelID]; !ok {
			continue
		}
		found := false
		for _, existing := range issue.LabelIDs {
			if existing == labelID {
				found = true
				break
			}
		}
		if !found {
			issue.LabelIDs = append(issue.LabelIDs, labelID)
		}
	}
	issue.UpdatedAt = time.Now().UTC()
	return map[string]any{
		"issueUpdate": map[string]any{"success": true, "issue": f.issuePayload(issue)},
	}, ""
}

func (f *LinearFake) handleReleases() (any, string) {
	if !f.SupportsReleases {
		return nil, `Cannot query field "releases" on type "Query".`
	}
	nodes := []any{}
	for _, release := range f.Releases {
		nodes = append(nodes, f.releasePayload(release))
	}
	return map[string]any{"releases": map[string]any{"nodes": nodes}}, ""
}

func (f *LinearFake) handleRelease(raw json.RawMessage) (any, string) {
	if !f.SupportsReleases {
		return nil, `Cannot query field "release" on type "Query".`
	}
	var vars struct {
		ID string `json:"id"`
	}
	if err := decodeFakeVars(raw, &vars); err != nil {
		return nil, err.Error()
	}
	release := f.releaseByRef(vars.ID)
	if release == nil {
		return map[string]any{"release": nil}, ""
	}
	return map[string]any{"release": f.releasePayload(release)}, ""
}

func (f *LinearFake) handleReleaseCreate(raw json.RawMessage) (any, string) {
	if !f.SupportsReleases {
		return nil, `Cannot query field "releaseCreate" on type "Mutation".`
	}
	if f.ReleaseMutationError != "" {
		return nil, f.ReleaseMutationError
	}
	var vars struct {
		Input struct {
			Name     string   `json:"name"`
			IssueIDs []string `json:"issueIds"`
		} `json:"input"`
	}
	if err := decodeFakeVars(raw, &vars); err != nil {
		return nil, err.Error()
	}
	release := &LinearFakeRelease{ID: f.nextOpaque("rel"), Name: vars.Input.Name}
	f.Releases[release.ID] = release
	for _, issueID := range vars.Input.IssueIDs {
		if issue := f.issueByID(issueID); issue != nil {
			issue.ReleaseID = release.ID
		}
	}
	return map[string]any{
		"releaseCreate": map[string]any{"success": true, "release": f.releasePayload(release)},
	}, ""
}

func (f *LinearFake) handleReleaseUpdate(raw json.RawMessage) (any, string) {
	if !f.SupportsReleases {
		return nil, `Cannot query field "releaseUpdate" on type "Mutation".`
	}
	if f.ReleaseMutationError != "" {
		return nil, f.ReleaseMutationError
	}
	var vars struct {
		ID    string `json:"id"`
		Input struct {
			Name     string   `json:"name"`
			IssueIDs []string `json:"issueIds"`
		} `json:"input"`
	}
	if err := decodeFakeVars(raw, &vars); err != nil {
		return nil, err.Error()
	}
	release := f.releaseByRef(vars.ID)
	if release == nil {
		return nil, "release not found"
	}
	if vars.Input.Name != "" {
		release.Name = vars.Input.Name
	}
	if vars.Input.IssueIDs != nil {
		for _, issue := range f.Issues {
			if issue.ReleaseID == release.ID {
				issue.ReleaseID = ""
			}
		}
		for _, issueID := range vars.Input.IssueIDs {
			if issue := f.issueByID(issueID); issue != nil {
				issue.ReleaseID = release.ID
			}
		}
	}
	return map[string]any{
		"releaseUpdate": map[string]any{"success": true, "release": f.releasePayload(release)},
	}, ""
}

func (f *LinearFake) handleIssueLabels(raw json.RawMessage) (any, string) {
	var vars struct {
		Name   string `json:"name"`
		TeamID string `json:"teamId"`
	}
	if err := decodeFakeVars(raw, &vars); err != nil {
		return nil, err.Error()
	}
	if f.SkipLabelLookups > 0 {
		f.SkipLabelLookups--
		return map[string]any{"issueLabels": map[string]any{"nodes": []any{}}}, ""
	}
	nodes := []any{}
	for _, label := range f.Labels {
		if !strings.EqualFold(label.Name, vars.Name) {
			continue
		}
		if vars.TeamID != "" && label.TeamID != vars.TeamID {
			continue
		}
		nodes = append(nodes, map[string]any{
			"id":   label.ID,
			"name": label.Name,
			"team": map[string]string{"id": label.TeamID},
		})
	}
	return map[string]any{"issueLabels": map[string]any{"nodes": nodes}}, ""
}

func (f *LinearFake) handleIssueLabelCreate(raw json.RawMessage) (any, string) {
	var vars struct {
		Input struct {
			Name   string `json:"name"`
			TeamID string `json:"teamId"`
		} `json:"input"`
	}
	if err := decodeFakeVars(raw, &vars); err != nil {
		return nil, err.Error()
	}
	for _, label := range f.Labels {
		if strings.EqualFold(label.Name, vars.Input.Name) && label.TeamID == vars.Input.TeamID {
			return nil, fmt.Sprintf("label %q already exists on team", vars.Input.Name)
		}
	}
	label := &LinearFakeLabel{ID: f.nextOpaque("lbl"), Name: vars.Input.Name, TeamID: vars.Input.TeamID}
	f.Labels[label.ID] = label
	return map[string]any{
		"issueLabelCreate": map[string]any{"success": true, "issueLabel": map[string]string{"id": label.ID, "name": label.Name}},
	}, ""
}

func (f *LinearFake) handleCommentCreate(raw json.RawMessage) (any, string) {
	var vars struct {
		Input struct {
			IssueID string `json:"issueId"`
			Body    string `json:"body"`
		} `json:"input"`
	}
	if err := decodeFakeVars(raw, &vars); err != nil {
		return nil, err.Error()
	}
	if f.issueByID(vars.Input.IssueID) == nil {
		return nil, "issue not found"
	}
	comment := LinearFakeComment{ID: f.nextOpaque("cmt"), IssueID: f.issueByID(vars.Input.IssueID).ID, Body: vars.Input.Body}
	f.Comments = append(f.Comments, comment)
	return map[string]any{"commentCreate": map[string]any{"success": true, "comment": map[string]string{"id": comment.ID, "body": comment.Body}}}, ""
}

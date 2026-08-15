package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	DefaultLinearGraphQLURL = "https://api.linear.app/graphql"

	LinearEnvAPIURL  = "LINEAR_API_URL"
	LinearEnvTeamKey = "LINEAR_TEAM_KEY"

	linearBackend              = "linear"
	linearExternalKindIssue    = "issue"
	linearExternalKindRelease  = "release"
	linearExternalKindTeam     = "team"
	linearExternalKindStatus   = "status:"
	linearSyncLinked           = "linked"
	linearStatusOverrideEnvFmt = "LINEAR_STATUS_%s"
)

// LinearClient is a small GraphQL client for the Linear verbs Loaf needs.
// Endpoint is injectable so tests can point at httptest fakes.
type LinearClient struct {
	Endpoint   string
	APIKey     string
	HTTPClient *http.Client
}

// LinearState is one Linear workflow state.
type LinearState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// LinearTeam is a Linear team plus its workflow states.
type LinearTeam struct {
	ID     string
	Key    string
	States []LinearState
}

// LinearIssue is the Linear issue fields the adapter reads and writes.
type LinearIssue struct {
	ID          string
	Identifier  string
	Title       string
	Description string
	URL         string
	UpdatedAt   time.Time
	State       LinearState
	ParentID    string
	ParentKey   string
	ChildKeys   []string
	ChildIDs    []string
	LabelNames  []string
}

// LinearRelease is a Linear Release plus member issue identifiers.
type LinearRelease struct {
	ID        string
	Name      string
	IssueKeys []string
	IssueIDs  []string
}

// LinearCreateIssueInput is the subset of IssueCreateInput Loaf sends.
type LinearCreateIssueInput struct {
	TeamID      string
	Title       string
	Description string
	ParentID    string
	StateID     string
}

// LinearUpdateIssueInput is the subset of IssueUpdateInput Loaf sends.
// Title is intentionally absent: the tracker owns the name.
type LinearUpdateIssueInput struct {
	Description   *string
	StateID       string
	ReleaseID     string
	AddedLabelIDs []string
}

type linearGraphQLRequest struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName,omitempty"`
	Variables     map[string]any `json:"variables,omitempty"`
}

type linearGraphQLError struct {
	Message string `json:"message"`
}

type linearGraphQLResponse struct {
	Data   json.RawMessage      `json:"data"`
	Errors []linearGraphQLError `json:"errors"`
}

// NewLinearClient constructs a client against endpoint. Empty endpoint uses
// the public Linear GraphQL URL.
func NewLinearClient(endpoint, apiKey string) *LinearClient {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = DefaultLinearGraphQLURL
	}
	return &LinearClient{
		Endpoint:   endpoint,
		APIKey:     strings.TrimSpace(apiKey),
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func linearAPITokenEnv() string {
	return strings.Join([]string{"LINEAR", "API", "KEY"}, "_")
}

// LinearClientFromEnv builds a client from the Linear bearer env var and
// optional LINEAR_API_URL. Missing token is an error — callers decide how to phrase it.
func LinearClientFromEnv() (*LinearClient, error) {
	key := strings.TrimSpace(os.Getenv(linearAPITokenEnv()))
	if key == "" {
		return nil, fmt.Errorf("%s is not set", linearAPITokenEnv())
	}
	return NewLinearClient(os.Getenv(LinearEnvAPIURL), key), nil
}

func (c *LinearClient) httpClient() *http.Client {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *LinearClient) do(ctx context.Context, operation, query string, variables map[string]any, dest any) error {
	if c == nil {
		return fmt.Errorf("linear client is nil")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("%s is not set", linearAPITokenEnv())
	}
	payload, err := json.Marshal(linearGraphQLRequest{Query: query, OperationName: operation, Variables: variables})
	if err != nil {
		return fmt.Errorf("encode linear %s: %w", operation, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("linear %s: %w", operation, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.APIKey)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("linear %s: %w", operation, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read linear %s: %w", operation, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("linear %s: HTTP %d: %s", operation, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded linearGraphQLResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("decode linear %s: %w", operation, err)
	}
	if len(decoded.Errors) > 0 {
		messages := make([]string, 0, len(decoded.Errors))
		for _, item := range decoded.Errors {
			if strings.TrimSpace(item.Message) != "" {
				messages = append(messages, item.Message)
			}
		}
		return &linearGraphQLFailure{Operation: operation, Messages: messages}
	}
	if dest == nil {
		return nil
	}
	if len(decoded.Data) == 0 || string(decoded.Data) == "null" {
		return fmt.Errorf("linear %s: empty data", operation)
	}
	if err := json.Unmarshal(decoded.Data, dest); err != nil {
		return fmt.Errorf("decode linear %s data: %w", operation, err)
	}
	return nil
}

type linearGraphQLFailure struct {
	Operation string
	Messages  []string
}

func (e *linearGraphQLFailure) Error() string {
	if e == nil {
		return "linear graphql failed"
	}
	if len(e.Messages) == 0 {
		return fmt.Sprintf("linear %s failed", e.Operation)
	}
	return fmt.Sprintf("linear %s: %s", e.Operation, strings.Join(e.Messages, "; "))
}

func linearUnsupportedField(err error) bool {
	var failure *linearGraphQLFailure
	if !asLinearGraphQLFailure(err, &failure) {
		return false
	}
	for _, message := range failure.Messages {
		lower := strings.ToLower(message)
		if strings.Contains(lower, "cannot query field") || strings.Contains(lower, "unknown field") || strings.Contains(lower, "undefined field") {
			return true
		}
	}
	return false
}

func asLinearGraphQLFailure(err error, target **linearGraphQLFailure) bool {
	if err == nil {
		return false
	}
	if failure, ok := err.(*linearGraphQLFailure); ok {
		*target = failure
		return true
	}
	return false
}

const linearTeamByKeyQuery = `
query TeamByKey($key: String!) {
  teams(filter: { key: { eq: $key } }) {
    nodes {
      id
      key
      states { nodes { id name type } }
    }
  }
}`

const linearIssueQuery = `
query Issue($id: String!) {
  issue(id: $id) {
    id
    identifier
    title
    description
    url
    updatedAt
    state { id name type }
    parent { id identifier }
    children { nodes { id identifier } }
    labels { nodes { id name } }
  }
}`

const linearIssueCreateMutation = `
mutation IssueCreate($input: IssueCreateInput!) {
  issueCreate(input: $input) {
    success
    issue {
      id
      identifier
      title
      description
      url
      updatedAt
      state { id name type }
      parent { id identifier }
    }
  }
}`

const linearIssueUpdateMutation = `
mutation IssueUpdate($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
    issue {
      id
      identifier
      title
      description
      url
      updatedAt
      state { id name type }
      parent { id identifier }
      labels { nodes { id name } }
    }
  }
}`

const linearReleasesQuery = `
query Releases($first: Int) {
  releases(first: $first) {
    nodes {
      id
      name
      issues { nodes { id identifier } }
    }
  }
}`

const linearReleaseQuery = `
query Release($id: String!) {
  release(id: $id) {
    id
    name
    issues { nodes { id identifier } }
  }
}`

const linearReleaseCreateMutation = `
mutation ReleaseCreate($input: ReleaseCreateInput!) {
  releaseCreate(input: $input) {
    success
    release {
      id
      name
      issues { nodes { id identifier } }
    }
  }
}`

const linearReleaseUpdateMutation = `
mutation ReleaseUpdate($id: String!, $input: ReleaseUpdateInput!) {
  releaseUpdate(id: $id, input: $input) {
    success
    release {
      id
      name
      issues { nodes { id identifier } }
    }
  }
}`

const linearIssueLabelsQuery = `
query IssueLabels($name: String!, $teamId: String!) {
  issueLabels(filter: { name: { eq: $name }, team: { id: { eq: $teamId } } }) {
    nodes { id name team { id } }
  }
}`

const linearIssueLabelCreateMutation = `
mutation IssueLabelCreate($input: IssueLabelCreateInput!) {
  issueLabelCreate(input: $input) {
    success
    issueLabel { id name }
  }
}`

const linearCommentCreateMutation = `
mutation CommentCreate($input: CommentCreateInput!) {
  commentCreate(input: $input) {
    success
    comment { id body }
  }
}`

type linearTeamByKeyData struct {
	Teams struct {
		Nodes []struct {
			ID     string `json:"id"`
			Key    string `json:"key"`
			States struct {
				Nodes []LinearState `json:"nodes"`
			} `json:"states"`
		} `json:"nodes"`
	} `json:"teams"`
}

type linearIssueNode struct {
	ID          string      `json:"id"`
	Identifier  string      `json:"identifier"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	URL         string      `json:"url"`
	UpdatedAt   string      `json:"updatedAt"`
	State       LinearState `json:"state"`
	Parent      *struct {
		ID         string `json:"id"`
		Identifier string `json:"identifier"`
	} `json:"parent"`
	Children *struct {
		Nodes []struct {
			ID         string `json:"id"`
			Identifier string `json:"identifier"`
		} `json:"nodes"`
	} `json:"children"`
	Labels *struct {
		Nodes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
}

func decodeLinearIssue(node linearIssueNode) (LinearIssue, error) {
	updatedAt, err := parseLinearTime(node.UpdatedAt)
	if err != nil {
		return LinearIssue{}, err
	}
	issue := LinearIssue{
		ID:          node.ID,
		Identifier:  node.Identifier,
		Title:       node.Title,
		Description: node.Description,
		URL:         node.URL,
		UpdatedAt:   updatedAt,
		State:       node.State,
	}
	if node.Parent != nil {
		issue.ParentID = node.Parent.ID
		issue.ParentKey = node.Parent.Identifier
	}
	if node.Children != nil {
		for _, child := range node.Children.Nodes {
			if child.Identifier != "" {
				issue.ChildKeys = append(issue.ChildKeys, child.Identifier)
			}
			if child.ID != "" {
				issue.ChildIDs = append(issue.ChildIDs, child.ID)
			}
		}
	}
	if node.Labels != nil {
		for _, label := range node.Labels.Nodes {
			if label.Name != "" {
				issue.LabelNames = append(issue.LabelNames, label.Name)
			}
		}
	}
	return issue, nil
}

func parseLinearTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse linear time %q: %w", value, err)
	}
	return parsed, nil
}

// TeamByKey resolves a Linear team and its workflow states.
func (c *LinearClient) TeamByKey(ctx context.Context, key string) (LinearTeam, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return LinearTeam{}, fmt.Errorf("linear team key must be nonempty")
	}
	var data linearTeamByKeyData
	if err := c.do(ctx, "TeamByKey", linearTeamByKeyQuery, map[string]any{"key": key}, &data); err != nil {
		return LinearTeam{}, err
	}
	if len(data.Teams.Nodes) == 0 {
		return LinearTeam{}, fmt.Errorf("linear team %q not found", key)
	}
	node := data.Teams.Nodes[0]
	return LinearTeam{ID: node.ID, Key: node.Key, States: append([]LinearState(nil), node.States.Nodes...)}, nil
}

// Issue fetches one Linear issue by identifier or UUID.
func (c *LinearClient) Issue(ctx context.Context, id string) (LinearIssue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return LinearIssue{}, fmt.Errorf("linear issue id must be nonempty")
	}
	var data struct {
		Issue *linearIssueNode `json:"issue"`
	}
	if err := c.do(ctx, "Issue", linearIssueQuery, map[string]any{"id": id}, &data); err != nil {
		return LinearIssue{}, err
	}
	if data.Issue == nil {
		return LinearIssue{}, fmt.Errorf("linear issue %q not found", id)
	}
	return decodeLinearIssue(*data.Issue)
}

// CreateIssue creates a Linear issue and returns the minted identifier.
func (c *LinearClient) CreateIssue(ctx context.Context, input LinearCreateIssueInput) (LinearIssue, error) {
	payload := map[string]any{
		"teamId": input.TeamID,
		"title":  input.Title,
	}
	if strings.TrimSpace(input.Description) != "" {
		payload["description"] = input.Description
	}
	if strings.TrimSpace(input.ParentID) != "" {
		payload["parentId"] = input.ParentID
	}
	if strings.TrimSpace(input.StateID) != "" {
		payload["stateId"] = input.StateID
	}
	var data struct {
		IssueCreate struct {
			Success bool             `json:"success"`
			Issue   *linearIssueNode `json:"issue"`
		} `json:"issueCreate"`
	}
	if err := c.do(ctx, "IssueCreate", linearIssueCreateMutation, map[string]any{"input": payload}, &data); err != nil {
		return LinearIssue{}, err
	}
	if !data.IssueCreate.Success || data.IssueCreate.Issue == nil {
		return LinearIssue{}, fmt.Errorf("linear IssueCreate failed")
	}
	return decodeLinearIssue(*data.IssueCreate.Issue)
}

// UpdateIssue writes description, state, labels, or release membership.
func (c *LinearClient) UpdateIssue(ctx context.Context, id string, input LinearUpdateIssueInput) (LinearIssue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return LinearIssue{}, fmt.Errorf("linear issue id must be nonempty")
	}
	payload := map[string]any{}
	if input.Description != nil {
		payload["description"] = *input.Description
	}
	if strings.TrimSpace(input.StateID) != "" {
		payload["stateId"] = input.StateID
	}
	if strings.TrimSpace(input.ReleaseID) != "" {
		payload["releaseId"] = input.ReleaseID
	}
	if len(input.AddedLabelIDs) > 0 {
		payload["addedLabelIds"] = append([]string(nil), input.AddedLabelIDs...)
	}
	if len(payload) == 0 {
		return c.Issue(ctx, id)
	}
	var data struct {
		IssueUpdate struct {
			Success bool             `json:"success"`
			Issue   *linearIssueNode `json:"issue"`
		} `json:"issueUpdate"`
	}
	if err := c.do(ctx, "IssueUpdate", linearIssueUpdateMutation, map[string]any{"id": id, "input": payload}, &data); err != nil {
		return LinearIssue{}, err
	}
	if !data.IssueUpdate.Success || data.IssueUpdate.Issue == nil {
		return LinearIssue{}, fmt.Errorf("linear IssueUpdate failed")
	}
	return decodeLinearIssue(*data.IssueUpdate.Issue)
}

type linearReleaseNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Issues *struct {
		Nodes []struct {
			ID         string `json:"id"`
			Identifier string `json:"identifier"`
		} `json:"nodes"`
	} `json:"issues"`
}

func decodeLinearRelease(node linearReleaseNode) LinearRelease {
	release := LinearRelease{ID: node.ID, Name: node.Name}
	if node.Issues != nil {
		for _, issue := range node.Issues.Nodes {
			if issue.Identifier != "" {
				release.IssueKeys = append(release.IssueKeys, issue.Identifier)
			}
			if issue.ID != "" {
				release.IssueIDs = append(release.IssueIDs, issue.ID)
			}
		}
	}
	return release
}

// ReleasesSupported reports whether the workspace exposes Linear Releases.
func (c *LinearClient) ReleasesSupported(ctx context.Context) (bool, error) {
	var data struct {
		Releases *struct {
			Nodes []linearReleaseNode `json:"nodes"`
		} `json:"releases"`
	}
	err := c.do(ctx, "Releases", linearReleasesQuery, map[string]any{"first": 1}, &data)
	if err == nil {
		return true, nil
	}
	if linearUnsupportedField(err) {
		return false, nil
	}
	return false, err
}

// Release fetches one Linear release by id or name.
func (c *LinearClient) Release(ctx context.Context, id string) (LinearRelease, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return LinearRelease{}, fmt.Errorf("linear release id must be nonempty")
	}
	var data struct {
		Release *linearReleaseNode `json:"release"`
	}
	if err := c.do(ctx, "Release", linearReleaseQuery, map[string]any{"id": id}, &data); err != nil {
		return LinearRelease{}, err
	}
	if data.Release == nil {
		return LinearRelease{}, fmt.Errorf("linear release %q not found", id)
	}
	return decodeLinearRelease(*data.Release), nil
}

// CreateRelease creates a Linear Release. issueIDs are Linear UUIDs.
func (c *LinearClient) CreateRelease(ctx context.Context, name string, issueIDs []string) (LinearRelease, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return LinearRelease{}, fmt.Errorf("linear release name must be nonempty")
	}
	payload := map[string]any{"name": name}
	if len(issueIDs) > 0 {
		payload["issueIds"] = append([]string(nil), issueIDs...)
	}
	var data struct {
		ReleaseCreate struct {
			Success bool               `json:"success"`
			Release *linearReleaseNode `json:"release"`
		} `json:"releaseCreate"`
	}
	if err := c.do(ctx, "ReleaseCreate", linearReleaseCreateMutation, map[string]any{"input": payload}, &data); err != nil {
		return LinearRelease{}, err
	}
	if !data.ReleaseCreate.Success || data.ReleaseCreate.Release == nil {
		return LinearRelease{}, fmt.Errorf("linear ReleaseCreate failed")
	}
	return decodeLinearRelease(*data.ReleaseCreate.Release), nil
}

// UpdateRelease updates a Linear Release name and membership.
func (c *LinearClient) UpdateRelease(ctx context.Context, id, name string, issueIDs []string) (LinearRelease, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return LinearRelease{}, fmt.Errorf("linear release id must be nonempty")
	}
	payload := map[string]any{}
	if strings.TrimSpace(name) != "" {
		payload["name"] = name
	}
	if issueIDs != nil {
		payload["issueIds"] = append([]string(nil), issueIDs...)
	}
	var data struct {
		ReleaseUpdate struct {
			Success bool               `json:"success"`
			Release *linearReleaseNode `json:"release"`
		} `json:"releaseUpdate"`
	}
	if err := c.do(ctx, "ReleaseUpdate", linearReleaseUpdateMutation, map[string]any{"id": id, "input": payload}, &data); err != nil {
		return LinearRelease{}, err
	}
	if !data.ReleaseUpdate.Success || data.ReleaseUpdate.Release == nil {
		return LinearRelease{}, fmt.Errorf("linear ReleaseUpdate failed")
	}
	return decodeLinearRelease(*data.ReleaseUpdate.Release), nil
}

// FindLabelID returns the id of a team-scoped label, or empty if missing.
func (c *LinearClient) FindLabelID(ctx context.Context, teamID, name string) (string, error) {
	name = strings.TrimSpace(name)
	teamID = strings.TrimSpace(teamID)
	if name == "" {
		return "", fmt.Errorf("linear label name must be nonempty")
	}
	if teamID == "" {
		return "", fmt.Errorf("linear label lookup requires team id")
	}
	var data struct {
		IssueLabels struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Team *struct {
					ID string `json:"id"`
				} `json:"team"`
			} `json:"nodes"`
		} `json:"issueLabels"`
	}
	if err := c.do(ctx, "IssueLabels", linearIssueLabelsQuery, map[string]any{"name": name, "teamId": teamID}, &data); err != nil {
		return "", err
	}
	for _, node := range data.IssueLabels.Nodes {
		if !strings.EqualFold(node.Name, name) {
			continue
		}
		if node.Team != nil && strings.TrimSpace(node.Team.ID) != "" && node.Team.ID != teamID {
			continue
		}
		return node.ID, nil
	}
	return "", nil
}

// CreateLabel creates a team-scoped Linear label.
func (c *LinearClient) CreateLabel(ctx context.Context, teamID, name string) (string, error) {
	name = strings.TrimSpace(name)
	teamID = strings.TrimSpace(teamID)
	if name == "" || teamID == "" {
		return "", fmt.Errorf("linear label create requires team id and name")
	}
	var data struct {
		IssueLabelCreate struct {
			Success    bool `json:"success"`
			IssueLabel *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"issueLabel"`
		} `json:"issueLabelCreate"`
	}
	if err := c.do(ctx, "IssueLabelCreate", linearIssueLabelCreateMutation, map[string]any{"input": map[string]any{"name": name, "teamId": teamID}}, &data); err != nil {
		return "", err
	}
	if !data.IssueLabelCreate.Success || data.IssueLabelCreate.IssueLabel == nil {
		return "", fmt.Errorf("linear IssueLabelCreate failed")
	}
	return data.IssueLabelCreate.IssueLabel.ID, nil
}

// CreateComment adds a comment on a Linear issue.
func (c *LinearClient) CreateComment(ctx context.Context, issueID, body string) error {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return fmt.Errorf("linear comment requires an issue id")
	}
	var data struct {
		CommentCreate struct {
			Success bool `json:"success"`
		} `json:"commentCreate"`
	}
	if err := c.do(ctx, "CommentCreate", linearCommentCreateMutation, map[string]any{"input": map[string]any{"issueId": issueID, "body": body}}, &data); err != nil {
		return err
	}
	if !data.CommentCreate.Success {
		return fmt.Errorf("linear CommentCreate failed")
	}
	return nil
}

// EnsureLabel returns an existing label id or creates the label on the team.
// A create race retries with a team-scoped find before failing.
func (c *LinearClient) EnsureLabel(ctx context.Context, teamID, name string) (string, error) {
	id, err := c.FindLabelID(ctx, teamID, name)
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}
	id, err = c.CreateLabel(ctx, teamID, name)
	if err == nil {
		return id, nil
	}
	found, findErr := c.FindLabelID(ctx, teamID, name)
	if findErr != nil {
		return "", findErr
	}
	if found != "" {
		return found, nil
	}
	return "", err
}

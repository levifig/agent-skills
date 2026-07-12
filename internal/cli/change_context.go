package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

const (
	journalContextLayerActiveChanges = "active-changes"
	activeChangeCursorVersion        = 2
	activeChangeCursorInvalidCode    = "journal-context-cursor-invalid"
	activeChangeCursorStaleCode      = "journal-context-cursor-stale"
	changeSourceUnavailableCode      = "change_source_unavailable"
)

type activeChangeItem struct {
	Slug               string   `json:"slug"`
	ChangePath         string   `json:"change_path"`
	SourceSHA256       string   `json:"source_sha256"`
	BranchProvenance   string   `json:"branch_provenance"`
	CurrentBranch      string   `json:"current_branch"`
	CurrentBranchMatch bool     `json:"current_branch_match"`
	RetainedAtHEAD     bool     `json:"retained_at_head"`
	WorktreeState      string   `json:"worktree_state"`
	Lineage            string   `json:"lineage"`
	Predecessor        string   `json:"predecessor"`
	ReleaseAfter       string   `json:"release_after"`
	ActiveReasons      []string `json:"active_reasons"`
	Findings           []string `json:"findings"`
	Gaps               []string `json:"gaps"`
}

type activeChangeLayer struct {
	Available      bool               `json:"source_available"`
	AvailableCount int                `json:"available_count"`
	ShownCount     int                `json:"shown_count"`
	Truncated      bool               `json:"truncated"`
	Cursor         string             `json:"cursor,omitempty"`
	ExpandCommand  string             `json:"expand_command"`
	Items          []activeChangeItem `json:"items"`
}

type activeChangeSource struct {
	HEAD        string
	Branch      string
	Detached    bool
	LineageKeys []string
	Items       []activeChangeItem
	Fingerprint string
}

type activeChangeSourceRecord struct {
	Path             string   `json:"path"`
	WorkSHA256       string   `json:"work_sha256"`
	HEADSHA256       string   `json:"head_sha256"`
	WorktreeState    string   `json:"worktree_state"`
	WorktreeEvidence []string `json:"worktree_evidence"`
	ActiveReasons    []string `json:"active_reasons"`
	Findings         []string `json:"findings"`
	Gaps             []string `json:"gaps"`
}

type activeChangeCursor struct {
	Version     int    `json:"v"`
	Layer       string `json:"layer"`
	ProjectID   string `json:"project"`
	Branch      string `json:"branch"`
	Fingerprint string `json:"fingerprint"`
	Limit       int    `json:"limit"`
	LastSlug    string `json:"last_slug"`
	LastPath    string `json:"last_path"`
	ShownCount  int    `json:"shown_count"`
	Checksum    string `json:"checksum"`
}

type journalContextCursorError struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	RestartCommand string `json:"restart_command,omitempty"`
}

func (e *journalContextCursorError) Error() string { return e.Code + ": " + e.Message }

func discoverActiveChanges(rootPath string) (activeChangeSource, error) {
	return discoverActiveChangesWithOutput(rootPath, commandOutput)
}

func discoverActiveChangesWithOutput(rootPath string, outputCommand changeGitOutput) (activeChangeSource, error) {
	head, err := outputCommand(rootPath, "git", "rev-parse", "HEAD")
	if err != nil {
		return activeChangeSource{}, fmt.Errorf("resolve Change source HEAD: %w", err)
	}
	branch, err := outputCommand(rootPath, "git", "branch", "--show-current")
	if err != nil {
		return activeChangeSource{}, fmt.Errorf("resolve Change source branch: %w", err)
	}
	head = strings.TrimSpace(head)
	branch = strings.TrimSpace(branch)
	workNodes, err := loadChangeNodes(rootPath)
	if err != nil {
		return activeChangeSource{}, fmt.Errorf("read working-tree Changes: %w", err)
	}
	headNodes, err := loadChangeNodesAtHEADWithOutput(rootPath, outputCommand)
	if err != nil {
		return activeChangeSource{}, err
	}

	workByPath := make(map[string]changeNode, len(workNodes))
	headByPath := make(map[string]changeNode, len(headNodes))
	paths := map[string]bool{}
	for _, node := range workNodes {
		workByPath[node.ChangeFile] = node
		paths[node.ChangeFile] = true
	}
	for _, node := range headNodes {
		headByPath[node.ChangeFile] = node
		paths[node.ChangeFile] = true
	}
	unionPaths := sortedKeys(paths)
	union := make([]changeNode, 0, len(unionPaths))
	lineages := map[string]bool{}
	retainLineage := func(node changeNode) {
		parsed := parseChangeFrontmatter(node.Content)
		if parsed.AtByteOne && node.Slug != "" && node.Lineage != "" {
			lineages[node.Lineage] = true
		}
	}
	for _, path := range unionPaths {
		if node, ok := headByPath[path]; ok {
			retainLineage(node)
		}
		if node, ok := workByPath[path]; ok {
			retainLineage(node)
		}
		node, ok := workByPath[path]
		if !ok {
			node = headByPath[path]
		}
		union = append(union, node)
	}
	graph := deriveChangeGraph(union)
	records := make([]activeChangeSourceRecord, 0, len(unionPaths))
	items := []activeChangeItem{}
	for _, path := range unionPaths {
		workNode, inWork := workByPath[path]
		headNode, inHEAD := headByPath[path]
		node := workNode
		if !inWork {
			node = headNode
		}
		workDigest, headDigest := "", ""
		if inWork {
			workDigest = sha256Text(workNode.Content)
		}
		if inHEAD {
			headDigest = sha256Text(headNode.Content)
		}
		statusOutput, err := outputCommand(rootPath, "git", "status", "--porcelain=v1", "--untracked-files=all", "--", node.Folder)
		if err != nil {
			return activeChangeSource{}, fmt.Errorf("inspect working-tree evidence for %s: %w", node.Folder, err)
		}
		worktreeEvidence := []string{}
		for _, line := range strings.Split(strings.TrimSpace(statusOutput), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				worktreeEvidence = append(worktreeEvidence, line)
			}
		}
		sort.Strings(worktreeEvidence)
		worktreeState := "clean"
		switch {
		case inHEAD && !inWork:
			worktreeState = "deleted"
		case !inHEAD && inWork:
			worktreeState = "untracked"
		case workDigest != headDigest || len(worktreeEvidence) != 0:
			worktreeState = "modified"
		}
		findings := graph.findingsForChange(node)
		gaps := graph.gapsForLineage(node.Lineage)
		reasons := []string{}
		if branch != "" && node.Branch == branch {
			reasons = append(reasons, "current_branch_match")
		}
		if worktreeState != "clean" {
			reasons = append(reasons, "working_tree_change")
		}
		if node.Lineage != "" && (len(findings) != 0 || len(gaps) != 0) {
			reasons = append(reasons, "lineage_unresolved")
		}
		records = append(records, activeChangeSourceRecord{Path: path, WorkSHA256: workDigest, HEADSHA256: headDigest, WorktreeState: worktreeState, WorktreeEvidence: worktreeEvidence, ActiveReasons: reasons, Findings: findings, Gaps: gaps})
		if len(reasons) == 0 {
			continue
		}
		sourceDigest := workDigest
		if sourceDigest == "" {
			sourceDigest = headDigest
		}
		items = append(items, activeChangeItem{
			Slug: node.Slug, ChangePath: path, SourceSHA256: sourceDigest, BranchProvenance: node.Branch, CurrentBranch: branch,
			CurrentBranchMatch: branch != "" && node.Branch == branch, RetainedAtHEAD: inHEAD,
			WorktreeState: worktreeState, Lineage: node.Lineage, Predecessor: node.Predecessor,
			ReleaseAfter: node.ReleaseAfter, ActiveReasons: reasons, Findings: findings, Gaps: gaps,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Slug != items[j].Slug {
			return items[i].Slug < items[j].Slug
		}
		return items[i].ChangePath < items[j].ChangePath
	})
	fingerprintBytes, err := json.Marshal(struct {
		Protocol string                     `json:"protocol"`
		HEAD     string                     `json:"head"`
		Branch   string                     `json:"branch"`
		Detached bool                       `json:"detached"`
		Records  []activeChangeSourceRecord `json:"records"`
	}{"context-change-source-v1", head, branch, branch == "", records})
	if err != nil {
		return activeChangeSource{}, fmt.Errorf("fingerprint Change source: %w", err)
	}
	return activeChangeSource{HEAD: head, Branch: branch, Detached: branch == "", LineageKeys: sortedKeys(lineages), Items: items, Fingerprint: sha256Text(string(fingerprintBytes))}, nil
}

func activeChangesPage(source activeChangeSource, projectID, branchScope string, limit int, encodedCursor string) (activeChangeLayer, error) {
	start := 0
	shownBefore := 0
	if encodedCursor != "" {
		cursor, err := decodeActiveChangeCursor(encodedCursor)
		if err != nil {
			return activeChangeLayer{}, err
		}
		if cursor.ProjectID != projectID || cursor.Layer != journalContextLayerActiveChanges || cursor.Branch != branchScope {
			return activeChangeLayer{}, &journalContextCursorError{Code: activeChangeCursorInvalidCode, Message: "cursor belongs to a different project, layer, or branch"}
		}
		if cursor.Limit != limit {
			return activeChangeLayer{}, &journalContextCursorError{Code: activeChangeCursorInvalidCode, Message: "cursor belongs to a different layer limit"}
		}
		if cursor.Fingerprint != source.Fingerprint {
			return activeChangeLayer{}, &journalContextCursorError{Code: activeChangeCursorStaleCode, Message: "journal context cursor is stale for layer active-changes; rerun without --cursor", RestartCommand: activeChangeRestartCommand(branchScope)}
		}
		shownBefore = cursor.ShownCount
		for i, item := range source.Items {
			if item.Slug == cursor.LastSlug && item.ChangePath == cursor.LastPath {
				start = i + 1
				break
			}
		}
		if start == 0 || start != shownBefore {
			return activeChangeLayer{}, &journalContextCursorError{Code: activeChangeCursorInvalidCode, Message: "cursor continuation does not match the active Change page"}
		}
	}
	end := min(start+limit, len(source.Items))
	items := append([]activeChangeItem(nil), source.Items[start:end]...)
	if items == nil {
		items = []activeChangeItem{}
	}
	layer := activeChangeLayer{Available: true, AvailableCount: len(source.Items), ShownCount: len(items), Truncated: end < len(source.Items), ExpandCommand: activeChangeRestartCommand(branchScope), Items: items}
	if layer.Truncated {
		last := source.Items[end-1]
		layer.Cursor = encodeActiveChangeCursor(activeChangeCursor{Layer: journalContextLayerActiveChanges, ProjectID: projectID, Branch: branchScope, Fingerprint: source.Fingerprint, Limit: limit, LastSlug: last.Slug, LastPath: last.ChangePath, ShownCount: shownBefore + len(items)})
		layer.ExpandCommand = "loaf journal context --layer active-changes --cursor " + journalContextShellQuote(layer.Cursor)
	}
	return layer, nil
}

func unavailableActiveChanges() activeChangeLayer {
	return activeChangeLayer{Available: false, ExpandCommand: "loaf journal context --layer active-changes", Items: []activeChangeItem{}}
}

func encodeActiveChangeCursor(cursor activeChangeCursor) string {
	cursor.Version = activeChangeCursorVersion
	cursor.Checksum = activeChangeCursorChecksum(cursor)
	data, err := json.Marshal(cursor)
	if err != nil {
		panic(fmt.Sprintf("marshal active Change cursor: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeActiveChangeCursor(encoded string) (activeChangeCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return activeChangeCursor{}, &journalContextCursorError{Code: activeChangeCursorInvalidCode, Message: "cursor is not valid base64url"}
	}
	var cursor activeChangeCursor
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return activeChangeCursor{}, &journalContextCursorError{Code: activeChangeCursorInvalidCode, Message: "cursor payload is malformed"}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return activeChangeCursor{}, &journalContextCursorError{Code: activeChangeCursorInvalidCode, Message: "cursor payload has trailing data"}
	}
	if cursor.Version != activeChangeCursorVersion || cursor.Layer == "" || cursor.ProjectID == "" || cursor.Limit <= 0 || cursor.LastPath == "" || cursor.ShownCount < 1 || cursor.Checksum == "" || cursor.Checksum != activeChangeCursorChecksum(cursor) {
		return activeChangeCursor{}, &journalContextCursorError{Code: activeChangeCursorInvalidCode, Message: "cursor payload has invalid fields or checksum"}
	}
	return cursor, nil
}

func activeChangeCursorChecksum(cursor activeChangeCursor) string {
	cursor.Checksum = ""
	data, err := json.Marshal(cursor)
	if err != nil {
		panic(fmt.Sprintf("marshal active Change cursor checksum: %v", err))
	}
	return sha256Text(string(data))
}

func activeChangeRestartCommand(branch string) string {
	command := "loaf journal context --layer active-changes"
	if branch != "" {
		command += " --branch " + journalContextShellQuote(branch)
	}
	return command
}

func sha256Text(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func writeActiveChangeItemHuman(out io.Writer, item activeChangeItem) {
	fmt.Fprintf(out, "    %s (%s)\n", firstNonEmpty(item.Slug, "(unparseable slug)"), item.ChangePath)
	fmt.Fprintf(out, "      worktree: %s; retained at HEAD: %t; branch provenance: %s; current branch: %s; current match: %t\n", item.WorktreeState, item.RetainedAtHEAD, firstNonEmpty(item.BranchProvenance, "(none)"), firstNonEmpty(item.CurrentBranch, "(detached)"), item.CurrentBranchMatch)
	fmt.Fprintf(out, "      active because: %s\n", strings.Join(item.ActiveReasons, ", "))
	if item.Lineage != "" {
		fmt.Fprintf(out, "      lineage: %s; predecessor: %s; release-after: %s\n", item.Lineage, firstNonEmpty(item.Predecessor, "(root)"), firstNonEmpty(item.ReleaseAfter, "(none)"))
	}
	for _, finding := range item.Findings {
		fmt.Fprintf(out, "      finding: %s\n", finding)
	}
	for _, gap := range item.Gaps {
		fmt.Fprintf(out, "      gap: %s\n", gap)
	}
}

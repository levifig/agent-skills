package state

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const journalContextCursorVersion = 1

const (
	JournalContextCursorInvalidCode = "journal-context-cursor-invalid"
	JournalContextCursorStaleCode   = "journal-context-cursor-stale"
)

type journalContextCursor struct {
	Version     int    `json:"v"`
	Layer       string `json:"layer"`
	ProjectID   string `json:"project"`
	Branch      string `json:"branch"`
	Watermark   int64  `json:"watermark"`
	Limit       int    `json:"limit"`
	After       string `json:"after"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type JournalContextCursorInvalidError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *JournalContextCursorInvalidError) Error() string { return e.Code + ": " + e.Message }

type JournalContextCursorStaleError struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	RestartCommand string `json:"restart_command"`
}

func (e *JournalContextCursorStaleError) Error() string { return e.Code + ": " + e.Message }

func newJournalContextCursorInvalid(message string) error {
	return &JournalContextCursorInvalidError{Code: JournalContextCursorInvalidCode, Message: message}
}

func newJournalContextCursorStale(branch, layer string, limit int) error {
	return &JournalContextCursorStaleError{Code: JournalContextCursorStaleCode, Message: "context candidates or source filter changed; restart expansion", RestartCommand: journalContextExpandCommand(layer, limit, branch, "")}
}

func encodeJournalContextCursor(cursor journalContextCursor) string {
	cursor.Version = journalContextCursorVersion
	data, err := json.Marshal(cursor)
	if err != nil {
		panic(fmt.Sprintf("marshal journal context cursor: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeAndValidateJournalContextCursor(encoded, expectedLayer, projectID, branch string) (*journalContextCursor, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, newJournalContextCursorInvalid("cursor is not valid base64url")
	}
	var cursor journalContextCursor
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return nil, newJournalContextCursorInvalid("cursor payload is malformed")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, newJournalContextCursorInvalid("cursor payload has trailing data")
	}
	if cursor.Version != journalContextCursorVersion || !validJournalContextCursorLayer(cursor.Layer) || cursor.ProjectID == "" || cursor.Watermark < 0 || cursor.Limit <= 0 || cursor.After == "" {
		return nil, newJournalContextCursorInvalid("cursor payload has invalid fields")
	}
	if cursor.ProjectID != projectID {
		return nil, newJournalContextCursorInvalid("cursor belongs to a different project")
	}
	if cursor.Branch != branch {
		return nil, newJournalContextCursorInvalid("cursor belongs to a different branch")
	}
	if expectedLayer != "" && cursor.Layer != expectedLayer {
		return nil, newJournalContextCursorInvalid("cursor belongs to a different layer")
	}
	return &cursor, nil
}

func validJournalContextCursorLayer(layer string) bool {
	switch layer {
	case JournalContextLayerLineage, JournalContextLayerBlockers, JournalContextLayerDeferrals, JournalContextLayerBranch, JournalContextLayerTasks:
		return true
	default:
		return false
	}
}

func cursorForLayer(cursor *journalContextCursor, layer string) *journalContextCursor {
	if cursor != nil && cursor.Layer == layer {
		return cursor
	}
	return nil
}

func journalContextExpandCommand(layer string, limit int, branch, cursor string) string {
	command := "loaf journal context --layer " + journalContextLayerOption(layer) + " --limit " + fmt.Sprintf("%d", limit)
	if branch != "" {
		command += " --branch " + posixShellQuote(branch)
	}
	if cursor != "" {
		command += " --cursor " + posixShellQuote(cursor)
	}
	return command
}

func journalContextLayerOption(layer string) string {
	switch layer {
	case JournalContextLayerSynthesis:
		return "project-synthesis"
	case JournalContextLayerCheckpoint:
		return "scoped-checkpoint"
	case JournalContextLayerLineage:
		return "active-lineage"
	case JournalContextLayerBlockers:
		return "unresolved-blockers"
	case JournalContextLayerDeferrals:
		return "deferred-intent"
	case JournalContextLayerBranch:
		return "branch-recency"
	case JournalContextLayerTasks:
		return "transitional-tasks"
	default:
		panic("unknown journal context layer " + layer)
	}
}

func journalContextJournalPage(candidates []journalContextJournalCandidate, limit int, cursor *journalContextCursor, layer, projectID, branch string, watermark int64, fingerprint string) JournalContextJournalLayer {
	result, _ := journalContextJournalPageChecked(candidates, limit, cursor, layer, projectID, branch, watermark, fingerprint)
	return result
}

func journalContextJournalPageChecked(candidates []journalContextJournalCandidate, limit int, cursor *journalContextCursor, layer, projectID, branch string, watermark int64, fingerprint string) (JournalContextJournalLayer, error) {
	start := 0
	if cursor != nil {
		start = findJournalContextAfter(len(candidates), func(i int) string { return journalContextJournalKey(candidates[i]) }, cursor.After)
		if start < 0 {
			return JournalContextJournalLayer{}, newJournalContextCursorInvalid("cursor continuation key was not found")
		}
	}
	end := min(start+limit, len(candidates))
	items := make([]JournalEntryRecord, 0, end-start)
	for _, candidate := range candidates[start:end] {
		items = append(items, candidate.Entry)
	}
	layerResult := JournalContextJournalLayer{Available: true, AvailableCount: len(candidates), ShownCount: len(items), Truncated: end < len(candidates), ExpandCommand: journalContextExpandCommand(layer, limit, branch, ""), Items: items}
	if layerResult.Truncated {
		next := encodeJournalContextCursor(journalContextCursor{Layer: layer, ProjectID: projectID, Branch: branch, Watermark: watermark, Limit: limit, After: journalContextJournalKey(candidates[end-1]), Fingerprint: fingerprint})
		layerResult.Cursor = next
		layerResult.ExpandCommand = journalContextExpandCommand(layer, limit, branch, next)
	}
	return layerResult, nil
}

func journalContextCheckpointPage(candidates []journalContextJournalCandidate, branch string) JournalContextCheckpointLayer {
	items := []JournalContextCheckpointItem{}
	if len(candidates) > 0 {
		entry := candidates[0].Entry
		items = append(items, JournalContextCheckpointItem{Entry: entry, Scope: entry.Scope, ProjectSynthesis: false, Label: "latest checkpoint (not project synthesis)"})
	}
	return JournalContextCheckpointLayer{Available: true, AvailableCount: len(items), ShownCount: len(items), Truncated: false, ExpandCommand: journalContextExpandCommand(JournalContextLayerCheckpoint, journalContextSynthesisLimit, branch, ""), Items: items}
}

func journalContextBlockerPage(candidates []journalContextBlockerCandidate, limit int, cursor *journalContextCursor, projectID, branch string, watermark int64) (JournalContextBlockerLayer, error) {
	start := 0
	if cursor != nil {
		start = findJournalContextAfter(len(candidates), func(i int) string { return journalContextBlockerKey(candidates[i]) }, cursor.After)
		if start < 0 {
			return JournalContextBlockerLayer{}, newJournalContextCursorInvalid("cursor continuation key was not found")
		}
	}
	end := min(start+limit, len(candidates))
	items := make([]JournalContextBlockerItem, 0, end-start)
	for _, candidate := range candidates[start:end] {
		items = append(items, candidate.Item)
	}
	layer := JournalContextBlockerLayer{Available: true, AvailableCount: len(candidates), ShownCount: len(items), Truncated: end < len(candidates), ExpandCommand: journalContextExpandCommand(JournalContextLayerBlockers, limit, branch, ""), Items: items}
	if layer.Truncated {
		layer.Cursor = encodeJournalContextCursor(journalContextCursor{Layer: JournalContextLayerBlockers, ProjectID: projectID, Branch: branch, Watermark: watermark, Limit: limit, After: journalContextBlockerKey(candidates[end-1])})
		layer.ExpandCommand = journalContextExpandCommand(JournalContextLayerBlockers, limit, branch, layer.Cursor)
	}
	return layer, nil
}

func journalContextDeferralPage(candidates []journalContextDeferralCandidate, limit int, cursor *journalContextCursor, projectID, branch string, watermark int64, fingerprint string) (JournalContextDeferralLayer, error) {
	start := 0
	if cursor != nil {
		start = findJournalContextAfter(len(candidates), func(i int) string { return journalContextDeferralKey(candidates[i]) }, cursor.After)
		if start < 0 {
			return JournalContextDeferralLayer{}, newJournalContextCursorInvalid("cursor continuation key was not found")
		}
	}
	end := min(start+limit, len(candidates))
	items := make([]JournalContextDeferralItem, 0, end-start)
	for _, candidate := range candidates[start:end] {
		items = append(items, candidate.Item)
	}
	layer := JournalContextDeferralLayer{Available: true, AvailableCount: len(candidates), ShownCount: len(items), Truncated: end < len(candidates), ExpandCommand: journalContextExpandCommand(JournalContextLayerDeferrals, limit, branch, ""), Items: items}
	if layer.Truncated {
		layer.Cursor = encodeJournalContextCursor(journalContextCursor{Layer: JournalContextLayerDeferrals, ProjectID: projectID, Branch: branch, Watermark: watermark, Limit: limit, After: journalContextDeferralKey(candidates[end-1]), Fingerprint: fingerprint})
		layer.ExpandCommand = journalContextExpandCommand(JournalContextLayerDeferrals, limit, branch, layer.Cursor)
	}
	return layer, nil
}

func journalContextTaskPage(candidates []JournalContextTask, limit int, cursor *journalContextCursor, projectID, branch string, watermark int64, fingerprint string) (JournalContextTaskLayer, error) {
	start := 0
	if cursor != nil {
		start = findJournalContextAfter(len(candidates), func(i int) string { return candidates[i].Ref }, cursor.After)
		if start < 0 {
			return JournalContextTaskLayer{}, newJournalContextCursorInvalid("cursor continuation key was not found")
		}
	}
	end := min(start+limit, len(candidates))
	items := append([]JournalContextTask(nil), candidates[start:end]...)
	if items == nil {
		items = []JournalContextTask{}
	}
	layer := JournalContextTaskLayer{Available: true, AvailableCount: len(candidates), ShownCount: len(items), Truncated: end < len(candidates), ExpandCommand: journalContextExpandCommand(JournalContextLayerTasks, limit, branch, ""), Items: items}
	if layer.Truncated {
		layer.Cursor = encodeJournalContextCursor(journalContextCursor{Layer: JournalContextLayerTasks, ProjectID: projectID, Branch: branch, Watermark: watermark, Limit: limit, After: candidates[end-1].Ref, Fingerprint: fingerprint})
		layer.ExpandCommand = journalContextExpandCommand(JournalContextLayerTasks, limit, branch, layer.Cursor)
	}
	return layer, nil
}

func findJournalContextAfter(length int, key func(int) string, after string) int {
	for i := 0; i < length; i++ {
		if key(i) == after {
			return i + 1
		}
	}
	return -1
}

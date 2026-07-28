package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// change.json is a closed machine schema: identity plus optional target_release.
// Status/lifecycle fields are banned; unknown keys are violations.

var changeJSONAllowedKeys = map[string]bool{
	"change":         true,
	"created":        true,
	"branch":         true,
	"target_release": true,
}

// changeJSONBannedKeys are status/lifecycle aliases rejected on the machine
// surface (Verification Contract V8). Matching is case-insensitive on the key.
var changeJSONBannedKeys = map[string]bool{
	"readiness":  true,
	"status":     true,
	"state":      true,
	"completion": true,
	"done":       true,
	"progress":   true,
	"phase":      true,
	"lifecycle":  true,
	"stage":      true,
	"workflow":   true,
}

// changeTargetReleaseRE is the canonical MAJOR.MINOR.PATCH literal: no v prefix,
// no leading zeros, no prerelease or build suffix.
var changeTargetReleaseRE = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type changeJSONMeta struct {
	Change        string
	Created       string
	Branch        string
	TargetRelease string
	RawKeys       []string
	Findings      []string
}

func parseChangeJSON(content string) changeJSONMeta {
	meta := changeJSONMeta{Findings: []string{}}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		meta.Findings = append(meta.Findings, "change.json is empty")
		return meta
	}

	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		meta.Findings = append(meta.Findings, fmt.Sprintf("malformed change.json: %v", err))
		return meta
	}
	if decoder.More() {
		meta.Findings = append(meta.Findings, "malformed change.json: trailing content after JSON value")
		return meta
	}

	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	meta.RawKeys = keys

	for _, key := range keys {
		lower := strings.ToLower(key)
		if changeJSONBannedKeys[lower] {
			meta.Findings = append(meta.Findings,
				fmt.Sprintf("status-like change.json key %q is banned; readiness is derived", key))
			continue
		}
		if !changeJSONAllowedKeys[key] {
			meta.Findings = append(meta.Findings,
				fmt.Sprintf("unknown change.json key %q; schema is closed", key))
		}
	}

	meta.Change = decodeChangeJSONString(raw, "change", &meta.Findings)
	meta.Created = decodeChangeJSONString(raw, "created", &meta.Findings)
	meta.Branch = decodeChangeJSONString(raw, "branch", &meta.Findings)
	if _, ok := raw["target_release"]; ok {
		meta.TargetRelease = decodeChangeJSONString(raw, "target_release", &meta.Findings)
		if meta.TargetRelease != "" && !changeTargetReleaseRE.MatchString(meta.TargetRelease) {
			meta.Findings = append(meta.Findings,
				fmt.Sprintf("target_release %q must be canonical MAJOR.MINOR.PATCH (no v, leading zeros, prerelease, or build)", meta.TargetRelease))
		}
	}

	if meta.Change == "" && !hasChangeJSONFindingPrefix(meta.Findings, `change.json field "change"`) {
		meta.Findings = append(meta.Findings, `change.json field "change" is required`)
	}
	if meta.Created == "" && !hasChangeJSONFindingPrefix(meta.Findings, `change.json field "created"`) {
		meta.Findings = append(meta.Findings, `change.json field "created" is required`)
	}
	if meta.Branch == "" && !hasChangeJSONFindingPrefix(meta.Findings, `change.json field "branch"`) {
		meta.Findings = append(meta.Findings, `change.json field "branch" is required`)
	}

	meta.Findings = sortedUnique(meta.Findings)
	return meta
}

func decodeChangeJSONString(raw map[string]json.RawMessage, key string, findings *[]string) string {
	value, ok := raw[key]
	if !ok {
		return ""
	}
	var asString string
	if err := json.Unmarshal(value, &asString); err != nil {
		*findings = append(*findings, fmt.Sprintf("change.json field %q must be a string", key))
		return ""
	}
	return asString
}

func hasChangeJSONFindingPrefix(findings []string, prefix string) bool {
	for _, finding := range findings {
		if strings.HasPrefix(finding, prefix) {
			return true
		}
	}
	return false
}

func isCanonicalChangeTargetRelease(value string) bool {
	return changeTargetReleaseRE.MatchString(value)
}

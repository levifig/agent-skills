package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// splitInstallTargets parses a --to value. Targets may be comma-separated;
// "all" stands alone and means every detected (install) or installed
// (upgrade) harness.
func splitInstallTargets(value string) []string {
	var targets []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !containsString(targets, part) {
			targets = append(targets, part)
		}
	}
	return targets
}

// installChecklistEntry is one row of the interactive harness picker.
type installChecklistEntry struct {
	Key    string
	Name   string
	Status string
}

// promptInstallChecklist is the interactive picker behind `-i`: it lists every
// candidate harness, keeps all of them on a bare Enter, narrows to the numbers
// or names typed, and selects nothing on "none". One reader is shared with
// every other prompt in the command so buffered answers are never lost.
func promptInstallChecklist(reader *bufio.Reader, out io.Writer, verb string, entries []installChecklistEntry) ([]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	fmt.Fprintf(out, "  %s\n", ansiBold(fmt.Sprintf("%s to which harnesses?", verb)))
	for index, entry := range entries {
		status := ""
		if entry.Status != "" {
			status = " " + ansiYellow("("+entry.Status+")")
		}
		fmt.Fprintf(out, "    %d. %s%s\n", index+1, entry.Name, status)
	}
	fmt.Fprintf(out, "  %s ", ansiGray("Enter keeps all; numbers or names narrow (e.g. 1 3); none skips:"))
	answer, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	fmt.Fprintln(out)
	trimmed := strings.TrimSpace(strings.ToLower(answer))
	if trimmed == "" || trimmed == "all" {
		keys := make([]string, 0, len(entries))
		for _, entry := range entries {
			keys = append(keys, entry.Key)
		}
		return keys, nil
	}
	if trimmed == "none" || trimmed == "n" {
		return []string{}, nil
	}
	// Comma-separated parts may be whole names ("claude code"); anything else
	// is whitespace-separated numbers or single-word names.
	var selected []string
	for _, part := range strings.Split(trimmed, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tokens := []string{part}
		if _, ok := resolveInstallChecklistToken(part, entries); !ok {
			tokens = strings.Fields(part)
		}
		for _, token := range tokens {
			key, ok := resolveInstallChecklistToken(token, entries)
			if !ok {
				return nil, fmt.Errorf("unknown selection %q; answer with numbers or names from the list", token)
			}
			if !containsString(selected, key) {
				selected = append(selected, key)
			}
		}
	}
	return selected, nil
}

func resolveInstallChecklistToken(token string, entries []installChecklistEntry) (string, bool) {
	if index, err := strconv.Atoi(token); err == nil {
		if index < 1 || index > len(entries) {
			return "", false
		}
		return entries[index-1].Key, true
	}
	for _, entry := range entries {
		if token == entry.Key || token == strings.ToLower(entry.Name) {
			return entry.Key, true
		}
	}
	return "", false
}

// installChecklistEntries renders detected tools, plus Claude Code when its
// CLI is present, in the order install would apply them.
func installChecklistEntries(tools []detectedInstallTool, hasClaudeCode bool) []installChecklistEntry {
	entries := make([]installChecklistEntry, 0, len(tools)+1)
	for _, tool := range tools {
		status := ""
		if tool.installed {
			status = "installed"
		}
		entries = append(entries, installChecklistEntry{Key: tool.key, Name: tool.name, Status: status})
	}
	if hasClaudeCode {
		entries = append(entries, installChecklistEntry{Key: claudeCodeInstallTarget, Name: installDisplayName(claudeCodeInstallTarget), Status: "plugin"})
	}
	return entries
}

// withoutString returns values without every occurrence of unwanted.
func withoutString(values []string, unwanted string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != unwanted {
			result = append(result, value)
		}
	}
	return result
}

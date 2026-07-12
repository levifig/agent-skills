package cli

import (
	"sort"
	"strings"
)

// CommandAuthority describes whether a command may be exposed to an
// unattended harness policy. The zero value is deliberately operator: an
// unclassified command must fail closed.
type CommandAuthority uint8

const (
	CommandAuthorityOperator CommandAuthority = iota
	CommandAuthorityBasic
)

func (a CommandAuthority) String() string {
	if a == CommandAuthorityBasic {
		return "basic"
	}
	return "operator"
}

type commandAuthorityPrefix struct {
	tokens []string
}

// basicCommandAuthorityPrefixes is the single source of truth for the
// command prefixes that a future harness adapter may expose. Prefixes are
// intentionally leaves rather than command-family parents: Codex prefix rules
// authorize arbitrary trailing arguments, so a parent rule would silently
// authorize unclassified and operator leaves.
var basicCommandAuthorityPrefixes = []commandAuthorityPrefix{
	// Routine state-plane command families.
	{tokens: []string{"journal", "context"}},
	{tokens: []string{"journal", "defer"}},
	{tokens: []string{"journal", "export"}},
	{tokens: []string{"journal", "log", "--execpolicy-safe"}},
	{tokens: []string{"journal", "recent"}},
	{tokens: []string{"journal", "search"}},
	{tokens: []string{"journal", "show"}},
	{tokens: []string{"task", "archive"}},
	{tokens: []string{"task", "create"}},
	{tokens: []string{"task", "list"}},
	{tokens: []string{"task", "refresh"}},
	{tokens: []string{"task", "show"}},
	{tokens: []string{"task", "status"}},
	{tokens: []string{"task", "sync"}},
	{tokens: []string{"task", "update"}},
	{tokens: []string{"report", "archive"}},
	{tokens: []string{"report", "finalize"}},
	{tokens: []string{"report", "generate", "release-readiness"}},
	{tokens: []string{"report", "generate", "triage"}},
	{tokens: []string{"report", "list"}},
	{tokens: []string{"report", "render"}},
	{tokens: []string{"report", "show"}},
	{tokens: []string{"finding", "list"}},
	{tokens: []string{"finding", "show"}},
	{tokens: []string{"finding", "verdict"}},
	{tokens: []string{"run", "complete"}},
	{tokens: []string{"run", "create"}},
	{tokens: []string{"run", "list"}},
	{tokens: []string{"run", "show"}},
	{tokens: []string{"plan", "link"}},
	{tokens: []string{"plan", "list"}},
	{tokens: []string{"plan", "show"}},
	{tokens: []string{"handoff", "link"}},
	{tokens: []string{"handoff", "list"}},
	{tokens: []string{"handoff", "show"}},
	{tokens: []string{"council", "link"}},
	{tokens: []string{"council", "list"}},
	{tokens: []string{"council", "show"}},
	{tokens: []string{"brainstorm", "archive"}},
	{tokens: []string{"brainstorm", "list"}},
	{tokens: []string{"brainstorm", "promote"}},
	{tokens: []string{"brainstorm", "show"}},
	{tokens: []string{"idea", "archive"}},
	{tokens: []string{"idea", "list"}},
	{tokens: []string{"idea", "promote"}},
	{tokens: []string{"idea", "resolve"}},
	{tokens: []string{"idea", "show"}},
	{tokens: []string{"spark", "capture"}},
	{tokens: []string{"spark", "list"}},
	{tokens: []string{"spark", "promote"}},
	{tokens: []string{"spark", "resolve"}},
	{tokens: []string{"spark", "show"}},
	{tokens: []string{"tag", "add"}},
	{tokens: []string{"tag", "list"}},
	{tokens: []string{"tag", "remove"}},
	{tokens: []string{"tag", "show"}},
	{tokens: []string{"bundle", "add"}},
	{tokens: []string{"bundle", "create"}},
	{tokens: []string{"bundle", "list"}},
	{tokens: []string{"bundle", "remove"}},
	{tokens: []string{"bundle", "show"}},
	{tokens: []string{"bundle", "update"}},
	{tokens: []string{"link", "create"}},
	{tokens: []string{"link", "list"}},
	{tokens: []string{"link", "remove"}},
	{tokens: []string{"trace"}},

	// Explicitly approved readers and diagnostics. `state doctor` is omitted
	// because --fix mutates state; the whole leaf therefore stays operator.
	// Body/file-consuming creation/import leaves and path-taking `change check`
	// are also omitted so a prefix rule cannot authorize caller-selected input
	// outside the centrally reviewed command contracts.
	{tokens: []string{"docs", "index"}},
	{tokens: []string{"state", "backup", "verify"}},
	{tokens: []string{"state", "export", "all"}},
	{tokens: []string{"state", "export", "release-readiness"}},
	{tokens: []string{"state", "export", "spec"}},
	{tokens: []string{"state", "export", "triage"}},
	{tokens: []string{"state", "path"}},
	{tokens: []string{"state", "status"}},
	{tokens: []string{"project", "identity"}},
	{tokens: []string{"project", "list"}},
	{tokens: []string{"project", "show"}},
	{tokens: []string{"change", "list"}},
	{tokens: []string{"kb", "check"}},
	{tokens: []string{"kb", "glossary", "check"}},
	{tokens: []string{"kb", "glossary", "list"}},
	{tokens: []string{"kb", "status"}},
	{tokens: []string{"kb", "validate"}},
	{tokens: []string{"check"}},
	{tokens: []string{"housekeeping"}},
	{tokens: []string{"spec", "archive"}},
	{tokens: []string{"spec", "list"}},
	{tokens: []string{"spec", "render"}},
	{tokens: []string{"spec", "show"}},
	{tokens: []string{"spec", "status"}},
	{tokens: []string{"version"}},
}

// CommandAuthorityFor classifies an invocation by an explicit safe prefix.
// Flags and positional arguments after the prefix are intentionally not
// interpreted here; adapters must rely on the registered command contract.
func CommandAuthorityFor(args []string) CommandAuthority {
	for _, entry := range basicCommandAuthorityPrefixes {
		if hasCommandPrefix(args, entry.tokens) {
			return CommandAuthorityBasic
		}
	}
	return CommandAuthorityOperator
}

// BasicCommandAuthorityPrefixes returns a deterministic defensive copy for
// policy renderers. Callers cannot mutate the central registry through the
// returned slices.
func BasicCommandAuthorityPrefixes() [][]string {
	prefixes := make([][]string, 0, len(basicCommandAuthorityPrefixes))
	for _, entry := range basicCommandAuthorityPrefixes {
		prefix := append([]string(nil), entry.tokens...)
		prefixes = append(prefixes, prefix)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return strings.Join(prefixes[i], "\x00") < strings.Join(prefixes[j], "\x00")
	})
	return prefixes
}

func hasCommandPrefix(args []string, prefix []string) bool {
	if len(args) < len(prefix) {
		return false
	}
	for i, token := range prefix {
		if args[i] != token {
			return false
		}
	}
	return true
}

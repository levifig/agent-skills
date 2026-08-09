package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// hookRecognition carries every input the closed ownership predicate needs.
// Nothing is read from global state here: callers inject the trusted executable
// paths (the currently resolved one plus any path previously recorded for the
// target) and the Loaf-managed hook-file destinations the installed manifest
// recorded. An entry is Loaf's only through these inputs — never through the
// `loaf-managed` marker, which stays human-legible provenance and nothing more.
type hookRecognition struct {
	target       string
	catalog      hookCatalog
	trustedPaths []string
	managedPaths []string
	homeDir      string
	goos         string
}

type hookOwnershipReason string

const (
	hookOwnershipExecutable  hookOwnershipReason = "executable-form"
	hookOwnershipManagedPath hookOwnershipReason = "managed-path"
	hookOwnershipLegacy      hookOwnershipReason = "legacy-allowlist"
)

type hookOwnership struct {
	owned  bool
	reason hookOwnershipReason
	// hookID is set only when the recognizing rule already identified the
	// entry. Path-backed and legacy entries leave it empty for pairing.
	hookID string
}

// hookEntryIdentity is the command an entry is identified by, tokenized and
// normalized. Foreign entries reach this too: computing the identity is not
// claiming the entry.
type hookEntryIdentity struct {
	tokens     []string
	executable bool
	ok         bool
}

// ownsEntry implements the closed recognition predicate. An entry is Loaf's iff
// it invokes the Loaf executable in one of three exact forms and matches a
// catalog signature or identity stem, or references an exact Loaf-managed
// hook-file destination, or matches the frozen legacy allowlist. Anything else
// is foreign and stays untouched forever.
func (r hookRecognition) ownsEntry(entry map[string]any) (hookOwnership, error) {
	if r.target == "codex" {
		// The frozen 0.2.20 matcher-group shape is part of the closed legacy
		// allowlist, recognized with any canonical absolute path: the release
		// that wrote it recorded no trusted path to compare against. Its
		// conflict signal is deliberately dropped — a modified group converges
		// through the identity test below instead of refusing the file.
		if owned, _ := codexHookOwnershipForOS(entry, r.goos); owned {
			return hookOwnership{owned: true, reason: hookOwnershipLegacy, hookID: r.legacyCodexHookID()}, nil
		}
	}
	identity := r.entryIdentity(entry)
	if identity.ok && identity.executable {
		hookID, _, matched, err := r.matchCatalogIdentity(r.catalog.Entries, identity.tokens)
		if err != nil {
			return hookOwnership{}, err
		}
		if matched {
			return hookOwnership{owned: true, reason: hookOwnershipExecutable, hookID: hookID}, nil
		}
	}
	if identity.ok && r.referencesManagedPath(identity.tokens) {
		return hookOwnership{owned: true, reason: hookOwnershipManagedPath}, nil
	}
	if r.matchesFrozenLegacyAllowlist(entry) {
		return hookOwnership{owned: true, reason: hookOwnershipLegacy}, nil
	}
	return hookOwnership{}, nil
}

// legacyCodexHookID names the one hook the frozen Codex matcher-group shape can
// ever describe, so a legacy-recognized group pairs without re-deriving it.
func (r hookRecognition) legacyCodexHookID() string {
	for _, entry := range r.catalog.Entries {
		if entry.HookID == "session-start-loaf" {
			return entry.HookID
		}
	}
	return ""
}

// matchesFrozenLegacyAllowlist is the enumerated allowlist as it stands at
// 0.2.20 — a closed list, never extended by inference. Prompt-prefix
// recognition is bounded to prompt-type entries: it is the one prefix rule in
// the predicate, and a command entry must never be claimed by it.
func (r hookRecognition) matchesFrozenLegacyAllowlist(entry map[string]any) bool {
	command, hasCommand := entry["command"].(string)
	if signature := installHookSignature(entry); signature != "" && legacyLoafHookSignatures[signature] {
		return true
	}
	if hasCommand && legacyLoafCommands[command] {
		return true
	}
	if hasCommand {
		return false
	}
	if prompt, ok := entry["prompt"].(string); ok {
		for _, prefix := range legacyLoafPromptPrefixes {
			if strings.HasPrefix(prompt, prefix) {
				return true
			}
		}
	}
	return false
}

// matchCatalogIdentity runs the signature pass then the stem pass. Zero stems
// means the command is the operator's own loaf-invoking hook; more than one is
// an integrity error rather than a guess.
func (r hookRecognition) matchCatalogIdentity(entries []hookCatalogEntry, tokens []string) (string, hookPairingPass, bool, error) {
	if hookID, ok := r.matchCatalogSignature(entries, tokens); ok {
		return hookID, hookPairingSignature, true, nil
	}
	hookIDs := r.matchCatalogStems(entries, tokens)
	switch len(hookIDs) {
	case 0:
		return "", "", false, nil
	case 1:
		return hookIDs[0], hookPairingStem, true, nil
	default:
		return "", "", false, fmt.Errorf("hook command carries the identity of more than one Loaf hook (%s); resolve the entry by hand before reconciling", strings.Join(hookIDs, ", "))
	}
}

func (r hookRecognition) matchCatalogSignature(entries []hookCatalogEntry, tokens []string) (string, bool) {
	for _, entry := range entries {
		for _, signature := range entry.Signatures {
			if hookTokensEqual(r.normalizeCommandTokens(signature), tokens) {
				return entry.HookID, true
			}
		}
	}
	return "", false
}

func (r hookRecognition) matchCatalogStems(entries []hookCatalogEntry, tokens []string) []string {
	var hookIDs []string
	for _, entry := range entries {
		for _, stem := range entry.Stems {
			if containsHookTokenSequence(tokens, r.normalizeTokens(stem)) {
				hookIDs = appendUniqueHookID(hookIDs, entry.HookID)
				break
			}
		}
	}
	return hookIDs
}

func (r hookRecognition) referencesManagedPath(tokens []string) bool {
	if len(r.managedPaths) == 0 {
		return false
	}
	managed := make(map[string]bool, len(r.managedPaths))
	for _, destination := range r.managedPaths {
		if normalized := r.normalizePathToken(destination); normalized != "" {
			managed[normalized] = true
		}
	}
	for _, token := range tokens {
		if managed[token] {
			return true
		}
	}
	return false
}

func (r hookRecognition) entryIdentity(entry map[string]any) hookEntryIdentity {
	command, windows, ok := r.entryCommand(entry)
	if !ok {
		return hookEntryIdentity{}
	}
	var tokens []hookCommandToken
	if windows {
		if unwrapped, ok := codexWindowsCommandTokens(command); ok {
			tokens = unwrapped
		}
	}
	if tokens == nil {
		parsed, ok := hookCommandTokensForOS(command, r.goos)
		if !ok {
			return hookEntryIdentity{}
		}
		tokens = parsed
	}
	if len(tokens) == 0 {
		return hookEntryIdentity{}
	}
	normalized := r.normalizeQuotedCommandTokens(tokens)
	return hookEntryIdentity{
		tokens:     normalized,
		executable: normalized[0] == hookExecutableSentinel,
		ok:         true,
	}
}

// entryCommand extracts the command an entry is identified by. Codex nests one
// command handler inside a matcher group; a group carrying anything other than
// exactly one handler is the operator's own construction and never Loaf's.
func (r hookRecognition) entryCommand(entry map[string]any) (string, bool, bool) {
	if r.target == "codex" {
		handlers, ok := entry["hooks"].([]any)
		if !ok || len(handlers) != 1 {
			return "", false, false
		}
		handler, ok := handlers[0].(map[string]any)
		if !ok {
			return "", false, false
		}
		if r.goos == "windows" {
			if command, ok := handler["commandWindows"].(string); ok {
				return command, true, true
			}
		}
		command, ok := handler["command"].(string)
		return command, r.goos == "windows", ok
	}
	command, ok := entry["command"].(string)
	return command, false, ok
}

// normalizeQuotedCommandTokens normalizes a command as the shell would read it:
// the first token collapses to the executable sentinel when it is one of the
// three accepted Loaf forms, and every path token becomes an absolute lexical
// path — but only where the shell itself would have expanded it.
func (r hookRecognition) normalizeQuotedCommandTokens(tokens []hookCommandToken) []string {
	normalized := make([]string, len(tokens))
	for i, token := range tokens {
		normalized[i] = r.normalizeQuotedToken(token)
	}
	if len(normalized) > 0 && r.isLoafExecutableToken(tokens[0].value, normalized[0]) {
		normalized[0] = hookExecutableSentinel
	}
	return normalized
}

// normalizeCommandTokens is the catalog side of the same normalization. Catalog
// signatures and stems are authored, not parsed out of somebody's file, so
// their tokens carry no quoting to respect.
func (r hookRecognition) normalizeCommandTokens(tokens []string) []string {
	return r.normalizeQuotedCommandTokens(unquotedHookTokens(tokens))
}

func (r hookRecognition) normalizeTokens(tokens []string) []string {
	normalized := make([]string, len(tokens))
	for i, token := range tokens {
		normalized[i] = r.normalizeToken(token)
	}
	return normalized
}

func (r hookRecognition) normalizeToken(token string) string {
	return r.normalizeQuotedToken(hookCommandToken{value: token})
}

func (r hookRecognition) normalizeQuotedToken(token hookCommandToken) string {
	if !looksLikeHookPathToken(token.value) {
		return token.value
	}
	if normalized := r.normalizeQuotedPathToken(token.value, token.quote); normalized != "" {
		return normalized
	}
	return token.value
}

// isLoafExecutableToken accepts exactly three forms: the install-time template,
// the bare `loaf` first token Cursor entries ship today, and an absolute path
// equal to a trusted executable path. Shell quoting is not part of the test —
// the tokenizer already removed it — but the path identity is.
func (r hookRecognition) isLoafExecutableToken(token string, normalized string) bool {
	if token == codexJournalExecutablePlaceholder || token == "loaf" {
		return true
	}
	for _, trusted := range r.trustedPaths {
		if trusted == "" {
			continue
		}
		if candidate := r.normalizePathToken(trusted); candidate != "" && candidate == normalized {
			return true
		}
	}
	return false
}

// normalizePathToken is the closed algorithm for a path Loaf itself recorded —
// a manifest destination or a trusted executable — which carries no shell
// quoting of its own.
func (r hookRecognition) normalizePathToken(token string) string {
	return r.normalizeQuotedPathToken(token, hookTokenUnquoted)
}

// normalizeQuotedPathToken expands and normalizes exactly as the shell that
// runs the hook would: separators are normalized for the platform first so a
// Windows spelling is recognizable at all, then `$HOME` and `~` expand only
// where the quoting the operator wrote actually permits it. A single-quoted
// token is literal in the shell, so treating it as an expansion here would
// claim an entry that never pointed at a Loaf path in the first place; `~`
// additionally does not expand inside double quotes.
//
// No symlink resolution and no directory containment: only an exact match on a
// full path can ever claim an entry.
func (r hookRecognition) normalizeQuotedPathToken(token string, quote hookTokenQuote) string {
	value := token
	home := r.homeDir
	if r.goos == "windows" {
		value = strings.ReplaceAll(value, `\`, "/")
		home = strings.ReplaceAll(home, `\`, "/")
	}
	if quote != hookTokenSingleQuoted {
		switch {
		case value == "$HOME" || value == "${HOME}":
			value = home
		case strings.HasPrefix(value, "$HOME/"):
			value = joinHookHomePath(home, value[len("$HOME/"):])
		case strings.HasPrefix(value, "${HOME}/"):
			value = joinHookHomePath(home, value[len("${HOME}/"):])
		case quote == hookTokenUnquoted && value == "~":
			value = home
		case quote == hookTokenUnquoted && strings.HasPrefix(value, "~/"):
			value = joinHookHomePath(home, value[len("~/"):])
		}
	}
	if !isAbsoluteHookPath(value, r.goos) {
		return ""
	}
	return path.Clean(value)
}

func joinHookHomePath(home string, rest string) string {
	if home == "" {
		return ""
	}
	return strings.TrimRight(home, `/\`) + "/" + rest
}

func isAbsoluteHookPath(value string, goos string) bool {
	if goos != "windows" {
		return strings.HasPrefix(value, "/")
	}
	if strings.HasPrefix(value, "//") {
		return true
	}
	return len(value) >= 3 && isASCIIWindowsDriveLetter(value[0]) && value[1] == ':' && value[2] == '/'
}

func looksLikeHookPathToken(token string) bool {
	return strings.ContainsAny(token, `/\`) || token == "~" || token == "$HOME" || token == "${HOME}"
}

// hookManagedDestinations anchors the installed manifest's Loaf-managed
// hook-file destinations at the target's config root. Destinations are recorded
// manifest-relative; ownership compares them as absolute lexical paths.
func hookManagedDestinations(configDir string, manifest targetAdapterManifest) []string {
	var destinations []string
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind != "hook-file" || artifact.Destination == "" {
			continue
		}
		destinations = append(destinations, filepath.Join(configDir, filepath.FromSlash(artifact.Destination)))
	}
	return destinations
}

// hookTokenQuote records how a token's first character was quoted, which is the
// only thing that decides whether the expansions at the front of a word happen.
type hookTokenQuote int

const (
	hookTokenUnquoted hookTokenQuote = iota
	hookTokenDoubleQuoted
	hookTokenSingleQuoted
)

// hookCommandToken is one shell word plus the quoting its first character
// carried. The quoting is kept rather than dropped because two commands that
// tokenize identically can still mean different things: `$HOME/x` names a path
// and `'$HOME/x'` names a literal filename that no expansion turns into one.
type hookCommandToken struct {
	value string
	quote hookTokenQuote
}

// hookCommandTokens splits a POSIX shell command into words. Callers that hold
// authored strings — catalog signatures and stems — use this; anything parsed
// out of somebody's hooks file goes through hookCommandTokensForOS so the
// quoting survives.
func hookCommandTokens(command string) ([]string, bool) {
	tokens, ok := hookCommandTokensForOS(command, "")
	if !ok {
		return nil, false
	}
	values := make([]string, 0, len(tokens))
	for _, token := range tokens {
		values = append(values, token.value)
	}
	return values, true
}

// hookCommandTokensForOS splits a hook command the way its shell groups words:
// single quotes are literal, double quotes honor the four escapes, and the
// quoting itself is dropped from the value while being recorded on the token.
// It recognizes rather than interprets — nothing is expanded here and operators
// are ordinary tokens — which is what lets two spellings of one command compare
// equal without any of it being executed.
//
// On Windows a backslash is a path separator rather than an escape, because the
// commands there are cmd.exe strings; treating it as an escape would silently
// eat the separators out of every Windows path.
func hookCommandTokensForOS(command string, goos string) ([]hookCommandToken, bool) {
	tokens := []hookCommandToken{}
	var current strings.Builder
	started := false
	leading := hookTokenUnquoted
	begin := func(quote hookTokenQuote) {
		if !started {
			leading = quote
			started = true
		}
	}
	flush := func() {
		if started {
			tokens = append(tokens, hookCommandToken{value: current.String(), quote: leading})
			current.Reset()
			started = false
			leading = hookTokenUnquoted
		}
	}
	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		switch character := runes[i]; character {
		case ' ', '\t', '\n', '\r':
			flush()
		case '\'':
			end := -1
			for j := i + 1; j < len(runes); j++ {
				if runes[j] == '\'' {
					end = j
					break
				}
			}
			if end < 0 {
				return nil, false
			}
			begin(hookTokenSingleQuoted)
			current.WriteString(string(runes[i+1 : end]))
			i = end
		case '"':
			begin(hookTokenDoubleQuoted)
			end := -1
			for j := i + 1; j < len(runes); j++ {
				if runes[j] == '\\' && j+1 < len(runes) && isHookDoubleQuoteEscape(runes[j+1]) {
					current.WriteRune(runes[j+1])
					j++
					continue
				}
				if runes[j] == '"' {
					end = j
					break
				}
				current.WriteRune(runes[j])
			}
			if end < 0 {
				return nil, false
			}
			i = end
		case '\\':
			if goos == "windows" {
				begin(hookTokenUnquoted)
				current.WriteRune(character)
				continue
			}
			if i+1 >= len(runes) {
				return nil, false
			}
			begin(hookTokenUnquoted)
			current.WriteRune(runes[i+1])
			i++
		default:
			begin(hookTokenUnquoted)
			current.WriteRune(character)
		}
	}
	flush()
	return tokens, true
}

func unquotedHookTokens(values []string) []hookCommandToken {
	tokens := make([]hookCommandToken, 0, len(values))
	for _, value := range values {
		tokens = append(tokens, hookCommandToken{value: value})
	}
	return tokens
}

func isHookDoubleQuoteEscape(character rune) bool {
	return character == '"' || character == '\\' || character == '$' || character == '`'
}

// codexWindowsCommandTokens unwraps the cmd.exe form install renders on
// Windows — `""C:\path\loaf" <suffix>"` — back into plain tokens. Everything
// else, including the template form that is byte-identical on both platforms,
// falls through to ordinary splitting.
func codexWindowsCommandTokens(command string) ([]hookCommandToken, bool) {
	if len(command) < 4 || !strings.HasPrefix(command, `""`) || !strings.HasSuffix(command, `"`) {
		return nil, false
	}
	executable, rest, ok := strings.Cut(command[2:len(command)-1], `"`)
	if !ok || executable == "" {
		return nil, false
	}
	tail, ok := hookCommandTokensForOS(rest, "windows")
	if !ok {
		return nil, false
	}
	return append([]hookCommandToken{{value: executable, quote: hookTokenDoubleQuoted}}, tail...), true
}

func hookTokensEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// containsHookTokenSequence reports whether needle appears in haystack as an
// exact contiguous run of whole tokens. Whole-token comparison is what keeps a
// longer hook id outside the identity of the shorter one it starts with.
func containsHookTokenSequence(haystack []string, needle []string) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
	for start := 0; start+len(needle) <= len(haystack); start++ {
		if hookTokensEqual(haystack[start:start+len(needle)], needle) {
			return true
		}
	}
	return false
}

func appendUniqueHookID(hookIDs []string, hookID string) []string {
	for _, existing := range hookIDs {
		if existing == hookID {
			return hookIDs
		}
	}
	return append(hookIDs, hookID)
}

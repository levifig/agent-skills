package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// hookPairingPass names which of the three deterministic passes resolved an
// entry to its hook ID.
type hookPairingPass string

const (
	hookPairingTemplate  hookPairingPass = "template"
	hookPairingSignature hookPairingPass = "signature"
	hookPairingStem      hookPairingPass = "stem"
)

type hookEntryPairing struct {
	index  int
	hookID string
	pass   hookPairingPass
}

// hookPairingOutcome partitions one event section of a hooks file. Foreign
// entries are recorded by index alone: nothing downstream may classify, read
// further into, or rewrite them.
type hookPairingOutcome struct {
	// paired holds the surviving entry per hook ID, in file order.
	paired []hookEntryPairing
	// duplicates are owned entries whose hook ID an earlier entry already
	// claimed. They are Loaf's by construction, so they are removable.
	duplicates []hookEntryPairing
	// retired are owned entries that pair to no ID this catalog still ships —
	// an older Loaf generation, removable.
	retired []int
	foreign []int
}

// pairHookEventEntries maps the owned entries of one event section to catalog
// hook IDs. Ownership is decided catalog-wide (a command's identity does not
// depend on which section it sits in) while pairing is per section, so a Loaf
// entry someone moved to the wrong event reads as a retired generation and is
// removed rather than converged in place.
func pairHookEventEntries(recognition hookRecognition, event string, entries []map[string]any) (hookPairingOutcome, error) {
	catalogEntries := recognition.catalog.entriesForEvent(event)
	outcome := hookPairingOutcome{}
	claimed := map[string]bool{}
	for index, entry := range entries {
		ownership, err := recognition.ownsEntry(entry)
		if err != nil {
			return hookPairingOutcome{}, fmt.Errorf("%s %s entry %d: %w", recognition.target, event, index, err)
		}
		if !ownership.owned {
			outcome.foreign = append(outcome.foreign, index)
			continue
		}
		hookID, pass, resolved, err := recognition.resolveHookID(catalogEntries, entry)
		if err != nil {
			return hookPairingOutcome{}, fmt.Errorf("%s %s entry %d: %w", recognition.target, event, index, err)
		}
		if !resolved {
			outcome.retired = append(outcome.retired, index)
			continue
		}
		pairing := hookEntryPairing{index: index, hookID: hookID, pass: pass}
		if claimed[hookID] {
			outcome.duplicates = append(outcome.duplicates, pairing)
			continue
		}
		claimed[hookID] = true
		outcome.paired = append(outcome.paired, pairing)
	}
	return outcome, nil
}

// resolveHookID runs the three passes in order: exact match against the current
// desired template, the closed signature map, then identity stems.
func (r hookRecognition) resolveHookID(entries []hookCatalogEntry, entry map[string]any) (string, hookPairingPass, bool, error) {
	for _, candidate := range entries {
		if hookTemplateMatchesEntry(candidate, entry) {
			return candidate.HookID, hookPairingTemplate, true, nil
		}
	}
	identity := r.entryIdentity(entry)
	if !identity.ok {
		hookID, pass, ok := resolvePromptHookID(entries, entry)
		return hookID, pass, ok, nil
	}
	return r.matchCatalogIdentity(entries, identity.tokens)
}

func resolvePromptHookID(entries []hookCatalogEntry, entry map[string]any) (string, hookPairingPass, bool) {
	prompt, ok := entry["prompt"].(string)
	if !ok || prompt == "" {
		return "", "", false
	}
	for _, candidate := range entries {
		if candidate.Type == "prompt" && candidate.Prompt == prompt {
			return candidate.HookID, hookPairingSignature, true
		}
	}
	return "", "", false
}

func hookTemplateMatchesEntry(candidate hookCatalogEntry, entry map[string]any) bool {
	template, err := decodeHookJSONValue(candidate.Template)
	if err != nil {
		return false
	}
	value, err := canonicalHookValue(entry)
	if err != nil {
		return false
	}
	return reflect.DeepEqual(template, value)
}

// canonicalHookValue re-decodes a value through JSON so that numbers compare by
// their literal form regardless of which decoder produced the input.
func canonicalHookValue(value any) (any, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return decodeHookJSONValue(body)
}

func decodeHookJSONValue(body []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// hook_file.go is the parsed form of a shared hooks file. Loaf owns some of its
// entries and none of the rest, so the document keeps everything it did not
// write exactly as it found it: top-level fields, event sections, and every
// foreign entry survive as raw JSON values in their original order. Only the
// entries reconciliation names are ever replaced, and the file is republished
// by rebuilding the same ordered document around them.
//
// The promise is JSON-value identity and relative order, not byte identity:
// re-serialization normalizes whitespace, which is what lets one writer own a
// file it did not format.

const hookFileEventsField = "hooks"

const hookFileDefaultMode fs.FileMode = 0o644

// hookFile is one hooks.json as read from disk. An absent file is a valid
// document with no fields, so the caller that adds the first entry does not
// need a separate creation path.
type hookFile struct {
	path    string
	exists  bool
	mode    fs.FileMode
	raw     []byte
	order   []string
	fields  map[string]json.RawMessage
	events  []string
	entries map[string][]json.RawMessage
}

// readHookFile parses a hooks file, enforcing the integrity preconditions that
// make reconciliation safe: a regular non-symlinked path, readable bytes,
// parseable JSON with no duplicate keys, an object at the top level, and a
// hooks section whose events are arrays of objects. Every failure preserves the
// file — nothing here writes — and says so, because the caller was about to
// rewrite what it could not read.
func readHookFile(path string) (hookFile, error) {
	file := hookFile{
		path:    path,
		mode:    hookFileDefaultMode,
		fields:  map[string]json.RawMessage{},
		entries: map[string][]json.RawMessage{},
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return file, nil
		}
		return hookFile{}, fmt.Errorf("inspect hooks file %s: %w", path, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return hookFile{}, fmt.Errorf("hooks file %s is not a regular file — preserving it as written", path)
	}
	body, err := readRegularFileNoFollow(path, projectFileReadLimit)
	if err != nil {
		return hookFile{}, fmt.Errorf("read hooks file %s: %w — preserving it as written", path, err)
	}
	if err := validateJSONNoDuplicateKeys(body); err != nil {
		return hookFile{}, fmt.Errorf("parse hooks file %s: %w — preserving it as written", path, err)
	}
	order, fields, err := decodeHookJSONObject(body)
	if err != nil {
		return hookFile{}, fmt.Errorf("parse hooks file %s: %w — preserving it as written", path, err)
	}
	file.exists = true
	file.mode = info.Mode().Perm()
	file.raw = body
	file.order = order
	file.fields = fields

	rawEvents, hasEvents := fields[hookFileEventsField]
	if !hasEvents {
		return file, nil
	}
	events, sections, err := decodeHookJSONObject(rawEvents)
	if err != nil {
		return hookFile{}, fmt.Errorf("parse hooks file %s: %q must be an object — preserving it as written", path, hookFileEventsField)
	}
	file.events = events
	for _, event := range events {
		var entries []json.RawMessage
		if err := json.Unmarshal(sections[event], &entries); err != nil || entries == nil {
			return hookFile{}, fmt.Errorf("parse hooks file %s: event %q must be an array — preserving it as written", path, event)
		}
		for index, entry := range entries {
			if _, err := decodeHookEntry(entry); err != nil {
				return hookFile{}, fmt.Errorf("parse hooks file %s: %s entry %d must be an object — preserving it as written", path, event, index)
			}
		}
		file.entries[event] = entries
	}
	return file, nil
}

// decodeHookJSONObject decodes a JSON object while recording the order its keys
// were written in. Go maps have no order and the file has one; preserving it is
// what keeps a republished document recognizable to whoever wrote it.
func decodeHookJSONObject(body []byte) ([]string, map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, nil, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, nil, fmt.Errorf("value must be a JSON object")
	}
	order := []string{}
	values := map[string]json.RawMessage{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, nil, fmt.Errorf("object key must be a string")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, err
		}
		order = append(order, name)
		values[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, nil, err
	}
	if decoder.More() {
		return nil, nil, fmt.Errorf("trailing JSON values")
	}
	return order, values, nil
}

// decodeHookEntry decodes one entry for the read-only inspection recognition
// performs. Numbers keep their literal form so an entry compared against a
// catalog template is not judged different for having been decoded.
func decodeHookEntry(raw json.RawMessage) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var entry map[string]any
	if err := decoder.Decode(&entry); err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("entry must be an object")
	}
	return entry, nil
}

// eventEntries decodes one event section for recognition and pairing. The raw
// values stay authoritative: decoding an entry is inspecting it, never claiming
// it, and the bytes republished for a foreign entry are the bytes read.
func (f hookFile) eventEntries(event string) ([]map[string]any, error) {
	entries := make([]map[string]any, 0, len(f.entries[event]))
	for index, raw := range f.entries[event] {
		entry, err := decodeHookEntry(raw)
		if err != nil {
			return nil, fmt.Errorf("read %s entry %d in %s: %w", event, index, f.path, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// setEventEntries replaces one event's entries, appending the event to the
// section when the file has never carried it. An event that empties out keeps
// its key: the file said it existed and reconciliation has no opinion about
// sections it did not create.
func (f *hookFile) setEventEntries(event string, entries []json.RawMessage) {
	if _, known := f.entries[event]; !known {
		f.events = append(f.events, event)
		if _, hasSection := f.fields[hookFileEventsField]; !hasSection {
			f.order = append(f.order, hookFileEventsField)
			f.fields[hookFileEventsField] = json.RawMessage("{}")
		}
	}
	f.entries[event] = entries
}

// seedHookFileFields gives a file that does not exist yet the top-level shape
// its target's builder emits, so a first install writes the document the
// harness expects rather than a bare hooks object.
func (f *hookFile) seed(target string) {
	if f.exists || len(f.order) > 0 {
		return
	}
	if target == "cursor" {
		f.order = append(f.order, "version")
		f.fields["version"] = json.RawMessage("1")
	}
	f.order = append(f.order, hookFileEventsField)
	f.fields[hookFileEventsField] = json.RawMessage("{}")
}

// marshal rebuilds the document in its original key order. Foreign entries are
// written back as the exact JSON values they were read as; only indentation is
// this writer's own.
func (f hookFile) marshal() ([]byte, error) {
	var compact bytes.Buffer
	compact.WriteByte('{')
	for index, key := range f.order {
		if index > 0 {
			compact.WriteByte(',')
		}
		name, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		compact.Write(name)
		compact.WriteByte(':')
		if key == hookFileEventsField {
			if err := f.writeEvents(&compact); err != nil {
				return nil, err
			}
			continue
		}
		compact.Write(f.fields[key])
	}
	compact.WriteByte('}')

	var indented bytes.Buffer
	if err := json.Indent(&indented, compact.Bytes(), "", "  "); err != nil {
		return nil, fmt.Errorf("serialize hooks file %s: %w", f.path, err)
	}
	indented.WriteByte('\n')
	return indented.Bytes(), nil
}

func (f hookFile) writeEvents(out *bytes.Buffer) error {
	out.WriteByte('{')
	for index, event := range f.events {
		if index > 0 {
			out.WriteByte(',')
		}
		name, err := json.Marshal(event)
		if err != nil {
			return err
		}
		out.Write(name)
		out.WriteString(":[")
		for position, entry := range f.entries[event] {
			if position > 0 {
				out.WriteByte(',')
			}
			out.Write(entry)
		}
		out.WriteString("]")
	}
	out.WriteByte('}')
	return nil
}

// publishHookFile writes the projected document with the ordering Decision 10
// specifies: stage the new bytes, re-read the destination, compare it against
// what was parsed, and only then rename. A third-party write that lands before
// the comparison aborts the publication with the destination untouched.
//
// The window between the comparison and the rename is the named residual: a
// writer that does not honour the lock can still lose its write there, and no
// mechanism short of mandatory locking closes it. SQLite holds no copy of
// foreign content, so nothing later can restore it — which is why this says so
// rather than calling it convergence.
func publishHookFile(file hookFile, body []byte, operations *hookReconcileOperations) error {
	directory := filepath.Dir(file.path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	staged, err := os.CreateTemp(directory, ".loaf-hooks-*")
	if err != nil {
		return err
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if err := stageHookFileBytes(staged, body, file.mode); err != nil {
		return err
	}
	if operations != nil && operations.beforeRename != nil {
		if err := operations.beforeRename(); err != nil {
			return err
		}
	}
	if err := ensureHookFileUnchanged(file); err != nil {
		return err
	}
	return os.Rename(stagedPath, file.path)
}

func stageHookFileBytes(staged *os.File, body []byte, mode fs.FileMode) error {
	defer staged.Close()
	if err := staged.Chmod(mode); err != nil {
		return err
	}
	if _, err := staged.Write(body); err != nil {
		return err
	}
	return staged.Sync()
}

// ensureHookFileUnchanged reports whether the destination still holds the bytes
// reconciliation read. Content is compared rather than a digest recorded:
// nothing about this file is judged by a stored fingerprint.
func ensureHookFileUnchanged(file hookFile) error {
	current, err := readHookFile(file.path)
	if err != nil {
		return err
	}
	if current.exists != file.exists || !bytes.Equal(current.raw, file.raw) {
		return fmt.Errorf("hooks file %s changed while Loaf was reconciling it — preserving it as written; rerun to reconcile against the current file", file.path)
	}
	return nil
}

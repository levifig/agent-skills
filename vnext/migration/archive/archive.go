// Package archive defines the integrity-protected, versioned handoff between the
// legacy Loaf database exporter and the vNext continuity rehearsal importer.
package archive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	Format  = "loaf-vnext-continuity-archive"
	Version = 1

	maxEncodedBytes          = 64 << 20
	maxRecords               = 100_000
	maxAggregatePayloadBytes = 32 << 20
)

const archiveDigestDomainV1 = "loaf:vnext:migration:archive:v1\x00"

// HandoffMappingUnparsedLegacyV1 declares that legacy handoff Markdown was
// preserved byte-for-byte as Situation without attempting field extraction.
const HandoffMappingUnparsedLegacyV1 = "unparsed_legacy_handoff_v1"

// JournalCategoryMappingUnsupportedToNoteV1 declares that effective legacy
// journal labels outside the closed vNext vocabulary were mapped to note.
const JournalCategoryMappingUnsupportedToNoteV1 = "unsupported_legacy_journal_category_to_note_v1"

// Archive is one immutable checksummed migration archive. Its SHA-256 detects
// accidental corruption; exporter provenance must be established out of band.
type Archive struct {
	Format        string  `json:"format"`
	Version       int     `json:"version"`
	Content       Content `json:"content"`
	ContentSHA256 string  `json:"content_sha256"`
}

// Content is the complete v1 archive preimage.
type Content struct {
	Source   Source             `json:"source"`
	Project  ProjectMapping     `json:"project"`
	Families FamilyManifest     `json:"included_families"`
	Records  []Record           `json:"records"`
	Expected ProjectionManifest `json:"expected_projection"`
}

// Source identifies the verified legacy backup used for export.
type Source struct {
	LegacySchemaVersion           int    `json:"legacy_schema_version"`
	BackupSHA256                  string `json:"backup_sha256"`
	BackupBytes                   int64  `json:"backup_bytes"`
	JournalFactRows               int    `json:"journal_fact_rows"`
	JournalProjectionRows         int    `json:"journal_projection_rows"`
	CollapsedRevisionRows         int    `json:"collapsed_revision_rows"`
	JournalOriginRows             int    `json:"journal_origin_rows"`
	DroppedSpecLinks              int    `json:"dropped_spec_links"`
	DroppedTaskLinks              int    `json:"dropped_task_links"`
	HandoffRows                   int    `json:"handoff_rows,omitempty"`
	HandoffMapping                string `json:"handoff_mapping,omitempty"`
	NormalizedJournalCategoryRows int    `json:"normalized_journal_category_rows,omitempty"`
	JournalCategoryMapping        string `json:"journal_category_mapping,omitempty"`
}

// ProjectMapping preserves the legacy durable identity exactly. Archive v1
// refuses identities that require a separate remapping registry.
type ProjectMapping struct {
	LegacyProjectID string               `json:"legacy_project_id"`
	ProjectID       continuity.ProjectID `json:"project_id"`
	Label           string               `json:"label"`
}

// FamilyManifest makes partial rehearsal archives incapable of silently
// becoming production cutover inputs.
type FamilyManifest struct {
	Project            bool `json:"project"`
	Journal            bool `json:"journal"`
	Wrap               bool `json:"wrap"`
	Ideas              bool `json:"ideas"`
	Sparks             bool `json:"sparks"`
	Handoffs           bool `json:"handoffs"`
	Decisions          bool `json:"decisions"`
	Explorations       bool `json:"explorations"`
	Findings           bool `json:"findings"`
	CompleteForCutover bool `json:"complete_for_cutover"`
}

// RecordKind is the closed record union supported by archive version 1.
type RecordKind string

const (
	RecordProject RecordKind = "project"
	RecordJournal RecordKind = "journal"
	RecordWrap    RecordKind = "wrap"
	RecordHandoff RecordKind = "handoff"
)

// Observation is source display provenance. It does not control vNext fold
// order and never creates a conversation lifecycle.
type Observation struct {
	ObservedAtMillis int64  `json:"observed_at_millis"`
	HarnessSessionID string `json:"harness_session_id,omitempty"`
	Branch           string `json:"branch,omitempty"`
	Worktree         string `json:"worktree,omitempty"`
}

// Record is a strict tagged union. Exactly one kind-specific payload is set.
type Record struct {
	Kind        RecordKind           `json:"kind"`
	SourceID    string               `json:"source_id,omitempty"`
	FactID      continuity.FactID    `json:"fact_id"`
	SubjectID   continuity.SubjectID `json:"subject_id"`
	Observation Observation          `json:"observation"`
	Project     *ProjectRecord       `json:"project,omitempty"`
	Journal     *JournalRecord       `json:"journal,omitempty"`
	Wrap        *WrapRecord          `json:"wrap,omitempty"`
	Handoff     *HandoffRecord       `json:"handoff,omitempty"`
}

// ProjectRecord is one project.registered payload.
type ProjectRecord struct {
	Label string `json:"label"`
}

// JournalRecord is one effective non-wrap journal.recorded payload.
type JournalRecord struct {
	Category continuity.JournalCategory `json:"category"`
	Scope    string                     `json:"scope,omitempty"`
	Text     string                     `json:"text"`
}

// WrapRecord is one deliberate wrap.recorded payload.
type WrapRecord struct {
	Scope     string `json:"scope,omitempty"`
	Synthesis string `json:"synthesis"`
}

// HandoffRecord is one legacy context-transfer packet. Archive v1 carries no
// focus because the legacy handoff model has no equivalent durable focus.
type HandoffRecord struct {
	Purpose           string   `json:"purpose"`
	Situation         string   `json:"situation,omitempty"`
	NextActions       string   `json:"next_actions,omitempty"`
	QuestionsAndRisks string   `json:"questions_and_risks,omitempty"`
	SuggestedSkills   []string `json:"suggested_skills"`
}

// ProjectionManifest checksums the semantic vNext result independently
// of environment-local clocks assigned during staged import.
type ProjectionManifest struct {
	ProjectCount          int    `json:"project_count"`
	EffectiveJournalCount int    `json:"effective_journal_count"`
	WrapCount             int    `json:"wrap_count"`
	HandoffCount          int    `json:"handoff_count,omitempty"`
	SHA256                string `json:"sha256"`
}

// Seal validates content, derives its expected semantic projection, and
// checksums the canonical preimage with a domain-separated SHA-256 digest.
func Seal(content Content) (Archive, error) {
	content.Expected = ProjectionManifest{}
	if err := validateContentV1(content, false); err != nil {
		return Archive{}, err
	}
	expected, err := expectedProjectionV1(content.Records)
	if err != nil {
		return Archive{}, err
	}
	content.Expected = expected
	archive := Archive{Format: Format, Version: Version, Content: content}
	digest, err := archiveDigestV1(archive)
	if err != nil {
		return Archive{}, err
	}
	archive.ContentSHA256 = digest
	return archive, nil
}

// Validate verifies structure, semantic projection, and checksummed bytes.
func (archive Archive) Validate() error {
	if archive.Format != Format {
		return fmt.Errorf("archive format must be %q", Format)
	}
	if archive.Version != Version {
		return fmt.Errorf("archive version must be %d", Version)
	}
	want, err := archiveDigestV1(Archive{Format: archive.Format, Version: archive.Version, Content: archive.Content})
	if err != nil {
		return err
	}
	if archive.ContentSHA256 != want {
		return fmt.Errorf("archive content digest does not match canonical content")
	}
	return validateContentV1(archive.Content, true)
}

// Marshal returns one canonical JSON document with a final newline.
func Marshal(archive Archive) ([]byte, error) {
	if err := archive.Validate(); err != nil {
		return nil, err
	}
	encoded, err := canonicalJSONV1(archive)
	if err != nil {
		return nil, fmt.Errorf("marshal migration archive: %w", err)
	}
	if len(encoded) > maxEncodedBytes {
		return nil, fmt.Errorf("migration archive exceeds %d encoded bytes", maxEncodedBytes)
	}
	return encoded, nil
}

// Parse strictly decodes and verifies the integrity of one canonical archive.
func Parse(encoded []byte) (Archive, error) {
	if len(encoded) > maxEncodedBytes {
		return Archive{}, fmt.Errorf("migration archive exceeds %d encoded bytes", maxEncodedBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var archive Archive
	if err := decoder.Decode(&archive); err != nil {
		return Archive{}, fmt.Errorf("decode migration archive: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Archive{}, fmt.Errorf("decode migration archive: multiple JSON values")
		}
		return Archive{}, fmt.Errorf("decode migration archive trailing data: %w", err)
	}
	canonical, err := canonicalJSONV1(archive)
	if err != nil {
		return Archive{}, fmt.Errorf("canonicalize migration archive: %w", err)
	}
	if !bytes.Equal(encoded, canonical) {
		return Archive{}, fmt.Errorf("migration archive is not canonical JSON")
	}
	if err := archive.Validate(); err != nil {
		return Archive{}, err
	}
	return archive, nil
}

func archiveDigestV1(archive Archive) (string, error) {
	preimage := struct {
		Format  string  `json:"format"`
		Version int     `json:"version"`
		Content Content `json:"content"`
	}{Format: archive.Format, Version: archive.Version, Content: archive.Content}
	encoded, err := canonicalJSONV1(preimage)
	if err != nil {
		return "", fmt.Errorf("encode archive digest preimage: %w", err)
	}
	digest := sha256.Sum256(append([]byte(archiveDigestDomainV1), encoded...))
	return hex.EncodeToString(digest[:]), nil
}

func canonicalJSONV1(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

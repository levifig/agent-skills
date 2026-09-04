package sqlite

import "github.com/levifig/loaf/vnext/continuity"

// legacyScratchpadFactV1 recognizes the exact retired vNext scratchpad wire
// family. It exists only so upgraded stores can leave historical rows intact
// while excluding them from every current projection and write surface.
func legacyScratchpadFactV1(subject continuity.SubjectRef, kind continuity.FactKind) bool {
	if subject.Kind != continuity.RecordKind("scratchpad") {
		return false
	}
	switch kind {
	case "scratchpad.opened",
		"scratchpad.participant-introduced",
		"scratchpad.message-recorded",
		"scratchpad.claim-recorded",
		"scratchpad.claim-released",
		"scratchpad.closed":
		return true
	default:
		return false
	}
}

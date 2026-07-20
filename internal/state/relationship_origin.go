package state

import (
	"sort"
	"strings"
)

// Relationship origin is closed at mechanism level: origin answers "by what
// mechanism did this row appear" while reason carries the operation detail
// ("recorded by intent create"). Writers, the doctor invariant SQL, and
// `state repair relationship-origin` all derive from this registry instead of
// restating the literals, so the vocabulary cannot drift again
// (path-stability-and-origin-hygiene, Decisions 2-4).
const (
	// relationshipOriginImported marks rows carried in by imports and migrations.
	relationshipOriginImported = "imported"
	// relationshipOriginManual marks rows written by explicit operator link
	// commands (Store.CreateLink) or operator-selected backfills; it is a live
	// code-written value, not a hand-authored-only one.
	relationshipOriginManual = "manual"
	// relationshipOriginCommand marks rows written by CLI ceremonies.
	relationshipOriginCommand = "command"
)

// allowedRelationshipOrigins returns the closed mechanism-level vocabulary in
// registration order. Consumers build SQL and validation from this list.
func allowedRelationshipOrigins() []string {
	return []string{relationshipOriginImported, relationshipOriginManual, relationshipOriginCommand}
}

// legacyRelationshipOriginReclassifications maps the retired origin values —
// written before the vocabulary closed — to their mechanism-level replacement.
// Repair reclassifies exactly these; any other unknown origin is foreign
// provenance that stays visible as a doctor warning and is never rewritten.
//
// 'intent-create', 'exploration-create', and 'legacy-conversion' are the
// retired per-ceremony values from the Intent, Exploration, and
// legacy-conversion writers.
//
// 'system' is retired writer provenance of a different shape: run.go and
// finding.go inlined it on the run→report, report→finding, run→finding,
// finding→verdict, and run→verdict relationships until 2026-07-19, when the
// source-literal parity scan exposed them and this Change normalized those
// writers to 'command'. Released alphas shipped the 'system' writers, so user
// databases can hold 'system' rows that no live writer produces — they are
// reclassifiable legacy, not foreign provenance.
func legacyRelationshipOriginReclassifications() map[string]string {
	return map[string]string{
		"intent-create":      relationshipOriginCommand,
		"legacy-conversion":  relationshipOriginCommand,
		"exploration-create": relationshipOriginCommand,
		"system":             relationshipOriginCommand,
	}
}

// legacyRelationshipOrigins returns the reclassifiable legacy origins in
// deterministic order for planning and reporting.
func legacyRelationshipOrigins() []string {
	legacy := legacyRelationshipOriginReclassifications()
	origins := make([]string, 0, len(legacy))
	for origin := range legacy {
		origins = append(origins, origin)
	}
	sort.Strings(origins)
	return origins
}

// relationshipOriginNotAllowedFragment builds a parameterized `column NOT IN
// (...)` fragment covering the allowed vocabulary, plus its bind arguments, so
// SQL consumers never inline the origin literals.
func relationshipOriginNotAllowedFragment(column string) (string, []any) {
	return parameterizedNotInFragment(column, allowedRelationshipOrigins())
}

// legacyRelationshipOriginNotInFragment builds the same parameterized fragment
// over the reclassifiable legacy origins.
func legacyRelationshipOriginNotInFragment(column string) (string, []any) {
	return parameterizedNotInFragment(column, legacyRelationshipOrigins())
}

func parameterizedNotInFragment(column string, values []string) (string, []any) {
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, value := range values {
		placeholders[i] = "?"
		args[i] = value
	}
	return column + " NOT IN (" + strings.Join(placeholders, ", ") + ")", args
}

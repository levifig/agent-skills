package state

const (
	LifecycleStatusDraft      = "draft"
	LifecycleStatusOpen       = "open"
	LifecycleStatusTodo       = "todo"
	LifecycleStatusInProgress = "in_progress"
	LifecycleStatusBlocked    = "blocked"
	LifecycleStatusReview     = "review"
	LifecycleStatusDone       = "done"
	LifecycleStatusPaused     = "paused"
	LifecycleStatusArchived   = "archived"
)

const (
	LifecycleEntitySpec       = "spec"
	LifecycleEntityTask       = "task"
	LifecycleEntityReport     = "report"
	LifecycleEntitySession    = "session"
	LifecycleEntityIdea       = "idea"
	LifecycleEntitySpark      = "spark"
	LifecycleEntityBrainstorm = "brainstorm"
	LifecycleEntityPlan       = "plan"
	LifecycleEntityHandoff    = "handoff"
	LifecycleEntityCouncil    = "council"
)

var lifecycleStatusOrder = []string{
	LifecycleStatusDraft,
	LifecycleStatusOpen,
	LifecycleStatusTodo,
	LifecycleStatusInProgress,
	LifecycleStatusBlocked,
	LifecycleStatusReview,
	LifecycleStatusDone,
	LifecycleStatusPaused,
	LifecycleStatusArchived,
}

var lifecycleEntityOrder = []string{
	LifecycleEntitySpec,
	LifecycleEntityTask,
	LifecycleEntityReport,
	LifecycleEntitySession,
	LifecycleEntityIdea,
	LifecycleEntitySpark,
	LifecycleEntityBrainstorm,
	LifecycleEntityPlan,
	LifecycleEntityHandoff,
	LifecycleEntityCouncil,
}

var lifecycleEntityStatuses = map[string][]string{
	LifecycleEntitySpec: {
		LifecycleStatusDraft,
		LifecycleStatusTodo,
		LifecycleStatusInProgress,
		LifecycleStatusDone,
		LifecycleStatusArchived,
	},
	LifecycleEntityTask: {
		LifecycleStatusTodo,
		LifecycleStatusInProgress,
		LifecycleStatusBlocked,
		LifecycleStatusReview,
		LifecycleStatusDone,
		LifecycleStatusArchived,
	},
	LifecycleEntityReport: {
		LifecycleStatusDraft,
		LifecycleStatusDone,
		LifecycleStatusArchived,
	},
	LifecycleEntitySession: {
		LifecycleStatusInProgress,
		LifecycleStatusPaused,
		LifecycleStatusBlocked,
		LifecycleStatusDone,
		LifecycleStatusArchived,
	},
	LifecycleEntityIdea: {
		LifecycleStatusOpen,
		LifecycleStatusDone,
		LifecycleStatusArchived,
	},
	LifecycleEntitySpark: {
		LifecycleStatusOpen,
		LifecycleStatusDone,
		LifecycleStatusArchived,
	},
	LifecycleEntityBrainstorm: {
		LifecycleStatusOpen,
		LifecycleStatusDone,
		LifecycleStatusArchived,
	},
	LifecycleEntityPlan: {
		LifecycleStatusDraft,
		LifecycleStatusDone,
		LifecycleStatusArchived,
	},
	LifecycleEntityHandoff: {
		LifecycleStatusDraft,
		LifecycleStatusDone,
		LifecycleStatusArchived,
	},
	LifecycleEntityCouncil: {
		LifecycleStatusDraft,
		LifecycleStatusDone,
		LifecycleStatusArchived,
	},
}

var lifecycleLegacyStatusMap = map[string]map[string]string{
	LifecycleEntitySpec: {
		"drafting":     LifecycleStatusDraft,
		"approved":     LifecycleStatusTodo,
		"implementing": LifecycleStatusInProgress,
		"complete":     LifecycleStatusDone,
		"archived":     LifecycleStatusArchived,
	},
	LifecycleEntityTask: {
		"todo":        LifecycleStatusTodo,
		"in_progress": LifecycleStatusInProgress,
		"blocked":     LifecycleStatusBlocked,
		"review":      LifecycleStatusReview,
		"done":        LifecycleStatusDone,
		"archived":    LifecycleStatusArchived,
	},
	LifecycleEntityReport: {
		"draft":    LifecycleStatusDraft,
		"final":    LifecycleStatusDone,
		"archived": LifecycleStatusArchived,
	},
	LifecycleEntitySession: {
		"active":   LifecycleStatusInProgress,
		"stopped":  LifecycleStatusPaused,
		"blocked":  LifecycleStatusBlocked,
		"done":     LifecycleStatusDone,
		"archived": LifecycleStatusArchived,
	},
	LifecycleEntityIdea: {
		"open":     LifecycleStatusOpen,
		"resolved": LifecycleStatusDone,
		"archived": LifecycleStatusArchived,
	},
	LifecycleEntitySpark: {
		"open":     LifecycleStatusOpen,
		"resolved": LifecycleStatusDone,
		"archived": LifecycleStatusArchived,
	},
	LifecycleEntityBrainstorm: {
		"open":     LifecycleStatusOpen,
		"resolved": LifecycleStatusDone,
		"archived": LifecycleStatusArchived,
	},
	LifecycleEntityPlan: {
		"draft":    LifecycleStatusDraft,
		"final":    LifecycleStatusDone,
		"archived": LifecycleStatusArchived,
	},
	LifecycleEntityHandoff: {
		"draft":    LifecycleStatusDraft,
		"final":    LifecycleStatusDone,
		"archived": LifecycleStatusArchived,
	},
	LifecycleEntityCouncil: {
		"draft":    LifecycleStatusDraft,
		"final":    LifecycleStatusDone,
		"archived": LifecycleStatusArchived,
	},
}

var nonLifecycleStatusVocabularies = []string{
	"finding_status",
	"verdict_outcome",
	"run_status",
}

// LifecycleStatuses returns canonical lifecycle statuses in global display order.
func LifecycleStatuses() []string {
	return append([]string(nil), lifecycleStatusOrder...)
}

// LifecycleEntityKinds returns lifecycle-managed entity kinds in stable order.
func LifecycleEntityKinds() []string {
	return append([]string(nil), lifecycleEntityOrder...)
}

// LifecycleStatusesForEntity returns canonical statuses accepted by one entity kind.
func LifecycleStatusesForEntity(kind string) []string {
	statuses, ok := lifecycleEntityStatuses[kind]
	if !ok {
		return nil
	}
	return append([]string(nil), statuses...)
}

// ValidLifecycleStatus reports whether status is part of the canonical lifecycle vocabulary.
func ValidLifecycleStatus(status string) bool {
	return containsString(lifecycleStatusOrder, status)
}

// ValidLifecycleEntityStatus reports whether an entity kind accepts a canonical lifecycle status.
func ValidLifecycleEntityStatus(kind string, status string) bool {
	return containsString(lifecycleEntityStatuses[kind], status)
}

// CanonicalLifecycleStatus maps legacy or canonical spellings to the canonical lifecycle status.
func CanonicalLifecycleStatus(kind string, status string) (string, bool) {
	if ValidLifecycleEntityStatus(kind, status) {
		return status, true
	}
	mappings, ok := lifecycleLegacyStatusMap[kind]
	if !ok {
		return "", false
	}
	canonical, ok := mappings[status]
	if !ok || !ValidLifecycleEntityStatus(kind, canonical) {
		return "", false
	}
	return canonical, true
}

// LifecycleStatusMatches reports whether a legacy or canonical status represents the canonical status.
func LifecycleStatusMatches(kind string, status string, canonical string) bool {
	mapped, ok := CanonicalLifecycleStatus(kind, status)
	return ok && mapped == canonical
}

// LifecycleStatusForDisplay returns the canonical display spelling when the status is lifecycle-managed.
func LifecycleStatusForDisplay(kind string, status string) string {
	canonical, ok := CanonicalLifecycleStatus(kind, status)
	if !ok {
		return status
	}
	return canonical
}

// LifecycleStatusFilterMatches compares a stored status and user filter through canonical lifecycle mappings.
func LifecycleStatusFilterMatches(kind string, status string, filter string) bool {
	if filter == "" {
		return true
	}
	canonicalStatus, statusOK := CanonicalLifecycleStatus(kind, status)
	canonicalFilter, filterOK := CanonicalLifecycleStatus(kind, filter)
	if statusOK && filterOK {
		return canonicalStatus == canonicalFilter
	}
	return status == filter
}

// NonLifecycleStatusVocabularies returns explicit status-like vocabularies excluded from lifecycle canonicalization.
func NonLifecycleStatusVocabularies() []string {
	return append([]string(nil), nonLifecycleStatusVocabularies...)
}

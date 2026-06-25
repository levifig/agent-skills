package state

var findingStatusOrder = []string{"open", "confirmed", "refuted", "partial", "archived"}
var findingSeverityOrder = []string{"critical", "high", "medium", "low", "info"}
var findingConfidenceOrder = []string{"high", "medium", "low"}
var verdictOutcomeOrder = []string{"confirmed", "refuted", "partial"}
var runStatusOrder = []string{"pending", "running", "completed", "failed", "archived"}

// ValidFindingStatus reports whether status is a known finding status.
func ValidFindingStatus(status string) bool {
	return containsString(findingStatusOrder, status)
}

// FindingStatuses returns valid finding statuses in display order.
func FindingStatuses() []string {
	return append([]string(nil), findingStatusOrder...)
}

// ValidFindingSeverity reports whether severity is a known finding severity.
func ValidFindingSeverity(severity string) bool {
	return containsString(findingSeverityOrder, severity)
}

// FindingSeverities returns valid finding severities in display order.
func FindingSeverities() []string {
	return append([]string(nil), findingSeverityOrder...)
}

// ValidFindingConfidence reports whether confidence is a known finding confidence.
func ValidFindingConfidence(confidence string) bool {
	return containsString(findingConfidenceOrder, confidence)
}

// FindingConfidences returns valid finding confidences in display order.
func FindingConfidences() []string {
	return append([]string(nil), findingConfidenceOrder...)
}

// ValidVerdictOutcome reports whether outcome is a known verdict outcome.
func ValidVerdictOutcome(outcome string) bool {
	return containsString(verdictOutcomeOrder, outcome)
}

// VerdictOutcomes returns valid verdict outcomes in display order.
func VerdictOutcomes() []string {
	return append([]string(nil), verdictOutcomeOrder...)
}

// ValidRunStatus reports whether status is a known run provenance status.
func ValidRunStatus(status string) bool {
	return containsString(runStatusOrder, status)
}

// RunStatuses returns valid run provenance statuses in display order.
func RunStatuses() []string {
	return append([]string(nil), runStatusOrder...)
}

func containsString(values []string, value string) bool {
	for _, valid := range values {
		if value == valid {
			return true
		}
	}
	return false
}

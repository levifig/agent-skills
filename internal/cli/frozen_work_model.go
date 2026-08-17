package cli

import (
	"fmt"
	"io"
)

const (
	workModelRetirementHorizon = "0.5.0"
	workModelRetirementIssue   = "LOAF-47"
)

func frozenWorkModelRetirementNote() string {
	return fmt.Sprintf("Retirement is planned for %s (%s)", workModelRetirementHorizon, workModelRetirementIssue)
}

func frozenWorkModelError(namespace string) error {
	return fmt.Errorf("%s is frozen pending migration; use loaf issue for new work. %s", namespace, frozenWorkModelRetirementNote())
}

func writeFrozenWorkModelNote(out io.Writer, namespace string, writeVerbs string) {
	fmt.Fprintf(out, "Deprecated: %s write verbs (%s) are frozen pending migration. Use loaf issue for new work. %s.\n", namespace, writeVerbs, frozenWorkModelRetirementNote())
}

func frozenWorkModelVerbSummary(command string) string {
	return fmt.Sprintf("Deprecated: %s is frozen pending migration. Use loaf issue for new work. %s.", command, frozenWorkModelRetirementNote())
}

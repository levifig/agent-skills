package cli

import (
	"fmt"
	"io"
)

const workModelMigrationIssue = "LOAF-42"

func frozenWorkModelError(namespace string) error {
	return fmt.Errorf("%s is frozen pending migration; use loaf issue for new work. Retirement is gated on %s", namespace, workModelMigrationIssue)
}

func writeFrozenWorkModelNote(out io.Writer, namespace string, writeVerbs string) {
	fmt.Fprintf(out, "Deprecated: %s write verbs (%s) are frozen pending migration. Use loaf issue for new work. Retirement is gated on %s.\n", namespace, writeVerbs, workModelMigrationIssue)
}

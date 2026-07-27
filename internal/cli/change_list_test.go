package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestChangeListUnitsProjection(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeNewLayoutChange(t, repo, "20260727-listed", "listed", "2.0.0", "")
	writeNewLayoutChange(t, repo, "20260727-other", "other", "2.1.0", "")
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "list", "--target", "2.0.0", "--json"}); err != nil {
		t.Fatalf("list: %v\n%s", err, stdout.String())
	}
	var result changeListUnitJSON
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result.Target != "2.0.0" || len(result.Units) != 1 || result.Units[0].Slug != "listed" {
		t.Fatalf("result = %+v", result)
	}
}

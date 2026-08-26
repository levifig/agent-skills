package auth_test

import (
	"errors"
	"testing"

	"github.com/levifig/loaf/internal/auth"
)

func TestCommandRequiresAttach(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args []string
		want bool
	}{
		{args: []string{"journal", "recent"}, want: true},
		{args: []string{"issue", "list"}, want: true},
		{args: []string{"auth", "setup"}, want: false},
		{args: []string{"build"}, want: false},
		{args: []string{"serve"}, want: false},
		{args: []string{"state", "init"}, want: false},
		{args: []string{"state", "doctor"}, want: true},
		{args: []string{"journal", "--help"}, want: false},
	}
	for _, tc := range cases {
		if got := auth.CommandRequiresAttach(tc.args); got != tc.want {
			t.Fatalf("CommandRequiresAttach(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestCheckAttachedRefusesWhenMissing(t *testing.T) {
	t.Parallel()
	err := auth.CheckAttached(t.TempDir(), "proj_test", "loaf journal recent")
	var refusal *auth.UnattachedError
	if !errors.As(err, &refusal) {
		t.Fatalf("CheckAttached() = %v, want *auth.UnattachedError", err)
	}
	if refusal.Code != auth.UnattachedCode {
		t.Fatalf("code = %q, want %q", refusal.Code, auth.UnattachedCode)
	}
}

func TestCheckAttachedAllowsWhenPresent(t *testing.T) {
	t.Parallel()
	dataHome := t.TempDir()
	projectID := "proj_attached"
	if err := auth.SaveAttachRecord(dataHome, auth.AttachRecord{ProjectID: projectID, EnvID: "cursor:loaf", Endpoint: "http://127.0.0.1:8080"}); err != nil {
		t.Fatalf("SaveAttachRecord() error = %v", err)
	}
	if err := auth.CheckAttached(dataHome, projectID, "loaf journal recent"); err != nil {
		t.Fatalf("CheckAttached() error = %v, want nil", err)
	}
}

package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// parseHelpSectionNames extracts the first token of each indented line in the
// named help section, e.g. the source names under "Sources:".
func parseHelpSectionNames(t *testing.T, help string, section string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	inSection := false
	for _, line := range strings.Split(help, "\n") {
		if strings.TrimSpace(line) == section {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			names[fields[0]] = true
		}
	}
	if len(names) == 0 {
		t.Fatalf("help output has no %q section entries:\n%s", section, help)
	}
	return names
}

func TestStateMigrateHelpListsEveryDispatchableSource(t *testing.T) {
	// stateMigrateSources is the registry runStateMigrate dispatches through,
	// so its key set IS the dispatcher's source set: a source cannot become
	// dispatchable without an entry here, and this test then forces it into
	// the writeStateMigrateHelp source list.
	var help bytes.Buffer
	writeStateMigrateHelp(&help)
	listed := parseHelpSectionNames(t, help.String(), "Sources:")
	for source, entry := range stateMigrateSources {
		if entry.run == nil {
			t.Errorf("stateMigrateSources[%q] has no run func", source)
		}
		if entry.help == nil {
			t.Errorf("stateMigrateSources[%q] has no help func", source)
		}
		if !listed[source] {
			t.Errorf("writeStateMigrateHelp does not list dispatchable source %q", source)
		}
	}
	for source := range listed {
		if _, ok := stateMigrateSources[source]; !ok {
			t.Errorf("writeStateMigrateHelp lists %q, which is not dispatchable via stateMigrateSources", source)
		}
	}
}

func TestStateMigrateEveryListedSourceDispatches(t *testing.T) {
	// Behavioral probe of the dispatcher: every source runStateMigrate accepts
	// must reach its runner instead of the not-implemented fallthrough. Each
	// runner rejects the unknown option during arg parsing, before any state
	// access, so no database is ever touched.
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	for source := range stateMigrateSources {
		t.Run(source, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: t.TempDir(),
			}.Run([]string{"state", "migrate", source, "--no-such-option"})
			if err == nil {
				t.Fatalf("Run(state migrate %s --no-such-option) error = nil, want unknown-option rejection from the source runner", source)
			}
			if strings.Contains(err.Error(), "is not implemented yet") {
				t.Fatalf("state migrate %q did not dispatch: %v", source, err)
			}
		})
	}
}

func TestRunnerStateMigrateHelpListsAllSources(t *testing.T) {
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: t.TempDir(),
	}.Run([]string{"state", "migrate", "--help"})
	if err != nil {
		t.Fatalf("Run(state migrate --help) error = %v", err)
	}
	wants := []string{
		"Usage: loaf state migrate <source> [options]",
		"markdown",
		"storage-home",
		"schema",
		"lifecycle-statuses",
		"journal-first",
		"deferrals",
	}
	for _, want := range wants {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerStateMigrateSourceHelpIsNative(t *testing.T) {
	for source := range stateMigrateSources {
		t.Run(source, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: t.TempDir(),
			}.Run([]string{"state", "migrate", source, "--help"})
			if err != nil {
				t.Fatalf("Run(state migrate %s --help) error = %v", source, err)
			}
			want := "Usage: loaf state migrate " + source
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("stdout = %q, want %q", stdout.String(), want)
			}
		})
	}
}

// migrateDispatchableSources is anchored to the runMigrate switch in cli.go.
// runMigrate dispatches through a switch rather than a registry, so this slice
// is the test-side transcription the help surface is held against; adding a
// case to that switch without adding it here leaves the new source unprobed,
// and adding it here without listing it in writeMigrateHelp fails parity.
var migrateDispatchableSources = []string{"lifecycle-statuses", "journal-first", "schema", "markdown", "storage-home", "worktree-storage"}

func TestRunnerMigrateHelpExitsZeroAndListsEverySource(t *testing.T) {
	// Bare `loaf migrate --help` used to fall through the switch and exit
	// non-zero with `migrate source "--help" is not implemented yet`.
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: t.TempDir(),
	}.Run([]string{"migrate", "--help"})
	if err != nil {
		t.Fatalf("Run(migrate --help) error = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "Usage: loaf migrate <source> [options]") {
		t.Fatalf("stdout = %q, want migrate usage line", stdout.String())
	}
	listed := parseHelpSectionNames(t, stdout.String(), "Sources:")
	for _, source := range migrateDispatchableSources {
		if !listed[source] {
			t.Errorf("writeMigrateHelp does not list dispatchable source %q", source)
		}
	}
	for source := range listed {
		found := false
		for _, dispatchable := range migrateDispatchableSources {
			if source == dispatchable {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("writeMigrateHelp lists %q, which the runMigrate switch does not dispatch", source)
		}
	}
}

func TestRunnerMigrateBareInvocationErrorsAndPrintsHelpToStderr(t *testing.T) {
	// Bare `loaf migrate` keeps its non-zero exit; the source list goes to
	// stderr so the error is self-correcting.
	var stdout, stderr bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stderr:     &stderr,
		WorkingDir: t.TempDir(),
	}.Run([]string{"migrate"})
	if err == nil {
		t.Fatalf("Run(migrate) error = nil, want missing-source error")
	}
	if !strings.Contains(err.Error(), "migrate requires a source") {
		t.Fatalf("Run(migrate) error = %v, want %q", err, "migrate requires a source")
	}
	if !strings.Contains(stderr.String(), "Usage: loaf migrate <source> [options]") {
		t.Fatalf("stderr = %q, want migrate usage line", stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty on the error path", stdout.String())
	}
}

func TestRunnerMigrateEveryListedSourceDispatches(t *testing.T) {
	// Behavioral probe: every source writeMigrateHelp advertises must reach a
	// runner instead of the not-implemented fallthrough. Each runner rejects
	// the unknown option during arg parsing, before any state access, so no
	// database is ever touched.
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	for _, source := range migrateDispatchableSources {
		t.Run(source, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: t.TempDir(),
			}.Run([]string{"migrate", source, "--no-such-option"})
			if err == nil {
				t.Fatalf("Run(migrate %s --no-such-option) error = nil, want unknown-option rejection from the source runner", source)
			}
			if strings.Contains(err.Error(), "is not implemented yet") {
				t.Fatalf("migrate %q did not dispatch: %v", source, err)
			}
		})
	}
}

func TestRunnerMigrateSourceHelpIsNative(t *testing.T) {
	// Anchored to the runMigrate switch in cli.go: every dispatchable
	// top-level migrate source must answer --help with its usage.
	for _, source := range migrateDispatchableSources {
		t.Run(source, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: t.TempDir(),
			}.Run([]string{"migrate", source, "--help"})
			if err != nil {
				t.Fatalf("Run(migrate %s --help) error = %v", source, err)
			}
			want := "Usage: loaf migrate " + source
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("stdout = %q, want %q", stdout.String(), want)
			}
		})
	}
}

func TestRunnerConversationLeafHelpIsNative(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "conversation handle add --help", args: []string{"conversation", "handle", "add", "--help"}, want: "Usage: loaf conversation handle add <conversation-id>"},
		{name: "conversation handle add -h", args: []string{"conversation", "handle", "add", "-h"}, want: "Usage: loaf conversation handle add <conversation-id>"},
		{name: "exploration conversation add --help", args: []string{"exploration", "conversation", "add", "--help"}, want: "Usage: loaf exploration conversation add <exploration> <conversation-id>"},
		{name: "exploration conversation add -h", args: []string{"exploration", "conversation", "add", "-h"}, want: "Usage: loaf exploration conversation add <exploration> <conversation-id>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: t.TempDir(),
			}.Run(tc.args)
			if err != nil {
				t.Fatalf("Run(%v) error = %v", tc.args, err)
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout = %q, want %q", stdout.String(), tc.want)
			}
		})
	}
}

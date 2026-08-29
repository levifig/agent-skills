package command

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestKernelCommandSurfaceIsLimitedToIntrospection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "help",
			args:       nil,
			wantCode:   0,
			wantStdout: "usage: loaf <version|ownership>\n",
		},
		{
			name:       "version",
			args:       []string{"version"},
			wantCode:   0,
			wantStdout: "loaf vnext schema vnext/1\n",
		},
		{
			name:     "ownership",
			args:     []string{"ownership"},
			wantCode: 0,
			wantStdout: strings.Join([]string{
				"loaf: flow-ceremonies, skills, templates, profiles, project-identity, private-continuity, derived-context, private-sync",
				"tracker: work-identity, work-definition, definition-of-done, workflow-state, hierarchy, assignment, collaboration",
				"git: code, promoted-artifacts",
				"harness: execution, model-selection, tool-boundaries, service-connections, service-credentials",
				"",
			}, "\n"),
		},
		{
			name:       "unknown command",
			args:       []string{"issue"},
			wantCode:   2,
			wantStderr: "unknown command \"issue\"\nusage: loaf <version|ownership>\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := New(&stdout, &stderr).Run(test.args)

			if code != test.wantCode {
				t.Errorf("Run() code = %d, want %d", code, test.wantCode)
			}
			if stdout.String() != test.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), test.wantStdout)
			}
			if stderr.String() != test.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), test.wantStderr)
			}
		})
	}
}

func TestKernelCommandSurfaceReturnsFailureWhenOutputFails(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	code := New(failingWriter{}, &stderr).Run([]string{"version"})
	if code != 1 {
		t.Errorf("Run() code = %d, want 1", code)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty output", stderr.String())
	}
}

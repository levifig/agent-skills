package state

import "github.com/levifig/loaf/internal/project"

// Runtime is the minimal state-runtime boundary. SQLite storage, migrations,
// and state commands build on this package in later SPEC-040 tasks.
type Runtime struct {
	root project.WorkingDirectory
}

// NewRuntime creates a state runtime rooted at a resolved project directory.
func NewRuntime(root project.WorkingDirectory) Runtime {
	return Runtime{root: root}
}

// Name identifies the current Go runtime without implying SQLite readiness.
func (r Runtime) Name() string {
	return "loaf state runtime (go skeleton)"
}

// RootPath returns the normalized project directory observed by the runtime.
func (r Runtime) RootPath() string {
	return r.root.Path()
}

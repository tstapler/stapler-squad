package session

import "testing"

// NewStartedInstanceForTest returns an Instance that reports itself started and
// routes all terminal I/O through pm, so tests outside this package (e.g.
// server/mcp handler tests) can observe SendKeys traffic without a real tmux
// session. The testing.TB parameter keeps it out of production call sites; the
// Instance is otherwise zero-valued, so only terminal I/O methods are safe.
func NewStartedInstanceForTest(tb testing.TB, title string, pm ProcessManager) *Instance {
	tb.Helper()
	inst := &Instance{Title: title}
	inst.processManager = pm
	inst.started.Store(true)
	return inst
}

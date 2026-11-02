package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/tymux"
)

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

// SetTymuxManagerFactoryForTest makes every tymux-backed Instance built until tb ends use
// the manager fn returns, so tests outside this package need no live tymuxd. Not safe
// alongside parallel tests that build tymux Instances.
func SetTymuxManagerFactoryForTest(tb testing.TB, fn func() tymux.TymuxManager) {
	tb.Helper()
	orig := newTymuxSessionFn
	newTymuxSessionFn = fn
	tb.Cleanup(func() { newTymuxSessionFn = orig })
}

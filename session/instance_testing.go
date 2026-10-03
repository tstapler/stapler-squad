package session

// NewStartedInstanceForTest returns an Instance that reports itself started and
// routes all terminal I/O through pm, so tests outside this package (e.g.
// server/mcp handler tests) can observe SendKeys traffic without a real tmux
// session. Production code must not call it.
func NewStartedInstanceForTest(title string, pm ProcessManager) *Instance {
	inst := &Instance{Title: title}
	inst.processManager = pm
	inst.started.Store(true)
	return inst
}

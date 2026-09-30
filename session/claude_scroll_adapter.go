package session

// ClaudeScrollAdapter is the ScrollAdapter for Claude Code sessions. Story
// 1.2.1's live spike confirmed PageUp (\x1b[5~) scrolls Claude Code's
// fullscreen conversation view -- GestureForwardStrategy, not the
// NativeDumpFallbackStrategy fallback ADR-001 anticipated needing.
type ClaudeScrollAdapter struct {
	strategy ScrollAdapter
}

// NewClaudeScrollAdapter constructs a ClaudeScrollAdapter wired to the
// spike-confirmed GestureForwardStrategy.
func NewClaudeScrollAdapter() *ClaudeScrollAdapter {
	return &ClaudeScrollAdapter{strategy: NewClaudeGestureForwardStrategy()}
}

func (a *ClaudeScrollAdapter) Name() string {
	return "claude"
}

// CanHandle delegates to isClaude, the existing hardened program-matching
// helper (session/instance_tmux.go), rather than reimplementing its rule --
// delegating directly is what actually closes the drift risk this adapter
// exists to avoid (the isClaudeAntigravityFamily-vs-AgyAdapter.CanHandle
// class of bug named in Tech Debt Disposition), since two independent
// implementations of the same rule can never re-diverge.
func (a *ClaudeScrollAdapter) CanHandle(program string) bool {
	return isClaude(program)
}

// claudeScrollAdapterZeroValue guards against a zero-value ClaudeScrollAdapter{}
// bypassing NewClaudeScrollAdapter, which is the only place strategy is set.
const claudeScrollAdapterZeroValue = "ClaudeScrollAdapter: use NewClaudeScrollAdapter, not a zero-value literal"

func (a *ClaudeScrollAdapter) Capability() ScrollForwardCapability {
	if a.strategy == nil {
		panic(claudeScrollAdapterZeroValue)
	}
	return a.strategy.Capability()
}

func (a *ClaudeScrollAdapter) KeySequences(dir ScrollDirection) [][]byte {
	if a.strategy == nil {
		panic(claudeScrollAdapterZeroValue)
	}
	return a.strategy.KeySequences(dir)
}

func (a *ClaudeScrollAdapter) CaptureVia() ScrollCaptureMode {
	if a.strategy == nil {
		panic(claudeScrollAdapterZeroValue)
	}
	return a.strategy.CaptureVia()
}

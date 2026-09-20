package session

import "testing"

func TestResolveScrollAdapter_should_ReturnClaudeScrollAdapter_When_ProgramIsClaude(t *testing.T) {
	adapter := resolveScrollAdapter("claude")
	if adapter == nil {
		t.Fatal("resolveScrollAdapter(\"claude\") = nil, want non-nil")
	}
	claudeAdapter, ok := adapter.(*ClaudeScrollAdapter)
	if !ok {
		t.Fatalf("resolveScrollAdapter(\"claude\") returned %T, want *ClaudeScrollAdapter", adapter)
	}
	if !claudeAdapter.CanHandle("claude") {
		t.Error("claudeAdapter.CanHandle(\"claude\") = false, want true")
	}
}

func TestResolveScrollAdapter_should_ReturnNil_When_ProgramIsUnregistered(t *testing.T) {
	if adapter := resolveScrollAdapter("bash"); adapter != nil {
		t.Fatalf("resolveScrollAdapter(\"bash\") = %T, want nil", adapter)
	}
}

// TestClaudeScrollAdapter_CanHandle_should_AgreeWithIsClaude_When_EvaluatedAgainstSharedTestVectors
// is a regression test for the isClaudeAntigravityFamily-vs-AgyAdapter.CanHandle
// class of drift bug named in Tech Debt Disposition: it runs the same vector
// set through both isClaude and ClaudeScrollAdapter{}.CanHandle and asserts
// they agree on every one.
func TestClaudeScrollAdapter_CanHandle_should_AgreeWithIsClaude_When_EvaluatedAgainstSharedTestVectors(t *testing.T) {
	vectors := []string{"claude", "CLAUDE", "env -u VAR claude", "claude-squad"}
	adapter := &ClaudeScrollAdapter{}

	for _, v := range vectors {
		want := isClaude(v)
		got := adapter.CanHandle(v)
		if got != want {
			t.Errorf("isClaude(%q) = %v, ClaudeScrollAdapter.CanHandle(%q) = %v, want agreement", v, want, v, got)
		}
	}

	// Explicit regression guard named in the acceptance criteria: "claude-squad"
	// must agree as false on both sides, not just agree with whatever isClaude
	// happens to return.
	if isClaude("claude-squad") || adapter.CanHandle("claude-squad") {
		t.Error("\"claude-squad\" should be false for both isClaude and CanHandle")
	}
}

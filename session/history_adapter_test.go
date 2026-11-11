package session

import "testing"

// TestResolveHistoryAdapter covers the shared resolution helper used by PortSessionHistory
// and both checkpoint create/fork call sites: claude and agy/antigravity resolve to their
// adapters, while gemini (real Gemini CLI — a different storage format from Antigravity's
// own ~/.gemini/antigravity-cli/... layout) and other unmatched programs resolve to nil.
func TestResolveHistoryAdapter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		program  string
		wantName string // "" means resolveHistoryAdapter must return nil
	}{
		{"claude", "claude"},
		{"agy", "agy"},
		{"antigravity", "agy"},
		{"gemini", "gemini"},
		{"opencode", "opencode"},
		{"pi", "pi"},
		{"aider", ""},
		{"bash", ""},
	}

	for _, tt := range tests {
		t.Run(tt.program, func(t *testing.T) {
			t.Parallel()
			got := DefaultHistoryAdapters().Resolve(tt.program)
			if tt.wantName == "" {
				if got != nil {
					t.Errorf("resolveHistoryAdapter(%q) = %q, want nil", tt.program, got.Name())
				}
				return
			}
			if got == nil || got.Name() != tt.wantName {
				t.Errorf("resolveHistoryAdapter(%q) = %v, want adapter named %q", tt.program, got, tt.wantName)
			}
		})
	}
}

// TestResolveHistoryAdapter_PriorityOrder pins first-match-wins ordering for programs that
// more than one adapter could claim (e.g. "antigravity-claude" matches both claude and agy).
func TestResolveHistoryAdapter_PriorityOrder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		program  string
		wantName string
	}{
		{"claude", "claude"},
		{"agy", "agy"},
		{"opencode", "opencode"},
		{"gemini", "gemini"},
		{"pi", "pi"},
		{"aider", ""},
		{"", ""},
		{"antigravity-claude", "claude"},
	}
	for _, tt := range tests {
		t.Run(tt.program, func(t *testing.T) {
			t.Parallel()
			got := DefaultHistoryAdapters().Resolve(tt.program)
			if tt.wantName == "" {
				if got != nil {
					t.Fatalf("got %q, want nil", got.Name())
				}
				return
			}
			if got == nil || got.Name() != tt.wantName {
				t.Fatalf("got %v, want %q", got, tt.wantName)
			}
		})
	}
}

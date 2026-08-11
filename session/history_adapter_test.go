package session

import "testing"

// TestResolveHistoryAdapter covers the shared resolution helper used by PortSessionHistory
// and both checkpoint create/fork call sites: claude and agy/antigravity resolve to their
// adapters, while gemini (real Gemini CLI — a different storage format from Antigravity's
// own ~/.gemini/antigravity-cli/... layout) and other unmatched programs resolve to nil.
func TestResolveHistoryAdapter(t *testing.T) {
	tests := []struct {
		program  string
		wantName string // "" means resolveHistoryAdapter must return nil
	}{
		{"claude", "claude"},
		{"agy", "agy"},
		{"antigravity", "agy"},
		{"gemini", ""},
		{"opencode", ""},
		{"aider", ""},
		{"bash", ""},
	}

	for _, tt := range tests {
		got := resolveHistoryAdapter(tt.program)
		if tt.wantName == "" {
			if got != nil {
				t.Errorf("resolveHistoryAdapter(%q) = %q, want nil", tt.program, got.Name())
			}
			continue
		}
		if got == nil || got.Name() != tt.wantName {
			t.Errorf("resolveHistoryAdapter(%q) = %v, want adapter named %q", tt.program, got, tt.wantName)
		}
	}
}

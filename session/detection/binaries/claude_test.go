package binaries

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/detection/dtypes"
)

func TestClaudeDetector_Name(t *testing.T) {
	t.Parallel()
	d := NewClaudeDetector()
	if d.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", d.Name(), "claude")
	}
}

func TestClaudeDetector_Patterns_should_haveReadyPattern(t *testing.T) {
	t.Parallel()
	d := NewClaudeDetector()
	p := d.Patterns()
	if len(p.Ready) == 0 {
		t.Fatal("Patterns().Ready is empty, expected at least one pattern")
	}
	if p.Ready[0].Name != "claude_prompt" {
		t.Errorf("Ready[0].Name = %q, want %q", p.Ready[0].Name, "claude_prompt")
	}
}

func TestClaudeDetector_Patterns_should_haveActivePatterns(t *testing.T) {
	t.Parallel()
	d := NewClaudeDetector()
	p := d.Patterns()
	if len(p.Active) == 0 {
		t.Fatal("Patterns().Active is empty")
	}
}

func TestClaudeDetector_Patterns_should_haveSuccessPatterns(t *testing.T) {
	t.Parallel()
	d := NewClaudeDetector()
	p := d.Patterns()
	if len(p.Success) == 0 {
		t.Fatal("Patterns().Success is empty")
	}
}

func TestClaudeDetector_FilterContent_should_returnUnchanged(t *testing.T) {
	t.Parallel()
	d := NewClaudeDetector()
	input := "some terminal output"
	if got := d.FilterContent(input); got != input {
		t.Errorf("FilterContent(%q) = %q, want same", input, got)
	}
}

func TestClassifyOSCTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		title      string
		wantStatus dtypes.OSCStatus
		wantOK     bool
	}{
		{name: "hand-listed spinner frame", title: "⠋ Thinking", wantStatus: dtypes.OSCStatusExecuting, wantOK: true},
		{name: "another hand-listed spinner frame", title: "⠹ working", wantStatus: dtypes.OSCStatusExecuting, wantOK: true},
		{
			name:       "braille character outside hand-listed frame set",
			title:      "⡇", // U+2847, not one of the standard cli-spinner frames
			wantStatus: dtypes.OSCStatusExecuting,
			wantOK:     true,
		},
		{name: "exact idle glyph", title: "✳", wantStatus: dtypes.OSCStatusIdle, wantOK: true},
		{
			name:       "transcription-slip guard: asterism U+273B not idle glyph",
			title:      "✻",
			wantStatus: dtypes.OSCStatusNone,
			wantOK:     false,
		},
		{
			name:       "transcription-slip guard: pinwheel U+273D not idle glyph",
			title:      "✽",
			wantStatus: dtypes.OSCStatusNone,
			wantOK:     false,
		},
		{name: "empty title", title: "", wantStatus: dtypes.OSCStatusNone, wantOK: false},
		{
			name:       "unrelated text",
			title:      "my-shell — bash",
			wantStatus: dtypes.OSCStatusNone,
			wantOK:     false,
		},
		{
			name:       "both spinner and idle glyph present: spinner wins",
			title:      "⠋ ✳",
			wantStatus: dtypes.OSCStatusExecuting,
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotOK := ClassifyOSCTitle(tt.title)
			if gotStatus != tt.wantStatus || gotOK != tt.wantOK {
				t.Errorf("ClassifyOSCTitle(%q) = (%v, %v), want (%v, %v)",
					tt.title, gotStatus, gotOK, tt.wantStatus, tt.wantOK)
			}
		})
	}
}

package ansi

import "testing"

func TestStripCSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "no escape codes", input: "Hello, world!", expected: "Hello, world!"},
		{name: "letter terminator", input: "\x1b[31mError\x1b[0m", expected: "Error"},
		{
			name:     "insert character at-sign terminator",
			input:    "esc\x1b[5@ to interrupt",
			expected: "esc to interrupt",
		},
		{
			name:     "tilde terminator",
			input:    "esc\x1b[3~ to interrupt",
			expected: "esc to interrupt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripCSI(tt.input)
			if got != tt.expected {
				t.Errorf("StripCSI(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestStripCSI_ZeroAllocsOnPlainText asserts the fast path for input without
// an ESC byte allocates nothing.
func TestStripCSI_ZeroAllocsOnPlainText(t *testing.T) {
	input := "hello world, plain terminal output from Claude"
	allocs := testing.AllocsPerRun(100, func() {
		_ = StripCSI(input)
	})
	if allocs != 0 {
		t.Errorf("StripCSI plain text: got %.0f allocs, want 0 (fast path broken)", allocs)
	}
}

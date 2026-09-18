package ansi

import "testing"

func TestExtractLastOSC(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		oscNums     []string
		wantPayload string
		wantOK      bool
	}{
		{
			name:        "single BEL-terminated match",
			data:        "\x1b]0;⠋ working\x07",
			oscNums:     []string{"0"},
			wantPayload: "⠋ working",
			wantOK:      true,
		},
		{
			name:        "single ST-terminated match",
			data:        "\x1b]0;✳\x1b\\",
			oscNums:     []string{"0"},
			wantPayload: "✳",
			wantOK:      true,
		},
		{
			name:        "multiple matches same num, last wins",
			data:        "\x1b]0;a\x07\x1b]0;b\x07",
			oscNums:     []string{"0"},
			wantPayload: "b",
			wantOK:      true,
		},
		{
			name:        "multiple oscNums, right-most overall wins regardless of arg order",
			data:        "\x1b]2;⠋ old\x07 ... \x1b]0;✳\x07",
			oscNums:     []string{"2", "0"},
			wantPayload: "✳",
			wantOK:      true,
		},
		{
			name:    "no ESC byte at all",
			data:    "plain text",
			oscNums: []string{"0"},
			wantOK:  false,
		},
		{
			name:    "unterminated",
			data:    "\x1b]0;stuck",
			oscNums: []string{"0"},
			wantOK:  false,
		},
		{
			name:    "stray terminator with no preceding prefix",
			data:    "garbage\x07more",
			oscNums: []string{"0"},
			wantOK:  false,
		},
		{
			name:    "empty string",
			data:    "",
			oscNums: []string{"0"},
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPayload, gotOK := ExtractLastOSC(tt.data, tt.oscNums...)
			if gotOK != tt.wantOK || gotPayload != tt.wantPayload {
				t.Errorf("ExtractLastOSC(%q, %v) = (%q, %v), want (%q, %v)",
					tt.data, tt.oscNums, gotPayload, gotOK, tt.wantPayload, tt.wantOK)
			}
		})
	}
}

// TestExtractLastOSC_ZeroAllocsOnPlainText asserts the fast path for input
// without an ESC-bracket prefix allocates nothing.
func TestExtractLastOSC_ZeroAllocsOnPlainText(t *testing.T) {
	input := "hello world, plain terminal output from Claude"
	allocs := testing.AllocsPerRun(100, func() {
		_, _ = ExtractLastOSC(input, "0")
	})
	if allocs != 0 {
		t.Errorf("ExtractLastOSC plain text: got %.0f allocs, want 0 (fast path broken)", allocs)
	}
}

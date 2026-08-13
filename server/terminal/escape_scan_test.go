package terminal

import "testing"

func TestScanEscapeSequence(t *testing.T) {
	tests := []struct {
		name  string
		input string
		start int
		// seqPrefix is the exact sequence bytes expected to be consumed,
		// starting at input[start]. want = len(seqPrefix).
		seqPrefix string
	}{
		{
			name:      "csi_sgr",
			input:     "\x1b[31mRed",
			start:     0,
			seqPrefix: "\x1b[31m",
		},
		{
			name:      "csi_private_mode",
			input:     "\x1b[?25hVisible",
			start:     0,
			seqPrefix: "\x1b[?25h",
		},
		{
			name:      "csi_unterminated_drops_rest",
			input:     "\x1b[31",
			start:     0,
			seqPrefix: "\x1b[31",
		},
		{
			name:      "osc_terminated_by_bel_with_letter_in_payload",
			input:     "\x1b]0;my title\x07Hello",
			start:     0,
			seqPrefix: "\x1b]0;my title\x07",
		},
		{
			name:      "osc_terminated_by_st",
			input:     "\x1b]8;;https://example.com\x1b\\Link",
			start:     0,
			seqPrefix: "\x1b]8;;https://example.com\x1b\\",
		},
		{
			name:      "osc_unterminated_drops_rest",
			input:     "\x1b]0;no terminator",
			start:     0,
			seqPrefix: "\x1b]0;no terminator",
		},
		{
			name:      "dcs_terminated_by_st",
			input:     "\x1bPq#0;2;0;0;0\x1b\\Sixel",
			start:     0,
			seqPrefix: "\x1bPq#0;2;0;0;0\x1b\\",
		},
		{
			name:      "dcs_terminated_by_single_byte_st",
			input:     "\x1bPq#0\x9cSixel",
			start:     0,
			seqPrefix: "\x1bPq#0\x9c",
		},
		{
			name:      "charset_designation",
			input:     "\x1b(BHello",
			start:     0,
			seqPrefix: "\x1b(B",
		},
		{
			name:      "charset_designation_unterminated",
			input:     "\x1b(",
			start:     0,
			seqPrefix: "\x1b(",
		},
		{
			name:      "simple_save_cursor",
			input:     "\x1b7Hello",
			start:     0,
			seqPrefix: "\x1b7",
		},
		{
			name:      "simple_reverse_index",
			input:     "\x1bMHello",
			start:     0,
			seqPrefix: "\x1bM",
		},
		{
			name:      "c1_range_second_byte",
			input:     "\x1b\x40Hello",
			start:     0,
			seqPrefix: "\x1b\x40",
		},
		{
			name:      "unrecognized_second_byte_consumes_only_esc",
			input:     "\x1b9Hello",
			start:     0,
			seqPrefix: "\x1b",
		},
		{
			name:      "lone_esc_at_end_of_buffer",
			input:     "text\x1b",
			start:     4,
			seqPrefix: "\x1b",
		},
		{
			name:      "not_an_escape_byte",
			input:     "Hello",
			start:     0,
			seqPrefix: "",
		},
		{
			// '@' (0x40) is a valid CSI final byte (Insert Character, ICH)
			// but is not a letter — regression guard for the terminator range.
			name:      "csi_insert_character_at_sign",
			input:     "\x1b[5@Hello",
			start:     0,
			seqPrefix: "\x1b[5@",
		},
		{
			// '~' (0x7E) is a valid CSI final byte used by many real xterm
			// sequences (Delete key, function keys, etc.) — regression
			// guard for the terminator range extending past 'z' (0x7A).
			name:      "csi_tilde_final_byte",
			input:     "\x1b[3~Hello",
			start:     0,
			seqPrefix: "\x1b[3~",
		},
		{
			// An invalid byte mid-CSI (not a param/intermediate/final byte)
			// gives up on the sequence, consuming only the ESC so the rest
			// (including '[') is reprocessed as ordinary text.
			name:      "csi_invalid_byte_mid_sequence",
			input:     "\x1b[3\x01mHello",
			start:     0,
			seqPrefix: "\x1b",
		},
		{
			// scanEscapeSequence must work correctly when start > 0 (a real
			// sequence mid-buffer), not just at start == 0.
			name:      "csi_mid_buffer_not_at_start",
			input:     "xy\x1b[31mRed",
			start:     2,
			seqPrefix: "\x1b[31m",
		},
		{
			name:      "osc_mid_buffer_not_at_start",
			input:     "xy\x1b]8;;https://example.com\x1b\\Link",
			start:     2,
			seqPrefix: "\x1b]8;;https://example.com\x1b\\",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := len(tt.seqPrefix)
			got := scanEscapeSequence([]byte(tt.input), tt.start)
			if got != want {
				if consumed, ok := boundedSlice(tt.input, tt.start, tt.start+got); ok {
					t.Errorf("scanEscapeSequence(%q, %d) = %d, want %d (consumed %q, expected %q)",
						tt.input, tt.start, got, want, consumed, tt.seqPrefix)
				} else {
					t.Errorf("scanEscapeSequence(%q, %d) = %d, want %d (result index out of range; expected %q)",
						tt.input, tt.start, got, want, tt.seqPrefix)
				}
			}
		})
	}
}

// TestScanUntilTerminator_SizeCap verifies an unterminated OSC/DCS payload
// (e.g. truncated at a chunk boundary, or adversarial/untrusted PTY output)
// doesn't force an unbounded scan — it gives up after maxUnterminatedScan
// bytes rather than scanning to the end of an arbitrarily large buffer.
func TestScanUntilTerminator_SizeCap(t *testing.T) {
	payload := make([]byte, maxUnterminatedScan+1000)
	payload[0] = '\x1b'
	payload[1] = ']'
	for i := 2; i < len(payload); i++ {
		payload[i] = 'x' // never a BEL or ST terminator
	}

	got := scanEscapeSequence(payload, 0)
	if got != maxUnterminatedScan {
		t.Errorf("scanEscapeSequence on unterminated OSC longer than the cap = %d, want %d", got, maxUnterminatedScan)
	}
}

// boundedSlice returns s[start:end] and true when the range is valid, or
// ("", false) when it isn't. A bounds violation and a valid-but-empty slice
// must never be conflated into the same string value (e.g. a sentinel like
// "<out of range>"), since that string could coincidentally equal real
// content being sliced — the ok bool is the only authoritative signal.
func boundedSlice(s string, start, end int) (string, bool) {
	if start < 0 || end > len(s) || start > end {
		return "", false
	}
	return s[start:end], true
}

package session

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestSanitizeUntrusted_should_StripC0C1CSIAndOSCAndCollapseNewlines_When_TableInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in, want string
	}{
		{"plain", "lint (unit)", "lint (unit)"},
		{"csi color", "evil\x1b[31m", "evil"},
		{"csi with params mid-text", "a\x1b[1;38;5;196mb\x1b[0mc", "abc"},
		{"osc bel", "x\x1b]0;t\x07y", "xy"},
		{"osc st", "x\x1b]8;;http://e\x1b\\y", "xy"},
		{"osc title with newline payload", "ok\x1b]0;title\x07", "ok"},
		{"unterminated osc swallows rest", "ok\x1b]0;never ends", "ok"},
		{"dcs", "a\x1bPq#0;2;0\x1b\\b", "ab"},
		{"two byte escape", "a\x1bcb", "ab"},
		{"lone trailing esc", "a\x1b", "a"},
		{"c1 csi rune", "a\u009b31mb", "ab"},
		{"c1 osc rune", "a\u009d0;t\u009cb", "ab"},
		{"c1 controls", "a\u0080\u0085\u0081b", "a b"},
		{"c0 controls", "a\x00\x01\x07\x08\x7fb", "ab"},
		{"crlf collapses", "a\r\nb.go", "a b.go"},
		{"newline run collapses", "a\n\n\r\n\nb", "a b"},
		{"tab and unicode line seps", "a\tb c d", "a b c d"},
		{"leading and trailing whitespace", "\n  x  \r\n", "x"},
		{"bidi override", "a\u202eb", "ab"},
		{"invalid utf8", "a\xff\x9bb", "ab"},
		{"multibyte kept", "héllo 世界 🙂", "héllo 世界 🙂"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, SanitizeUntrusted(tc.in, 1000))
		})
	}
}

func TestSanitizeUntrusted_should_TruncateOnRuneBoundary_When_OverByteLimit(t *testing.T) {
	t.Parallel()
	in := "x\x1b]0;t\x07" + strings.Repeat("x", 5000)
	got := SanitizeUntrusted(in, 100)
	require.Len(t, got, 100)

	// 世 is 3 bytes: a 4-byte budget must back off to 1 rune, never a split.
	got = SanitizeUntrusted("世界", 4)
	require.Equal(t, "世", got)
	require.True(t, utf8.ValidString(got))
	require.Equal(t, "", SanitizeUntrusted("abc", 0))
}

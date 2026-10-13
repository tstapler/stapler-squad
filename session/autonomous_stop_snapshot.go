package session

import (
	"strings"
	"unicode/utf8"
)

const (
	// stopTailMaxLines and stopTailMaxBytes bound the pane tail kept in a StopSnapshot.
	stopTailMaxLines = 40
	stopTailMaxBytes = 4096
)

// BoundedStopTail returns the last stopTailMaxLines lines of raw, ANSI-stripped,
// with trailing blank lines trimmed and at most stopTailMaxBytes bytes. The
// result is always valid UTF-8: invalid bytes are dropped and the byte cut
// advances to a rune boundary. It does not redact; see the service-side
// sanitizer before sending the result to any durable sink.
func BoundedStopTail(raw string) string {
	s := stripANSI([]byte(raw))
	s = strings.ToValidUTF8(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.TrimRight(s, " \t\n")
	if s == "" {
		return ""
	}

	if lines := strings.Split(s, "\n"); len(lines) > stopTailMaxLines {
		s = strings.Join(lines[len(lines)-stopTailMaxLines:], "\n")
	}

	return TailBytes(s, stopTailMaxBytes)
}

// TailBytes keeps the last n bytes of s, advancing the cut forward to a rune
// boundary so valid UTF-8 input stays valid.
func TailBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := len(s) - n
	for cut < len(s) && !utf8.RuneStart(s[cut]) {
		cut++
	}
	return s[cut:]
}

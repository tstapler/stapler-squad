package session

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SanitizeUntrusted makes third-party text (GitHub check names, logins, file
// paths) safe to type into a terminal: it drops invalid UTF-8, ANSI/OSC/DCS
// escape sequences, C0/C1 control characters and bidi overrides, collapses
// each run of newlines (and tabs, U+0085, U+2028/9) into one space, trims, and
// truncates to maxBytes on a rune boundary. It cannot stop semantic prompt
// injection; callers must also keep untrusted text out of instruction
// position and never include comment bodies.
func SanitizeUntrusted(s string, maxBytes int) string {
	s = strings.ToValidUTF8(s, "")
	var b strings.Builder
	b.Grow(len(s))
	pendingSpace := false
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == 0x1b:
			i = skipEscape(runes, i)
		case r == 0x9b: // C1 CSI
			i = skipCSI(runes, i+1)
		case r == 0x90 || r == 0x98 || r == 0x9d || r == 0x9e || r == 0x9f: // C1 DCS/SOS/OSC/PM/APC
			i = skipStringSequence(runes, i+1)
		case isLineBreakLike(r):
			pendingSpace = true
		case r == ' ':
			if pendingSpace {
				continue // the pending space already stands for this run
			}
			b.WriteRune(r)
		case unicode.IsControl(r), isBidiControl(r):
			// dropped
		default:
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(r)
		}
	}
	return TruncateUTF8Bytes(strings.TrimSpace(b.String()), maxBytes)
}

func isLineBreakLike(r rune) bool {
	switch r {
	case '\n', '\r', '\t', 0x0b, 0x0c, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

func isBidiControl(r rune) bool {
	return (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) || r == 0x200e || r == 0x200f || r == 0x061c
}

// skipEscape returns the index of the last rune of the escape sequence that
// starts with ESC at runes[i]. Unterminated string sequences swallow the rest
// of the input (fail closed).
func skipEscape(runes []rune, i int) int {
	if i+1 >= len(runes) {
		return i
	}
	switch runes[i+1] {
	case '[':
		return skipCSI(runes, i+2)
	case ']', 'P', 'X', '^', '_': // OSC, DCS, SOS, PM, APC
		return skipStringSequence(runes, i+2)
	}
	// nF/Fp/Fe/Fs: intermediates 0x20-0x2f then one final byte.
	j := i + 1
	for j < len(runes) && runes[j] >= 0x20 && runes[j] <= 0x2f {
		j++
	}
	if j < len(runes) {
		return j
	}
	return len(runes) - 1
}

// skipCSI consumes parameter/intermediate bytes up to and including the final
// byte (0x40-0x7e); start is the index after the introducer.
func skipCSI(runes []rune, start int) int {
	j := start
	for j < len(runes) {
		if runes[j] >= 0x40 && runes[j] <= 0x7e {
			return j
		}
		if runes[j] < 0x20 || runes[j] > 0x3f {
			return j - 1 // malformed: stop before the offending rune, let it be handled normally
		}
		j++
	}
	return len(runes) - 1
}

// skipStringSequence consumes up to a BEL, C1 ST (U+009C) or ESC \ terminator.
func skipStringSequence(runes []rune, start int) int {
	for j := start; j < len(runes); j++ {
		switch runes[j] {
		case 0x07, 0x9c:
			return j
		case 0x1b:
			if j+1 < len(runes) && runes[j+1] == '\\' {
				return j + 1
			}
		}
	}
	return len(runes) - 1
}

// TruncateUTF8Bytes cuts s to at most maxBytes bytes without splitting a
// multi-byte rune. The limit is bytes, not runes, because
// MaxSteerMessageLength is a byte limit.
func TruncateUTF8Bytes(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := s[:maxBytes]
	for len(cut) > 0 {
		r, size := utf8.DecodeLastRuneInString(cut)
		if r != utf8.RuneError || size != 1 {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return cut
}

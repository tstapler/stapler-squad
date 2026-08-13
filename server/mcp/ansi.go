package mcp

import (
	"bytes"
	"unicode/utf8"
)

// stripANSI removes ANSI escape sequences and replaces invalid UTF-8 bytes
// with the Unicode replacement character (U+FFFD).
//
// It handles the common CSI sequences (ESC [ ... m/A/B/…), OSC sequences
// (ESC ] ... ST/BEL), and bare ESC + single-char sequences. Partial escape
// sequences at the end of the input are silently dropped.
func stripANSI(b []byte) []byte {
	out := make([]byte, 0, len(b))
	i := 0
	for i < len(b) {
		if b[i] != 0x1b {
			// Fast path: validate UTF-8 and copy rune or replacement character.
			r, size := utf8.DecodeRune(b[i:])
			if r == utf8.RuneError && size == 1 {
				// Invalid byte — emit Unicode replacement character U+FFFD (0xEF 0xBF 0xBD).
				out = append(out, 0xef, 0xbf, 0xbd)
			} else {
				out = append(out, b[i:i+size]...)
			}
			i += size
			continue
		}

		// ESC — need at least one more byte.
		if i+1 >= len(b) {
			break
		}

		switch b[i+1] {
		case '[': // CSI: ESC [ <params> <final>
			i += 2
			// Skip parameter bytes (0x20-0x3f)
			for i < len(b) && b[i] >= 0x20 && b[i] <= 0x3f {
				i++
			}
			// Skip intermediate bytes (0x20-0x2f) — already covered above
			// Skip final byte (0x40-0x7e)
			if i < len(b) && b[i] >= 0x40 && b[i] <= 0x7e {
				i++
			}
		case ']': // OSC: ESC ] ... ST (ESC \) or BEL (0x07)
			i += 2
			for i < len(b) {
				if b[i] == 0x07 {
					i++
					break
				}
				if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default: // ESC + single char (e.g. ESC M, ESC =, ESC >)
			i += 2
		}
	}
	return out
}

// splitLines splits raw terminal bytes (after ANSI stripping) into lines on
// \n, \r\n, and bare \r. It does not produce a trailing empty entry when the
// input ends with a newline.
func splitLines(b []byte) [][]byte {
	var lines [][]byte
	for len(b) > 0 {
		nl := bytes.IndexAny(b, "\r\n")
		if nl < 0 {
			lines = append(lines, b)
			break
		}
		lines = append(lines, b[:nl])
		// Consume \r\n as a single newline.
		if b[nl] == '\r' && nl+1 < len(b) && b[nl+1] == '\n' {
			b = b[nl+2:]
		} else {
			b = b[nl+1:]
		}
	}
	return lines
}

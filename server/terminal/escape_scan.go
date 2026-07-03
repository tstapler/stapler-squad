package terminal

// maxUnterminatedScan caps how far scanUntilTerminator looks for a terminator
// before giving up, mirroring pkg/analytics/escape_code_parser.go's OSC bound.
// Without this, a pathological or truncated OSC/DCS payload (untrusted PTY
// output) could force an unbounded scan.
const maxUnterminatedScan = 65536

// scanEscapeSequence returns the number of bytes, starting at b[start] (which
// must be the ESC byte 0x1b), that make up one complete ANSI/DEC escape
// sequence. It follows the same termination rules used by
// pkg/analytics/escape_code_parser.go:
//
//   - CSI (ESC [ ...)            terminates on a final byte in 0x40-0x7E
//     (e.g. 'm' for SGR, '@' for Insert Character, 'h'/'l' for mode set/reset)
//   - OSC (ESC ] ...)            terminates on BEL (0x07) or ST (ESC \)
//   - DCS/PM/APC/SOS
//     (ESC P/^/_/X ...)          terminate on ST (ESC \) or single-byte ST (0x9C)
//   - charset designation (ESC (, ), *, or + then a designator byte) is 3 bytes
//   - other simple escapes (ESC 7, ESC 8, ESC c, C1-range second bytes, ...) are 2 bytes
//
// Treating any letter as a universal terminator (the previous behavior of
// this package) is wrong in two ways: it's too permissive for OSC/DCS/PM/APC/SOS,
// whose payloads (window titles, hyperlink URLs, base64 data, ...) routinely
// contain a letter well before their real terminator; and it's too strict for
// CSI, which per ECMA-48 also terminates on non-letter bytes like '@' (0x40,
// Insert Character) and '~' (0x7E, used by many real xterm sequences).
//
// If a sequence runs off the end of b before it terminates, the remaining
// bytes are treated as consumed rather than risk splitting a sequence that
// is simply incomplete in this chunk. If b[start+1] is not a recognized
// sequence-type byte, only the stray ESC itself is consumed so the
// following byte is processed normally.
func scanEscapeSequence(b []byte, start int) int {
	if start >= len(b) || b[start] != '\x1b' {
		return 0
	}
	if start+1 >= len(b) {
		return len(b) - start
	}

	switch b[start+1] {
	case '[':
		return scanCSI(b, start)
	case ']':
		return scanUntilTerminator(b, start, true /* allowBEL */)
	case 'P', '^', '_', 'X':
		return scanUntilTerminator(b, start, false /* allowBEL */)
	case '(', ')', '*', '+':
		if start+2 < len(b) {
			return 3
		}
		return len(b) - start
	case '7', '8', 'c':
		return 2
	default:
		// C1-range second bytes (0x40-0x5F) are all simple two-byte escapes
		// (index, next-line, reverse-index, single shifts, etc).
		if b[start+1] >= 0x40 && b[start+1] <= 0x5F {
			return 2
		}
		return 1
	}
}

// scanCSI scans a CSI sequence (ESC [ params... intermediates... final) and
// returns the total byte count from start, or the number of remaining bytes
// if the sequence is unterminated. If an invalid byte is encountered before a
// terminator, only the ESC is treated as consumed (matching the "give up and
// let the rest be reprocessed as ordinary bytes" behavior of
// pkg/analytics/escape_code_parser.go's parseCSI on the same input) rather
// than swallowing the partially-scanned parameter bytes.
func scanCSI(b []byte, start int) int {
	i := start + 2
	for i < len(b) {
		c := b[i]
		switch {
		case c >= 0x30 && c <= 0x3F, c >= 0x20 && c <= 0x2F:
			// Parameter bytes (0-9 ; : < = > ?) and intermediate bytes
			// (space through /).
			i++
		case c >= 0x40 && c <= 0x7E:
			// Final byte per ECMA-48: '@' through '~', not just letters.
			return i - start + 1
		default:
			// Not a valid CSI byte: give up on this sequence, consuming
			// only the ESC so the rest (including the '[') is reprocessed
			// as ordinary text.
			return 1
		}
	}
	return len(b) - start
}

// scanUntilTerminator scans an OSC/DCS/PM/APC/SOS sequence, terminating on
// ST (ESC \) or, when allowBEL is true, also on BEL (0x07); DCS/PM/APC/SOS
// additionally accept a single-byte ST (0x9C). Returns the total byte count
// from start, or the number of remaining bytes (capped at
// maxUnterminatedScan) if unterminated.
func scanUntilTerminator(b []byte, start int, allowBEL bool) int {
	limit := len(b)
	if limit > start+maxUnterminatedScan {
		limit = start + maxUnterminatedScan
	}
	for i := start + 2; i < limit; i++ {
		if allowBEL && b[i] == 0x07 {
			return i - start + 1
		}
		if b[i] == '\x1b' && i+1 < len(b) && b[i+1] == '\\' {
			return i - start + 2
		}
		if !allowBEL && b[i] == 0x9C {
			return i - start + 1
		}
	}
	return limit - start
}

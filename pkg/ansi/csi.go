// Package ansi provides shared helpers for scanning ECMA-48/ANSI escape
// sequences. It exists to give the CSI final-byte range a single source of
// truth: the same [a-zA-Z]-only bug (missing '@', '~', and other non-letter
// terminators per ECMA-48) was independently reimplemented across the
// codebase (session/detection, session/detection/ratelimit, session/tmux,
// server/services, pkg/analytics) before being fixed in BUG-025. Callers
// should use this package instead of hand-rolling the character class or
// byte range again — CSIFinalByteClass/StripCSI for regex-based scanners,
// IsCSIFinalByte for manual byte-level scanners (e.g.
// pkg/analytics/escape_code_parser.go).
//
// Only the entry points callers actually need are exported; the compiled
// regex and the raw byte bounds are implementation details of StripCSI and
// IsCSIFinalByte respectively.
package ansi

import (
	"regexp"
	"strings"
)

// CSIFinalByteClass is the regex character class for a CSI sequence's final
// byte. Per ECMA-48, CSI final bytes span 0x40-0x7E ('@' through '~'), not
// just ASCII letters — sequences like Insert Character (CSI Ps @) or
// tilde-terminated function-key sequences would otherwise be left
// unstripped, leaking raw escape bytes into text that downstream pattern
// matching operates on. Exported because callers combining the CSI branch
// with other alternatives (OSC, charset designation, etc.) need to build
// their own regexp rather than use the simple csiRegex below — see
// session/detection/detector.go's ansiStripRegex.
const CSIFinalByteClass = `[@-~]`

// csiFinalByteMin and csiFinalByteMax bound the same ECMA-48 CSI final-byte
// range as CSIFinalByteClass, for IsCSIFinalByte's byte-range check.
const (
	csiFinalByteMin byte = 0x40
	csiFinalByteMax byte = 0x7E
)

// IsCSIFinalByte reports whether b is a valid CSI sequence final byte per
// ECMA-48 (0x40-0x7E). Byte-level twin of CSIFinalByteClass for parsers that
// scan byte-by-byte rather than via regexp.
func IsCSIFinalByte(b byte) bool {
	return b >= csiFinalByteMin && b <= csiFinalByteMax
}

// csiRegex matches a complete, simple CSI sequence: ESC [ + parameter bytes
// + one final byte in CSIFinalByteClass. Used by StripCSI; callers needing a
// different combination of alternatives should build their own regexp from
// CSIFinalByteClass instead of depending on this one.
var csiRegex = regexp.MustCompile(`\x1b\[[0-9;]*` + CSIFinalByteClass)

// StripCSI removes CSI escape sequences from s. Inputs without an ESC byte
// take a fast path that avoids the regexp entirely.
func StripCSI(s string) string {
	if !strings.ContainsRune(s, '\x1b') {
		return s
	}
	return csiRegex.ReplaceAllString(s, "")
}

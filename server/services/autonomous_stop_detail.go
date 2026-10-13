package services

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tstapler/stapler-squad/session"
)

const (
	// stopTailWithheldMarker replaces a whole tail that matched a secret rule.
	stopTailWithheldMarker = "[tail withheld: secret pattern detected]"
	// stopTailSinkMaxBytes caps the sanitized tail stored in the stuck-row context.
	stopTailSinkMaxBytes = 1024
	// stopTailLineSeparator joins the tail's lines into one line.
	stopTailLineSeparator = " ⏎ "
	// keyBodyMinLen is the shortest line treated as headerless key material.
	keyBodyMinLen = 40
)

// secretNameSegments are the NAME segments that mark an assignment as secret-bearing.
var secretNameSegments = map[string]struct{}{
	"SECRET": {}, "TOKEN": {}, "PASSWORD": {}, "PASSWD": {},
	"KEY": {}, "CREDENTIAL": {}, "CREDENTIALS": {},
}

// sanitizeStopTail makes a bounded pane tail safe for a durable sink. It
// withholds the whole tail when any secret rule fires (fail closed), else
// redacts known credential shapes, folds the tail to one line and caps it at
// stopTailSinkMaxBytes, keeping the end. Accepted residual: a short secret in
// free prose with no marker, or one split across the capture's line cut.
func sanitizeStopTail(raw string) string {
	s := strings.ToValidUTF8(raw, "")
	if strings.TrimSpace(s) == "" {
		return ""
	}
	if session.RunPreGateSecurityCheck(s) != nil || tailHasSecretLine(s) {
		return stopTailWithheldMarker
	}

	s = redactErrorDetail(s)
	return capTailBytes(foldToOneLine(s), stopTailSinkMaxBytes)
}

func tailHasSecretLine(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		if hasSecretEnvAssignment(line) || looksLikeKeyBody(line) {
			return true
		}
	}
	return false
}

// hasSecretEnvAssignment reports whether line contains NAME=value where NAME is
// made only of letters, digits, '_' and '-' and has a secret-bearing segment.
// Names are matched case-insensitively: lower-case client_secret_key is covered.
func hasSecretEnvAssignment(line string) bool {
	for i := 0; i < len(line); i++ {
		if line[i] != '=' {
			continue
		}
		start := i
		for start > 0 && isEnvNameByte(line[start-1]) {
			start--
		}
		name := strings.TrimLeft(line[start:i], "-")
		if name == "" {
			continue
		}
		for _, seg := range strings.FieldsFunc(strings.ToUpper(name), func(r rune) bool { return r == '_' || r == '-' }) {
			if _, ok := secretNameSegments[seg]; ok {
				return true
			}
		}
	}
	return false
}

func isEnvNameByte(b byte) bool {
	return b == '_' || b == '-' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// looksLikeKeyBody reports whether the whole (trimmed) line is base64-alphabet
// text of at least keyBodyMinLen runes with no "//", i.e. headerless key material
// rather than a path or URL (those contain '.', ':' or "//").
func looksLikeKeyBody(line string) bool {
	line = strings.TrimSpace(line)
	if len(line) < keyBodyMinLen || strings.Contains(line, "//") {
		return false
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		isBase64 := c == '+' || c == '/' || c == '=' ||
			(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !isBase64 {
			return false
		}
	}
	return true
}

// foldToOneLine collapses whitespace runs and joins non-empty lines with
// stopTailLineSeparator.
func foldToOneLine(s string) string {
	var parts []string
	for _, line := range strings.Split(s, "\n") {
		if f := strings.FieldsFunc(line, unicode.IsSpace); len(f) > 0 {
			parts = append(parts, strings.Join(f, " "))
		}
	}
	return strings.Join(parts, stopTailLineSeparator)
}

// capTailBytes keeps the last max bytes of s, cutting forward to a rune boundary.
func capTailBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := len(s) - max
	for cut < len(s) && !utf8.RuneStart(s[cut]) {
		cut++
	}
	return s[cut:]
}

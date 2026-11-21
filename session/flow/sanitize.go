package flow

import (
	"strings"
	"unicode"
)

// SanitizeText flattens control characters and whitespace runs to single
// spaces and caps the length at maxRunes, so raw titles and notes are safe to
// emit. The RPC mapper and the MCP flow tools share it so both flatten alike.
func SanitizeText(s string, maxRunes int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := []rune(b.String())
	if len(out) > maxRunes {
		return string(out[:maxRunes-1]) + "…"
	}
	return string(out)
}
